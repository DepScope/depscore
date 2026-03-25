// internal/tui/inspect.go
package tui

import (
	"fmt"
	"sort"
	"strings"
)

// renderInspect renders the inspect panel for the currently inspected node.
func (m Model) renderInspect(maxHeight, maxWidth int) string {
	n := m.graph.Nodes[m.inspecting]
	if n == nil {
		return ""
	}

	var lines []string

	// Title
	title := stylePanelTitle.Render(fmt.Sprintf(" %s ", n.Name))
	lines = append(lines, title)
	lines = append(lines, "")

	// Basic info
	lines = append(lines, kv("Type", n.Type.String()))
	lines = append(lines, kv("ID", n.ID))
	version := n.Version
	if version == "" {
		version = "-"
	}
	lines = append(lines, kv("Version", version))
	if n.Ref != "" {
		lines = append(lines, kv("Ref", n.Ref))
	}
	lines = append(lines, kv("Score", riskColorFor(n.Score, n.Risk).Render(fmt.Sprintf("%d (%s)", n.Score, n.Risk))))
	if n.Pinning.String() != "n/a" {
		lines = append(lines, kv("Pinning", n.Pinning.String()))
	}
	lines = append(lines, "")

	// Transitive risk (from metadata)
	if tr, ok := n.Metadata["transitive_risk_score"]; ok {
		trScore := fmt.Sprintf("%v", tr)
		if trRisk, ok := n.Metadata["transitive_risk"]; ok {
			trScore = fmt.Sprintf("%v (%v)", tr, trRisk)
		}
		lines = append(lines, kv("Transitive Risk", trScore))
	}

	// Ecosystem + constraint info
	if eco, ok := n.Metadata["ecosystem"].(string); ok && eco != "" {
		lines = append(lines, kv("Ecosystem", eco))
	}
	if ct, ok := n.Metadata["constraint_type"].(string); ok && ct != "" {
		lines = append(lines, kv("Constraint", ct))
	}
	if depth, ok := n.Metadata["depth"]; ok {
		lines = append(lines, kv("Depth", fmt.Sprintf("%v", depth)))
	}

	// Action-specific: first-party, action type
	if fp, ok := n.Metadata["first_party"].(bool); ok {
		if fp {
			lines = append(lines, kv("First-party", "yes (GitHub-maintained)"))
		} else {
			lines = append(lines, kv("First-party", "no (third-party)"))
		}
	}
	if at, ok := n.Metadata["action_type"].(string); ok {
		lines = append(lines, kv("Action Type", at))
	}

	// Issues (from metadata if stored)
	if issues, ok := n.Metadata["issues"].([]any); ok && len(issues) > 0 {
		lines = append(lines, "")
		lines = append(lines, stylePanelTitle.Render(fmt.Sprintf("Issues (%d)", len(issues))))
		for i, iss := range issues {
			if i >= 10 {
				lines = append(lines, fmt.Sprintf("  ... and %d more", len(issues)-10))
				break
			}
			lines = append(lines, "  "+fmt.Sprintf("%v", iss))
		}
	}

	// Other metadata (filtered: skip already-shown keys)
	shownKeys := map[string]bool{
		"transitive_risk_score": true, "transitive_risk": true,
		"ecosystem": true, "constraint_type": true, "depth": true,
		"first_party": true, "action_type": true, "issues": true,
	}
	var extraMeta []string
	if len(n.Metadata) > 0 {
		keys := make([]string, 0, len(n.Metadata))
		for k := range n.Metadata {
			if !shownKeys[k] {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := fmt.Sprintf("%v", n.Metadata[k])
			extraMeta = append(extraMeta, kv("  "+k, v))
		}
	}
	if len(extraMeta) > 0 {
		lines = append(lines, "")
		lines = append(lines, stylePanelTitle.Render("Details"))
		lines = append(lines, extraMeta...)
	}
	lines = append(lines, "")

	// Edges: outgoing
	outgoing := m.findOutgoingEdges(m.inspecting)
	if len(outgoing) > 0 {
		lines = append(lines, stylePanelTitle.Render("Outgoing edges"))
		for _, desc := range outgoing {
			lines = append(lines, "  "+desc)
		}
		lines = append(lines, "")
	}

	// Edges: incoming
	incoming := m.findIncomingEdges(m.inspecting)
	if len(incoming) > 0 {
		lines = append(lines, stylePanelTitle.Render("Incoming edges"))
		for _, desc := range incoming {
			lines = append(lines, "  "+desc)
		}
		lines = append(lines, "")
	}

	// Truncate to height
	if len(lines) > maxHeight {
		lines = lines[:maxHeight-1]
		lines = append(lines, styleLabel.Render("  ... (scroll not available)"))
	}

	// Pad to height
	for len(lines) < maxHeight {
		lines = append(lines, "")
	}

	// Truncate width
	for i, line := range lines {
		if lipglossWidth(line) > maxWidth-2 {
			lines[i] = truncate(line, maxWidth-2)
		}
	}

	content := strings.Join(lines, "\n")
	return stylePanelBorder.Width(maxWidth - 2).Render(content)
}

// kv formats a key-value pair for the inspect panel.
func kv(key, value string) string {
	return styleLabel.Render(key+": ") + styleValue.Render(value)
}

// findOutgoingEdges returns descriptions of outgoing edges from a node.
func (m Model) findOutgoingEdges(nodeID string) []string {
	var result []string
	for _, e := range m.graph.Edges {
		if e.From == nodeID {
			target := m.graph.Nodes[e.To]
			name := e.To
			if target != nil {
				name = target.Name
			}
			result = append(result, fmt.Sprintf("--%s--> %s", e.Type.String(), name))
		}
	}
	sort.Strings(result)
	return result
}

// findIncomingEdges returns descriptions of incoming edges to a node.
func (m Model) findIncomingEdges(nodeID string) []string {
	var result []string
	for _, e := range m.graph.Edges {
		if e.To == nodeID {
			source := m.graph.Nodes[e.From]
			name := e.From
			if source != nil {
				name = source.Name
			}
			result = append(result, fmt.Sprintf("%s --%s-->", name, e.Type.String()))
		}
	}
	sort.Strings(result)
	return result
}
