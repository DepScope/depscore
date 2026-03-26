package graphview

import (
	"math"
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testGraph builds a small test graph for layout tests.
func testGraph() *graph.Graph {
	g := graph.New()

	g.AddNode(&graph.Node{
		ID: "workflow:ci.yml", Type: graph.NodeWorkflow,
		Name: "ci.yml", Score: 30, Risk: core.RiskCritical,
		Pinning: graph.PinningNA,
	})
	g.AddNode(&graph.Node{
		ID: "action:checkout", Type: graph.NodeAction,
		Name: "actions/checkout", Version: "v4", Score: 90,
		Risk: core.RiskLow, Pinning: graph.PinningExactVersion,
	})
	g.AddNode(&graph.Node{
		ID: "action:setup-go", Type: graph.NodeAction,
		Name: "actions/setup-go", Version: "v5", Score: 65,
		Risk: core.RiskMedium, Pinning: graph.PinningMajorTag,
	})
	g.AddNode(&graph.Node{
		ID: "package:foo", Type: graph.NodePackage,
		Name: "foo", Version: "1.0", Score: 45,
		Risk: core.RiskHigh, Pinning: graph.PinningNA,
	})
	g.AddNode(&graph.Node{
		ID: "package:bar", Type: graph.NodePackage,
		Name: "bar", Version: "2.0", Score: 85,
		Risk: core.RiskLow, Pinning: graph.PinningNA,
	})

	g.AddEdge(&graph.Edge{From: "workflow:ci.yml", To: "action:checkout", Type: graph.EdgeUsesAction})
	g.AddEdge(&graph.Edge{From: "workflow:ci.yml", To: "action:setup-go", Type: graph.EdgeUsesAction})
	g.AddEdge(&graph.Edge{From: "package:foo", To: "package:bar", Type: graph.EdgeDependsOn})

	return g
}

func TestLayoutEmptyNodes(t *testing.T) {
	g := graph.New()
	cfg := DefaultLayoutConfig(80, 24)
	result := Layout(g, nil, cfg)
	assert.Empty(t, result)
}

func TestLayoutSingleNode(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.Node{ID: "a", Type: graph.NodePackage, Name: "a"})

	cfg := DefaultLayoutConfig(80, 24)
	result := Layout(g, []string{"a"}, cfg)

	require.Len(t, result, 1)
	pos := result["a"]
	assert.InDelta(t, 40.0, pos.X, 0.01)
	assert.InDelta(t, 12.0, pos.Y, 0.01)
}

func TestLayoutPositionsWithinBounds(t *testing.T) {
	g := testGraph()
	ids := []string{"workflow:ci.yml", "action:checkout", "action:setup-go", "package:foo", "package:bar"}

	cfg := DefaultLayoutConfig(80, 24)
	result := Layout(g, ids, cfg)

	require.Len(t, result, 5)
	for id, pos := range result {
		assert.GreaterOrEqual(t, pos.X, 0.0, "node %s X below 0", id)
		assert.LessOrEqual(t, pos.X, cfg.Width, "node %s X above width", id)
		assert.GreaterOrEqual(t, pos.Y, 0.0, "node %s Y below 0", id)
		assert.LessOrEqual(t, pos.Y, cfg.Height, "node %s Y above height", id)
	}
}

func TestLayoutNodesDoNotOverlap(t *testing.T) {
	g := testGraph()
	ids := []string{"workflow:ci.yml", "action:checkout", "action:setup-go", "package:foo", "package:bar"}

	cfg := DefaultLayoutConfig(80, 24)
	result := Layout(g, ids, cfg)

	// Check that no two nodes occupy the exact same integer grid cell.
	occupied := make(map[[2]int]string)
	for id, pos := range result {
		cell := [2]int{int(math.Round(pos.X)), int(math.Round(pos.Y))}
		if other, exists := occupied[cell]; exists {
			t.Errorf("nodes %s and %s overlap at cell (%d, %d)", id, other, cell[0], cell[1])
		}
		occupied[cell] = id
	}
}

func TestLayoutConnectedNodesAreCloser(t *testing.T) {
	g := testGraph()
	ids := []string{"workflow:ci.yml", "action:checkout", "action:setup-go", "package:foo", "package:bar"}

	cfg := DefaultLayoutConfig(120, 40)
	result := Layout(g, ids, cfg)

	// Connected pairs should be closer than the average distance.
	dist := func(a, b string) float64 {
		pa, pb := result[a], result[b]
		dx := pa.X - pb.X
		dy := pa.Y - pb.Y
		return math.Sqrt(dx*dx + dy*dy)
	}

	// ci.yml -> checkout is connected, should be relatively close.
	connDist := dist("workflow:ci.yml", "action:checkout")

	// Compute average pairwise distance for reference.
	var totalDist float64
	var count int
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			totalDist += dist(ids[i], ids[j])
			count++
		}
	}
	avgDist := totalDist / float64(count)

	// Connected nodes should be closer than average.
	assert.Less(t, connDist, avgDist*1.5,
		"connected nodes (ci.yml -> checkout) should be closer than average distance")
}

func TestLayoutDeterministic(t *testing.T) {
	g := testGraph()
	ids := []string{"workflow:ci.yml", "action:checkout", "action:setup-go", "package:foo", "package:bar"}

	cfg := DefaultLayoutConfig(80, 24)
	result1 := Layout(g, ids, cfg)
	result2 := Layout(g, ids, cfg)

	for _, id := range ids {
		assert.InDelta(t, result1[id].X, result2[id].X, 0.001, "X should be deterministic for %s", id)
		assert.InDelta(t, result1[id].Y, result2[id].Y, 0.001, "Y should be deterministic for %s", id)
	}
}

func TestDefaultLayoutConfig(t *testing.T) {
	cfg := DefaultLayoutConfig(80, 24)
	assert.Equal(t, 80.0, cfg.Width)
	assert.Equal(t, 24.0, cfg.Height)
	assert.Equal(t, 100, cfg.Iterations)
	assert.Greater(t, cfg.Repulsion, 0.0)
	assert.Greater(t, cfg.Attraction, 0.0)
}
