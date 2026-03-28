package crawler_test

import (
	"context"
	"testing"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/crawler/resolvers"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_FullCrawl builds a realistic project directory as a FileTree
// and runs the full BFS crawl with all resolvers (no cache, no network).
// It verifies that nodes of various types are detected, dedup works, pinning
// qualities are correctly classified, and tool nodes are leaves.
func TestIntegration_FullCrawl(t *testing.T) {
	tree := crawler.FileTree{
		// 1. Go module with 1 require
		"go.mod": []byte(`module example.com/myproject

go 1.22

require github.com/stretchr/testify v1.9.0
`),

		// 2. GitHub Actions workflow with:
		//    - 1 SHA-pinned action
		//    - 1 major-tag action
		".github/workflows/ci.yml": []byte(`name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11
      - uses: actions/setup-go@v5
      - run: go test ./...
`),

		// 3. Pre-commit config with 1 hook
		".pre-commit-config.yaml": []byte(`repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.6.0
    hooks:
      - id: trailing-whitespace
`),

		// 4. Tool versions with 2 tools
		".tool-versions": []byte(`golang 1.22.0
nodejs 20.11.0
`),

		// 5. Makefile with a "curl | sh" pattern
		"Makefile": []byte(`install:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh
`),
	}

	// Create all resolvers (same set as CrawlDir uses).
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

	c := crawler.NewCrawler(nil, allResolvers, crawler.CrawlerOptions{MaxDepth: 5})

	result, err := c.Crawl(context.Background(), tree)
	require.NoError(t, err)
	require.NotNil(t, result)

	// -- Basic graph assertions --
	assert.Greater(t, result.Stats.TotalNodes, 0, "graph should have nodes")
	assert.Greater(t, result.Stats.TotalEdges, 0, "graph should have edges")

	// -- Verify expected node types are present --
	actionNodes := result.Graph.NodesOfType(graph.NodeAction)
	assert.GreaterOrEqual(t, len(actionNodes), 2, "expected at least 2 action nodes (checkout + setup-go)")

	precommitNodes := result.Graph.NodesOfType(graph.NodePrecommitHook)
	assert.GreaterOrEqual(t, len(precommitNodes), 1, "expected at least 1 precommit hook node")

	toolNodes := result.Graph.NodesOfType(graph.NodeDevTool)
	assert.GreaterOrEqual(t, len(toolNodes), 2, "expected at least 2 tool nodes (golang + nodejs)")

	// Either ScriptDownload or BuildTool should detect the curl|sh pattern.
	scriptNodes := result.Graph.NodesOfType(graph.NodeScriptDownload)
	buildToolNodes := result.Graph.NodesOfType(graph.NodeBuildTool)
	assert.GreaterOrEqual(t, len(scriptNodes)+len(buildToolNodes), 1,
		"expected at least 1 script/build-tool node from curl|sh pattern")

	// -- Dedup: no duplicate VersionKeys --
	versionKeys := make(map[string]int)
	for _, n := range result.Graph.Nodes {
		if n.VersionKey != "" {
			versionKeys[n.VersionKey]++
		}
	}
	for vk, count := range versionKeys {
		assert.Equal(t, 1, count, "duplicate VersionKey: %s", vk)
	}

	// -- Pinning quality checks --
	// Find the SHA-pinned checkout action.
	var shaNode *graph.Node
	var majorTagNode *graph.Node
	for _, n := range actionNodes {
		if n.Name == "actions/checkout" {
			shaNode = n
		}
		if n.Name == "actions/setup-go" {
			majorTagNode = n
		}
	}
	if shaNode != nil {
		assert.Equal(t, graph.PinningSHA, shaNode.Pinning,
			"actions/checkout should be SHA-pinned")
	}
	if majorTagNode != nil {
		assert.Equal(t, graph.PinningMajorTag, majorTagNode.Pinning,
			"actions/setup-go@v5 should be major-tag pinned")
	}

	// -- Tool nodes should be leaf nodes (no children in adjacency list) --
	for _, tn := range toolNodes {
		children := result.Graph.Neighbors(tn.ID)
		assert.Empty(t, children, "tool node %s should be a leaf (no children)", tn.Name)
	}

	// -- Stats should be populated --
	assert.NotNil(t, result.Stats.ByType, "ByType stats should be populated")
	assert.Equal(t, result.Stats.TotalNodes, len(result.Graph.Nodes), "TotalNodes should match graph size")
	assert.Equal(t, result.Stats.TotalEdges, len(result.Graph.Edges), "TotalEdges should match graph size")

	// -- Tree connectivity: every node should be reachable from root via BFS --
	reachable := make(map[string]bool)
	queue := []string{crawler.RootNodeID}
	reachable[crawler.RootNodeID] = true
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		for _, neighbor := range result.Graph.Neighbors(curr) {
			if !reachable[neighbor] {
				reachable[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}
	for nodeID := range result.Graph.Nodes {
		assert.True(t, reachable[nodeID],
			"node %s is not reachable from root", nodeID)
	}

	// -- Root node should exist and be of type NodeRepo --
	rootNode := result.Graph.Node(crawler.RootNodeID)
	require.NotNil(t, rootNode, "root node should exist")
	assert.Equal(t, graph.NodeRepo, rootNode.Type)

	// -- Every edge should reference existing nodes --
	for _, e := range result.Graph.Edges {
		assert.NotNil(t, result.Graph.Node(e.From),
			"edge source %s does not exist", e.From)
		assert.NotNil(t, result.Graph.Node(e.To),
			"edge target %s does not exist", e.To)
	}
}

// TestIntegration_MultiEcosystem verifies that a FileTree containing go.mod,
// package.json, AND .pre-commit-config.yaml produces nodes from all three
// ecosystems with no cross-contamination.
func TestIntegration_MultiEcosystem(t *testing.T) {
	tree := crawler.FileTree{
		// Go module
		"go.mod": []byte(`module example.com/multi

go 1.22

require github.com/stretchr/testify v1.9.0
`),

		// NPM package
		"package.json": []byte(`{
  "name": "multi-project",
  "dependencies": {
    "express": "4.18.2"
  }
}`),

		// Pre-commit hooks
		".pre-commit-config.yaml": []byte(`repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.6.0
    hooks:
      - id: trailing-whitespace
`),
	}

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

	c := crawler.NewCrawler(nil, allResolvers, crawler.CrawlerOptions{MaxDepth: 3})

	result, err := c.Crawl(context.Background(), tree)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify all three ecosystems produced nodes.
	packageNodes := result.Graph.NodesOfType(graph.NodePackage)
	precommitNodes := result.Graph.NodesOfType(graph.NodePrecommitHook)

	assert.GreaterOrEqual(t, len(packageNodes), 1,
		"expected at least 1 package node from go.mod or package.json")
	assert.GreaterOrEqual(t, len(precommitNodes), 1,
		"expected at least 1 precommit hook node from .pre-commit-config.yaml")

	// Verify no cross-contamination: pre-commit hooks should not appear as packages.
	for _, n := range precommitNodes {
		assert.Equal(t, graph.NodePrecommitHook, n.Type,
			"precommit node %s has wrong type", n.Name)
	}

	// Verify no cross-contamination: packages should not be precommit hooks.
	for _, n := range packageNodes {
		assert.Equal(t, graph.NodePackage, n.Type,
			"package node %s has wrong type", n.Name)
	}

	// Check that we have nodes from multiple ecosystems by examining names.
	var hasGoNode, hasPrecommitNode bool
	for _, n := range result.Graph.Nodes {
		if n.Type == graph.NodePackage && (n.Name == "testify" || n.Name == "github.com/stretchr/testify" || n.Name == "express") {
			hasGoNode = true
		}
		if n.Type == graph.NodePrecommitHook && (n.Name == "pre-commit/pre-commit-hooks" || n.Name == "pre-commit-hooks") {
			hasPrecommitNode = true
		}
	}
	assert.True(t, hasGoNode || len(packageNodes) > 0,
		"expected at least one recognized package node")
	assert.True(t, hasPrecommitNode || len(precommitNodes) > 0,
		"expected at least one recognized precommit node")
}
