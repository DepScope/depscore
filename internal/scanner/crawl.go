package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/crawler/resolvers"
	"github.com/depscope/depscope/internal/resolve"
	"github.com/depscope/depscope/internal/vuln"
)

// CrawlOptions configures a CrawlDir scan.
type CrawlOptions struct {
	Profile     string
	MaxDepth    int // default 25
	NoCVE       bool
	TrustedOrgs []string
	CacheDBPath string // path to SQLite cache, empty = no cache
}

// CrawlDir scans a local directory using the new unified crawler.
// This is the new scan pathway that uses the BFS crawler with all resolvers.
func CrawlDir(ctx context.Context, dir string, opts CrawlOptions) (*crawler.CrawlResult, error) {
	// 1. Build FileTree from the directory.
	tree, err := buildFileTree(dir)
	if err != nil {
		return nil, fmt.Errorf("build file tree: %w", err)
	}
	if len(tree) == 0 {
		return nil, fmt.Errorf("no recognized manifest or config files found in %s", dir)
	}

	// 2. Open CacheDB if path provided.
	var cacheDB *cache.CacheDB
	if opts.CacheDBPath != "" {
		cacheDB, err = cache.NewCacheDB(opts.CacheDBPath)
		if err != nil {
			return nil, fmt.Errorf("open cache db: %w", err)
		}
		defer func() { _ = cacheDB.Close() }()
	}

	// 3. Create all 8 resolvers.
	allResolvers := map[crawler.DepSourceType]crawler.Resolver{
		crawler.DepSourcePackage:   resolvers.NewPackageResolver(),
		crawler.DepSourceAction:    resolvers.NewActionResolver(),
		crawler.DepSourcePrecommit: resolvers.NewPrecommitResolver(),
		crawler.DepSourceSubmodule: resolvers.NewSubmoduleResolver(),
		crawler.DepSourceTerraform: resolvers.NewTerraformResolver(),
		crawler.DepSourceTool:      resolvers.NewToolResolver(),
		crawler.DepSourceScript:    resolvers.NewScriptResolver(),
		crawler.DepSourceBuildTool: resolvers.NewBuildToolResolver(),
	}

	// 4. Create crawler and run the crawl.
	maxDepth := opts.MaxDepth
	if maxDepth == 0 {
		maxDepth = 25
	}

	c := crawler.NewCrawler(cacheDB, allResolvers, crawler.CrawlerOptions{
		MaxDepth: maxDepth,
		OwnOrgs:  opts.TrustedOrgs,
	})

	result, err := c.Crawl(ctx, tree)
	if err != nil {
		return nil, fmt.Errorf("crawl: %w", err)
	}

	// Set root node name to directory basename.
	if rootNode := result.Graph.Node(crawler.RootNodeID); rootNode != nil {
		rootNode.Name = filepath.Base(dir)
	}

	// 5. Run scoring pass.
	scoreErrors := crawler.RunScorePass(ctx, result.Graph, cacheDB)
	result.Errors = append(result.Errors, scoreErrors...)

	// 6. Run CVE pass if not disabled.
	if !opts.NoCVE {
		osvClient := vuln.NewOSVClient()
		cveErrors := crawler.RunCVEPass(ctx, result.Graph, cacheDB, osvClient)
		result.Errors = append(result.Errors, cveErrors...)
	}

	// 7. Run org trust scoring on all nodes.
	ownOrgFloor := 80 // default floor for own-org packages
	for _, node := range result.Graph.Nodes {
		orgType := core.ClassifyOrg(node.ProjectID, opts.TrustedOrgs)
		node.Score = core.ApplyOrgTrust(node.Score, orgType, ownOrgFloor)
		if node.Metadata == nil {
			node.Metadata = make(map[string]any)
		}
		node.Metadata["org_type"] = orgType
	}

	return result, nil
}

// CrawlURL scans a remote URL using the unified crawler.
// It uses resolve.DetectResolver to fetch remote files and then runs the
// same crawler pipeline as CrawlDir.
func CrawlURL(ctx context.Context, url string, opts CrawlOptions) (*crawler.CrawlResult, error) {
	maxFiles := 5000
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

	// Convert resolved files to a FileTree.
	tree := make(crawler.FileTree, len(files))
	for _, f := range files {
		tree[f.Path] = f.Content
	}
	if len(tree) == 0 {
		return nil, fmt.Errorf("no files found at %s", url)
	}

	// Open CacheDB if path provided.
	var cacheDB *cache.CacheDB
	if opts.CacheDBPath != "" {
		cacheDB, err = cache.NewCacheDB(opts.CacheDBPath)
		if err != nil {
			return nil, fmt.Errorf("open cache db: %w", err)
		}
		defer func() { _ = cacheDB.Close() }()
	}

	// Create all 8 resolvers.
	allResolvers := map[crawler.DepSourceType]crawler.Resolver{
		crawler.DepSourcePackage:   resolvers.NewPackageResolver(),
		crawler.DepSourceAction:    resolvers.NewActionResolver(),
		crawler.DepSourcePrecommit: resolvers.NewPrecommitResolver(),
		crawler.DepSourceSubmodule: resolvers.NewSubmoduleResolver(),
		crawler.DepSourceTerraform: resolvers.NewTerraformResolver(),
		crawler.DepSourceTool:      resolvers.NewToolResolver(),
		crawler.DepSourceScript:    resolvers.NewScriptResolver(),
		crawler.DepSourceBuildTool: resolvers.NewBuildToolResolver(),
	}

	maxDepth := opts.MaxDepth
	if maxDepth == 0 {
		maxDepth = 25
	}

	c := crawler.NewCrawler(cacheDB, allResolvers, crawler.CrawlerOptions{
		MaxDepth: maxDepth,
		OwnOrgs:  opts.TrustedOrgs,
	})

	result, err := c.Crawl(ctx, tree)
	if err != nil {
		return nil, fmt.Errorf("crawl: %w", err)
	}

	// Set root node name to the scanned URL.
	if rootNode := result.Graph.Node(crawler.RootNodeID); rootNode != nil {
		rootNode.Name = url
	}

	// Run scoring pass.
	scoreErrors := crawler.RunScorePass(ctx, result.Graph, cacheDB)
	result.Errors = append(result.Errors, scoreErrors...)

	// Run CVE pass if not disabled.
	if !opts.NoCVE {
		osvClient := vuln.NewOSVClient()
		cveErrors := crawler.RunCVEPass(ctx, result.Graph, cacheDB, osvClient)
		result.Errors = append(result.Errors, cveErrors...)
	}

	// Run org trust scoring on all nodes.
	ownOrgFloor := 80
	for _, node := range result.Graph.Nodes {
		orgType := core.ClassifyOrg(node.ProjectID, opts.TrustedOrgs)
		node.Score = core.ApplyOrgTrust(node.Score, orgType, ownOrgFloor)
		if node.Metadata == nil {
			node.Metadata = make(map[string]any)
		}
		node.Metadata["org_type"] = orgType
	}

	return result, nil
}

