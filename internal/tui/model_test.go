package tui

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testGraph builds a small test graph:
//
//	workflow:ci.yml (CRITICAL, score 30)
//	  ├── action:actions/checkout (LOW, score 90)
//	  └── action:actions/setup-go (MEDIUM, score 65)
//	package:go/foo@1.0 (HIGH, score 45)
//	  └── package:go/bar@2.0 (LOW, score 85)
func testGraph() *graph.Graph {
	g := graph.New()

	g.AddNode(&graph.Node{
		ID: "workflow:ci.yml", Type: graph.NodeWorkflow,
		Name: "ci.yml", Version: "", Score: 30,
		Risk: core.RiskCritical, Pinning: graph.PinningNA,
		Metadata: map[string]any{"file": ".github/workflows/ci.yml"},
	})
	g.AddNode(&graph.Node{
		ID: "action:actions/checkout", Type: graph.NodeAction,
		Name: "actions/checkout", Version: "v4", Score: 90,
		Risk: core.RiskLow, Pinning: graph.PinningExactVersion,
		Metadata: map[string]any{"action_type": "github"},
	})
	g.AddNode(&graph.Node{
		ID: "action:actions/setup-go", Type: graph.NodeAction,
		Name: "actions/setup-go", Version: "v5", Score: 65,
		Risk: core.RiskMedium, Pinning: graph.PinningMajorTag,
		Metadata: map[string]any{"action_type": "github"},
	})
	g.AddNode(&graph.Node{
		ID: "package:go/foo@1.0", Type: graph.NodePackage,
		Name: "foo", Version: "1.0", Score: 45,
		Risk: core.RiskHigh, Pinning: graph.PinningNA,
		Metadata: map[string]any{"ecosystem": "go", "depth": 1},
	})
	g.AddNode(&graph.Node{
		ID: "package:go/bar@2.0", Type: graph.NodePackage,
		Name: "bar", Version: "2.0", Score: 85,
		Risk: core.RiskLow, Pinning: graph.PinningNA,
		Metadata: map[string]any{"ecosystem": "go", "depth": 2},
	})

	// Edges
	g.AddEdge(&graph.Edge{From: "workflow:ci.yml", To: "action:actions/checkout", Type: graph.EdgeUsesAction})
	g.AddEdge(&graph.Edge{From: "workflow:ci.yml", To: "action:actions/setup-go", Type: graph.EdgeUsesAction})
	g.AddEdge(&graph.Edge{From: "package:go/foo@1.0", To: "package:go/bar@2.0", Type: graph.EdgeDependsOn, Depth: 1})

	return g
}

func TestNewModel(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	assert.NotNil(t, m.graph)
	assert.NotEmpty(t, m.roots)
	assert.NotEmpty(t, m.visible)
	assert.Equal(t, viewTree, m.mode)
	assert.Equal(t, 0, m.cursor)
	assert.Equal(t, "", m.filterLevel)
	assert.Equal(t, "", m.inspecting)
	assert.False(t, m.searching)
}

func TestRootsAreNodesWithNoIncomingEdges(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	// workflow:ci.yml and package:go/foo@1.0 have no incoming edges
	// action:actions/checkout, action:actions/setup-go, package:go/bar@2.0 have incoming edges
	require.Len(t, m.roots, 2)

	rootSet := make(map[string]bool)
	for _, r := range m.roots {
		rootSet[r] = true
	}
	assert.True(t, rootSet["workflow:ci.yml"], "workflow should be a root")
	assert.True(t, rootSet["package:go/foo@1.0"], "foo should be a root")
}

func TestRootsSortWorkflowsFirst(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	// Workflows have higher NodeType value, so they sort first
	assert.Equal(t, "workflow:ci.yml", m.roots[0])
}

func TestTreeVisibleShowsRoots(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	// Initially only roots are visible (nothing expanded)
	assert.Len(t, m.visible, 2)
	assert.Contains(t, m.visible, "workflow:ci.yml")
	assert.Contains(t, m.visible, "package:go/foo@1.0")
}

func TestExpandCollapse(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	// Expand workflow root
	m.expanded["workflow:ci.yml"] = true
	m.rebuildVisible()

	// Should now show workflow + 2 actions + foo = 4
	assert.Len(t, m.visible, 4)

	// Collapse
	m.expanded["workflow:ci.yml"] = false
	m.rebuildVisible()
	assert.Len(t, m.visible, 2)
}

func TestExpandShowsChildren(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	// Expand foo
	m.expanded["package:go/foo@1.0"] = true
	m.rebuildVisible()

	// Should show: workflow, foo, bar = 3
	assert.Len(t, m.visible, 3)
	assert.Contains(t, m.visible, "package:go/bar@2.0")
}

func TestFilterCycling(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	assert.Equal(t, "", m.FilterLevel())

	m.cycleFilter()
	assert.Equal(t, "HIGH", m.FilterLevel())

	m.cycleFilter()
	assert.Equal(t, "CRITICAL", m.FilterLevel())

	m.cycleFilter()
	assert.Equal(t, "", m.FilterLevel())
}

