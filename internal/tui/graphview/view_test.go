package graphview

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGraphViewModel(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)

	assert.Equal(t, ZoomRisk, m.ZoomLevel())
	// Risk view: only HIGH + CRITICAL nodes.
	// ci.yml is CRITICAL (score 30), foo is HIGH (score 45).
	vis := m.VisibleNodes()
	assert.Len(t, vis, 2)
	assert.Contains(t, vis, "workflow:ci.yml")
	assert.Contains(t, vis, "package:foo")
}

func TestZoomRiskFiltersCorrectly(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)

	for _, id := range m.VisibleNodes() {
		n := g.Nodes[id]
		require.NotNil(t, n, "node %s should exist", id)
		assert.True(t, n.Risk == core.RiskHigh || n.Risk == core.RiskCritical,
			"node %s has risk %s, expected HIGH or CRITICAL", id, n.Risk)
	}
}

func TestZoomInToNeighborhood(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)
	m.SetSize(80, 24)

	// Select ci.yml (index 0 after sort by score: ci.yml=30, foo=45).
	assert.Equal(t, "workflow:ci.yml", m.Selected())

	m.ZoomIn()
	assert.Equal(t, ZoomNeighborhood, m.ZoomLevel())

	vis := m.VisibleNodes()
	// Should include ci.yml + its 1-hop neighbors (checkout, setup-go)
	// + possibly 2-hop neighbors.
	assert.Contains(t, vis, "workflow:ci.yml")
	assert.Contains(t, vis, "action:checkout")
	assert.Contains(t, vis, "action:setup-go")
	assert.GreaterOrEqual(t, len(vis), 3)
}

func TestZoomOut(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)

	m.ZoomIn()
	assert.Equal(t, ZoomNeighborhood, m.ZoomLevel())

	m.ZoomOut()
	assert.Equal(t, ZoomRisk, m.ZoomLevel())
}

func TestZoomOutFromRiskDoesNothing(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)

	m.ZoomOut()
	assert.Equal(t, ZoomRisk, m.ZoomLevel())
}

func TestSetZoomCluster(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)

	m.SetZoomCluster()
	assert.Equal(t, ZoomCluster, m.ZoomLevel())

	vis := m.VisibleNodes()
	assert.Len(t, vis, 5, "cluster view should show all nodes")
}

func TestClusterNodesGroupedByType(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)

	m.SetZoomCluster()
	vis := m.VisibleNodes()

	// Verify sorted by type first. NodePackage < NodeAction < NodeWorkflow.
	var lastType graph.NodeType
	for _, id := range vis {
		n := g.Nodes[id]
		assert.GreaterOrEqual(t, n.Type, lastType,
			"nodes should be sorted by type: %s has type %d after type %d", id, n.Type, lastType)
		lastType = n.Type
	}
}

func TestZoomOutFromCluster(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)

	m.SetZoomCluster()
	assert.Equal(t, ZoomCluster, m.ZoomLevel())

	m.ZoomOut()
	assert.Equal(t, ZoomRisk, m.ZoomLevel())
}

func TestSelectNextPrev(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)
	m.SetZoomCluster() // Show all 5 nodes.

	// Reset to known position.
	m.selected = 0
	first := m.Selected()
	require.NotEmpty(t, first)

	m.SelectNext()
	assert.Equal(t, 1, m.selected)
	second := m.Selected()
	assert.NotEqual(t, first, second)

	m.SelectPrev()
	assert.Equal(t, 0, m.selected)
	assert.Equal(t, first, m.Selected())

	// SelectPrev at 0 stays at 0.
	m.SelectPrev()
	assert.Equal(t, 0, m.selected)

	// SelectNext past end clamps.
	for range 20 {
		m.SelectNext()
	}
	assert.Equal(t, len(m.visible)-1, m.selected)
}

func TestSelectNextClamps(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)

	// Risk view has 2 nodes.
	m.SelectNext()
	m.SelectNext()
	m.SelectNext() // should clamp
	assert.Equal(t, 1, m.selected)
}

func TestViewTooSmall(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)
	m.SetSize(5, 5)

	output := m.View()
	assert.Contains(t, output, "Terminal too small")
}

func TestViewNoNodes(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.Node{
		ID: "package:a", Type: graph.NodePackage,
		Name: "a", Score: 90, Risk: core.RiskLow,
		Pinning: graph.PinningNA,
	})
	m := NewGraphViewModel(g)

	// Risk view filters to HIGH+CRITICAL only; "a" is LOW.
	m.SetSize(80, 24)
	output := m.View()
	assert.Contains(t, output, "No nodes to display")
}

func TestViewRendersSuccessfully(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)
	m.SetSize(80, 24)

	output := m.View()
	assert.NotEmpty(t, output)
	assert.NotContains(t, output, "too small")
	assert.NotContains(t, output, "No nodes")
}

func TestViewCaching(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)
	m.SetSize(80, 24)

	output1 := m.View()
	output2 := m.View()
	assert.Equal(t, output1, output2, "cached renders should be identical")
}

func TestSetSizeInvalidatesCacheOnChange(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)
	m.SetSize(80, 24)

	_ = m.View()
	assert.False(t, m.dirty, "should not be dirty after render")

	m.SetSize(100, 30)
	assert.True(t, m.dirty, "should be dirty after resize")
}

func TestSetSizeNoChangeDoesNotInvalidate(t *testing.T) {
	g := testGraph()
	m := NewGraphViewModel(g)
	m.SetSize(80, 24)
	_ = m.View()

	m.SetSize(80, 24) // Same size.
	assert.False(t, m.dirty, "same size should not invalidate")
}

func TestZoomInNoSelection(t *testing.T) {
	g := graph.New()
	m := NewGraphViewModel(g)

	// No visible nodes, no selection.
	m.ZoomIn()
	assert.Equal(t, ZoomRisk, m.ZoomLevel(), "zoom should not change with no selection")
}

func TestNeighborhoodIncludesReverseEdges(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.Node{ID: "a", Type: graph.NodePackage, Name: "a", Score: 30, Risk: core.RiskCritical, Pinning: graph.PinningNA})
	g.AddNode(&graph.Node{ID: "b", Type: graph.NodePackage, Name: "b", Score: 40, Risk: core.RiskHigh, Pinning: graph.PinningNA})
	g.AddNode(&graph.Node{ID: "c", Type: graph.NodePackage, Name: "c", Score: 50, Risk: core.RiskMedium, Pinning: graph.PinningNA})
	g.AddEdge(&graph.Edge{From: "c", To: "a", Type: graph.EdgeDependsOn})
	g.AddEdge(&graph.Edge{From: "a", To: "b", Type: graph.EdgeDependsOn})

	m := NewGraphViewModel(g)
	// Risk view: a (CRITICAL, 30), b (HIGH, 40).
	assert.Equal(t, "a", m.Selected()) // lowest score

	m.ZoomIn()
	vis := m.VisibleNodes()
	// a's neighborhood: b (outgoing), c (incoming via reverse edge).
	assert.Contains(t, vis, "a")
	assert.Contains(t, vis, "b")
	assert.Contains(t, vis, "c", "reverse edge neighbor should be included")
}
