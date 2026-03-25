// internal/graph/integration_test.go
package graph

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullPipeline(t *testing.T) {
	// Simulate a real scan result with multiple ecosystems
	sr := &core.ScanResult{
		Profile:        "enterprise",
		PassThreshold:  70,
		DirectDeps:     3,
		TransitiveDeps: 2,
		Packages: []core.PackageResult{
			{Name: "flask", Version: "3.2.0", Ecosystem: "python", OwnScore: 81, OwnRisk: core.RiskLow, Depth: 1},
			{Name: "click", Version: "8.3.1", Ecosystem: "python", OwnScore: 81, OwnRisk: core.RiskLow, Depth: 1,
				DependsOn: []string{"colorama"}, DependsOnCount: 1},
			{Name: "colorama", Version: "0.4.6", Ecosystem: "python", OwnScore: 47, OwnRisk: core.RiskHigh, Depth: 2},
			{Name: "express", Version: "4.18.0", Ecosystem: "npm", OwnScore: 90, OwnRisk: core.RiskLow, Depth: 1},
			{Name: "qs", Version: "6.11.0", Ecosystem: "npm", OwnScore: 72, OwnRisk: core.RiskMedium, Depth: 2},
		},
		DepsMap: map[string][]string{
			"flask":   {"click"},
			"click":   {"colorama"},
			"express": {"qs"},
		},
	}

	// Build graph
	g := BuildFromScanResult(sr)
	require.NotNil(t, g)

	// Verify node count
	assert.Len(t, g.NodesOfType(NodePackage), 5)

	// Verify edge count
	assert.Len(t, g.Edges, 3)

	// Propagate risk
	Propagate(g)

	// Flask should have transitive risk pulled down by colorama (colorama own score: 47)
	flask := g.Node("package:python/flask@3.2.0")
	require.NotNil(t, flask)
	trScore, _ := flask.Metadata["transitive_risk_score"].(int)
	assert.Less(t, trScore, 81)

	// Convert back to ScanResult
	back := ToScanResult(g, sr)
	assert.Equal(t, "enterprise", back.Profile)
	assert.Len(t, back.Packages, 5)

	// Verify NodesOfType filtering by ecosystem metadata
	allPkgs := g.NodesOfType(NodePackage)
	var pypiCount int
	for _, n := range allPkgs {
		if eco, ok := n.Metadata["ecosystem"].(string); ok && eco == "python" {
			pypiCount++
		}
	}
	assert.Equal(t, 3, pypiCount)
}
