# Phase 1: Graph Infrastructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the depscope scan pipeline to produce a dependency graph instead of a flat list, enabling future node types (actions, Docker images, etc.) to plug in.

**Architecture:** New `internal/graph` package with `Graph`, `Node`, `Edge` types. A builder converts the existing `[]manifest.Package` + scoring results into a graph. Risk propagation operates on graph edges. `ScanResult` becomes a view over the graph (backward compatible). The `--only` flag filters which ecosystems to scan.

**Tech Stack:** Go, existing depscope packages. No new external dependencies.

**Spec:** `docs/superpowers/specs/2026-03-25-supply-chain-graph-actions-design.md`

---

### Task 1: Graph types — `internal/graph/types.go`

**Files:**
- Create: `internal/graph/types.go`
- Test: `internal/graph/types_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/graph/types_test.go
package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeTypeString(t *testing.T) {
	assert.Equal(t, "package", NodePackage.String())
	assert.Equal(t, "repo", NodeRepo.String())
	assert.Equal(t, "action", NodeAction.String())
	assert.Equal(t, "workflow", NodeWorkflow.String())
	assert.Equal(t, "docker_image", NodeDockerImage.String())
	assert.Equal(t, "script_download", NodeScriptDownload.String())
}

func TestEdgeTypeString(t *testing.T) {
	assert.Equal(t, "depends_on", EdgeDependsOn.String())
	assert.Equal(t, "hosted_at", EdgeHostedAt.String())
	assert.Equal(t, "uses_action", EdgeUsesAction.String())
	assert.Equal(t, "bundles", EdgeBundles.String())
	assert.Equal(t, "triggers", EdgeTriggers.String())
	assert.Equal(t, "resolves_to", EdgeResolvesTo.String())
	assert.Equal(t, "pulls_image", EdgePullsImage.String())
	assert.Equal(t, "downloads", EdgeDownloads.String())
}

func TestPinningQualityString(t *testing.T) {
	assert.Equal(t, "sha", PinningSHA.String())
	assert.Equal(t, "exact_version", PinningExactVersion.String())
	assert.Equal(t, "major_tag", PinningMajorTag.String())
	assert.Equal(t, "branch", PinningBranch.String())
	assert.Equal(t, "unpinned", PinningUnpinned.String())
	assert.Equal(t, "digest", PinningDigest.String())
	assert.Equal(t, "n/a", PinningNA.String())
}

func TestNodeID(t *testing.T) {
	assert.Equal(t, "package:python/litellm@1.82.8", NodeID(NodePackage, "python/litellm@1.82.8"))
	assert.Equal(t, "action:actions/checkout@v4", NodeID(NodeAction, "actions/checkout@v4"))
	assert.Equal(t, "repo:github.com/pallets/flask", NodeID(NodeRepo, "github.com/pallets/flask"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write minimal implementation**

```go
// internal/graph/types.go
package graph

import "github.com/depscope/depscope/internal/core"

// NodeType identifies what kind of supply chain entity a node represents.
type NodeType int

const (
	NodePackage        NodeType = iota // versioned software dependency
	NodeRepo                          // source code repository
	NodeAction                        // CI/CD action reference
	NodeWorkflow                      // workflow file
	NodeDockerImage                   // container base image
	NodeScriptDownload                // curl/wget binary in CI steps
	// Future: NodeHook, NodeTerraformModule, NodeGitSubmodule, NodeBuildTool, NodeOSPackage, NodeVendoredCode
)

func (t NodeType) String() string {
	switch t {
	case NodePackage:
		return "package"
	case NodeRepo:
		return "repo"
	case NodeAction:
		return "action"
	case NodeWorkflow:
		return "workflow"
	case NodeDockerImage:
		return "docker_image"
	case NodeScriptDownload:
		return "script_download"
	default:
		return "unknown"
	}
}

// EdgeType identifies the relationship between two nodes.
type EdgeType int

const (
	EdgeDependsOn  EdgeType = iota // package → package
	EdgeHostedAt                   // package → repo
	EdgeUsesAction                 // workflow → action
	EdgeBundles                    // action → package
	EdgeTriggers                   // workflow → workflow
	EdgeResolvesTo                 // action → repo (tag→SHA)
	EdgePullsImage                 // workflow/action → docker_image
	EdgeDownloads                  // workflow → script_download
	// Future: EdgeUsesHook, EdgeUsesModule, EdgeIncludesSubmodule, EdgeBuiltWith, EdgeInstallsOSPkg, EdgeVendors, EdgeAttests
)

