package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFileTree_BasicFiles(t *testing.T) {
	tmp := t.TempDir()

	// Create a go.mod file.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644))

	// Create a .pre-commit-config.yaml file.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".pre-commit-config.yaml"), []byte("repos: []\n"), 0o644))

	// Create a .github/workflows directory with a CI file.
	wfDir := filepath.Join(tmp, ".github", "workflows")
	require.NoError(t, os.MkdirAll(wfDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wfDir, "ci.yml"), []byte("name: CI\non: push\njobs: {}\n"), 0o644))

	// Create a file that should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "README.md"), []byte("# readme"), 0o644))

	// Create node_modules dir that should be skipped.
	nmDir := filepath.Join(tmp, "node_modules", "foo")
	require.NoError(t, os.MkdirAll(nmDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nmDir, "package.json"), []byte(`{}`), 0o644))

	tree, err := buildFileTree(tmp)
	require.NoError(t, err)

	// Should contain the three known files.
	assert.Contains(t, tree, "go.mod")
	assert.Contains(t, tree, ".pre-commit-config.yaml")
	assert.Contains(t, tree, ".github/workflows/ci.yml")

	// Should NOT contain README.md or anything from node_modules.
	assert.NotContains(t, tree, "README.md")
	assert.NotContains(t, tree, "node_modules/foo/package.json")
}

func TestBuildFileTree_TerraformFiles(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.tf"), []byte(`
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.1.0"
}
`), 0o644))

	tree, err := buildFileTree(tmp)
	require.NoError(t, err)
	assert.Contains(t, tree, "main.tf")
}

func TestBuildFileTree_EmptyDir(t *testing.T) {
	tmp := t.TempDir()

	tree, err := buildFileTree(tmp)
	require.NoError(t, err)
	assert.Empty(t, tree)
}

func TestCrawlDir_BasicScan(t *testing.T) {
	tmp := t.TempDir()

	// Create a minimal go.mod.
	gomod := `module example.com/myapp

go 1.21

require (
	golang.org/x/text v0.14.0
)
`
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(gomod), 0o644))

	// Create a minimal GitHub Actions workflow.
	wfDir := filepath.Join(tmp, ".github", "workflows")
	require.NoError(t, os.MkdirAll(wfDir, 0o755))
	ciYml := `name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
`
	require.NoError(t, os.WriteFile(filepath.Join(wfDir, "ci.yml"), []byte(ciYml), 0o644))

	// Create a minimal .pre-commit-config.yaml.
	precommit := `repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.5.0
    hooks:
      - id: trailing-whitespace
