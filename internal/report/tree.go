// internal/report/tree.go
package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/depscope/depscope/internal/graph"
)

// TreeOptions configures the RenderTree output.
type TreeOptions struct {
	MaxDepth   int              // 0 = unlimited
	TypeFilter []graph.NodeType // empty = show all
	RiskFilter []string         // empty = show all, e.g. "HIGH", "CRITICAL"
	CollapseAt int              // auto-collapse subtrees deeper than this (0 = no collapse)
	JSON       bool             // output as JSON instead
}

// treeNode is used for JSON output.
type treeNode struct {
	ID       string     `json:"id"`
	Type     string     `json:"type"`
	Name     string     `json:"name"`
	Version  string     `json:"version"`
	Score    int        `json:"score"`
	Risk     string     `json:"risk"`
	Pinning  string     `json:"pinning"`
	Mutable  bool       `json:"mutable,omitempty"`
	Depth    int        `json:"depth"`
	Children []treeNode `json:"children,omitempty"`
}

// RenderTree takes a graph and renders a Unicode dependency tree.
func RenderTree(g *graph.Graph, opts TreeOptions) string {
	if opts.JSON {
		return renderTreeJSON(g, opts)
	}

	roots := findRoots(g)
	if len(roots) == 0 {
		return "  (no nodes found)\n"
	}

	// Track stats for the summary line.
	var (
		totalNodes int
		highCount  int
		critCount  int
		maxDepth   int
	)

	var b strings.Builder

	// Render a project header line.
	b.WriteString("depscope/\n")

	for i, rootID := range roots {
		n := g.Nodes[rootID]
		if n == nil {
			continue
		}

		if !passesFilters(n, opts) {
			continue
		}

		isLast := isLastVisible(roots, i, g, opts)
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		b.WriteString(connector)
		b.WriteString(formatTreeNode(n))
		b.WriteString("\n")
		totalNodes++
		countRisk(n, &highCount, &critCount)
		if maxDepth < 0 {
			maxDepth = 0
		}

		childPrefix := "│   "
		if isLast {
			childPrefix = "    "
		}
		stats := renderChildren(g, rootID, childPrefix, 1, opts, &b)
		totalNodes += stats.nodes
		highCount += stats.high
		critCount += stats.critical
		if stats.maxDepth > maxDepth {
			maxDepth = stats.maxDepth
		}
	}

	// Summary line
	b.WriteString(fmt.Sprintf("\nSummary: %d nodes | %d HIGH | %d CRITICAL | max depth: %d\n",
		totalNodes, highCount, critCount, maxDepth))

	return b.String()
}

type renderStats struct {
	nodes    int
	high     int
	critical int
	maxDepth int
}

func renderChildren(g *graph.Graph, parentID, prefix string, depth int, opts TreeOptions, b *strings.Builder) renderStats {
	var stats renderStats
	stats.maxDepth = depth

	if opts.MaxDepth > 0 && depth > opts.MaxDepth {
		return stats
	}

	if opts.CollapseAt > 0 && depth > opts.CollapseAt {
		children := sortedNeighbors(g, parentID)
		visibleCount := countVisible(g, children, opts)
		if visibleCount > 0 {
			b.WriteString(prefix)
			fmt.Fprintf(b, "└── ... (%d collapsed)\n", visibleCount)
		}
		return stats
	}

	children := sortedNeighbors(g, parentID)

	// Filter children that pass filters.
	var visible []string
	for _, cid := range children {
		cn := g.Nodes[cid]
		if cn == nil {
			continue
		}
		if passesFilters(cn, opts) {
			visible = append(visible, cid)
		}
	}

	for i, childID := range visible {
		cn := g.Nodes[childID]
		if cn == nil {
			continue
		}

		isLast := i == len(visible)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		b.WriteString(prefix)
		b.WriteString(connector)
		b.WriteString(formatTreeNode(cn))
		b.WriteString("\n")
		stats.nodes++
		countRisk(cn, &stats.high, &stats.critical)

		childPrefix := prefix + "│   "
		if isLast {
			childPrefix = prefix + "    "
		}

		sub := renderChildren(g, childID, childPrefix, depth+1, opts, b)
		stats.nodes += sub.nodes
		stats.high += sub.high
		stats.critical += sub.critical
		if sub.maxDepth > stats.maxDepth {
			stats.maxDepth = sub.maxDepth
		}
	}

	return stats
}

