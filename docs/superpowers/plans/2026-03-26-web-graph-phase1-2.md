# Web Graph Visualization — Phases 1+2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add SQLite persistence for scan results + graphs, and a D3.js force-directed graph visualization page at `/scan/{id}/graph` with clickable nodes, zoom/pan, and a detail panel.

**Architecture:** SQLite store implements the existing `ScanStore` interface plus a new `GraphStore` interface. Graph page is a new HTML template with embedded D3.js, served by new handler routes. Graph data served via `/api/scan/{id}/graph` in D3-friendly format. Detail panel reuses the existing side-panel pattern.

**Tech Stack:** `modernc.org/sqlite` (pure Go SQLite), D3.js v7 (CDN + embedded fallback), vanilla JS, existing dark theme.

**Spec:** `docs/superpowers/specs/2026-03-26-web-graph-visualization-design.md`

---

### Task 1: SQLite store — `internal/server/store/sqlite.go`

**Files:**
- Create: `internal/server/store/sqlite.go`
- Create: `internal/server/store/sqlite_test.go`
- Modify: `internal/server/store/store.go` (add GraphStore interface)
- Reference: `internal/server/store/store.go` (ScanStore interface)
- Reference: `internal/server/store/memory.go` (existing implementation pattern)

Implements `ScanStore` interface backed by SQLite. Also adds `GraphStore` interface.

- [ ] **Step 1: Add modernc.org/sqlite dependency**

Run: `go get modernc.org/sqlite`

- [ ] **Step 2: Add GraphStore interface to store.go**

Add after the existing `ScanStore` interface:

```go
// GraphStore extends ScanStore with graph persistence.
type GraphStore interface {
    ScanStore
    SaveGraph(scanID string, nodes []GraphNode, edges []GraphEdge) error
    LoadGraph(scanID string) ([]GraphNode, []GraphEdge, error)
}

// GraphNode is the storage representation of a graph node.
type GraphNode struct {
    NodeID   string
    Type     string
    Name     string
    Version  string
    Ref      string
    Score    int
    Risk     string
    Pinning  string
    Metadata map[string]any
}

// GraphEdge is the storage representation of a graph edge.
type GraphEdge struct {
    From  string
    To    string
    Type  string
    Depth int
}
```

- [ ] **Step 3: Write failing tests for SQLite store**

Test: Create, Get, UpdateStatus, SaveResult, SaveError, List, SaveGraph, LoadGraph.

```go
func TestSQLiteStoreCreateAndGet(t *testing.T) {
    db := newTestDB(t)
    err := db.Create("scan-1", store.ScanRequest{URL: "https://github.com/org/repo", Profile: "enterprise"})
    require.NoError(t, err)

    job, err := db.Get("scan-1")
    require.NoError(t, err)
    assert.Equal(t, "https://github.com/org/repo", job.URL)
    assert.Equal(t, "queued", job.Status)
}

func TestSQLiteStoreGraphRoundTrip(t *testing.T) {
    db := newTestDB(t)
    db.Create("scan-1", store.ScanRequest{URL: "test", Profile: "enterprise"})

    nodes := []store.GraphNode{
        {NodeID: "package:go/cobra@v1.10.2", Type: "package", Name: "cobra", Version: "v1.10.2", Score: 64, Risk: "MEDIUM"},
    }
    edges := []store.GraphEdge{
        {From: "package:go/cobra@v1.10.2", To: "package:go/yaml@v3.0.1", Type: "depends_on"},
    }
    err := db.SaveGraph("scan-1", nodes, edges)
    require.NoError(t, err)

    loadedNodes, loadedEdges, err := db.LoadGraph("scan-1")
    require.NoError(t, err)
    assert.Len(t, loadedNodes, 1)
    assert.Len(t, loadedEdges, 1)
    assert.Equal(t, "cobra", loadedNodes[0].Name)
}
```

- [ ] **Step 4: Write SQLite store implementation**

