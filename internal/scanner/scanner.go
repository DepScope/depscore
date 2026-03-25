package scanner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/resolve"
	"github.com/depscope/depscope/internal/vcs"
	"github.com/depscope/depscope/internal/vuln"
)

// Options controls scan behaviour.
type Options struct {
	Profile  string
	MaxFiles int
	NoCVE    bool     // skip CVE scanning (OSV queries)
	Only     []string // filter to these ecosystems (empty = all); values are internal constants: python, go, rust, npm, php
}

// ScanURL resolves a remote URL, parses manifests, fetches registry data,
// scores each package, and propagates transitive risk.
func ScanURL(ctx context.Context, url string, opts Options) (*core.ScanResult, error) {
	cfg := config.ProfileByName(opts.Profile)

	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 5000
	}

	resolver := resolve.DetectResolver(url, resolve.DetectOptions{
		GitHubToken: os.Getenv("GITHUB_TOKEN"),
		GitLabToken: os.Getenv("GITLAB_TOKEN"),
		MaxFiles:    maxFiles,
	})

	files, cleanup, err := resolver.Resolve(ctx, url)
	defer cleanup()
	if err != nil {
		return nil, fmt.Errorf("resolve remote: %w", err)
	}

	var pkgs []manifest.Package
	groups := groupByDirectory(files)
	for dir, group := range groups {
		filenames := make([]string, 0, len(group))
		fileMap := make(map[string][]byte)
		for _, f := range group {
			name := filepath.Base(f.Path)
			filenames = append(filenames, name)
			fileMap[name] = f.Content
		}
		eco, err := manifest.DetectEcosystemFromFiles(filenames)
		if err != nil {
			log.Printf("warning: skipping %s: %v", dir, err)
			continue
		}
		if len(opts.Only) > 0 {
			allowed := make(map[string]bool)
			for _, o := range opts.Only {
				allowed[o] = true
			}
			if !allowed[string(eco)] {
				continue
			}
		}
		parsed, err := manifest.ParserFor(eco).ParseFiles(fileMap)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", dir, err)
		}
		pkgs = append(pkgs, parsed...)
	}

	return scorePipeline(pkgs, cfg, opts.NoCVE)
}

// ScanDir scans a local directory. If multiple ecosystems are detected
// (e.g. package.json + composer.json), all are scanned and merged.
func ScanDir(dir string, opts Options) (*core.ScanResult, error) {
	cfg := config.ProfileByName(opts.Profile)

	ecosystems := manifest.DetectAllEcosystems(dir)
	if len(opts.Only) > 0 {
		allowed := make(map[string]bool)
		for _, o := range opts.Only {
			allowed[o] = true
		}
		var filtered []manifest.Ecosystem
		for _, eco := range ecosystems {
			if allowed[string(eco)] {
				filtered = append(filtered, eco)
			}
		}
		ecosystems = filtered
	}
	if len(ecosystems) == 0 {
		return nil, fmt.Errorf("no recognized manifest found in %s", dir)
	}

	var allPkgs []manifest.Package
	for _, eco := range ecosystems {
		pkgs, err := manifest.ParserFor(eco).Parse(dir)
		if err != nil {
			log.Printf("warning: %s parser failed: %v", eco, err)
			continue
		}
		allPkgs = append(allPkgs, pkgs...)
	}

	if len(allPkgs) == 0 {
		return nil, fmt.Errorf("no packages found in %s", dir)
	}

	return scorePipeline(allPkgs, cfg, opts.NoCVE)
}

