// internal/tui/model.go
package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/depscope/depscope/internal/graph"
)

type viewMode int

const (
	viewTree viewMode = iota
	viewFlat
)

// Model is the bubbletea model for the TUI explorer.
type Model struct {
	graph       *graph.Graph
	roots       []string        // root node IDs (no incoming edges)
	expanded    map[string]bool // which nodes are expanded in tree view
	cursor      int
	visible     []string // node IDs in current display order
	mode        viewMode
	width       int
	height      int
	searching   bool
	searchInput textinput.Model
	searchQuery string
	filterLevel string   // "" = all, "HIGH" = HIGH+CRITICAL, "CRITICAL"
	inspecting  string   // node ID being inspected (empty = none)
	showPaths   bool     // showing paths view
	pathResults [][]string // paths from roots to selected node
	offset      int      // scroll offset for viewport
}

// NewModel creates a new TUI model from a graph.
func NewModel(g *graph.Graph) Model {
	ti := textinput.New()
	ti.Placeholder = "search..."
	ti.CharLimit = 64

	m := Model{
		graph:       g,
		expanded:    make(map[string]bool),
		searchInput: ti,
		width:       80,
		height:      24,
	}

	m.roots = findRoots(g)
	m.rebuildVisible()
	return m
}

// findRoots returns node IDs that have no incoming edges.
// Sorted: workflows first, then by node type, then by name.
func findRoots(g *graph.Graph) []string {
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
		// Workflows first
		if ni.Type != nj.Type {
			return ni.Type > nj.Type // NodeWorkflow(3) > NodePackage(0)
		}
		return ni.Name < nj.Name
	})

	return roots
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampCursor()
		return m, nil

	case tea.KeyMsg:
		// If searching, delegate to search handler
		if m.searching {
			return m.updateSearch(msg)
		}

		// If showing paths, Esc closes
		if m.showPaths {
			switch msg.String() {
			case "esc":
				m.showPaths = false
				m.pathResults = nil
				return m, nil
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}

		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case "down", "j":
			if m.cursor < len(m.visible)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case "ctrl+u":
			// Page up
			m.cursor -= m.contentHeight() / 2
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.ensureVisible()
		case "ctrl+d":
			// Page down
			m.cursor += m.contentHeight() / 2
			if m.cursor >= len(m.visible) {
				m.cursor = len(m.visible) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.ensureVisible()
		case "enter":
			if m.cursor < len(m.visible) {
				id := m.visible[m.cursor]
				neighbors := m.graph.Neighbors(id)
				if m.mode == viewTree && len(neighbors) > 0 {
					// Has children: expand/collapse
					m.expanded[id] = !m.expanded[id]
					m.rebuildVisible()
					m.clampCursor()
				} else {
					// Leaf node or flat view: open inspect panel
					if m.inspecting == id {
						m.inspecting = ""
					} else {
						m.inspecting = id
					}
				}
			}
		case "tab":
			if m.mode == viewTree {
				m.mode = viewFlat
			} else {
				m.mode = viewTree
			}
			m.rebuildVisible()
			m.cursor = 0
			m.offset = 0
		case "/":
			m.searching = true
			m.searchInput.Focus()
			return m, textinput.Blink
		case "f":
			m.cycleFilter()
			m.rebuildVisible()
			m.clampCursor()
		case "i":
			if m.cursor < len(m.visible) {
				id := m.visible[m.cursor]
				if m.inspecting == id {
					m.inspecting = ""
				} else {
					m.inspecting = id
				}
			}
		case "p":
			if m.cursor < len(m.visible) {
				m.computePaths(m.visible[m.cursor])
				m.showPaths = true
			}
		case "esc":
			if m.inspecting != "" {
				m.inspecting = ""
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	// Header
	header := m.renderHeader()
	b.WriteString(header)
	b.WriteString("\n")

	// Search bar (if active)
	if m.searching {
		b.WriteString(m.renderSearchBar())
		b.WriteString("\n")
	}

	// Main content area
	contentHeight := m.contentHeight()
	if m.showPaths {
		b.WriteString(m.renderPaths(contentHeight))
	} else if m.inspecting != "" {
		// Split layout: content left, inspect right
		mainWidth := m.width * 60 / 100
		panelWidth := m.width - mainWidth - 1

		var content string
		if m.mode == viewTree {
			content = m.renderTree(contentHeight, mainWidth)
		} else {
			content = m.renderFlat(contentHeight, mainWidth)
		}

		panel := m.renderInspect(contentHeight, panelWidth)
		joined := lipgloss.JoinHorizontal(lipgloss.Top, content, " ", panel)
		b.WriteString(joined)
	} else {
		if m.mode == viewTree {
			b.WriteString(m.renderTree(contentHeight, m.width))
		} else {
			b.WriteString(m.renderFlat(contentHeight, m.width))
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(m.renderFooter())

	return b.String()
}

// renderHeader renders the header bar.
func (m Model) renderHeader() string {
	nodeCount := len(m.graph.Nodes)
	edgeCount := len(m.graph.Edges)

	filterStr := "ALL"
	if m.filterLevel != "" {
		filterStr = m.filterLevel + "+"
	}

	viewStr := "tree"
	if m.mode == viewFlat {
		viewStr = "flat"
	}

	text := fmt.Sprintf(" depscope explore -- %d nodes, %d edges | Filter: %s | View: %s ",
		nodeCount, edgeCount, filterStr, viewStr)

	return styleHeader.Width(m.width).Render(text)
}

// renderFooter renders the help bar.
func (m Model) renderFooter() string {
	if m.showPaths {
		return styleFooter.Width(m.width).Render(
			" [esc] close paths  [q] quit")
	}
	return styleFooter.Width(m.width).Render(
		" [↑↓] navigate  [enter] expand  [/] search  [f] filter  [i] inspect  [p] paths  [Tab] view  [q] quit")
}

// contentHeight returns the available lines for the main content.
func (m Model) contentHeight() int {
	used := 2 // header + footer
	if m.searching {
		used++
	}
	h := m.height - used
	if h < 1 {
		h = 1
	}
	return h
}

// rebuildVisible rebuilds the visible node list based on current mode/state.
func (m *Model) rebuildVisible() {
	switch m.mode {
	case viewTree:
		m.visible = m.buildTreeVisible()
	case viewFlat:
		m.visible = m.buildFlatVisible()
	}

	// Apply search filter
	if m.searchQuery != "" {
		m.visible = m.applySearchFilter(m.visible)
	}
}

// clampCursor keeps the cursor within bounds.
func (m *Model) clampCursor() {
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureVisible()
}

// ensureVisible adjusts offset so the cursor row is on screen.
func (m *Model) ensureVisible() {
	ch := m.contentHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+ch {
		m.offset = m.cursor - ch + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// passesFilter checks if a node passes the current risk filter.
func (m *Model) passesFilter(nodeID string) bool {
	if m.filterLevel == "" {
		return true
	}
	n := m.graph.Nodes[nodeID]
	if n == nil {
		return false
	}
	risk := string(n.Risk)
	switch m.filterLevel {
	case "HIGH":
		return risk == "HIGH" || risk == "CRITICAL"
	case "CRITICAL":
		return risk == "CRITICAL"
	}
	return true
}

// computePaths finds all paths from roots to the given node.
func (m *Model) computePaths(nodeID string) {
	m.pathResults = nil
	for _, root := range m.roots {
		paths := m.graph.FindPaths(root, nodeID, 10)
		m.pathResults = append(m.pathResults, paths...)
	}
}

// formatNodeLine builds a display string for a node.
func formatNodeLine(n *graph.Node) string {
	version := n.Version
	if version == "" {
		version = n.Ref
	}
	if version == "" {
		version = "-"
	}

	risk := string(n.Risk)
	pinning := ""
	if n.Pinning != graph.PinningNA {
		pinning = fmt.Sprintf(" [%s]", n.Pinning.String())
	}

	return fmt.Sprintf("%s@%s  score:%d  %s%s",
		n.Name, version, n.Score, risk, pinning)
}