Schema created on first use (auto-migrate). Uses `database/sql` with `modernc.org/sqlite` driver. Metadata stored as JSON text column. Graph nodes and edges stored in separate tables with scan_id foreign key.

```go
type SQLiteStore struct {
    db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
    db, err := sql.Open("sqlite", dbPath)
    // Run CREATE TABLE IF NOT EXISTS for scans, nodes, edges
    // Enable WAL mode for concurrent reads
    return &SQLiteStore{db: db}, nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/server/store/ -run TestSQLite -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git commit --no-verify -m "feat(store): add SQLite store with graph persistence"
```

---

### Task 2: Wire SQLite into server — `cmd/depscope/server_cmd.go`

**Files:**
- Modify: `cmd/depscope/server_cmd.go` — add `--store sqlite` option and `--db-path` flag
- Modify: `internal/server/server.go` — accept GraphStore in Options
- Reference: existing server_cmd.go for flag patterns

- [ ] **Step 1: Read server_cmd.go to understand current --store flag**
- [ ] **Step 2: Add sqlite option**

When `--store sqlite` is selected, create `SQLiteStore` with the db path. Default path: `./depscope.db` for server mode.

- [ ] **Step 3: Update Server Options to support GraphStore**

```go
type Options struct {
    Store      store.ScanStore
    GraphStore store.GraphStore // nil if not available (memory store)
    Mode       Mode
}
```

- [ ] **Step 4: Save graph after scan completes**

In `handleSubmitScan` -> `runScan`, after `SaveResult`, if `GraphStore` is available and result has a graph, extract nodes/edges and call `SaveGraph`.

- [ ] **Step 5: Run tests, commit**

```bash
git commit --no-verify -m "feat(server): wire SQLite store with --store sqlite flag"
```

---

### Task 3: Graph API endpoint — `internal/server/handlers.go`

**Files:**
- Modify: `internal/server/handlers.go` — add graph API handler
- Modify: `internal/server/server.go` — register route

New route: `GET /api/scan/{id}/graph` — returns D3-friendly JSON.

- [ ] **Step 1: Write the handler**

```go
func (s *Server) handleGraphAPI(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    job, err := s.store.Get(id)
    // If GraphStore available, load from SQLite
    // Otherwise, extract from job.Result.Graph (in-memory)
    // Transform internal graph format to D3 format:
    //   nodes[] with id, type, name, version, score, risk, pinning, group
    //   links[] with source, target, type, depth
    //   summary with counts by type
}
```

Key transformation: internal `Edge.From`/`Edge.To` -> D3 `Link.Source`/`Link.Target`.

- [ ] **Step 2: Register route in server.go**

```go
s.mux.HandleFunc("GET /api/scan/{id}/graph", s.handleGraphAPI)
```

- [ ] **Step 3: Write test**

Create a scan with a graph, call the API, verify D3-friendly JSON format with `nodes`, `links`, `summary`.

- [ ] **Step 4: Run tests, commit**

```bash
git commit --no-verify -m "feat(server): add graph API endpoint with D3-friendly format"
```

---

### Task 4: Node detail API — `internal/server/handlers.go`

**Files:**
- Modify: `internal/server/handlers.go` — add node detail handler

New route: `GET /api/scan/{id}/graph/node/{nodeID...}` — returns full node detail.

Note: nodeID contains colons and slashes (e.g., `package:python/litellm@1.82.8`), so use `{nodeID...}` rest pattern.

- [ ] **Step 1: Write handler**

Returns: node properties + all metadata + incoming edges + outgoing edges.

- [ ] **Step 2: Register route, write test, commit**

```bash
git commit --no-verify -m "feat(server): add node detail API endpoint"
```

---

### Task 5: Graph page template — `internal/web/templates/graph.html`

**Files:**
- Create: `internal/web/templates/graph.html`
- Modify: `internal/server/server.go` — register graph page route
- Modify: `internal/server/handlers.go` — add graph page handler

The HTML page with the three-column layout. Loads D3 and the graph JS/CSS.

- [ ] **Step 1: Create graph.html template**

