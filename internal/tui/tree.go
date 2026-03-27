// internal/tui/tree.go
package tui

import (
	"fmt"
	"sort"
	"strings"
)

// buildTreeVisible builds the list of visible node IDs in tree order.
// Walks root nodes; for each expanded node, recursively includes children.
func (m *Model) buildTreeVisible() []string {
	var visible []string
	for _, root := range m.roots {
		if !m.passesFilter(root) {
			// Still include if any descendant passes filter
			if !m.hasFilteredDescendant(root) {
				continue
			}
		}
		visible = append(visible, root)
		if m.expanded[root] {
			visible = append(visible, m.treeChildren(root, 1)...)
		}
	}
	return visible
}

// treeChildren recursively collects visible children of a node.
func (m *Model) treeChildren(parentID string, depth int) []string {
	neighbors := m.sortedNeighbors(parentID)
	var result []string
	for _, childID := range neighbors {
		if m.filterLevel != "" && !m.passesFilter(childID) && !m.hasFilteredDescendant(childID) {
			continue
		}
		result = append(result, childID)
		if m.expanded[childID] {
			result = append(result, m.treeChildren(childID, depth+1)...)
		}
	}
	return result
}

// hasFilteredDescendant checks if any descendant of a node passes the filter.
func (m *Model) hasFilteredDescendant(nodeID string) bool {
	for _, childID := range m.graph.Neighbors(nodeID) {
		if m.passesFilter(childID) {
			return true
		}
		if m.hasFilteredDescendant(childID) {
			return true
		}
	}
	return false
}

// sortedNeighbors returns neighbors sorted by type then name.
func (m *Model) sortedNeighbors(nodeID string) []string {
	neighbors := m.graph.Neighbors(nodeID)
	sorted := make([]string, len(neighbors))
	copy(sorted, neighbors)

	sort.Slice(sorted, func(i, j int) bool {
		ni := m.graph.Nodes[sorted[i]]
		nj := m.graph.Nodes[sorted[j]]
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

// renderTree renders the tree view to fit within maxHeight lines and maxWidth columns.
func (m Model) renderTree(maxHeight, maxWidth int) string {
	if len(m.visible) == 0 {
		return styleLabel.Render("  (no nodes match current filter)")
	}

	// Build the indentation context: for each visible node, compute depth and whether it's last child.
	// We need to walk the tree structure to compute proper prefixes.
	// Rebuild a depth map by walking the tree.
	depthMap := make(map[string]int)
	parentMap := make(map[string]string)
	lastChildMap := make(map[string]bool)

	// Walk roots
	for i, root := range m.roots {
		depthMap[root] = 0
		lastChildMap[root] = (i == len(m.filterRoots())-1)
		if m.expanded[root] {
			m.computeTreeInfo(root, 1, depthMap, parentMap, lastChildMap)
		}
	}

	// Compute the prefix for each visible node using the ancestry chain.
	lines := make([]string, 0, maxHeight)
	end := m.offset + maxHeight
	if end > len(m.visible) {
		end = len(m.visible)
	}

	for idx := m.offset; idx < end; idx++ {
		nodeID := m.visible[idx]
		n := m.graph.Nodes[nodeID]
		if n == nil {
			continue
		}

		depth := depthMap[nodeID]
		prefix := m.buildTreePrefix(nodeID, depth, depthMap, parentMap, lastChildMap)

		// Format node content
		nodeText := formatNodeLine(n)

		// Determine if there are children (show expand indicator)
		hasChildren := len(m.graph.Neighbors(nodeID)) > 0
		expandChar := " "
		if hasChildren {
			if m.expanded[nodeID] {
				expandChar = "▼"
			} else {
				expandChar = "▶"
			}
		}

		// Color by risk
		rs := riskStyle(string(n.Risk))

		line := fmt.Sprintf("%s%s %s", prefix, expandChar, rs.Render(nodeText))

		// Truncate if too wide
		if lipglossWidth(line) > maxWidth {
			line = truncate(line, maxWidth)
		}

		// Pad to width
		line = padRight(line, maxWidth)

		// Highlight selected row
		if idx == m.cursor {
			line = styleSelected.Render(line)
		}

		lines = append(lines, line)
	}

	// Pad remaining lines to fill height
	for len(lines) < maxHeight {
		lines = append(lines, strings.Repeat(" ", maxWidth))
	}

	return strings.Join(lines, "\n")
}

// filterRoots returns roots that pass the current filter (or have matching descendants).
func (m *Model) filterRoots() []string {
	var result []string
	for _, root := range m.roots {
		if m.passesFilter(root) || m.hasFilteredDescendant(root) {
			result = append(result, root)
		}
	}
	return result
}

// computeTreeInfo recursively fills in depth/parent/lastChild maps.
func (m *Model) computeTreeInfo(parentID string, depth int, depthMap map[string]int, parentMap map[string]string, lastChildMap map[string]bool) {
	neighbors := m.sortedNeighbors(parentID)
	// Filter neighbors
	var filtered []string
	for _, childID := range neighbors {
		if m.filterLevel != "" && !m.passesFilter(childID) && !m.hasFilteredDescendant(childID) {
			continue
		}
		filtered = append(filtered, childID)
	}

	for i, childID := range filtered {
		depthMap[childID] = depth
		parentMap[childID] = parentID
		lastChildMap[childID] = (i == len(filtered)-1)
		if m.expanded[childID] {
			m.computeTreeInfo(childID, depth+1, depthMap, parentMap, lastChildMap)
		}
	}
}

// buildTreePrefix builds the tree connector prefix for a node.
func (m *Model) buildTreePrefix(nodeID string, depth int, depthMap map[string]int, parentMap map[string]string, lastChildMap map[string]bool) string {
	if depth == 0 {
		return ""
	}

	// Build prefix by walking up the parent chain
	prefixParts := make([]string, depth)

	current := nodeID
	for d := depth - 1; d >= 0; d-- {
		if d == depth-1 {
			// This level: branch or last-child connector
			if lastChildMap[current] {
				prefixParts[d] = treeLastChild
			} else {
				prefixParts[d] = treeBranch
			}
		} else {
			// Ancestor level: pipe or space
			parent := parentMap[current]
			if !lastChildMap[current] {
				prefixParts[d] = treePipe
			} else {
				prefixParts[d] = treeSpace
			}
			current = parent
		}
		// Walk up
		if d > 0 {
			current = parentMap[current]
		}
	}

	return styleTreeBranch.Render(strings.Join(prefixParts, ""))
}

// lipglossWidth measures the printed width of a string.
func lipglossWidth(s string) int {
	// Use a simple approach: count runes, accounting for ANSI escape sequences
	return len(stripAnsi(s))
}

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// truncate truncates a string (preserving ANSI) to roughly maxWidth visible characters.
func truncate(s string, maxWidth int) string {
	if maxWidth <= 3 {
		return "..."
	}
	plain := stripAnsi(s)
	if len(plain) <= maxWidth {
		return s
	}
	// Simple truncation: cut the raw string and add ellipsis
	// This is approximate but works for display purposes
	count := 0
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			b.WriteRune(r)
			continue
		}
		if inEsc {
			b.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if count >= maxWidth-3 {
			b.WriteString("...")
			break
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}

// padRight pads a string to the given width with spaces.
func padRight(s string, width int) string {
	w := lipglossWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
