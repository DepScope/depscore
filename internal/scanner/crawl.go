package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/crawler/resolvers"
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

	// 5. Run CVE pass if not disabled.
	if !opts.NoCVE {
		osvClient := vuln.NewOSVClient()
		cveErrors := crawler.RunCVEPass(ctx, result.Graph, cacheDB, osvClient)
		result.Errors = append(result.Errors, cveErrors...)
	}

	// 6. Run org trust scoring on all nodes.
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
