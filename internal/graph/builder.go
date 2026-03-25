// internal/graph/builder.go
package graph

import (
	"github.com/depscope/depscope/internal/core"
)

// BuildFromScanResult converts a ScanResult into a Graph.
// Each PackageResult becomes a package node. DepsMap entries become depends_on edges.
func BuildFromScanResult(sr *core.ScanResult) *Graph {
	g := New()

	// Build package nodes
	for _, pkg := range sr.Packages {
		nodeID := NodeID(NodePackage, pkg.Ecosystem+"/"+pkg.Name+"@"+pkg.Version)
		g.AddNode(&Node{
			ID:      nodeID,
			Type:    NodePackage,
			Name:    pkg.Name,
			Version: pkg.Version,
			Score:   pkg.OwnScore,
			Risk:    pkg.OwnRisk,
			Pinning: PinningNA,
			Metadata: map[string]any{
				"ecosystem":       pkg.Ecosystem,
				"constraint_type": pkg.ConstraintType,
				"depth":           pkg.Depth,
			},
		})
	}

	// Build lookup from name to node ID (for edge construction)
	nameToID := make(map[string]string)
	for _, pkg := range sr.Packages {
		nodeID := NodeID(NodePackage, pkg.Ecosystem+"/"+pkg.Name+"@"+pkg.Version)
		nameToID[pkg.Name] = nodeID
	}

	// Build depends_on edges from DepsMap
	for parent, children := range sr.DepsMap {
		parentID, ok := nameToID[parent]
		if !ok {
			continue
		}
		for _, child := range children {
			childID, ok := nameToID[child]
			if !ok {
				continue
			}
			g.AddEdge(&Edge{
				From:  parentID,
				To:    childID,
				Type:  EdgeDependsOn,
				Depth: 1,
			})
		}
	}

	return g
}

// ToScanResult converts the graph back to a ScanResult.
// This preserves backward compatibility with existing report formatters.
func ToScanResult(g *Graph, original *core.ScanResult) *core.ScanResult {
	// Extract package nodes back to PackageResult slice
	var packages []core.PackageResult
	for _, node := range g.NodesOfType(NodePackage) {
		eco, _ := node.Metadata["ecosystem"].(string)
		ct, _ := node.Metadata["constraint_type"].(string)
		depth, _ := node.Metadata["depth"].(int)

		pr := core.PackageResult{
			Name:           node.Name,
			Version:        node.Version,
			Ecosystem:      eco,
			ConstraintType: ct,
			Depth:          depth,
			OwnScore:       node.Score,
			OwnRisk:        node.Risk,
		}
		packages = append(packages, pr)
	}

	return &core.ScanResult{
		Profile:        original.Profile,
		PassThreshold:  original.PassThreshold,
		DirectDeps:     original.DirectDeps,
		TransitiveDeps: original.TransitiveDeps,
		Packages:       packages,
		AllIssues:      original.AllIssues,
		DepsMap:        original.DepsMap,
		RiskPaths:      original.RiskPaths,
		Suspicious:     original.Suspicious,
	}
}
