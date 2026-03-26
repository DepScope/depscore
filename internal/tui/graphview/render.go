// internal/tui/graphview/render.go
package graphview

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
)

// Node symbols by type.
const (
	SymbolPackage  = '●'
	SymbolAction   = '◆'
	SymbolWorkflow = '■'
	SymbolDocker   = '⬡'
	SymbolScript   = '▲'
)

// nodeSymbol returns the Unicode symbol for a node type.
func nodeSymbol(t graph.NodeType) rune {
	switch t {
	case graph.NodeAction:
		return SymbolAction
	case graph.NodeWorkflow:
		return SymbolWorkflow
	case graph.NodeDockerImage:
		return SymbolDocker
	case graph.NodeScriptDownload:
		return SymbolScript
	default:
		return SymbolPackage
	}
}

// Risk colors.
var (
	styleRiskCritical = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	styleRiskHigh     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8800"))
	styleRiskMedium   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))
	styleRiskLow      = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CC00"))
	styleRiskUnknown  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
)

// riskStyle returns the lipgloss style for a risk level.
func riskStyle(risk core.RiskLevel) lipgloss.Style {
	switch risk {
	case core.RiskCritical:
		return styleRiskCritical
	case core.RiskHigh:
		return styleRiskHigh
	case core.RiskMedium:
		return styleRiskMedium
	case core.RiskLow:
		return styleRiskLow
	default:
		return styleRiskUnknown
	}
}

// Edge colors by type.
var (
	styleEdgeDependsOn = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	styleEdgeUses      = lipgloss.NewStyle().Foreground(lipgloss.Color("#4488FF"))
	styleEdgeBundles   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8800"))
	styleEdgeDownloads = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	styleEdgePulls     = lipgloss.NewStyle().Foreground(lipgloss.Color("#00CCCC"))
	styleEdgeTriggers  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8844FF"))
)

// edgeStyle returns the lipgloss style for an edge type.
func edgeStyle(t graph.EdgeType) lipgloss.Style {
	switch t {
	case graph.EdgeUsesAction:
		return styleEdgeUses
	case graph.EdgeBundles:
		return styleEdgeBundles
	case graph.EdgeDownloads:
		return styleEdgeDownloads
	case graph.EdgePullsImage:
		return styleEdgePulls
	case graph.EdgeTriggers:
		return styleEdgeTriggers
	default:
		return styleEdgeDependsOn
	}
}

// Render produces a terminal string representing the graph.
// It computes force-directed layout, draws edges, places node symbols
// colored by risk, and adds truncated name labels.
func Render(g *graph.Graph, nodeIDs []string, width, height int) string {
	if width < 10 || height < 5 || len(nodeIDs) == 0 {
		return ""
	}

	// Reserve space for labels (leave right margin).
	layoutWidth := width - 2
	layoutHeight := height

	cfg := DefaultLayoutConfig(layoutWidth, layoutHeight)
	positions := Layout(g, nodeIDs, cfg)

	// Map float positions to integer grid coords.
	gridPos := make(map[string][2]int, len(nodeIDs))
	occupied := make(map[[2]int]bool)
	for _, id := range nodeIDs {
		p := positions[id]
		gx := int(math.Round(p.X))
		gy := int(math.Round(p.Y))
		// Clamp to grid.
		gx = clampInt(gx, 1, layoutWidth-2)
		gy = clampInt(gy, 0, layoutHeight-1)
		// Resolve collisions by nudging.
		cell := [2]int{gx, gy}
		for occupied[cell] {
			cell[0]++
			if cell[0] >= layoutWidth {
				cell[0] = 1
				cell[1]++
			}
			if cell[1] >= layoutHeight {
				break
			}
		}
		gridPos[id] = cell
		occupied[cell] = true
	}

	// Create character grid.
	grid := make([][]Cell, height)
	for y := range height {
		grid[y] = make([]Cell, width)
		for x := range width {
			grid[y][x] = Cell{Char: ' '}
		}
	}

	// Draw edges first (so nodes overwrite them).
	nodeSet := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeSet[id] = true
	}
	for _, e := range g.Edges {
		if !nodeSet[e.From] || !nodeSet[e.To] {
			continue
		}
		fromPos := gridPos[e.From]
		toPos := gridPos[e.To]
		style := edgeStyle(e.Type)
		DrawEdge(grid, fromPos[0], fromPos[1], toPos[0], toPos[1], style)
	}

	// Place node symbols and labels.
	for _, id := range nodeIDs {
		n := g.Nodes[id]
		if n == nil {
			continue
		}
		pos := gridPos[id]
		x, y := pos[0], pos[1]
		if y < 0 || y >= height || x < 0 || x >= width {
			continue
		}

		sym := nodeSymbol(n.Type)
		style := riskStyle(n.Risk)
		grid[y][x] = Cell{Char: sym, Style: style}

		// Truncated label to the right of the node.
		label := truncate(n.Name, 12)
		for i, ch := range label {
			lx := x + 1 + i
			if lx >= width {
				break
			}
			grid[y][lx] = Cell{Char: ch, Style: style}
		}
	}

	// Convert grid to styled string.
	return gridToString(grid)
}

// gridToString converts a 2D cell grid into a single styled string.
func gridToString(grid [][]Cell) string {
	var b strings.Builder
	for y, row := range grid {
		if y > 0 {
			b.WriteByte('\n')
		}
		for _, cell := range row {
			ch := cell.Char
			if ch == 0 {
				ch = ' '
			}
			styled := cell.Style.Render(string(ch))
			b.WriteString(styled)
		}
	}
	return b.String()
}

// truncate shortens s to maxLen characters, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}

// clampInt restricts v to the range [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
