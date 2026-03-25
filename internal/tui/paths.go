// internal/tui/paths.go
package tui

import (
	"fmt"
	"strings"
)

// renderPaths renders the paths view, showing all paths from roots to the selected node.
func (m Model) renderPaths(maxHeight int) string {
	if len(m.pathResults) == 0 {
		return styleLabel.Render("  (no paths found to selected node)")
	}

	var lines []string

	selected := ""
	if m.cursor < len(m.visible) {
		selected = m.visible[m.cursor]
	}

	title := stylePanelTitle.Render(fmt.Sprintf(" Paths to: %s ", selected))
	lines = append(lines, title)
	lines = append(lines, "")

	for i, path := range m.pathResults {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, styleLabel.Render(fmt.Sprintf("  Path %d:", i+1)))

		for j, nodeID := range path {
			n := m.graph.Nodes[nodeID]
			if n == nil {
				continue
			}

			rs := riskStyle(string(n.Risk))
			nodeText := rs.Render(fmt.Sprintf("%s@%s (score:%d, %s)", n.Name, n.Version, n.Score, string(n.Risk)))

			if j == 0 {
				lines = append(lines, "    "+nodeText)
			} else {
				arrow := stylePathArrow.Render("    │")
				lines = append(lines, arrow)
				arrowDown := stylePathArrow.Render("    ▼")
				lines = append(lines, arrowDown)
				lines = append(lines, "    "+nodeText)
			}
		}

		// Limit displayed paths to avoid overwhelming output
		if i >= 9 {
			lines = append(lines, "")
			lines = append(lines, styleLabel.Render(fmt.Sprintf("  ... and %d more paths", len(m.pathResults)-10)))
			break
		}
	}

	// Pad to height
	for len(lines) < maxHeight {
		lines = append(lines, "")
	}
	if len(lines) > maxHeight {
		lines = lines[:maxHeight]
	}

	return strings.Join(lines, "\n")
}