// scorePipeline fetches registry data, scores each package, and propagates
// transitive risk. It is shared by ScanURL and ScanDir.
func scorePipeline(pkgs []manifest.Package, cfg config.Config, noCVE bool) (*core.ScanResult, error) {
	fetchers := registry.FetchersByEcosystem{
		"PyPI":       registry.NewPyPIClient(),
		"npm":        registry.NewNPMClient(),
		"crates.io":  registry.NewCratesClient(),
		"Go":         registry.NewGoProxyClient(),
		"Packagist":  registry.NewPackagistClient(),
	}

	fetchResults := registry.FetchAll(pkgs, fetchers, int64(cfg.Concurrency))

	// Create OSV client for CVE scanning (unless disabled)
	var osvClient *vuln.OSVClient
	if !noCVE {
		osvClient = vuln.NewOSVClient()
	}

	// Create VCS client for repo health lookups
	var vcsClient vcs.Client
	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken != "" {
		vcsClient = vcs.NewGitHubClient(vcs.WithToken(ghToken))
	} else {
		vcsClient = vcs.NewGitHubClient() // unauthenticated, 60 req/hr
	}

	// Cache VCS lookups by repo URL to avoid redundant API calls
	repoCache := make(map[string]*vcs.RepoInfo)

	// (osvClient created above, nil if noCVE)

	scored := make([]core.PackageResult, 0, len(pkgs))
	for _, pkg := range pkgs {
		fr := fetchResults[pkg.Key()]

		// Fetch VCS data if registry gave us a source repo URL
		var repoInfo *vcs.RepoInfo
		if fr != nil && fr.Info != nil && fr.Info.SourceRepoURL != "" {
			repoURL := fr.Info.SourceRepoURL
			if cached, ok := repoCache[repoURL]; ok {
				repoInfo = cached
			} else {
				ri, err := vcsClient.RepoFromURL(repoURL)
				if err != nil {
					log.Printf("VCS lookup failed for %s: %v", repoURL, err)
				} else {
					repoInfo = ri
				}
				repoCache[repoURL] = repoInfo // cache even nil (failed lookup)
			}
		}

		result := core.Score(pkg, fr, repoInfo, cfg.Weights)

		// Lookup CVEs via OSV (skip if --no-cve)
		// If no resolved version, use the version from registry fetch (latest)
		cveVersion := pkg.ResolvedVersion
		if cveVersion == "" && fr != nil && fr.Info != nil && fr.Info.Version != "" {
			cveVersion = fr.Info.Version
			result.Version = fr.Info.Version // update the result with resolved version
		}
		if osvClient != nil && cveVersion != "" {
			findings, err := osvClient.Query(pkg.Ecosystem.String(), pkg.Name, cveVersion)
			if err != nil {
				log.Printf("OSV query failed for %s@%s: %v", pkg.Name, pkg.ResolvedVersion, err)
			}
			if err == nil && len(findings) > 0 {
				for _, f := range findings {
					result.Vulnerabilities = append(result.Vulnerabilities, core.Vulnerability{
						ID:       f.ID,
						Summary:  f.Summary,
						Severity: string(f.Severity),
					})
					result.Issues = append(result.Issues, core.Issue{
						Package:  pkg.Name,
						Severity: core.IssueSeverity(f.Severity),
						Message:  "CVE: " + f.ID + " — " + f.Summary,
					})
				}
			}
		}

		// Apply CVE penalty to the reputation score
		core.ApplyCVEPenalty(&result)

		scored = append(scored, result)
	}

	depsMap := manifest.BuildDepsMap(pkgs)
	scored = core.Propagate(scored, depsMap)

	// Compute actual graph depths (BFS from root packages)
	graphDepths := computeGraphDepths(scored, depsMap)

	// Populate DependsOn, counts, and actual depth on each PackageResult
	for i := range scored {
		deps := depsMap[scored[i].Name]
		scored[i].DependsOn = deps
		scored[i].DependsOnCount = len(deps)
		if d, ok := graphDepths[scored[i].Name]; ok {
			scored[i].Depth = d
		}
	}
	// Build reverse map for DependedOnCount
	dependedOn := make(map[string]int)
	for _, deps := range depsMap {
		for _, d := range deps {
			dependedOn[d]++
		}
	}
	for i := range scored {
		scored[i].DependedOnCount = dependedOn[scored[i].Name]
	}

	directCount, transitiveCount := 0, 0
	for _, pkg := range pkgs {
		if pkg.Depth <= 1 {
			directCount++
		} else {
			transitiveCount++
		}
	}

	// Warn if only direct deps found (no lockfile = no transitive visibility)
	if transitiveCount == 0 && directCount > 0 {
		log.Printf("warning: no lockfile found — only %d direct dependencies scanned. "+
			"Transitive dependencies are not visible. "+
			"Add a lockfile (uv.lock, poetry.lock, Cargo.lock, package-lock.json, composer.lock) for full tree analysis.", directCount)
	}

	// Warn about packages without resolved versions (no CVE scanning possible)
	noVersion := 0
	for _, r := range scored {
		if r.Version == "" {
			noVersion++
		}
	}
	if noVersion > 0 {
		log.Printf("warning: %d/%d packages have no resolved version — CVE scanning limited. Use a lockfile for version-specific CVE checks.", noVersion, len(scored))
	}

	var allIssues []core.Issue
	for _, r := range scored {
		allIssues = append(allIssues, r.Issues...)
	}

	// Trace risk paths through the dependency graph
	riskPaths := core.FindRiskPaths(scored, depsMap, cfg.PassThreshold, 10)

	// Build registry info map for suspicious package detection
	regInfos := make(map[string]*registry.PackageInfo)
	for _, pkg := range pkgs {
		fr := fetchResults[pkg.Key()]
		if fr != nil && fr.Info != nil {
			regInfos[pkg.Name] = fr.Info
		}
	}
	suspicious := core.DetectSuspicious(scored, regInfos)

	result := &core.ScanResult{
		Profile:        cfg.Profile,
		PassThreshold:  cfg.PassThreshold,
		DirectDeps:     directCount,
		TransitiveDeps: transitiveCount,
		Packages:       scored,
		AllIssues:      allIssues,
		DepsMap:        depsMap,
		RiskPaths:      riskPaths,
		Suspicious:     suspicious,
	}

	// Build supply chain graph from scan results
	g := graph.BuildFromScanResult(result)
	graph.Propagate(g)
	result.Graph = g

	return result, nil
}

// computeGraphDepths does BFS from root packages (depth 1) to compute
// actual graph depth for each package. Root packages are those not
// depended on by any other package.
func computeGraphDepths(results []core.PackageResult, depsMap map[string][]string) map[string]int {
	// Find root packages (not a dependency of anything)
	isDep := make(map[string]bool)
	for _, deps := range depsMap {
		for _, d := range deps {
			isDep[d] = true
		}
	}

	depths := make(map[string]int)
	queue := []string{}

	for _, r := range results {
		if !isDep[r.Name] {
			depths[r.Name] = 1
			queue = append(queue, r.Name)
		}
	}

	// BFS
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		childDepth := depths[name] + 1
		for _, child := range depsMap[name] {
			if _, seen := depths[child]; !seen {
				depths[child] = childDepth
				queue = append(queue, child)
			}
		}
	}

	// Any unreachable packages get depth 1
	for _, r := range results {
		if _, ok := depths[r.Name]; !ok {
			depths[r.Name] = 1
		}
	}

	return depths
}

// groupByDirectory groups ManifestFiles by their parent directory.
func groupByDirectory(files []resolve.ManifestFile) map[string][]resolve.ManifestFile {
	groups := make(map[string][]resolve.ManifestFile)
	for _, f := range files {
		dir := filepath.Dir(f.Path)
		groups[dir] = append(groups[dir], f)
	}
	return groups
}
