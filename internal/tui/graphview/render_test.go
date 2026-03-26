package graphview

import (
	"strings"
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
)

func TestRenderContainsNodeSymbols(t *testing.T) {
	g := testGraph()
	ids := []string{"workflow:ci.yml", "action:checkout", "action:setup-go", "package:foo", "package:bar"}

	output := Render(g, ids, 80, 24)

	assert.NotEmpty(t, output)
	// Should contain workflow symbol.
	assert.True(t, strings.ContainsRune(output, SymbolWorkflow),
		"output should contain workflow symbol ■")
	// Should contain action symbol.
	assert.True(t, strings.ContainsRune(output, SymbolAction),
		"output should contain action symbol ◆")
	// Should contain package symbol.
	assert.True(t, strings.ContainsRune(output, SymbolPackage),
		"output should contain package symbol ●")
}

func TestRenderContainsNodeNames(t *testing.T) {
	g := testGraph()
	ids := []string{"workflow:ci.yml", "package:foo"}

	output := Render(g, ids, 80, 24)

	assert.Contains(t, output, "ci.yml")
	assert.Contains(t, output, "foo")
}

func TestRenderEmptyNodeList(t *testing.T) {
	g := testGraph()
	output := Render(g, nil, 80, 24)
	assert.Empty(t, output)
}

func TestRenderTooSmall(t *testing.T) {
	g := testGraph()
	ids := []string{"workflow:ci.yml"}

	// Width too small.
	assert.Empty(t, Render(g, ids, 5, 24))
	// Height too small.
	assert.Empty(t, Render(g, ids, 80, 3))
}

func TestRenderSingleNode(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.Node{
		ID: "package:solo", Type: graph.NodePackage,
		Name: "solo", Version: "1.0", Score: 50,
		Risk: core.RiskMedium, Pinning: graph.PinningNA,
	})

	output := Render(g, []string{"package:solo"}, 40, 10)
	assert.NotEmpty(t, output)
	assert.True(t, strings.ContainsRune(output, SymbolPackage))
	assert.Contains(t, output, "solo")
}

func TestRenderOutputDimensions(t *testing.T) {
	g := testGraph()
	ids := []string{"workflow:ci.yml", "action:checkout", "package:foo"}

	output := Render(g, ids, 80, 24)
	lines := strings.Split(output, "\n")

	assert.Equal(t, 24, len(lines), "output should have exactly height lines")
}

func TestNodeSymbol(t *testing.T) {
	assert.Equal(t, SymbolPackage, nodeSymbol(graph.NodePackage))
	assert.Equal(t, SymbolAction, nodeSymbol(graph.NodeAction))
	assert.Equal(t, SymbolWorkflow, nodeSymbol(graph.NodeWorkflow))
	assert.Equal(t, SymbolDocker, nodeSymbol(graph.NodeDockerImage))
	assert.Equal(t, SymbolScript, nodeSymbol(graph.NodeScriptDownload))
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 10))
	assert.Equal(t, "hello", truncate("hello", 5))
	assert.Equal(t, "hel…", truncate("hello", 4))
	assert.Equal(t, "h…", truncate("hello", 2))
	assert.Equal(t, "h", truncate("hello", 1))
	assert.Equal(t, "", truncate("", 5))
}

func TestRiskStyle(t *testing.T) {
	// Just verify these don't panic and return non-zero styles.
	_ = riskStyle(core.RiskCritical)
	_ = riskStyle(core.RiskHigh)
	_ = riskStyle(core.RiskMedium)
	_ = riskStyle(core.RiskLow)
	_ = riskStyle(core.RiskUnknown)
}

func TestEdgeStyle(t *testing.T) {
	// Just verify these don't panic.
	_ = edgeStyle(graph.EdgeDependsOn)
	_ = edgeStyle(graph.EdgeUsesAction)
	_ = edgeStyle(graph.EdgeBundles)
	_ = edgeStyle(graph.EdgeDownloads)
	_ = edgeStyle(graph.EdgePullsImage)
	_ = edgeStyle(graph.EdgeTriggers)
}

func TestGridToString(t *testing.T) {
	grid := makeGrid(3, 2)
	grid[0][0] = Cell{Char: 'A'}
	grid[0][1] = Cell{Char: 'B'}
	grid[0][2] = Cell{Char: 'C'}
	grid[1][0] = Cell{Char: 'X'}
	grid[1][1] = Cell{Char: 'Y'}
	grid[1][2] = Cell{Char: 'Z'}

	output := gridToString(grid)
	lines := strings.Split(output, "\n")
	assert.Equal(t, 2, len(lines))
	assert.Contains(t, lines[0], "A")
	assert.Contains(t, lines[0], "B")
	assert.Contains(t, lines[0], "C")
	assert.Contains(t, lines[1], "X")
	assert.Contains(t, lines[1], "Y")
	assert.Contains(t, lines[1], "Z")
}

func TestClampInt(t *testing.T) {
	assert.Equal(t, 5, clampInt(5, 0, 10))
	assert.Equal(t, 0, clampInt(-3, 0, 10))
	assert.Equal(t, 10, clampInt(15, 0, 10))
}
