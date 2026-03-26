// internal/tui/graphview/view.go
package graphview

import (
	"sort"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
)

// ZoomLevel represents the current graph zoom state.
type ZoomLevel int

const (
	ZoomRisk         ZoomLevel = iota // Only HIGH + CRITICAL nodes
	ZoomNeighborhood                  // Selected node + 1-2 hop neighbors
	ZoomCluster                       // All nodes grouped by type
)

// GraphViewModel manages the graph view state and rendering.
type GraphViewModel struct {
	graph     *graph.Graph
	zoomLevel ZoomLevel
	selected  int      // index into visibleNodes
	visible   []string // node IDs currently visible
	rendered  string   // cached render output
	width     int
	height    int
	dirty     bool // needs re-render
}

// NewGraphViewModel creates a new graph view model.
func NewGraphViewModel(g *graph.Graph) *GraphViewModel {
	m := &GraphViewModel{
		graph:     g,
		zoomLevel: ZoomRisk,
		dirty:     true,
	}
	m.rebuildVisible()
	return m
}

// ZoomLevel returns the current zoom level.
func (m *GraphViewModel) ZoomLevel() ZoomLevel {
	return m.zoomLevel
}

// Selected returns the currently selected node ID, or "" if none.
func (m *GraphViewModel) Selected() string {
	if m.selected >= 0 && m.selected < len(m.visible) {
		return m.visible[m.selected]
	}
	return ""
}

// VisibleNodes returns a copy of the currently visible node IDs.
func (m *GraphViewModel) VisibleNodes() []string {
	result := make([]string, len(m.visible))
	copy(result, m.visible)
	return result
}

// SetSize updates the viewport dimensions.
func (m *GraphViewModel) SetSize(width, height int) {
	if m.width != width || m.height != height {
		m.width = width
		m.height = height
		m.dirty = true
	}
}

// ZoomIn enters neighborhood view centered on the currently selected node.
func (m *GraphViewModel) ZoomIn() {
	sel := m.Selected()
	if sel == "" {
		return
	}
	if m.zoomLevel == ZoomRisk {
		m.zoomLevel = ZoomNeighborhood
		m.dirty = true
		m.rebuildVisible()
		// Re-select the same node if it's still visible.
		m.selectByID(sel)
	}
}

// ZoomOut goes back one zoom level.
func (m *GraphViewModel) ZoomOut() {
	switch m.zoomLevel {
	case ZoomNeighborhood:
		m.zoomLevel = ZoomRisk
	case ZoomCluster:
		m.zoomLevel = ZoomRisk
	default:
		return
	}
	sel := m.Selected()
	m.dirty = true
	m.rebuildVisible()
	m.selectByID(sel)
}

// SetZoomCluster switches to the cluster (all-nodes) view.
func (m *GraphViewModel) SetZoomCluster() {
	if m.zoomLevel != ZoomCluster {
		sel := m.Selected()
		m.zoomLevel = ZoomCluster
		m.dirty = true
		m.rebuildVisible()
		m.selectByID(sel)
	}
}

// SelectNext moves selection to the next node.
func (m *GraphViewModel) SelectNext() {
	if len(m.visible) == 0 {
		return
	}
	m.selected++
	if m.selected >= len(m.visible) {
		m.selected = len(m.visible) - 1
	}
	m.dirty = true
}

// SelectPrev moves selection to the previous node.
func (m *GraphViewModel) SelectPrev() {
	if len(m.visible) == 0 {
		return
	}
	m.selected--
	if m.selected < 0 {
		m.selected = 0
	}
	m.dirty = true
}

// View returns the rendered graph string for the current viewport.
func (m *GraphViewModel) View() string {
	if m.width < 10 || m.height < 5 {
		return "Terminal too small for graph view (min 80x24)"
	}
	if len(m.visible) == 0 {
		return "No nodes to display at current zoom level"
	}
	if m.dirty || m.rendered == "" {
		m.rendered = Render(m.graph, m.visible, m.width, m.height)
		m.dirty = false
	}
	return m.rendered
}

// rebuildVisible rebuilds the visible node list based on zoom level.
func (m *GraphViewModel) rebuildVisible() {
	switch m.zoomLevel {
	case ZoomRisk:
		m.visible = m.riskNodes()
	case ZoomNeighborhood:
		m.visible = m.neighborhoodNodes()
	case ZoomCluster:
		m.visible = m.clusterNodes()
	}
	m.clampSelection()
}

// riskNodes returns HIGH + CRITICAL nodes, sorted by score ascending.
func (m *GraphViewModel) riskNodes() []string {
	var ids []string
	for id, n := range m.graph.Nodes {
		if n.Risk == core.RiskHigh || n.Risk == core.RiskCritical {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return m.graph.Nodes[ids[i]].Score < m.graph.Nodes[ids[j]].Score
	})
	return ids
}

// neighborhoodNodes returns the selected node plus 1-2 hop neighbors.
func (m *GraphViewModel) neighborhoodNodes() []string {
	sel := m.Selected()
	if sel == "" {
		return m.riskNodes() // fallback
	}

	seen := make(map[string]bool)
	seen[sel] = true

	// Build reverse adjacency for incoming edges.
	revAdj := make(map[string][]string)
	for _, e := range m.graph.Edges {
		revAdj[e.To] = append(revAdj[e.To], e.From)
	}

	// 1-hop neighbors (outgoing + incoming).
	var hop1 []string
	for _, neighbor := range m.graph.Neighbors(sel) {
		if !seen[neighbor] && m.graph.Nodes[neighbor] != nil {
			seen[neighbor] = true
			hop1 = append(hop1, neighbor)
		}
	}
	for _, neighbor := range revAdj[sel] {
		if !seen[neighbor] && m.graph.Nodes[neighbor] != nil {
			seen[neighbor] = true
			hop1 = append(hop1, neighbor)
		}
	}

	// 2-hop neighbors.
	for _, h1 := range hop1 {
		for _, neighbor := range m.graph.Neighbors(h1) {
			if !seen[neighbor] && m.graph.Nodes[neighbor] != nil {
				seen[neighbor] = true
			}
		}
		for _, neighbor := range revAdj[h1] {
			if !seen[neighbor] && m.graph.Nodes[neighbor] != nil {
				seen[neighbor] = true
			}
		}
	}

	var ids []string
	for id := range seen {
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool {
		return m.graph.Nodes[ids[i]].Score < m.graph.Nodes[ids[j]].Score
	})
	return ids
}

// clusterNodes returns all nodes sorted by type, then by name.
func (m *GraphViewModel) clusterNodes() []string {
	var ids []string
	for id := range m.graph.Nodes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		ni := m.graph.Nodes[ids[i]]
		nj := m.graph.Nodes[ids[j]]
		if ni.Type != nj.Type {
			return ni.Type < nj.Type
		}
		return ni.Name < nj.Name
	})
	return ids
}

// selectByID selects the node with the given ID, if present.
func (m *GraphViewModel) selectByID(id string) {
	for i, vis := range m.visible {
		if vis == id {
			m.selected = i
			return
		}
	}
	m.clampSelection()
}

// clampSelection keeps the selection index within bounds.
func (m *GraphViewModel) clampSelection() {
	if m.selected >= len(m.visible) {
		m.selected = len(m.visible) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}
