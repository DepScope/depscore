# Phase 6: TUI Graph View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a graph view mode to the `depscope explore` TUI — force-directed layout rendered with Unicode characters, three zoom levels (risk overview, neighborhood, clusters).

**Architecture:** New `internal/tui/graphview` sub-package with force layout algorithm (Fruchterman-Reingold), Unicode edge rendering (Bresenham line drawing), and bubbletea integration as a third view mode.

**Tech Stack:** Pure Go, bubbletea, lipgloss, no external dependencies.

**Spec:** `docs/superpowers/specs/2026-03-26-web-graph-visualization-design.md` (TUI Graph View section)

---

### Task 1: Force layout algorithm — `internal/tui/graphview/layout.go`

**Files:**
- Create: `internal/tui/graphview/layout.go`
- Test: `internal/tui/graphview/layout_test.go`

Simplified Fruchterman-Reingold force-directed layout:

```go
type Position struct { X, Y float64 }

type LayoutConfig struct {
    Width, Height float64
    Iterations    int
    Repulsion     float64
    Attraction    float64
}

// Layout computes positions for graph nodes using force-directed placement.
func Layout(g *graph.Graph, nodeIDs []string, cfg LayoutConfig) map[string]Position
```

Algorithm:
1. Random initial positions within bounds
2. For each iteration:
   - Repulsion force between all node pairs (inverse square)
   - Attraction force along edges (spring)
   - Apply forces with temperature cooling
3. Return final positions

Tests: verify nodes don't overlap, positions within bounds, edges pull connected nodes closer.

- [ ] **Step 1: Write tests**
- [ ] **Step 2: Write layout algorithm**
- [ ] **Step 3: Run tests, commit**

```bash
git commit --no-verify -m "feat(tui): add force-directed layout algorithm"
```

---

### Task 2: Unicode edge rendering — `internal/tui/graphview/edges.go`

**Files:**
- Create: `internal/tui/graphview/edges.go`
- Test: `internal/tui/graphview/edges_test.go`

Modified Bresenham line drawing that outputs Unicode characters:

```go
type Cell struct {
    Char  rune
    Color lipgloss.Style
}

// DrawEdge draws a line from (x1,y1) to (x2,y2) on the grid using Unicode line characters.
func DrawEdge(grid [][]Cell, x1, y1, x2, y2 int, style lipgloss.Style)
```

Character selection based on angle:
- Horizontal: `─`
- Vertical: `│`
- Diagonal up-right: `╱`
- Diagonal down-right: `╲`
- Corners: `╭`, `╮`, `╰`, `╯`
- Crossings: `┼`
- Arrows at end: `→`, `↓`, `↑`, `←`

Tests: verify horizontal line, vertical line, diagonal, crossing detection.

- [ ] **Step 1: Write tests**
- [ ] **Step 2: Write edge rendering**
- [ ] **Step 3: Commit**

```bash
git commit --no-verify -m "feat(tui): add Unicode edge rendering with Bresenham"
```

---

### Task 3: Graph canvas renderer — `internal/tui/graphview/render.go`

**Files:**
- Create: `internal/tui/graphview/render.go`
- Test: `internal/tui/graphview/render_test.go`

Combines layout + edge drawing + node placement into a renderable string:

```go
// Render produces a string representing the graph in the terminal.
func Render(g *graph.Graph, nodeIDs []string, width, height int) string
```

Steps:
1. Compute layout positions
2. Create character grid (width × height)
3. Draw edges on grid
4. Place node characters (●◆■⬡▲) at positions, colored by risk
5. Add short labels next to nodes (truncated name)
6. Convert grid to string

- [ ] **Step 1: Write render function**
- [ ] **Step 2: Write test (verify output contains expected node chars)**
- [ ] **Step 3: Commit**

```bash
git commit --no-verify -m "feat(tui): add graph canvas renderer"
```

---

### Task 4: Graph view mode — `internal/tui/graphview/view.go`

**Files:**
- Create: `internal/tui/graphview/view.go`
- Modify: `internal/tui/model.go` — add viewGraph mode, `g` key handler

Three zoom levels:
- Risk overview: only HIGH+CRITICAL nodes
- Neighborhood: selected node + 1-2 hop neighbors
- Cluster: all nodes, grouped by type

```go
type GraphViewModel struct {
    graph     *graph.Graph
    zoomLevel int  // 0=risk, 1=neighborhood, 2=cluster
    selected  string // selected node ID
    rendered  string // cached render output
}

func (m *GraphViewModel) Render(width, height int) string
func (m *GraphViewModel) ZoomIn(nodeID string)  // enter neighborhood
func (m *GraphViewModel) ZoomOut()               // back to overview
func (m *GraphViewModel) SelectNode(direction)   // navigate between nodes
```

Wire into existing TUI model:
- `g` key switches to graph view mode
- `+`/`-` zoom in/out between levels
- `Enter` in graph view enters neighborhood of selected node
- `Esc` returns to previous zoom level or back to tree view
- Arrow keys navigate between nodes

- [ ] **Step 1: Write GraphViewModel**
- [ ] **Step 2: Wire into model.go (add viewGraph constant, g key handler)**
- [ ] **Step 3: Update footer help text**
- [ ] **Step 4: Test zoom level transitions**
- [ ] **Step 5: Commit**

```bash
git commit --no-verify -m "feat(tui): add graph view mode with zoom levels"
```

---

### Task 5: Build and smoke test

- [ ] **Step 1: Build**

Run: `go build -o depscope ./cmd/depscope/`

- [ ] **Step 2: Test graph view**

Run: `./depscope explore . --no-cve`
- Press `g` to enter graph view (risk overview)
- Press `Enter` on a node for neighborhood view
- Press `Esc` to go back
- Press `-` for cluster view
- Press `g` again to return to tree view

- [ ] **Step 3: Full test suite**

Run: `go test ./... -race -count=1`

- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "test(tui): verify graph view mode"
```