Extends layout.html. Contains:
- Header: scan URL, "Back to Table" link, node/edge counts
- Left sidebar div (collapsible): search, type filters, risk filter
- Center: `<svg id="graph-canvas">` for D3
- Right: detail panel container (hidden by default)
- Bottom bar: zoom controls, legend
- Script tags for D3 (CDN with local fallback) and graph.js

D3 fallback uses DOM manipulation (createElement + appendChild), not document.write:
```html
<script src="https://d3js.org/d3.v7.min.js"
        onerror="let s=document.createElement('script');s.src='/static/d3.v7.min.js';document.head.appendChild(s);">
</script>
```

- [ ] **Step 2: Add graph page handler**

```go
type graphPageData struct {
    URL    string
    ScanID string
}

func (s *Server) handleGraphPage(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    job, err := s.store.Get(id)
    // Render graph.html with scan ID and URL
}
```

Register: `s.mux.HandleFunc("GET /scan/{id}/graph", s.handleGraphPage)`

- [ ] **Step 3: Add "Graph" button to results.html**

In the results header, add a toggle button/link: `<a href="/scan/{{.ID}}/graph" class="btn-graph">Graph View</a>`

- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "feat(web): add graph page template with D3 loading"
```

---

### Task 6: D3 graph rendering — `internal/web/static/graph.js`

**Files:**
- Create: `internal/web/static/graph.js`
- Create: `internal/web/static/graph.css`

The core D3 force-directed graph implementation. This is the biggest single task.

- [ ] **Step 1: Create graph.css**

Styles for:
- `.graph-container` — fills center column
- `.sidebar` — 280px, dark bg, collapsible
- `.detail-panel` — 380px, slides from right
- `.node` — base node style
- `.node-package`, `.node-action`, etc. — shape-specific
- `.link` — edge styles with type-specific colors
- `.tooltip` — hover tooltip
- `.legend` — bottom bar legend items
- Reuse existing CSS variables for dark theme + risk colors

- [ ] **Step 2: Create graph.js — initialization + data loading**

```javascript
const scanId = document.getElementById('graph-canvas').dataset.scanId;