func (t EdgeType) String() string {
	switch t {
	case EdgeDependsOn:
		return "depends_on"
	case EdgeHostedAt:
		return "hosted_at"
	case EdgeUsesAction:
		return "uses_action"
	case EdgeBundles:
		return "bundles"
	case EdgeTriggers:
		return "triggers"
	case EdgeResolvesTo:
		return "resolves_to"
	case EdgePullsImage:
		return "pulls_image"
	case EdgeDownloads:
		return "downloads"
	default:
		return "unknown"
	}
}

// PinningQuality describes how securely a dependency reference is pinned.
type PinningQuality int

const (
	PinningSHA          PinningQuality = iota // immutable hash
	PinningDigest                             // Docker image digest
	PinningExactVersion                       // exact version tag (e.g., v4.2.0)
	PinningMajorTag                           // major version tag (e.g., v4)
	PinningBranch                             // branch name (e.g., main)
	PinningUnpinned                           // no version reference
	PinningNA                                 // not applicable (packages use constraints)
)

func (p PinningQuality) String() string {
	switch p {
	case PinningSHA:
		return "sha"
	case PinningDigest:
		return "digest"
	case PinningExactVersion:
		return "exact_version"
	case PinningMajorTag:
		return "major_tag"
	case PinningBranch:
		return "branch"
	case PinningUnpinned:
		return "unpinned"
	case PinningNA:
		return "n/a"
	default:
		return "unknown"
	}
}

// Node represents a single entity in the supply chain graph.
type Node struct {
	ID       string            // e.g., "package:python/litellm@1.82.8"
	Type     NodeType
	Name     string
	Version  string            // resolved version or SHA
	Ref      string            // original reference (tag, branch, constraint)
	Score    int               // 0-100 reputation score
	Risk     core.RiskLevel
	Pinning  PinningQuality
	Metadata map[string]any    // ecosystem-specific data
}

// Edge represents a relationship between two nodes.
type Edge struct {
	From  string // NodeID
	To    string // NodeID
	Type  EdgeType
	Depth int    // distance from root
}

