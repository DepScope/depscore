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
}