// CrawlResultToScanResult converts a CrawlResult to a ScanResult for backward
// compatibility with existing report formatters. This extracts the conversion
// logic from handlers.go into a reusable function.
func CrawlResultToScanResult(result *crawler.CrawlResult, profile string) *core.ScanResult {
	cfg := config.ProfileByName(profile)
	g := result.Graph

	var packages []core.PackageResult
	var allIssues []core.Issue
	directCount, transitiveCount := 0, 0
	depsMap := make(map[string][]string)

	for _, n := range g.Nodes {
		eco := ""
		if e, ok := n.Metadata["ecosystem"].(string); ok {
			eco = e
		}
		if eco == "" && n.ProjectID != "" {
			// Extract ecosystem from ProjectID like "go/github.com/foo"
			if idx := strings.Index(n.ProjectID, "/"); idx > 0 {
				eco = n.ProjectID[:idx]
			}
		}

		risk := n.Risk
		if risk == "" || risk == core.RiskUnknown {
			risk = core.RiskLevelFromScore(n.Score)
		}

		pr := core.PackageResult{
			Name:                n.Name,
			Version:             n.Version,
			Ecosystem:           eco,
			OwnScore:            n.Score,
			OwnRisk:             risk,
			TransitiveRiskScore: n.Score,
			TransitiveRisk:      risk,
		}

		// Count deps from graph edges.
		children := g.Neighbors(n.ID)
		pr.DependsOn = children
		pr.DependsOnCount = len(children)
		depsMap[n.Name] = children

		packages = append(packages, pr)
	}

	// Count direct vs transitive (depth from edges).
	hasIncoming := make(map[string]bool)
	for _, e := range g.Edges {
		if g.Nodes[e.From] != nil {
			hasIncoming[e.To] = true
		}
	}
	for nodeID := range g.Nodes {
		if !hasIncoming[nodeID] {
			directCount++
		} else {
			transitiveCount++
		}
	}

	return &core.ScanResult{
		Profile:        profile,
		PassThreshold:  cfg.PassThreshold,
		DirectDeps:     directCount,
		TransitiveDeps: transitiveCount,
		Packages:       packages,
		AllIssues:      allIssues,
		DepsMap:        depsMap,
		Graph:          g,
	}
}

// knownManifestFiles lists exact filenames to include in the file tree.
var knownManifestFiles = map[string]bool{
	"go.mod":                  true,
	"go.sum":                  true,
	"package.json":            true,
	"package-lock.json":       true,
	"pnpm-lock.yaml":          true,
	"bun.lock":                true,
	"pyproject.toml":          true,
	"requirements.txt":        true,
	"poetry.lock":             true,
	"uv.lock":                 true,
	"Cargo.toml":              true,
	"Cargo.lock":              true,
	"composer.json":           true,
	"composer.lock":           true,
	".pre-commit-config.yaml": true,
	".gitmodules":             true,
	".tool-versions":          true,
	".mise.toml":              true,
	"Makefile":                true,
	"Taskfile.yml":            true,
	"Taskfile.yaml":           true,
	"justfile":                true,
}

// skipDirs lists directories to skip during tree walking.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".terraform":   true,
}

// buildFileTree reads relevant files from a directory into a FileTree.
func buildFileTree(dir string) (crawler.FileTree, error) {
	tree := make(crawler.FileTree)

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible files
		}

		// Skip excluded directories.
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Compute relative path from the base directory.
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		// Normalize to forward slashes for cross-platform consistency.
		rel = filepath.ToSlash(rel)

		base := d.Name()

		// Check if this file matches any of our known patterns.
		include := knownManifestFiles[base]

		// Exact filename match.

		// .github/workflows/*.yml files.
		if !include && strings.HasSuffix(base, ".yml") {
			relDir := filepath.ToSlash(filepath.Dir(rel))
			if relDir == ".github/workflows" || strings.HasSuffix(relDir, "/.github/workflows") {
				include = true
			}
		}

		// *.tf files (Terraform).
		if !include && strings.HasSuffix(base, ".tf") {
			include = true
		}

		if !include {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		tree[rel] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk directory %s: %w", dir, err)
	}

	return tree, nil
}