`
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".pre-commit-config.yaml"), []byte(precommit), 0o644))

	result, err := CrawlDir(context.Background(), tmp, CrawlOptions{
		NoCVE: true, // skip CVE queries in tests
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Graph)

	// Verify we got nodes of different types.
	pkgNodes := result.Graph.NodesOfType(graph.NodePackage)
	actionNodes := result.Graph.NodesOfType(graph.NodeAction)
	hookNodes := result.Graph.NodesOfType(graph.NodePrecommitHook)

	assert.NotEmpty(t, pkgNodes, "expected at least one Package node from go.mod")
	assert.NotEmpty(t, actionNodes, "expected at least one Action node from ci.yml")
	assert.NotEmpty(t, hookNodes, "expected at least one PrecommitHook node from .pre-commit-config.yaml")

	// Verify total node and edge counts are sensible.
	assert.Greater(t, result.Stats.TotalNodes, 0)
	assert.Greater(t, result.Stats.TotalEdges, 0)
}

func TestCrawlDir_EmptyDir(t *testing.T) {
	tmp := t.TempDir()

	_, err := CrawlDir(context.Background(), tmp, CrawlOptions{
		NoCVE: true,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no recognized manifest")
}

// TestBuildFileTree_SkipsDirs verifies that .git, node_modules, vendor, and
// .terraform directories are excluded from the file tree.
func TestBuildFileTree_SkipsDirs(t *testing.T) {
	dir := t.TempDir()

	skipList := []string{".git", "node_modules", "vendor", ".terraform"}
	for _, d := range skipList {
		sub := filepath.Join(dir, d)
		require.NoError(t, os.MkdirAll(sub, 0o755))
		// Place a go.mod inside each skipped dir.
		require.NoError(t, os.WriteFile(filepath.Join(sub, "go.mod"), []byte("module skip.example.com\n\ngo 1.21\n"), 0o644))
	}

	tree, err := buildFileTree(dir)
	require.NoError(t, err)

	// None of the skipped-dir files should be present.
	for key := range tree {
		for _, skip := range skipList {
			assert.NotContains(t, key, skip+"/", "path %q should have been skipped (dir: %s)", key, skip)
		}
	}
}

// TestBuildFileTree_WorkflowYML verifies that .github/workflows/*.yml files
// are included in the file tree.
func TestBuildFileTree_WorkflowYML(t *testing.T) {
	dir := t.TempDir()

	wfDir := filepath.Join(dir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(wfDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wfDir, "ci.yml"), []byte("name: CI\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(wfDir, "release.yml"), []byte("name: Release\n"), 0o644))

	// A .yml outside of .github/workflows should NOT be included.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("version: '3'\n"), 0o644))

	tree, err := buildFileTree(dir)
	require.NoError(t, err)

	assert.Contains(t, tree, ".github/workflows/ci.yml")
	assert.Contains(t, tree, ".github/workflows/release.yml")
	assert.NotContains(t, tree, "docker-compose.yml")
}

// TestBuildFileTree_TerraformTF verifies that *.tf files are included.
func TestBuildFileTree_TerraformTF(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(`variable "region" {}\n`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "outputs.tf"), []byte(`output "id" {}\n`), 0o644))

	tree, err := buildFileTree(dir)
	require.NoError(t, err)

	assert.Contains(t, tree, "variables.tf")
	assert.Contains(t, tree, "outputs.tf")
}

// TestCrawlDir_WithCache verifies that CrawlDir correctly opens and uses a
// SQLite cache when CacheDBPath is set.
func TestCrawlDir_WithCache(t *testing.T) {
	dir := t.TempDir()

	gomod := `module example.com/cached

go 1.21

require (
	golang.org/x/text v0.14.0
)
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644))

	cacheDir := t.TempDir()
	cacheDBPath := filepath.Join(cacheDir, "test-cache.db")

	result, err := CrawlDir(context.Background(), dir, CrawlOptions{
		NoCVE:       true,
		CacheDBPath: cacheDBPath,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Graph.Nodes)

	// Cache file should have been created.
	_, statErr := os.Stat(cacheDBPath)
	assert.NoError(t, statErr, "SQLite cache file should exist after scan")
}

// TestCrawlDir_InvalidCacheDB verifies that an invalid cache path returns an
// error (e.g. a directory path used as a DB file path).
func TestCrawlDir_InvalidCacheDB(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644))

	// Use a sub-directory path that doesn't exist as a DB path.
	_, err := CrawlDir(context.Background(), dir, CrawlOptions{
		NoCVE:       true,
		CacheDBPath: filepath.Join(dir, "nonexistent", "subdir", "cache.db"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open cache db")
}

func TestCrawlDir_WithTrustedOrgs(t *testing.T) {
	tmp := t.TempDir()

	gomod := `module example.com/myapp

go 1.21

require (
	golang.org/x/text v0.14.0
)
`
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(gomod), 0o644))

	result, err := CrawlDir(context.Background(), tmp, CrawlOptions{
		NoCVE:       true,
		TrustedOrgs: []string{"golang.org"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify that nodes have org_type metadata set.
	for _, node := range result.Graph.Nodes {
		if node.Metadata != nil {
			_, hasOrgType := node.Metadata["org_type"]
			assert.True(t, hasOrgType, "node %s should have org_type metadata", node.ID)
		}
	}
}
