package scanner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/resolve"
)

// Options controls scan behaviour.
type Options struct {
	Profile  string
	MaxFiles int
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
		parsed, err := manifest.ParserFor(eco).ParseFiles(fileMap)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", dir, err)
		}
		pkgs = append(pkgs, parsed...)
	}

	return scorePipeline(pkgs, cfg)
}

// ScanDir scans a local directory.
func ScanDir(dir string, opts Options) (*core.ScanResult, error) {
	cfg := config.ProfileByName(opts.Profile)

	eco, err := manifest.DetectEcosystem(dir)
	if err != nil {
		return nil, fmt.Errorf("detect ecosystem: %w", err)
	}
	pkgs, err := manifest.ParserFor(eco).Parse(dir)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return scorePipeline(pkgs, cfg)
}

// scorePipeline fetches registry data, scores each package, and propagates
// transitive risk. It is shared by ScanURL and ScanDir.
func scorePipeline(pkgs []manifest.Package, cfg config.Config) (*core.ScanResult, error) {
	fetchers := registry.FetchersByEcosystem{
		"PyPI":      registry.NewPyPIClient(),
		"npm":       registry.NewNPMClient(),
		"crates.io": registry.NewCratesClient(),
		"Go":        registry.NewGoProxyClient(),
	}

	fetchResults := registry.FetchAll(pkgs, fetchers, int64(cfg.Concurrency))

	scored := make([]core.PackageResult, 0, len(pkgs))
	for _, pkg := range pkgs {
		fr := fetchResults[pkg.Key()]
		result := core.Score(pkg, fr, cfg.Weights)
		scored = append(scored, result)
	}

	depsMap := manifest.BuildDepsMap(pkgs)
	scored = core.Propagate(scored, depsMap)

	// Populate DependsOn and counts on each PackageResult
	for i := range scored {
		deps := depsMap[scored[i].Name]
		scored[i].DependsOn = deps
		scored[i].DependsOnCount = len(deps)
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

	var allIssues []core.Issue
	for _, r := range scored {
		allIssues = append(allIssues, r.Issues...)
	}

	result := &core.ScanResult{
		Profile:        cfg.Profile,
		PassThreshold:  cfg.PassThreshold,
		DirectDeps:     directCount,
		TransitiveDeps: transitiveCount,
		Packages:       scored,
		AllIssues:      allIssues,
		DepsMap:        depsMap,
	}
	return result, nil
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
