// internal/tui/flat.go
package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/depscope/depscope/internal/graph"
)

// buildFlatVisible builds the visible list for flat view: all nodes sorted by score ascending.
func (m *Model) buildFlatVisible() []string {
	var ids []string
	for id := range m.graph.Nodes {
		if m.passesFilter(id) {
			ids = append(ids, id)
		}
	}

	// Sort by score ascending (worst first), then by type, then by name
	sort.Slice(ids, func(i, j int) bool {
		ni := m.graph.Nodes[ids[i]]
		nj := m.graph.Nodes[ids[j]]
		if ni.Score != nj.Score {
			return ni.Score < nj.Score
		}
		if ni.Type != nj.Type {
			return ni.Type < nj.Type
		}
		return ni.Name < nj.Name
	})

	return ids
}

// renderFlat renders the flat sorted view.
func (m Model) renderFlat(maxHeight, maxWidth int) string {
	if len(m.visible) == 0 {
		return styleLabel.Render("  (no nodes match current filter)")
	}

	// Column header
	headerLine := fmt.Sprintf("  %-10s %-30s %-15s %6s %-10s %-12s",
		"TYPE", "NAME", "VERSION", "SCORE", "RISK", "PINNING")
	if lipglossWidth(headerLine) > maxWidth {
		headerLine = truncate(headerLine, maxWidth)
	}
	headerLine = padRight(headerLine, maxWidth)
	headerLine = styleLabel.Render(headerLine)

	lines := []string{headerLine}

	end := m.offset + maxHeight - 1 // -1 for the header
	if end > len(m.visible) {
		end = len(m.visible)
	}

	start := m.offset
	if start < 0 {
		start = 0
	}

	prevType := graph.NodeType(-1)
	for idx := start; idx < end; idx++ {
		nodeID := m.visible[idx]
		n := m.graph.Nodes[nodeID]
		if n == nil {
			continue
		}

		// Group separator
		if n.Type != prevType && prevType != graph.NodeType(-1) {
			sep := padRight(styleTreeBranch.Render("  "+strings.Repeat("─", maxWidth-4)), maxWidth)
			lines = append(lines, sep)
		}
		prevType = n.Type

		version := n.Version
		if version == "" {
			version = n.Ref
		}
		if version == "" {
			version = "-"
		}

		pinning := n.Pinning.String()
		risk := string(n.Risk)
		rs := riskStyle(risk)

		line := fmt.Sprintf("  %-10s %-30s %-15s %6d %-10s %-12s",
			n.Type.String(), n.Name, version, n.Score, risk, pinning)

		if lipglossWidth(line) > maxWidth {
			line = truncate(line, maxWidth)
		}
		line = padRight(line, maxWidth)

		// Colorize the risk portion
		line = rs.Render(line)

		if idx == m.cursor {
			line = styleSelected.Render(line)
		}

		lines = append(lines, line)
	}

	// Pad remaining lines
	for len(lines) < maxHeight {
		lines = append(lines, strings.Repeat(" ", maxWidth))
	}

	return strings.Join(lines, "\n")
}
