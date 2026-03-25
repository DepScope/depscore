// internal/graph/propagator.go
package graph

import "github.com/depscope/depscope/internal/core"

// Propagate walks the graph and sets transitive risk scores on each node.
// For each node, the transitive risk score is the minimum effective score
// found across all descendants. This matches the behavior of core.Propagate
// but operates on graph edges instead of a flat deps map.
func Propagate(g *Graph) {
	for _, node := range g.Nodes {
		if node.Type != NodePackage {
			continue
		}
		minScore := 100
		visited := make(map[string]bool)
		walkGraphDescendants(g, node.ID, &minScore, visited)
		node.Metadata["transitive_risk_score"] = minScore
		node.Metadata["transitive_risk"] = core.RiskLevelFromScore(minScore)
	}
}

func walkGraphDescendants(g *Graph, nodeID string, minScore *int, visited map[string]bool) {
	if visited[nodeID] {
		return
	}
	visited[nodeID] = true

	for _, neighborID := range g.Neighbors(nodeID) {
		neighbor := g.Node(neighborID)
		if neighbor == nil {
			continue
		}
		depth := 1
		if d, ok := neighbor.Metadata["depth"].(int); ok && d > 0 {
			depth = d
		}
		eff := core.EffectiveScore(neighbor.Score, depth)
		if eff < *minScore {
			*minScore = eff
		}
		walkGraphDescendants(g, neighborID, minScore, visited)
	}
}
