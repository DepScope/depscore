package report

import (
	"strings"
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestGraph creates a 5-node, 3-level test graph:
//
//	workflow:ci.yml
//	  ├── action:actions/checkout@v4     (score 92, exact version)
//	  │   └── package:npm/@actions/core@1.10.1 (score 85)
//	  └── hook:pre-commit/mirrors-mypy@v1      (score 81, major tag → MUTABLE)
//	package:go/cobra@v1.8.0              (score 91)
//	  └── package:go/pflag@v1.0.5        (score 87)
func buildTestGraph() *graph.Graph {
	g := graph.New()

	g.AddNode(&graph.Node{
		ID: "workflow:ci.yml", Type: graph.NodeWorkflow,
		Name: "ci.yml", Version: "", Score: 78,
		Risk: core.RiskMedium, Pinning: graph.PinningNA,
		Metadata: map[string]any{},
	})
	g.AddNode(&graph.Node{
		ID: "action:actions/checkout", Type: graph.NodeAction,
		Name: "actions/checkout", Version: "v4", Score: 92,
		Risk: core.RiskLow, Pinning: graph.PinningExactVersion,
		Metadata: map[string]any{},
	})
	g.AddNode(&graph.Node{
		ID: "package:npm/@actions/core@1.10.1", Type: graph.NodePackage,
		Name: "@actions/core", Version: "1.10.1", Score: 85,
		Risk: core.RiskLow, Pinning: graph.PinningNA,
		Metadata: map[string]any{},
	})
	g.AddNode(&graph.Node{
		ID: "precommit_hook:pre-commit/mirrors-mypy", Type: graph.NodePrecommitHook,
		Name: "pre-commit/mirrors-mypy", Version: "v1", Score: 81,
		Risk: core.RiskLow, Pinning: graph.PinningMajorTag,
		Metadata: map[string]any{},
	})
	g.AddNode(&graph.Node{
		ID: "package:go/cobra@v1.8.0", Type: graph.NodePackage,
		Name: "cobra", Version: "v1.8.0", Score: 91,
		Risk: core.RiskLow, Pinning: graph.PinningNA,
		Metadata: map[string]any{},
	})
	g.AddNode(&graph.Node{
		ID: "package:go/pflag@v1.0.5", Type: graph.NodePackage,
		Name: "pflag", Version: "v1.0.5", Score: 87,
		Risk: core.RiskLow, Pinning: graph.PinningNA,
		Metadata: map[string]any{},
	})

	// Edges: workflow → action, workflow → hook
	g.AddEdge(&graph.Edge{From: "workflow:ci.yml", To: "action:actions/checkout", Type: graph.EdgeUsesAction})
	g.AddEdge(&graph.Edge{From: "workflow:ci.yml", To: "precommit_hook:pre-commit/mirrors-mypy", Type: graph.EdgeUsesHook})
	// action → package
	g.AddEdge(&graph.Edge{From: "action:actions/checkout", To: "package:npm/@actions/core@1.10.1", Type: graph.EdgeBundles})
	// package → package
	g.AddEdge(&graph.Edge{From: "package:go/cobra@v1.8.0", To: "package:go/pflag@v1.0.5", Type: graph.EdgeDependsOn})

	return g
}

func TestRenderTreeBasic(t *testing.T) {
	g := buildTestGraph()
	output := RenderTree(g, TreeOptions{})

	// Check header
	assert.Contains(t, output, "depscope/")

	// Check that expected nodes appear
	assert.Contains(t, output, "[action] actions/checkout@v4")
	assert.Contains(t, output, "●92")
	assert.Contains(t, output, "[package] @actions/core@1.10.1")
	assert.Contains(t, output, "●85")
	assert.Contains(t, output, "[hook] pre-commit/mirrors-mypy@v1")
	assert.Contains(t, output, "⚡MUTABLE")
	assert.Contains(t, output, "[package] cobra@v1.8.0")
	assert.Contains(t, output, "[package] pflag@v1.0.5")

	// Check summary line
	assert.Contains(t, output, "Summary:")
	assert.Contains(t, output, "6 nodes")
	assert.Contains(t, output, "max depth: 3")

	// Check tree connectors
	assert.Contains(t, output, "├── ")
	assert.Contains(t, output, "└── ")
}

func TestRenderTreeMaxDepth(t *testing.T) {
	g := buildTestGraph()
	output := RenderTree(g, TreeOptions{MaxDepth: 1})

	// Depth-1 children should appear
	assert.Contains(t, output, "actions/checkout")
	// Depth-2 children should NOT appear
	assert.NotContains(t, output, "@actions/core")
}

func TestRenderTreeTypeFilter(t *testing.T) {
	g := buildTestGraph()

	// Filter for packages only — roots include cobra, and @actions/core is a child of action
	output := RenderTree(g, TreeOptions{TypeFilter: []graph.NodeType{graph.NodePackage}})

	// Package nodes should appear
	assert.Contains(t, output, "[package] cobra")
	assert.Contains(t, output, "[package] pflag")

	// Non-package nodes should not appear
	assert.NotContains(t, output, "[action]")
	assert.NotContains(t, output, "[hook]")
	assert.NotContains(t, output, "[workflow]")
}

func TestRenderTreeRiskFilter(t *testing.T) {
	g := buildTestGraph()

	// Set a root node to HIGH risk for testing
	g.Nodes["package:go/cobra@v1.8.0"].Risk = core.RiskHigh
	g.Nodes["package:go/cobra@v1.8.0"].Score = 55

	// Also set the workflow to CRITICAL so it and its children show
	g.Nodes["workflow:ci.yml"].Risk = core.RiskCritical
	g.Nodes["workflow:ci.yml"].Score = 30

	output := RenderTree(g, TreeOptions{RiskFilter: []string{"HIGH", "CRITICAL"}})

	// HIGH/CRITICAL root nodes should show
	assert.Contains(t, output, "cobra")
	assert.Contains(t, output, "ci.yml")

	// LOW-risk nodes should not appear (pflag is LOW, is a child of HIGH cobra)
	assert.NotContains(t, output, "pflag")
}

func TestRenderTreeCollapse(t *testing.T) {
	g := buildTestGraph()
	output := RenderTree(g, TreeOptions{CollapseAt: 1})

	// Root-level children should appear
	assert.Contains(t, output, "actions/checkout")
	// Deeper children should be collapsed
	assert.Contains(t, output, "collapsed")
}

func TestRenderTreeJSON(t *testing.T) {
	g := buildTestGraph()
	output := RenderTree(g, TreeOptions{JSON: true})

	// Should be valid JSON
	assert.True(t, strings.HasPrefix(output, "["), "JSON output should start with [")

	// Should contain expected node data
	assert.Contains(t, output, `"name": "actions/checkout"`)
	assert.Contains(t, output, `"score": 92`)
}

func TestRenderTreeMutableMarker(t *testing.T) {
	g := buildTestGraph()
	output := RenderTree(g, TreeOptions{})

	// Major tag should be MUTABLE
	assert.Contains(t, output, "⚡MUTABLE")

	// Count MUTABLE markers - only one node has MajorTag pinning
	assert.Equal(t, 1, strings.Count(output, "⚡MUTABLE"))
}

func TestRenderTreeEmptyGraph(t *testing.T) {
	g := graph.New()
	output := RenderTree(g, TreeOptions{})
	assert.Contains(t, output, "no nodes found")
}

func TestFindRoots(t *testing.T) {
	g := buildTestGraph()
	roots := findRoots(g)

	require.Len(t, roots, 2)

	// workflow should come first (higher NodeType value)
	assert.Equal(t, "workflow:ci.yml", roots[0])
	assert.Equal(t, "package:go/cobra@v1.8.0", roots[1])
}

func TestPassesFilters(t *testing.T) {
	n := &graph.Node{
		Type: graph.NodeAction,
		Risk: core.RiskHigh,
	}

	// No filters
	assert.True(t, passesFilters(n, TreeOptions{}))

	// Type filter match
	assert.True(t, passesFilters(n, TreeOptions{TypeFilter: []graph.NodeType{graph.NodeAction}}))

	// Type filter no match
	assert.False(t, passesFilters(n, TreeOptions{TypeFilter: []graph.NodeType{graph.NodePackage}}))

	// Risk filter match
	assert.True(t, passesFilters(n, TreeOptions{RiskFilter: []string{"HIGH"}}))

	// Risk filter no match
	assert.False(t, passesFilters(n, TreeOptions{RiskFilter: []string{"LOW"}}))
}

func TestIsMutable(t *testing.T) {
	assert.True(t, isMutable(graph.PinningMajorTag))
	assert.True(t, isMutable(graph.PinningBranch))
	assert.True(t, isMutable(graph.PinningUnpinned))
	assert.False(t, isMutable(graph.PinningSHA))
	assert.False(t, isMutable(graph.PinningExactVersion))
	assert.False(t, isMutable(graph.PinningNA))
}
