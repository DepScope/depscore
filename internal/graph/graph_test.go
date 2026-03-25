// internal/graph/graph_test.go
package graph

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGraph(t *testing.T) {
	g := New()
	assert.NotNil(t, g)
	assert.Empty(t, g.Nodes)
	assert.Empty(t, g.Edges)
}

func TestAddNode(t *testing.T) {
	g := New()
	n := &Node{
		ID:      "package:python/litellm@1.82.8",
		Type:    NodePackage,
		Name:    "litellm",
		Version: "1.82.8",
		Score:   45,
		Risk:    core.RiskHigh,
		Pinning: PinningNA,
	}
	g.AddNode(n)
	assert.Len(t, g.Nodes, 1)

	got := g.Node("package:python/litellm@1.82.8")
	require.NotNil(t, got)
	assert.Equal(t, "litellm", got.Name)
}

func TestAddEdge(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "package:python/flask@3.2.0", Type: NodePackage, Name: "flask"})
	g.AddNode(&Node{ID: "package:python/click@8.3.1", Type: NodePackage, Name: "click"})
	g.AddEdge(&Edge{From: "package:python/flask@3.2.0", To: "package:python/click@8.3.1", Type: EdgeDependsOn, Depth: 1})

	assert.Len(t, g.Edges, 1)
}

func TestNeighbors(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "a", Type: NodePackage, Name: "a"})
	g.AddNode(&Node{ID: "b", Type: NodePackage, Name: "b"})
	g.AddNode(&Node{ID: "c", Type: NodePackage, Name: "c"})
	g.AddEdge(&Edge{From: "a", To: "b", Type: EdgeDependsOn})
	g.AddEdge(&Edge{From: "a", To: "c", Type: EdgeDependsOn})

	neighbors := g.Neighbors("a")
	assert.Len(t, neighbors, 2)
}

func TestNodesOfType(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "package:a", Type: NodePackage, Name: "a"})
	g.AddNode(&Node{ID: "repo:b", Type: NodeRepo, Name: "b"})
	g.AddNode(&Node{ID: "package:c", Type: NodePackage, Name: "c"})

	pkgs := g.NodesOfType(NodePackage)
	assert.Len(t, pkgs, 2)
}

func TestFindPaths(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "a", Type: NodePackage, Name: "a"})
	g.AddNode(&Node{ID: "b", Type: NodePackage, Name: "b"})
	g.AddNode(&Node{ID: "c", Type: NodePackage, Name: "c"})
	g.AddEdge(&Edge{From: "a", To: "b", Type: EdgeDependsOn})
	g.AddEdge(&Edge{From: "b", To: "c", Type: EdgeDependsOn})

	paths := g.FindPaths("a", "c", 10)
	require.Len(t, paths, 1)
	assert.Equal(t, []string{"a", "b", "c"}, paths[0])
}

func TestFindPathsNone(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "a", Type: NodePackage, Name: "a"})
	g.AddNode(&Node{ID: "b", Type: NodePackage, Name: "b"})
	// no edge between a and b

	paths := g.FindPaths("a", "b", 10)
	assert.Empty(t, paths)
}
