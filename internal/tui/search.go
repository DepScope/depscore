// internal/tui/search.go
package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// updateSearch handles key events while in search mode.
func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Confirm search: apply filter, exit search mode
		m.searchQuery = m.searchInput.Value()
		m.searching = false
		m.searchInput.Blur()
		m.rebuildVisible()
		m.cursor = 0
		m.offset = 0
		return m, nil

	case "esc":
		// Cancel search
		m.searching = false
		m.searchQuery = ""
		m.searchInput.SetValue("")
		m.searchInput.Blur()
		m.rebuildVisible()
		m.clampCursor()
		return m, nil
	}

	// Forward to textinput
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)

	// Live filter as user types
	m.searchQuery = m.searchInput.Value()
	m.rebuildVisible()
	m.clampCursor()

	return m, cmd
}

// renderSearchBar renders the search input at the top.
func (m Model) renderSearchBar() string {
	return "  / " + m.searchInput.View()
}

// applySearchFilter filters a list of node IDs to those matching the search query.
func (m *Model) applySearchFilter(ids []string) []string {
	if m.searchQuery == "" {
		return ids
	}

	query := strings.ToLower(m.searchQuery)
	var filtered []string
	for _, id := range ids {
		n := m.graph.Nodes[id]
		if n == nil {
			continue
		}
		name := strings.ToLower(n.Name)
		nodeID := strings.ToLower(id)
		if strings.Contains(name, query) || strings.Contains(nodeID, query) {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

// StartSearch is an exported helper for initiating search mode (useful for testing).
func (m *Model) StartSearch() {
	m.searching = true
	m.searchInput.Focus()
}

// SetSearchQuery is an exported helper to set search directly (useful for testing).
func (m *Model) SetSearchQuery(q string) {
	m.searchQuery = q
	m.searchInput.SetValue(q)
	m.rebuildVisible()
	m.clampCursor()
}

// Exported textinput constructor for tests that need it.
func newSearchInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "search..."
	ti.CharLimit = 64
	return ti
}
