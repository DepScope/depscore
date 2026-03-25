// internal/tui/styles.go
package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/depscope/depscope/internal/core"
)

// Risk-level colors.
var (
	colorCritical = lipgloss.Color("#FF0000") // red
	colorHigh     = lipgloss.Color("#FF8800") // orange
	colorMedium   = lipgloss.Color("#FFFF00") // yellow
	colorLow      = lipgloss.Color("#00CC00") // green
	colorUnknown  = lipgloss.Color("#888888") // grey
)

// Risk-level text styles.
var (
	styleCritical = lipgloss.NewStyle().Foreground(colorCritical).Bold(true)
	styleHigh     = lipgloss.NewStyle().Foreground(colorHigh).Bold(true)
	styleMedium   = lipgloss.NewStyle().Foreground(colorMedium)
	styleLow      = lipgloss.NewStyle().Foreground(colorLow)
	styleUnknown  = lipgloss.NewStyle().Foreground(colorUnknown)
)

// UI element styles.
var (
	styleSelected = lipgloss.NewStyle().
			Background(lipgloss.Color("#3C3C3C")).
			Bold(true)

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#5A4FCF")).
			Padding(0, 1)

	styleFooter = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AAAAAA")).
			Background(lipgloss.Color("#1A1A1A")).
			Padding(0, 1)

	styleTreeBranch = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555"))

	stylePanelBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#5A4FCF")).
				Padding(0, 1)

	stylePanelTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#5A4FCF"))

	styleLabel = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))

	styleValue = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#DDDDDD"))

	stylePathArrow = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5A4FCF")).
			Bold(true)
)

// riskStyle returns the lipgloss style for a given risk level string.
func riskStyle(risk string) lipgloss.Style {
	switch risk {
	case "CRITICAL":
		return styleCritical
	case "HIGH":
		return styleHigh
	case "MEDIUM":
		return styleMedium
	case "LOW":
		return styleLow
	default:
		return styleUnknown
	}
}

// riskColorFor returns a style colored by risk level, for use in inspect panel.
func riskColorFor(_ int, risk core.RiskLevel) lipgloss.Style {
	return riskStyle(string(risk))
}

// Tree branch characters.
const (
	treeBranch    = "├── "
	treeLastChild = "└── "
	treePipe      = "│   "
	treeSpace     = "    "
)
