// internal/graph/propagator_test.go
package graph

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestPropagateGraph(t *testing.T) {
	g := New()

	// flask (81) → click (81) → colorama (47)
	g.AddNode(&Node{ID: "a", Type: NodePackage, Name: "flask", Score: 81, Risk: core.RiskLow, Metadata: map[string]any{"depth": 1}})
	g.AddNode(&Node{ID: "b", Type: NodePackage, Name: "click", Score: 81, Risk: core.RiskLow, Metadata: map[string]any{"depth": 1}})
	g.AddNode(&Node{ID: "c", Type: NodePackage, Name: "colorama", Score: 47, Risk: core.RiskHigh, Metadata: map[string]any{"depth": 2}})
	g.AddEdge(&Edge{From: "a", To: "b", Type: EdgeDependsOn})
	g.AddEdge(&Edge{From: "b", To: "c", Type: EdgeDependsOn})

	Propagate(g)

	// flask's transitive risk should be affected by colorama (47)
	flask := g.Node("a")
	trScore, ok := flask.Metadata["transitive_risk_score"].(int)
	assert.True(t, ok)
	assert.Less(t, trScore, 81) // should be pulled down by colorama

	// colorama has no children, so transitive = 100 (sentinel)
	colorama := g.Node("c")
	cScore, _ := colorama.Metadata["transitive_risk_score"].(int)
	assert.Equal(t, 100, cScore)
}

func TestPropagateGraphCycle(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "a", Type: NodePackage, Name: "a", Score: 80, Metadata: map[string]any{"depth": 1}})
	g.AddNode(&Node{ID: "b", Type: NodePackage, Name: "b", Score: 60, Metadata: map[string]any{"depth": 2}})
	g.AddEdge(&Edge{From: "a", To: "b", Type: EdgeDependsOn})
	g.AddEdge(&Edge{From: "b", To: "a", Type: EdgeDependsOn}) // cycle

	// Should not infinite loop
	Propagate(g)

	a := g.Node("a")
	_, ok := a.Metadata["transitive_risk_score"].(int)
	assert.True(t, ok) // should complete without hanging
}