// NodeID constructs a canonical node identifier.
func NodeID(nodeType NodeType, key string) string {
	return nodeType.String() + ":" + key
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/graph/types.go internal/graph/types_test.go
git commit -m "feat(graph): add core types for supply chain graph"
```

---

### Task 2: Graph operations — `internal/graph/graph.go`

**Files:**
- Create: `internal/graph/graph.go`
- Test: `internal/graph/graph_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/ -run "TestNewGraph|TestAddNode|TestAddEdge|TestNeighbors|TestNodesOfType|TestFindPaths" -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/graph/graph.go
package graph

// Graph represents a supply chain dependency graph.
type Graph struct {
	Nodes map[string]*Node // keyed by Node.ID
	Edges []*Edge
	adj   map[string][]string // adjacency list: from → [to IDs]
}

// New creates an empty graph.
func New() *Graph {
	return &Graph{
		Nodes: make(map[string]*Node),
		adj:   make(map[string][]string),
	}
}

// AddNode adds a node to the graph. If a node with the same ID exists, it is replaced.
func (g *Graph) AddNode(n *Node) {
	g.Nodes[n.ID] = n
}

// Node returns the node with the given ID, or nil if not found.
func (g *Graph) Node(id string) *Node {
	return g.Nodes[id]
}

// AddEdge adds an edge to the graph and updates the adjacency list.
func (g *Graph) AddEdge(e *Edge) {
	g.Edges = append(g.Edges, e)
	g.adj[e.From] = append(g.adj[e.From], e.To)
}

// Neighbors returns the IDs of all nodes directly reachable from the given node.
func (g *Graph) Neighbors(id string) []string {
	return g.adj[id]
}

// NodesOfType returns all nodes matching the given type.
func (g *Graph) NodesOfType(t NodeType) []*Node {
	var result []*Node
	for _, n := range g.Nodes {
		if n.Type == t {
			result = append(result, n)
		}
	}
	return result
}

// FindPaths returns all paths from src to dst, up to maxDepth.
// Uses DFS with backtracking.
func (g *Graph) FindPaths(src, dst string, maxDepth int) [][]string {
	var result [][]string
	g.dfs(src, dst, maxDepth, []string{src}, make(map[string]bool), &result)
	return result
}

func (g *Graph) dfs(current, dst string, maxDepth int, path []string, visited map[string]bool, result *[][]string) {
	if current == dst && len(path) > 1 {
		p := make([]string, len(path))
		copy(p, path)
		*result = append(*result, p)
		return
	}
	if len(path) > maxDepth {
		return
	}
	visited[current] = true
	for _, next := range g.adj[current] {
		if !visited[next] {
			g.dfs(next, dst, maxDepth, append(path, next), visited, result)
		}
	}
	visited[current] = false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/graph/graph.go internal/graph/graph_test.go
git commit -m "feat(graph): add graph operations — add, query, find paths"
```

---

### Task 3: Graph builder — `internal/graph/builder.go`

**Files:**
- Create: `internal/graph/builder.go`
- Test: `internal/graph/builder_test.go`
- Reference: `internal/manifest/manifest.go:29-37` (Package struct)
- Reference: `internal/core/types.go:41-56` (PackageResult struct)
- Reference: `internal/core/types.go:75-86` (ScanResult struct)

The builder converts existing `[]PackageResult` + `DepsMap` into a Graph, and provides a method to convert back to `ScanResult` for backward compatibility.

- [ ] **Step 1: Write the failing test**

```go
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
		Profile:       "enterprise",
		PassThreshold: 70,
		DirectDeps:    2,
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
		Profile:       "enterprise",
		PassThreshold: 70,
		DirectDeps:    1,
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/ -run "TestBuild|TestToScan" -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
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
			Name:       node.Name,
			Version:    node.Version,
			Ecosystem:  eco,
			ConstraintType: ct,
			Depth:      depth,
			OwnScore:   node.Score,
			OwnRisk:    node.Risk,
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/graph/builder.go internal/graph/builder_test.go
git commit -m "feat(graph): add builder to convert ScanResult ↔ Graph"
```

---

### Task 4: Graph-based risk propagation — `internal/graph/propagator.go`

**Files:**
- Create: `internal/graph/propagator.go`
- Test: `internal/graph/propagator_test.go`
- Reference: `internal/core/propagator.go` (existing flat-list propagation to match behavior)

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/ -run TestPropagateGraph -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/ -run TestPropagateGraph -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/graph/propagator.go internal/graph/propagator_test.go
git commit -m "feat(graph): add graph-based risk propagation"
```

---

### Task 5: Wire graph into scanner pipeline — `internal/scanner/scanner.go`

**Files:**
- Modify: `internal/scanner/scanner.go:100-273` (scorePipeline)
- Modify: `internal/core/types.go:75-86` (add Graph field to ScanResult)
- Test: existing tests in `cmd/depscope/` must keep passing

This task adds the graph as an output of the scan pipeline without changing existing behavior. The graph is built at the end of `scorePipeline` and attached to `ScanResult`.

- [ ] **Step 1: Add Graph field to ScanResult**

Add to `internal/core/types.go` after line 85:

```go
// In ScanResult struct, add:
Graph any // *graph.Graph — supply chain graph (typed as any to avoid circular import)
```

- [ ] **Step 2: Run existing tests to confirm they pass before changes**

Run: `go test ./cmd/depscope/ ./internal/core/ ./internal/scanner/ -v`
Expected: PASS

- [ ] **Step 3: Build graph at end of scorePipeline**

Add to `internal/scanner/scanner.go`, import `"github.com/depscope/depscope/internal/graph"` and add after `result := &core.ScanResult{...}` (around line 271):

```go
// Build supply chain graph from scan results
g := graph.BuildFromScanResult(result)
graph.Propagate(g)
result.Graph = g
```

- [ ] **Step 4: Run all tests to verify backward compatibility**

Run: `go test ./... -count=1`
Expected: All tests pass. Existing output unchanged since report formatters don't use the Graph field.

- [ ] **Step 5: Commit**

```bash
git add internal/core/types.go internal/scanner/scanner.go
git commit -m "feat(graph): wire graph construction into scan pipeline"
```

---

### Task 6: `--only` flag on scan command — `cmd/depscope/scan.go`

**Files:**
- Modify: `cmd/depscope/scan.go` (add --only flag, filter ecosystems)
- Modify: `internal/scanner/scanner.go` (accept ecosystem filter)
- Test: `cmd/depscope/scan_test.go`

- [ ] **Step 1: Add Only field to scanner.Options**

In `internal/scanner/scanner.go`, add to Options struct:

```go
type Options struct {
	Profile  string
	MaxFiles int
	NoCVE    bool
	Only     []string // filter to these ecosystems (empty = all)
}
```

- [ ] **Step 2: Filter ecosystems in ScanDir**

In `ScanDir`, after `ecosystems := manifest.DetectAllEcosystems(dir)`, add filtering:

```go
if len(opts.Only) > 0 {
	allowed := make(map[string]bool)
	for _, o := range opts.Only {
		allowed[o] = true
	}
	var filtered []manifest.Ecosystem
	for _, eco := range ecosystems {
		if allowed[string(eco)] {
			filtered = append(filtered, eco)
		}
	}
	ecosystems = filtered
}
```

- [ ] **Step 3: Add --only flag to scan command**

In `cmd/depscope/scan.go` init():

```go
scanCmd.Flags().StringSlice("only", nil, "filter to specific ecosystems: python, go, rust, npm, php")
```

In `runScan`, parse and pass to Options:

```go
only, _ := cmd.Flags().GetStringSlice("only")
opts := scanner.Options{
	Profile:  cfg.Profile,
	MaxFiles: maxFiles,
	NoCVE:    noCVE,
	Only:     only,
}
```

- [ ] **Step 4: Write test**

```go
// In cmd/depscope/scan_test.go or discover_test.go
func TestScanOnlyFlag(t *testing.T) {
	root := t.TempDir()
	// Create go.mod and pyproject.toml
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\ngo 1.22\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]\ndependencies = [\"requests>=2.0\"]\n"), 0o644))

	// Scan with --only go should only find Go packages
	rootCmd.SetArgs([]string{"scan", root, "--only", "go", "--no-cve"})
	// This tests that filtering works without errors
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./cmd/depscope/ ./internal/scanner/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/depscope/scan.go internal/scanner/scanner.go
git commit -m "feat(scan): add --only flag to filter ecosystems"
```

---

### Task 7: Cache tier constants — `internal/cache/tiers.go`

**Files:**
- Create: `internal/cache/tiers.go`
- Test: `internal/cache/tiers_test.go`

Define named TTL constants for the multi-tier caching strategy. The existing `DiskCache` API already supports arbitrary TTLs — this just standardizes them.

- [ ] **Step 1: Write the failing test**

```go
// internal/cache/tiers_test.go
package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheTierDurations(t *testing.T) {
	assert.Equal(t, 24*time.Hour, TTLRegistryMetadata)
	assert.Equal(t, 6*time.Hour, TTLCVEData)
	assert.Equal(t, 12*time.Hour, TTLRepoMetadata)
	assert.Equal(t, 87600*time.Hour, TTLImmutable)
	assert.Equal(t, 1*time.Hour, TTLActionRef)
	assert.Equal(t, 6*time.Hour, TTLDockerMetadata)
}

func TestCacheKeyBuilders(t *testing.T) {
	assert.Equal(t, "registry:PyPI:litellm:1.82.8", RegistryKey("PyPI", "litellm", "1.82.8"))
	assert.Equal(t, "repo:pallets/flask", RepoKey("pallets/flask"))
	assert.Equal(t, "repo:pallets/flask:abc123", RepoSHAKey("pallets/flask", "abc123"))
	assert.Equal(t, "action:actions/checkout:v4", ActionRefKey("actions/checkout", "v4"))
	assert.Equal(t, "docker:python:3.12-slim", DockerKey("python", "3.12-slim"))
	assert.Equal(t, "cve:PyPI:litellm:1.82.8", CVEKey("PyPI", "litellm", "1.82.8"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cache/ -run "TestCacheTier|TestCacheKey" -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/cache/tiers.go
package cache

import "time"

// TTL constants for the multi-tier caching strategy.
const (
	TTLRegistryMetadata = 24 * time.Hour      // package registry data (PyPI, npm, etc.)
	TTLCVEData          = 6 * time.Hour        // vulnerability data (OSV, NVD)
	TTLRepoMetadata     = 12 * time.Hour       // repo stars, maintainers, archived status
	TTLImmutable        = 87600 * time.Hour    // content at a specific SHA (10 years)
	TTLActionRef        = 1 * time.Hour        // tag → SHA resolution (tags can move)
	TTLDockerMetadata   = 6 * time.Hour        // Docker Hub image metadata
)

// Key builders for consistent cache key formatting.

func RegistryKey(ecosystem, name, version string) string {
	return "registry:" + ecosystem + ":" + name + ":" + version
}

func CVEKey(ecosystem, name, version string) string {
	return "cve:" + ecosystem + ":" + name + ":" + version
}

func RepoKey(ownerRepo string) string {
	return "repo:" + ownerRepo
}

func RepoSHAKey(ownerRepo, sha string) string {
	return "repo:" + ownerRepo + ":" + sha
}

func ActionRefKey(ownerRepo, ref string) string {
	return "action:" + ownerRepo + ":" + ref
}

func DockerKey(image, tag string) string {
	return "docker:" + image + ":" + tag
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cache/ -run "TestCacheTier|TestCacheKey" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cache/tiers.go internal/cache/tiers_test.go
git commit -m "feat(cache): add multi-tier cache TTL constants and key builders"
```

---

### Task 8: Graph in JSON output — `internal/report/json.go`

**Files:**
- Modify: `internal/report/json.go` (add optional graph to JSON output)
- Test: `internal/report/json_test.go`

Add the graph as an optional field in JSON output when it's available in ScanResult. This enables consumers to access the full graph structure.

- [ ] **Step 1: Add graph serialization to json.go**

In `internal/report/json.go`, add a graph section to `jsonScanResult`:

```go
// Add to jsonScanResult struct:
Graph *jsonGraph `json:"graph,omitempty"`

// New types:
type jsonGraph struct {
	Nodes []jsonGraphNode `json:"nodes"`
	Edges []jsonGraphEdge `json:"edges"`
}

type jsonGraphNode struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Score   int    `json:"score"`
	Risk    string `json:"risk"`
	Pinning string `json:"pinning,omitempty"`
}

type jsonGraphEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Type  string `json:"type"`
	Depth int    `json:"depth,omitempty"`
}
```

In `WriteJSON`, after building the output struct, check if `result.Graph` is set and serialize it:

```go
if result.Graph != nil {
	if g, ok := result.Graph.(*graph.Graph); ok {
		// build jsonGraph from g
	}
}
```

- [ ] **Step 2: Write test**

```go
func TestWriteJSONWithGraph(t *testing.T) {
	// Create a ScanResult with a Graph attached
	// Verify JSON output contains "graph" key with nodes and edges
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/report/ -v`
Expected: PASS (both new and existing tests)

- [ ] **Step 4: Commit**

```bash
git add internal/report/json.go internal/report/json_test.go
git commit -m "feat(report): include graph in JSON output"
```

---

### Task 9: Full integration test + regression

**Files:**
- Test: `internal/graph/integration_test.go`

- [ ] **Step 1: Write integration test**

```go
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
		Profile:       "enterprise",
		PassThreshold: 70,
		DirectDeps:    3,
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

	// Flask should have transitive risk from colorama
	flask := g.Node("package:python/flask@3.2.0")
	require.NotNil(t, flask)
	trScore, _ := flask.Metadata["transitive_risk_score"].(int)
	assert.Less(t, trScore, 81)

	// Convert back to ScanResult
	back := ToScanResult(g, sr)
	assert.Equal(t, "enterprise", back.Profile)
	assert.Len(t, back.Packages, 5)

	// Verify NodesOfType filtering
	pypiPkgs := g.NodesOfType(NodePackage)
	var pypiCount int
	for _, n := range pypiPkgs {
		if eco, ok := n.Metadata["ecosystem"].(string); ok && eco == "python" {
			pypiCount++
		}
	}
	assert.Equal(t, 3, pypiCount)
}
```

- [ ] **Step 2: Run integration test**

Run: `go test ./internal/graph/ -run TestFullPipeline -v`
Expected: PASS

- [ ] **Step 3: Run full regression test suite**

Run: `go test ./... -count=1`
Expected: All packages pass. No regressions.

- [ ] **Step 4: Commit**

```bash
git add internal/graph/integration_test.go
git commit -m "test(graph): add full pipeline integration test"
```

---

### Task 10: Build and verify

- [ ] **Step 1: Build binary**

Run: `go build -o depscope ./cmd/depscope/`
Expected: Compiles without errors

- [ ] **Step 2: Verify --only flag help**

Run: `./depscope scan --help`
Expected: Shows `--only` flag in help output

- [ ] **Step 3: Test --only flag**

Run: `./depscope scan . --only go --no-cve`
Expected: Only scans Go dependencies (cobra, testify, etc.)

- [ ] **Step 4: Verify JSON output includes graph**

Run: `./depscope scan . --output json --no-cve | python3 -c "import sys,json; d=json.load(sys.stdin); print('graph' in d, len(d.get('graph',{}).get('nodes',[])))"`
Expected: `True` followed by a node count

- [ ] **Step 5: Full test suite with race detector**

Run: `go test ./... -race -count=1`
Expected: All tests pass

- [ ] **Step 6: Commit if any fixes needed**

```bash
git commit -am "fix(graph): fixes from smoke testing"
```
