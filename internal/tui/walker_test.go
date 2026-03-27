package tui

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkerModel_EmptyGraph(t *testing.T) {
	g := graph.New()
	w := NewWalkerModel(g)

	assert.Equal(t, "", w.CurrentNodeID(), "current should be empty (root level)")
	assert.Empty(t, w.children, "empty graph should have no children")
	assert.Equal(t, "", w.SelectedNodeID(), "no selected node in empty graph")

	// Enter on empty graph should not panic.
	w.Enter()
	assert.Equal(t, "", w.CurrentNodeID())

	// Back on empty graph should not panic.
	w.Back()
	assert.Equal(t, "", w.CurrentNodeID())

	// View should render without panic.
	w.SetSize(80, 24)
	output := w.View()
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "leaf node")
}

func TestWalkerModel_LeafNode(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.Node{
		ID: "package:go/leaf@1.0", Type: graph.NodePackage,
		Name: "leaf", Version: "1.0", Score: 85,
		Risk: core.RiskLow, Pinning: graph.PinningNA,
		Metadata: map[string]any{},
	})

	w := NewWalkerModel(g)
	require.Len(t, w.children, 1, "root level should show 1 child")

	// Drill into the leaf node.
	w.Enter()
	assert.Equal(t, "package:go/leaf@1.0", w.CurrentNodeID())
	assert.Empty(t, w.children, "leaf node should have no children")

	// Enter on leaf does nothing (no children to drill into).
	w.Enter()
	assert.Equal(t, "package:go/leaf@1.0", w.CurrentNodeID(),
		"Enter on leaf should stay at the same node")

	// View should show "no children" message.
	w.SetSize(80, 24)
	output := w.View()
	assert.Contains(t, output, "leaf node")
}

func TestWalkerModel_BackAtRoot(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.Node{
		ID: "action:test", Type: graph.NodeAction,
		Name: "test", Version: "v1", Score: 90,
		Risk: core.RiskLow, Pinning: graph.PinningExactVersion,
		Metadata: map[string]any{},
	})

	w := NewWalkerModel(g)

	// At root level, Back() should do nothing (no crash).
	w.Back()
	assert.Equal(t, "", w.CurrentNodeID(), "Back at root should stay at root")
	assert.Len(t, w.children, 1, "children should remain unchanged")
	assert.Len(t, w.path, 0, "path should remain empty")
}

func TestWalkerModel_DrillAndBack(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.Node{
		ID: "workflow:ci.yml", Type: graph.NodeWorkflow,
		Name: "ci.yml", Version: "", Score: 70,
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
		ID: "action:actions/setup-go", Type: graph.NodeAction,
		Name: "actions/setup-go", Version: "v5", Score: 88,
		Risk: core.RiskLow, Pinning: graph.PinningMajorTag,
		Metadata: map[string]any{},
	})
	g.AddEdge(&graph.Edge{From: "workflow:ci.yml", To: "action:actions/checkout", Type: graph.EdgeUsesAction})
	g.AddEdge(&graph.Edge{From: "workflow:ci.yml", To: "action:actions/setup-go", Type: graph.EdgeUsesAction})

	w := NewWalkerModel(g)

	// At root level, workflow should be a root.
	assert.Equal(t, "", w.CurrentNodeID())
	require.Len(t, w.children, 1, "only workflow is a root")
	assert.Equal(t, "workflow:ci.yml", w.children[0])

	// Drill into workflow.
	w.Enter()
	assert.Equal(t, "workflow:ci.yml", w.CurrentNodeID())
	assert.Len(t, w.children, 2, "workflow has 2 action children")

	// Now go back.
	w.Back()
	assert.Equal(t, "", w.CurrentNodeID(), "should be back at root")
	require.Len(t, w.children, 1, "root children restored")
	assert.Equal(t, "workflow:ci.yml", w.children[0])
}

func TestWalkerModel_CursorBounds(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.Node{
		ID: "package:a", Type: graph.NodePackage,
		Name: "a", Version: "1.0", Score: 80,
		Risk: core.RiskLow, Pinning: graph.PinningNA,
		Metadata: map[string]any{},
	})
	g.AddNode(&graph.Node{
		ID: "package:b", Type: graph.NodePackage,
		Name: "b", Version: "2.0", Score: 70,
		Risk: core.RiskMedium, Pinning: graph.PinningNA,
		Metadata: map[string]any{},
	})

	w := NewWalkerModel(g)
	require.Len(t, w.children, 2)

	// Cursor starts at 0.
	assert.Equal(t, 0, w.cursor)

	// CursorUp at 0 should stay at 0.
	w.CursorUp()
	assert.Equal(t, 0, w.cursor)

	// CursorDown should move to 1.
	w.CursorDown()
	assert.Equal(t, 1, w.cursor)

	// CursorDown at last element should stay.
	w.CursorDown()
	assert.Equal(t, 1, w.cursor)
}
