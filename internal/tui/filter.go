// internal/tui/filter.go
package tui

// cycleFilter cycles through filter states: "" -> "HIGH" -> "CRITICAL" -> "".
func (m *Model) cycleFilter() {
	switch m.filterLevel {
	case "":
		m.filterLevel = "HIGH"
	case "HIGH":
		m.filterLevel = "CRITICAL"
	case "CRITICAL":
		m.filterLevel = ""
	default:
		m.filterLevel = ""
	}
}

// FilterLevel returns the current filter level (exported for testing).
func (m *Model) FilterLevel() string {
	return m.filterLevel
}

// SetFilterLevel sets the filter level (exported for testing).
func (m *Model) SetFilterLevel(level string) {
	m.filterLevel = level
	m.rebuildVisible()
	m.clampCursor()
}

// VisibleCount returns the number of visible nodes (exported for testing).
func (m *Model) VisibleCount() int {
	return len(m.visible)
}

// VisibleIDs returns a copy of the visible node IDs (exported for testing).
func (m *Model) VisibleIDs() []string {
	result := make([]string, len(m.visible))
	copy(result, m.visible)
	return result
}
