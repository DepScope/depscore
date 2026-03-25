package report_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteJSONWithGraph(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.Node{ID: "package:python/flask@3.2.0", Type: graph.NodePackage, Name: "flask", Version: "3.2.0", Score: 81, Risk: core.RiskLow, Pinning: graph.PinningNA})
	g.AddNode(&graph.Node{ID: "package:python/click@8.3.1", Type: graph.NodePackage, Name: "click", Version: "8.3.1", Score: 81, Risk: core.RiskLow, Pinning: graph.PinningNA})
	g.AddEdge(&graph.Edge{From: "package:python/flask@3.2.0", To: "package:python/click@8.3.1", Type: graph.EdgeDependsOn, Depth: 1})

	sr := core.ScanResult{
		Profile:       "enterprise",
		PassThreshold: 70,
		Packages:      []core.PackageResult{{Name: "flask", Version: "3.2.0", Ecosystem: "python", OwnScore: 81, OwnRisk: core.RiskLow}},
		Graph:         g,
	}

	var buf bytes.Buffer
	err := report.WriteJSON(&buf, sr)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))

	graphData, ok := parsed["graph"].(map[string]any)
	require.True(t, ok, "graph key should exist")

	nodes := graphData["nodes"].([]any)
	assert.Len(t, nodes, 2)

	edges := graphData["edges"].([]any)
	assert.Len(t, edges, 1)
}

func TestWriteJSONWithoutGraphOmitsKey(t *testing.T) {
	sr := core.ScanResult{
		Profile:       "enterprise",
		PassThreshold: 70,
		Packages:      []core.PackageResult{{Name: "flask", Version: "3.2.0", Ecosystem: "python", OwnScore: 81, OwnRisk: core.RiskLow}},
	}

	var buf bytes.Buffer
	err := report.WriteJSON(&buf, sr)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))

	_, hasGraph := parsed["graph"]
	assert.False(t, hasGraph, "graph key should be absent when Graph is nil")
}