func formatTreeNode(n *graph.Node) string {
	version := n.Version
	if version == "" {
		version = n.Ref
	}
	if version == "" {
		version = "latest"
	}

	typeLabel := shortType(n.Type)
	mutableMarker := ""
	if isMutable(n.Pinning) {
		mutableMarker = " ⚡MUTABLE"
	}

	return fmt.Sprintf("[%s] %s@%s ●%d%s", typeLabel, n.Name, version, n.Score, mutableMarker)
}

func shortType(t graph.NodeType) string {
	switch t {
	case graph.NodePackage:
		return "package"
	case graph.NodeAction:
		return "action"
	case graph.NodeWorkflow:
		return "workflow"
	case graph.NodeDockerImage:
		return "docker"
	case graph.NodeScriptDownload:
		return "script"
	case graph.NodePrecommitHook:
		return "hook"
	case graph.NodeTerraformModule:
		return "terraform"
	case graph.NodeGitSubmodule:
		return "submodule"
	case graph.NodeDevTool:
		return "tool"
	case graph.NodeBuildTool:
		return "build"
	default:
		return t.String()
	}
}

func isMutable(p graph.PinningQuality) bool {
	return p == graph.PinningMajorTag || p == graph.PinningBranch || p == graph.PinningUnpinned
}

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
		if ni.Type != nj.Type {
			return ni.Type > nj.Type // workflows first
		}
		return ni.Name < nj.Name
	})

	return roots
}

func sortedNeighbors(g *graph.Graph, nodeID string) []string {
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

func passesFilters(n *graph.Node, opts TreeOptions) bool {
	// Type filter
	if len(opts.TypeFilter) > 0 {
		found := false
		for _, t := range opts.TypeFilter {
			if n.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Risk filter
	if len(opts.RiskFilter) > 0 {
		found := false
		risk := string(n.Risk)
		for _, r := range opts.RiskFilter {
			if risk == r {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func countRisk(n *graph.Node, high, crit *int) {
	risk := string(n.Risk)
	if risk == "HIGH" {
		*high++
	}
	if risk == "CRITICAL" {
		*crit++
	}
}

func countVisible(g *graph.Graph, children []string, opts TreeOptions) int {
	count := 0
	for _, id := range children {
		n := g.Nodes[id]
		if n != nil && passesFilters(n, opts) {
			count++
		}
	}
	return count
}

func isLastVisible(roots []string, idx int, g *graph.Graph, opts TreeOptions) bool {
	for i := idx + 1; i < len(roots); i++ {
		n := g.Nodes[roots[i]]
		if n != nil && passesFilters(n, opts) {
			return false
		}
	}
	return true
}

func renderTreeJSON(g *graph.Graph, opts TreeOptions) string {
	roots := findRoots(g)
	var nodes []treeNode
	for _, rootID := range roots {
		n := g.Nodes[rootID]
		if n == nil || !passesFilters(n, opts) {
			continue
		}
		nodes = append(nodes, buildTreeNode(g, rootID, 0, opts))
	}

	data, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}

func buildTreeNode(g *graph.Graph, nodeID string, depth int, opts TreeOptions) treeNode {
	n := g.Nodes[nodeID]
	tn := treeNode{
		ID:      n.ID,
		Type:    n.Type.String(),
		Name:    n.Name,
		Version: n.Version,
		Score:   n.Score,
		Risk:    string(n.Risk),
		Pinning: n.Pinning.String(),
		Mutable: isMutable(n.Pinning),
		Depth:   depth,
	}

	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		return tn
	}

	children := sortedNeighbors(g, nodeID)
	for _, cid := range children {
		cn := g.Nodes[cid]
		if cn == nil || !passesFilters(cn, opts) {
			continue
		}
		tn.Children = append(tn.Children, buildTreeNode(g, cid, depth+1, opts))
	}
	return tn
}