func TestFilterHighShowsHighAndCritical(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	// Expand everything so all nodes are potentially visible
	for id := range g.Nodes {
		m.expanded[id] = true
	}

	m.SetFilterLevel("HIGH")

	// HIGH filter: shows HIGH + CRITICAL nodes (and their ancestors/descendants that match)
	// CRITICAL: workflow:ci.yml (score 30)
	// HIGH: package:go/foo@1.0 (score 45)
	// LOW nodes should not appear unless they are ancestors of matching nodes
	for _, id := range m.VisibleIDs() {
		n := g.Nodes[id]
		assert.True(t, n.Risk == core.RiskHigh || n.Risk == core.RiskCritical,
			"node %s with risk %s should not be visible with HIGH filter", n.Name, n.Risk)
	}
}

func TestFilterCriticalShowsOnlyCritical(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	m.SetFilterLevel("CRITICAL")

	// Only CRITICAL nodes
	for _, id := range m.VisibleIDs() {
		n := g.Nodes[id]
		assert.Equal(t, core.RiskCritical, n.Risk,
			"node %s with risk %s should not be visible with CRITICAL filter", n.Name, n.Risk)
	}
}

func TestSearchFilter(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	// Expand everything
	for id := range g.Nodes {
		m.expanded[id] = true
	}
	m.rebuildVisible()
	allCount := m.VisibleCount()

	// Search for "checkout"
	m.SetSearchQuery("checkout")
	assert.Less(t, m.VisibleCount(), allCount)
	assert.Contains(t, m.VisibleIDs(), "action:actions/checkout")

	// Clear search
	m.SetSearchQuery("")
	assert.Equal(t, allCount, m.VisibleCount())
}

func TestFlatViewSortsByScore(t *testing.T) {
	g := testGraph()
	m := NewModel(g)
	m.mode = viewFlat
	m.rebuildVisible()

	// Flat view: all nodes sorted by score ascending
	assert.Len(t, m.visible, 5)

	// First should be lowest score
	firstNode := g.Nodes[m.visible[0]]
	lastNode := g.Nodes[m.visible[len(m.visible)-1]]
	assert.LessOrEqual(t, firstNode.Score, lastNode.Score)
}

func TestViewToggle(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	assert.Equal(t, viewTree, m.mode)

	// Toggle to flat
	m.mode = viewFlat
	m.rebuildVisible()
	assert.Equal(t, viewFlat, m.mode)
	// Flat view shows all 5 nodes
	assert.Len(t, m.visible, 5)

	// Toggle back to tree
	m.mode = viewTree
	m.rebuildVisible()
	assert.Equal(t, viewTree, m.mode)
	// Tree shows only roots (2)
	assert.Len(t, m.visible, 2)
}

func TestCursorClamp(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	m.cursor = 100
	m.clampCursor()
	assert.Equal(t, len(m.visible)-1, m.cursor)

	m.cursor = -5
	m.clampCursor()
	assert.Equal(t, 0, m.cursor)
}

func TestViewRendersWithoutPanic(t *testing.T) {
	g := testGraph()
	m := NewModel(g)
	m.width = 120
	m.height = 30

	// Tree view
	output := m.View()
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "depscope explore")

	// Flat view
	m.mode = viewFlat
	m.rebuildVisible()
	output = m.View()
	assert.NotEmpty(t, output)

	// With inspect
	m.inspecting = "workflow:ci.yml"
	output = m.View()
	assert.NotEmpty(t, output)

	// With paths
	m.inspecting = ""
	m.showPaths = true
	m.pathResults = [][]string{{"workflow:ci.yml", "action:actions/checkout"}}
	output = m.View()
	assert.NotEmpty(t, output)
}

func TestFormatNodeLine(t *testing.T) {
	n := &graph.Node{
		Name:    "actions/checkout",
		Version: "v4",
		Score:   90,
		Risk:    core.RiskLow,
		Pinning: graph.PinningExactVersion,
	}

	line := formatNodeLine(n)
	assert.Contains(t, line, "actions/checkout")
	assert.Contains(t, line, "v4")
	assert.Contains(t, line, "90")
	assert.Contains(t, line, "LOW")
	assert.Contains(t, line, "exact_version")
}

func TestEmptyGraph(t *testing.T) {
	g := graph.New()
	m := NewModel(g)

	assert.Empty(t, m.roots)
	assert.Empty(t, m.visible)
	assert.Equal(t, 0, m.cursor)

	// Render should not panic
	m.width = 80
	m.height = 24
	output := m.View()
	assert.NotEmpty(t, output)
}

func TestPassesFilter(t *testing.T) {
	g := testGraph()
	m := NewModel(g)

	// No filter: everything passes
	assert.True(t, m.passesFilter("workflow:ci.yml"))
	assert.True(t, m.passesFilter("action:actions/checkout"))

	// HIGH filter
	m.filterLevel = "HIGH"
	assert.True(t, m.passesFilter("workflow:ci.yml"))        // CRITICAL passes HIGH+
	assert.True(t, m.passesFilter("package:go/foo@1.0"))     // HIGH passes HIGH+
	assert.False(t, m.passesFilter("action:actions/checkout")) // LOW doesn't pass
	assert.False(t, m.passesFilter("package:go/bar@2.0"))     // LOW doesn't pass

	// CRITICAL filter
	m.filterLevel = "CRITICAL"
	assert.True(t, m.passesFilter("workflow:ci.yml"))         // CRITICAL passes
	assert.False(t, m.passesFilter("package:go/foo@1.0"))     // HIGH doesn't pass
	assert.False(t, m.passesFilter("action:actions/checkout")) // LOW doesn't pass
}