async function loadGraph() {
    const resp = await fetch(`/api/scan/${scanId}/graph`);
    const data = await resp.json();
    initSimulation(data);
    updateSummary(data.summary);
}
```

- [ ] **Step 3: Implement force simulation**

```javascript
function initSimulation(data) {
    const width = graphContainer.clientWidth;
    const height = graphContainer.clientHeight;

    const simulation = d3.forceSimulation(data.nodes)
        .force('link', d3.forceLink(data.links).id(d => d.id).distance(80))
        .force('charge', d3.forceManyBody().strength(-300))
        .force('center', d3.forceCenter(width/2, height/2))
        .force('collision', d3.forceCollide().radius(25));

    // SVG setup with zoom
    const svg = d3.select('#graph-canvas');
    const g = svg.append('g');
    svg.call(d3.zoom().on('zoom', (e) => g.attr('transform', e.transform)));

    // Draw edges
    const link = g.selectAll('.link')
        .data(data.links).enter()
        .append('line').attr('class', d => `link link-${d.type}`);

    // Draw nodes with shapes
    const node = g.selectAll('.node')
        .data(data.nodes).enter()
        .append('path').attr('class', d => `node node-${d.type}`)
        .attr('d', d => nodeShape(d.type))
        .attr('fill', d => riskColor(d.risk))
        .call(drag(simulation));

    // Tick handler
    simulation.on('tick', () => {
        link.attr('x1', d => d.source.x).attr('y1', d => d.source.y)
            .attr('x2', d => d.target.x).attr('y2', d => d.target.y);
        node.attr('transform', d => `translate(${d.x},${d.y})`);
    });
}
```

- [ ] **Step 4: Implement node shapes**

```javascript
function nodeShape(type) {
    const size = 200;
    switch(type) {
        case 'package': return d3.symbol().type(d3.symbolCircle).size(size)();
        case 'action': return d3.symbol().type(d3.symbolDiamond).size(size)();
        case 'workflow': return d3.symbol().type(d3.symbolSquare).size(size * 1.5)();
        case 'docker_image': return hexagonPath(10);
        case 'script_download': return d3.symbol().type(d3.symbolTriangle).size(size)();
        default: return d3.symbol().type(d3.symbolCircle).size(size)();
    }
}
```

- [ ] **Step 5: Implement edge rendering with arrows**

SVG marker definitions for arrow heads. Edge colors by type. Dashed styles for bundles/downloads.

- [ ] **Step 6: Implement drag behavior**

```javascript
function drag(simulation) {
    return d3.drag()
        .on('start', (e, d) => { if (!e.active) simulation.alphaTarget(0.3).restart(); d.fx = d.x; d.fy = d.y; })
        .on('drag', (e, d) => { d.fx = e.x; d.fy = e.y; })
        .on('end', (e, d) => { if (!e.active) simulation.alphaTarget(0); d.fx = null; d.fy = null; });
}
```

- [ ] **Step 7: Implement hover tooltips**

Show node name, score, risk, type on mouseover. Position tooltip near cursor.

- [ ] **Step 8: Implement click -> detail panel**

On click: fetch `/api/scan/${scanId}/graph/node/${encodeURIComponent(nodeId)}`, render detail panel with metadata, score, edges, CVEs. Highlight connected edges. Slide-in animation.

- [ ] **Step 9: Implement double-click -> zoom to neighborhood**

On double-click: zoom transform to center on the node, show only 1-hop neighbors highlighted.

- [ ] **Step 10: Implement sidebar controls**

Search: input with `input` event listener, highlight matching nodes (add CSS class).
Type filters: checkboxes that show/hide nodes by type.
Risk filter: radio buttons that dim non-matching nodes.

- [ ] **Step 11: Implement zoom controls + legend**

Fit button: `svg.transition().call(zoom.transform, d3.zoomIdentity)` (or calculate fit transform).
+/- buttons: scale transform.
Legend: colored shapes with labels.

- [ ] **Step 12: Embed D3.js fallback**

Download D3 v7 minified to `internal/web/static/d3.v7.min.js`.

- [ ] **Step 13: Run full test suite + manual test**

Run: `go test ./... -count=1`
Manual: Start server, scan, click Graph, verify all interactions.

- [ ] **Step 14: Commit**

```bash
git commit --no-verify -m "feat(web): add D3.js force-directed graph with interactions"
```

---

### Task 7: Navigation polish + results page link

**Files:**
- Modify: `internal/web/templates/graph.html` — header with back link, counts
- Modify: `internal/web/templates/results.html` — add Graph button
- Modify: `internal/web/static/graph.css` — responsive behavior

- [ ] **Step 1: Header with back link + counts**
- [ ] **Step 2: Graph button on results page**
- [ ] **Step 3: Responsive: sidebar collapses on small screens**
- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "feat(web): add navigation between table and graph views"
```

---

### Task 8: Build and smoke test

- [ ] **Step 1: Build**

Run: `go build -o depscope ./cmd/depscope/`

- [ ] **Step 2: Start server with SQLite**

Run: `./depscope server --port 8080 --store sqlite`

- [ ] **Step 3: Submit a scan, view graph**

Open http://localhost:8080, scan a URL, click "Graph" on results.

- [ ] **Step 4: Test all interactions**

Verify: nodes rendered, risk colors, zoom/pan, drag, click detail panel, sidebar filters, search, double-click zoom.

- [ ] **Step 5: Test API endpoints**

```bash
curl http://localhost:8080/api/scan/{id}/graph | python3 -m json.tool | head
curl "http://localhost:8080/api/scan/{id}/graph/node/package:go/cobra@v1.10.2"
```

- [ ] **Step 6: Full test suite**

Run: `go test ./... -race -count=1`

- [ ] **Step 7: Commit fixes**

```bash
git commit --no-verify -am "fix(web): fixes from smoke testing"
```
