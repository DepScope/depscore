// internal/graph/builder_test.go
package graph

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFromScanResult(t *testing.T) {
	sr := &core.ScanResult{
		Profile:        "enterprise",
		PassThreshold:  70,
		DirectDeps:     2,
		TransitiveDeps: 1,
		Packages: []core.PackageResult{
			{Name: "flask", Version: "3.2.0", Ecosystem: "python", OwnScore: 81, OwnRisk: core.RiskLow, Depth: 1},
			{Name: "click", Version: "8.3.1", Ecosystem: "python", OwnScore: 81, OwnRisk: core.RiskLow, Depth: 1},
			{Name: "colorama", Version: "0.4.6", Ecosystem: "python", OwnScore: 47, OwnRisk: core.RiskHigh, Depth: 2},
		},
		DepsMap: map[string][]string{
			"flask": {"click"},
			"click": {"colorama"},
		},
	}

	g := BuildFromScanResult(sr)
	require.NotNil(t, g)

	// Should have 3 package nodes
	pkgNodes := g.NodesOfType(NodePackage)
	assert.Len(t, pkgNodes, 3)

	// Check flask node
	flask := g.Node("package:python/flask@3.2.0")
	require.NotNil(t, flask)
	assert.Equal(t, "flask", flask.Name)
	assert.Equal(t, 81, flask.Score)

	// Should have 2 depends_on edges
	assert.Len(t, g.Edges, 2)

	// Should be able to find path flask → click → colorama
	paths := g.FindPaths("package:python/flask@3.2.0", "package:python/colorama@0.4.6", 10)
	assert.Len(t, paths, 1)
}

func TestToScanResult(t *testing.T) {
	sr := &core.ScanResult{
		Profile:        "enterprise",
		PassThreshold:  70,
		DirectDeps:     1,
		TransitiveDeps: 0,
		Packages: []core.PackageResult{
			{Name: "flask", Version: "3.2.0", Ecosystem: "python", OwnScore: 81, OwnRisk: core.RiskLow, Depth: 1},
		},
		DepsMap: map[string][]string{},
	}

	g := BuildFromScanResult(sr)
	back := ToScanResult(g, sr)

	assert.Equal(t, sr.Profile, back.Profile)
	assert.Equal(t, sr.PassThreshold, back.PassThreshold)
	assert.Len(t, back.Packages, 1)
	assert.Equal(t, "flask", back.Packages[0].Name)
}
