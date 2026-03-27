// internal/tui/walker.go
package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/depscope/depscope/internal/graph"
)

// WalkerModel provides interactive tree navigation within the TUI.
type WalkerModel struct {
	graph    *graph.Graph
	current  string   // current node ID
	children []string // child node IDs of current
	cursor   int      // selected child index
	path     []string // breadcrumb path of node IDs
	width    int
	height   int
}

// NewWalkerModel creates a WalkerModel starting from the roots.
func NewWalkerModel(g *graph.Graph) *WalkerModel {
	roots := walkerFindRoots(g)
	w := &WalkerModel{
		graph:    g,
		current:  "", // empty = at root level
		children: roots,
		cursor:   0,
		path:     nil,
		width:    80,
		height:   24,
	}
	return w
}

// SetSize updates the walker dimensions.
func (w *WalkerModel) SetSize(width, height int) {
	w.width = width
	w.height = height
}

// Enter drills into the selected child node.
func (w *WalkerModel) Enter() {
	if len(w.children) == 0 || w.cursor >= len(w.children) {
		return
	}

	selected := w.children[w.cursor]
	children := walkerSortedNeighbors(w.graph, selected)

	// Push current to breadcrumb.
	w.path = append(w.path, w.current)
	w.current = selected
	w.children = children
	w.cursor = 0
}

// Back goes up to the parent node.
func (w *WalkerModel) Back() {
	if len(w.path) == 0 {
		return
	}

	// Pop from breadcrumb.
	parent := w.path[len(w.path)-1]
	w.path = w.path[:len(w.path)-1]

	w.current = parent
	if parent == "" {
		// Back at root level.
		w.children = walkerFindRoots(w.graph)
	} else {
		w.children = walkerSortedNeighbors(w.graph, parent)
	}

	// Try to keep cursor near where we were.
	w.cursor = 0
}

// CursorUp moves the cursor up.
func (w *WalkerModel) CursorUp() {
	if w.cursor > 0 {
		w.cursor--
	}
}

// CursorDown moves the cursor down.
func (w *WalkerModel) CursorDown() {
	if w.cursor < len(w.children)-1 {
		w.cursor++
	}
}

// CurrentNodeID returns the currently focused node ID.
func (w *WalkerModel) CurrentNodeID() string {
	return w.current
}

// SelectedNodeID returns the ID of the child under the cursor.
func (w *WalkerModel) SelectedNodeID() string {
	if len(w.children) == 0 || w.cursor >= len(w.children) {
		return ""
	}
	return w.children[w.cursor]
}

// View renders the walker as a string.
func (w *WalkerModel) View() string {
	var b strings.Builder

	// Breadcrumb.
	breadcrumb := w.renderBreadcrumb()
	b.WriteString(breadcrumb)
	b.WriteString("\n")

	// Current node info.
	if w.current != "" {
		n := w.graph.Nodes[w.current]
		if n != nil {
			info := w.renderNodeInfo(n)
			b.WriteString(info)
			b.WriteString("\n")
		}
	} else {
		b.WriteString(styleHeader.Width(w.width).Render(" Root level"))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Children list.
	if len(w.children) == 0 {
		b.WriteString(styleLabel.Render("  (no children — leaf node)"))
		b.WriteString("\n")
	} else {
		header := styleLabel.Render(fmt.Sprintf("  Children (%d):", len(w.children)))
		b.WriteString(header)
		b.WriteString("\n")

		// Calculate visible window.
		contentHeight := w.height - 6 // header, breadcrumb, info, spacing, children header
		if contentHeight < 1 {
			contentHeight = 1
		}

		offset := 0
		if w.cursor >= contentHeight {
			offset = w.cursor - contentHeight + 1
		}

		end := offset + contentHeight
		if end > len(w.children) {
			end = len(w.children)
		}

		for i := offset; i < end; i++ {
			childID := w.children[i]
			cn := w.graph.Nodes[childID]
			if cn == nil {
				continue
			}

			line := w.formatChildLine(cn)

			if i == w.cursor {
				line = styleSelected.Render(padRight(line, w.width))
			}

			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderBreadcrumb renders the navigation path.
func (w *WalkerModel) renderBreadcrumb() string {
	parts := []string{"root"}
	for _, pid := range w.path {
		if pid == "" {
			continue
		}
		n := w.graph.Nodes[pid]
		if n != nil {
			parts = append(parts, n.Name)
		}
	}
	if w.current != "" {
		n := w.graph.Nodes[w.current]
		if n != nil {
			parts = append(parts, n.Name)
		}
	}

	crumb := strings.Join(parts, " > ")
	return styleLabel.Render("  " + crumb)
}

// renderNodeInfo renders details about the current node.
func (w *WalkerModel) renderNodeInfo(n *graph.Node) string {
	version := n.Version
	if version == "" {
		version = n.Ref
	}
	if version == "" {
		version = "-"
	}

	risk := string(n.Risk)
	rs := riskStyle(risk)

	pinning := ""
	if n.Pinning != graph.PinningNA {
		pinning = fmt.Sprintf("  pinning:%s", n.Pinning.String())
	}

	info := fmt.Sprintf("  [%s] %s@%s  score:%d  %s%s",
		n.Type.String(), n.Name, version, n.Score, risk, pinning)

	return rs.Render(info)
}

// formatChildLine formats a child node for the list.
func (w *WalkerModel) formatChildLine(n *graph.Node) string {
	version := n.Version
	if version == "" {
		version = n.Ref
	}
	if version == "" {
		version = "-"
	}

	risk := string(n.Risk)
	rs := riskStyle(risk)

	hasChildren := len(w.graph.Neighbors(n.ID)) > 0
	arrow := "  "
	if hasChildren {
		arrow = " >"
	}

	line := fmt.Sprintf("  %s [%s] %s@%s  ●%d %s",
		arrow, n.Type.String(), n.Name, version, n.Score, risk)

	return rs.Render(line)
}

// walkerFindRoots returns root node IDs for the walker (no incoming edges).
func walkerFindRoots(g *graph.Graph) []string {
	hasIncoming := make(map[string]bool)
	for _, e := range g.Edges {
		hasIncoming[e.To] = true
	}

	var roots []string
	for id := range g.Nodes {
		if !hasIncoming[id] {
			roots = append(roots, id)
		}
	}

	sort.Slice(roots, func(i, j int) bool {
		ni := g.Nodes[roots[i]]
		nj := g.Nodes[roots[j]]
		if ni.Type != nj.Type {
			return ni.Type > nj.Type // workflows first
		}
		return ni.Name < nj.Name
	})

	return roots
}

// walkerSortedNeighbors returns neighbors sorted by type then name.
func walkerSortedNeighbors(g *graph.Graph, nodeID string) []string {
	neighbors := g.Neighbors(nodeID)
	sorted := make([]string, len(neighbors))
	copy(sorted, neighbors)

	sort.Slice(sorted, func(i, j int) bool {
		ni := g.Nodes[sorted[i]]
		nj := g.Nodes[sorted[j]]
		if ni == nil || nj == nil {
			return sorted[i] < sorted[j]
		}
		if ni.Type != nj.Type {
			return ni.Type < nj.Type
		}
		return ni.Name < nj.Name
	})
	return sorted
}
