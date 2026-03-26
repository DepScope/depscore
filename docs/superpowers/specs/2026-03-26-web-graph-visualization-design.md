# Web Graph Visualization Design Spec

## Problem

depscope builds a rich supply chain graph (176+ nodes, 1125+ edges) but the web UI only shows a flat table. Users can't see relationships, trace blast radius of a compromised package, or understand how a vulnerable action deep in their CI pipeline connects to their application. The TUI explorer helps but isn't shareable or accessible to non-terminal users.

## Solution

Add an interactive web-based graph visualization page to the existing `depscope server` web UI. D3.js force-directed layout with risk-colored nodes, blast radius analysis, zero-day simulation, and gap analysis. Backed by SQLite for scan persistence.

## Page Layout

New page at `/scan/{id}/graph`, accessible via a "Graph" toggle button on the existing results page header.

Three-column layout matching the existing dark theme:

```
┌─────────────────────────────────────────────────────────────────────┐
│  depscope    ← Back to Table    scan: github.com/org/repo          │
│              175 nodes · 1125 edges · 5 actions · 2 workflows      │
├──────────┬──────────────────────────────────────────────┬───────────┤
│ CONTROLS │            D3 Force Graph                    │  DETAIL   │
│ (280px)  │           (fills remaining)                  │  PANEL    │
│ collaps. │                                              │  (380px)  │
│          │                                              │  on click │
├──────────┴──────────────────────────────────────────────┴───────────┤
│  [↕] sidebar    zoom: fit | + | -     legend: ● ◆ ■ ⬡ ▲           │
└─────────────────────────────────────────────────────────────────────┘
```

- **Left sidebar** (280px, collapsible): search, type filters, risk filter, blast radius, zero-day simulation, gap analysis
- **Center**: D3 force-directed SVG graph with zoom/pan/drag
- **Right detail panel** (380px, slides in on node click): full node info, score breakdown, edges, CVEs
- **Bottom bar**: zoom controls, legend, sidebar toggle

## Graph Visualization

### Node Rendering

| Node type | Shape | Default color | Size |
|-----------|-------|--------------|------|
| package | Circle | Risk-colored (red/orange/yellow/green) | Scaled by connection count |
| action | Diamond | Risk-colored | Fixed medium |
| workflow | Square | Purple (neutral) | Fixed large |
| docker_image | Hexagon | Risk-colored | Fixed medium |
| script_download | Triangle | Always red | Fixed small |

Risk colors match existing CSS variables: LOW=#00CC00, MEDIUM=#FF8800, HIGH=#FF4400, CRITICAL=#FF0000.

### Edge Rendering

| Edge type | Color | Style |
|-----------|-------|-------|
| depends_on | Grey (#666) | Solid arrow |
| uses_action | Blue (#4488FF) | Solid arrow |
| bundles | Orange (#FF8800) | Dashed arrow |
| pulls_image | Cyan (#00CCCC) | Solid arrow |
| downloads | Red (#FF0000) | Dashed arrow |
| triggers | Purple (#8844FF) | Dotted arrow |

### Interactions

- **Drag** — reposition nodes, simulation re-adjusts
- **Zoom/pan** — mouse wheel + drag on background
- **Hover** — tooltip with name, score, risk level
- **Click** — opens detail panel, highlights connected edges (others dim)
- **Double-click** — zoom to fit the node's 1-hop neighborhood

### Force Simulation

- Charge force (repulsion) between all nodes (-300 strength)
- Link force (attraction) along edges (distance based on edge type)
- Center force to keep graph centered
- Collision force to prevent node overlap (radius + padding)
- Warm start, cool down over ~3 seconds. Drag reheats.

## Left Sidebar Controls

### Search

Text input with live filtering. As user types, matching nodes pulse/highlight in the graph. Enter zooms to first match.

### Type Filters

Checkboxes for each node type:
- ☑ Packages (169)
- ☑ Actions (5)
- ☑ Workflows (2)
- ☑ Docker images (0)
- ☑ Script downloads (0)

Unchecking hides those nodes and their edges from the graph.

### Risk Filter

Radio buttons:
- ○ All nodes
- ○ HIGH + CRITICAL only
- ○ CRITICAL only

Filters dim non-matching nodes to 10% opacity (keep them visible for context but not prominent).

### Blast Radius

Tab toggle: `CVE` | `Package`

**CVE mode:**
- Input: CVE ID (e.g., `CVE-2024-99999`)
- "Analyze" button
- Backend queries OSV.dev to find affected package + version range, then traces through the graph

**Package mode:**
- Input: package name (e.g., `litellm`)
- Input: compromised version range (e.g., `>=1.82.7,<1.83.0`)
- "Analyze" button

**Visual effect:**
- Affected nodes pulse red with glow animation
- Paths between affected nodes highlighted with thick red edges
- Unaffected nodes dim to 20% opacity
- Sidebar shows: affected count, list of affected nodes (clickable → zoom to node), exposure paths

**"Reset" button** restores normal view.

### Zero-Day Simulation

Form fields:
- Package name (text input)
- Affected version range (text input)
- Severity (dropdown: critical / high / medium / low)
- Description (optional textarea)
- "Simulate Zero-Day" button (red)

Backend creates a temporary CVE-like entry, runs blast radius analysis against the graph. Same visual effect as blast radius. Result is NOT persisted.

### Gap Analysis

"Analyze Gaps" button. Highlights nodes with security gaps:

| Gap type | Color | Description |
|----------|-------|-------------|
| Unpinned actions | Orange | Actions not SHA-pinned (major tag or worse) |
| Unpinned Docker | Cyan | Docker images not digest-pinned |
| No lockfile | Yellow | Packages with constraint only, no resolved version |
| Broad permissions | Purple | Workflows with no `permissions:` block or write access |
| Script downloads | Red | Always a gap |

Sidebar lists gaps grouped by type with counts. Clicking a gap zooms to the node.

## Right Detail Panel

Slides in from right on node click (reuses existing side panel animation pattern from results.html).

### Package Detail
- Name, version, ecosystem
- Score gauge (SVG arc, reuse existing component)
- Risk badge + transitive risk
- Registry link (PyPI, npm, crates.io, pkg.go.dev, packagist.org)
- 8 reputation checks with pass/fail status
- CVEs (linked to osv.dev)
- Issues list
- Outgoing edges (dependencies)
- Incoming edges (depended on by)

### Action Detail
- Name, ref, resolved SHA (truncated, copy button)
- Pinning quality badge (SHA ✓ / exact version / major tag ⚠ / branch ⚠⚠)
- First-party badge (GitHub-maintained / third-party)
- Action type (composite / node / docker)
- Score with 7-factor breakdown
- Bundled packages count (clickable → highlights in graph)
- Permissions scope

### Workflow Detail
- File path
- Actions count
- Permissions block content
- Script downloads detected (count)

### Docker Image Detail
- Image name, tag/digest
- Pinning quality badge
- Official / Verified Publisher status
- Score with 5-factor breakdown

### Script Download Detail
- URL
- Detection pattern (curl|bash, wget|sh, etc.)
- CRITICAL badge, score 0
- Source workflow + line number

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/scan/{id}/graph` | HTML page (graph UI) |
| GET | `/api/scan/{id}/graph` | Graph JSON (D3-friendly format) |
| GET | `/api/scan/{id}/graph/node/{nodeID}` | Single node detail with full metadata |
| POST | `/api/scan/{id}/blast-radius` | Blast radius analysis |
| POST | `/api/scan/{id}/simulate` | Zero-day simulation |
| GET | `/api/scan/{id}/gaps` | Gap analysis |

### Graph JSON Format (D3-friendly)

```json
{
  "nodes": [
    {
      "id": "package:python/litellm@1.82.8",
      "type": "package",
      "name": "litellm",
      "version": "1.82.8",
      "score": 45,
      "risk": "HIGH",
      "pinning": "n/a",
      "group": "python",
      "metadata": {}
    }
  ],
  "links": [
    {
      "source": "workflow:ci.yml",
      "target": "action:actions/checkout@v4",
      "type": "uses_action",
      "depth": 1
    }
  ],
  "summary": {
    "total_nodes": 176,
    "total_edges": 1125,
    "by_type": {"package": 169, "action": 5, "workflow": 2}
  }
}
```

Note: D3 expects `links` with `source`/`target`, not `edges` with `from`/`to`. The endpoint transforms the internal graph format.

### Blast Radius Request/Response

**Request:**
```json
{
  "mode": "cve",
  "cve_id": "CVE-2024-99999"
}
```
or:
```json
{
  "mode": "package",
  "package": "litellm",
  "range": ">=1.82.7,<1.83.0"
}
```

**Response:**
```json
{
  "affected_nodes": ["package:python/litellm@1.82.8", "action:org/deploy@v2"],
  "paths": [
    ["workflow:ci.yml", "action:org/deploy@v2", "package:python/litellm@1.82.8"]
  ],
  "total_affected": 2,
  "blast_depth": 3
}
```

### Simulate Request/Response

```json
{
  "package": "lodash",
  "range": ">=4.17.0,<4.17.22",
  "severity": "critical",
  "description": "Simulated prototype pollution RCE"
}
```

Uses the same version range format as blast radius package mode. Response: same format as blast radius.

Note: `repo` nodes (from `EdgeHostedAt`/`EdgeResolvesTo`) are structural — hidden from the graph view by default to reduce clutter. They can be shown via a future "show repos" toggle.

### Gap Analysis Response

```json
{
  "gaps": [
    {"type": "unpinned_action", "node_id": "action:org/deploy@main", "detail": "branch-pinned, should be SHA"},
    {"type": "broad_permissions", "node_id": "workflow:release.yml", "detail": "no permissions block defined"},
    {"type": "script_download", "node_id": "script:https://install.sh", "detail": "curl|bash detected"}
  ],
  "summary": {"unpinned_action": 3, "unpinned_docker": 0, "no_lockfile": 0, "broad_permissions": 1, "script_download": 0}
}
```

## SQLite Storage

### Schema

```sql
CREATE TABLE scans (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    profile TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    error TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);

CREATE TABLE nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id TEXT NOT NULL REFERENCES scans(id),
    node_id TEXT NOT NULL,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    version TEXT,
    ref TEXT,
    score INTEGER DEFAULT 0,
    risk TEXT,
    pinning TEXT,
    metadata JSON,
    UNIQUE(scan_id, node_id)
);

CREATE TABLE edges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id TEXT NOT NULL REFERENCES scans(id),
    from_node TEXT NOT NULL,
    to_node TEXT NOT NULL,
    type TEXT NOT NULL,
    depth INTEGER DEFAULT 0
);

CREATE INDEX idx_nodes_scan ON nodes(scan_id);
CREATE INDEX idx_nodes_type ON nodes(scan_id, type);
CREATE INDEX idx_edges_scan ON edges(scan_id);
CREATE INDEX idx_edges_from ON edges(scan_id, from_node);
CREATE INDEX idx_edges_to ON edges(scan_id, to_node);
```

### Storage Flow

```
scan request → scorePipeline → build graph → store to SQLite → respond
server restart → load scan list from SQLite → load graph on demand
graph page → load graph from SQLite into memory → serve API from in-memory graph
```

### New Package: `internal/server/store/sqlite.go`

Implements the existing `ScanStore` interface (already defined in `internal/server/store/store.go`) plus new graph methods:

```go
type GraphStore interface {
    SaveGraph(scanID string, g *graph.Graph) error
    LoadGraph(scanID string) (*graph.Graph, error)
}
```

The existing `ScanStore` interface has `Save`, `Get`, `SetStatus` methods. SQLite implements both `ScanStore` and `GraphStore`. The in-memory store continues to work for tests and simple usage. DynamoDB store is unchanged.

### Database Location

- CLI: `~/.cache/depscope/depscope.db`
- Server: `--store sqlite` flag (default for server mode), file at `./depscope.db` or `--db-path` flag
- Lambda: continues using DynamoDB (SQLite not available in Lambda)

## Tech Stack

- **D3.js v7** — loaded via CDN `<script>` tag (no build step, matches existing pattern)
- **SQLite** — via `modernc.org/sqlite` (pure Go, no CGO, cross-platform)
- **Vanilla JS** — no frameworks, consistent with existing UI
- **Existing dark theme** — reuse CSS variables and component patterns

## TUI Graph View

Add a third view mode to the existing `depscope explore` TUI (alongside tree and flat). Press `g` to switch to graph view.

### Three Zoom Levels

| Level | Key | What it shows | Node count |
|-------|-----|---------------|------------|
| Risk overview | `g` (default) | Only HIGH + CRITICAL nodes | ~15-30 |
| Neighborhood | `Enter` on a node | Selected node + 1-2 hop neighbors | ~5-20 |
| Cluster view | `-` to zoom out | Nodes grouped by type (packages, actions, workflows) | All, grouped |

`+`/`-` keys zoom between levels. `Enter` in graph view enters neighborhood of selected node. `Esc` returns to previous zoom level.

### Node Rendering (Unicode)

```
●  package (circle)
◆  action (diamond)
■  workflow (square)
⬡  docker image (hexagon)
▲  script download (triangle)
```

Colored by risk: red=CRITICAL, orange=HIGH, yellow=MEDIUM, green=LOW. Using ANSI terminal colors.

### Edge Rendering (Unicode Line Drawing)

```
─  horizontal        │  vertical
╭  top-left curve    ╮  top-right curve
╰  bottom-left       ╯  bottom-right
╱  diagonal up       ╲  diagonal down
→  right arrow       ↓  down arrow       ↑  up arrow       ←  left arrow
┼  crossing
```

Edges colored by type: grey=depends_on, blue=uses_action, orange=bundles, red=downloads, cyan=pulls_image, purple=triggers.

### Layout Algorithm

1. Compute force-directed layout in Go (simplified force simulation):
   - Repulsion force between all nodes
   - Attraction force along edges
   - Iterate ~100 steps to settle
2. Map floating-point positions to character grid (terminal width × height)
3. Place node characters at grid positions
4. Draw edges using Bresenham-like line drawing with Unicode characters
5. Handle crossings with `┼` character
6. Re-layout on terminal resize

### Example: Risk Overview

```
          ●colorama(35)
         ╱
   ●click(81)──→●yaml.v3(35)
      │
   ◆lint(52)──→●@actions/core(0)
      │
   ◆tag(45)──→●@semantic-release(0)
```

### Example: Neighborhood View (centered on ci.yml)

```
                    ●@actions/core
                   ╱         ╲
   ■ci.yml──→◆checkout    ●@actions/github
      │    ╲
      │     ◆setup-go──→●@actions/cache
      │         ╲
      └──→◆lint──→●@octokit/rest
               ╲
                ●@actions/tool-cache
```

### Integration with Existing TUI

- New view mode accessed via `g` key (alongside tree=default, flat=Tab)
- Same footer: `[↑↓] navigate [g] graph [enter] zoom in [esc] zoom out [/] search [f] filter [i] inspect [q] quit`
- Search (`/`) highlights matching nodes in the graph
- Filter (`f`) applies to graph (only show filtered risk levels)
- Inspect (`i`) opens the same detail panel as tree/flat views
- Blast radius not in TUI (web only — too complex for terminal sidebar)

### New Package: `internal/tui/graphview`

| File | Responsibility |
|------|---------------|
| `layout.go` | Force-directed layout algorithm (Go port of simplified D3 force) |
| `render.go` | Map layout positions to character grid, draw nodes + edges with Unicode |
| `edges.go` | Bresenham line drawing with Unicode characters, crossing detection |
| `view.go` | Bubbletea integration — handles zoom levels, navigation, resize |

### Tech

- Pure Go, no external dependencies beyond existing bubbletea/lipgloss
- Force layout: simple Fruchterman-Reingold algorithm (~100 lines)
- Edge routing: modified Bresenham with Unicode character selection
- Character grid: 2D byte array mapped to terminal coordinates

## Implementation Phases

| Phase | What | Priority |
|-------|------|----------|
| 1 | SQLite store + graph persistence | Foundation |
| 2 | Graph page + D3 visualization + node click detail | Core value |
| 3 | Blast radius (CVE + package mode) | High value |
| 4 | Zero-day simulation | Medium value |
| 5 | Gap analysis | Medium value |
| 6 | TUI graph view (risk overview + neighborhood + clusters) | High value |
| 7 | Polish: animations, responsive, performance | Nice to have |

Each phase produces working software. Phase 2 is the big visual payoff. Phase 6 can run in parallel with phases 3-5 since it's in a different package.

## Error Handling

- **D3 rendering with large graphs (500+ nodes)**: Use node/edge limits with "show top N by risk" option. Force simulation with lower alpha for faster settling.
- **TUI graph with many nodes**: Risk overview limits to HIGH+ by default. Neighborhood limits to 2 hops. Cluster view groups nodes so individual count doesn't matter.
- **Blast radius with no matches**: Show "No affected nodes found" message, don't change graph state.
- **Simulation with unknown package**: Show "Package not found in graph" warning.
- **SQLite file corruption**: Fall back to in-memory store, log warning.
- **CDN unavailable (D3)**: Embed a fallback D3 bundle in the binary as a static asset. Check CDN first, fall back to local.
- **Terminal too small for graph**: Fall back to tree view with warning "Terminal too small for graph view (min 80×24)".

## Testing Strategy

- **SQLite store**: Unit tests for Save/Load graph round-trip, blast radius queries
- **API endpoints**: httptest-based tests with fixture graphs
- **D3 visualization**: Manual testing (hard to automate SVG rendering). Verify data format with unit tests on the JSON serialization.
- **Blast radius logic**: Unit tests with known graph topologies and expected affected sets
- **Gap analysis**: Unit tests checking each gap type detection
- **TUI graph layout**: Unit tests for force layout (verify nodes don't overlap, edges connect correct nodes)
- **TUI edge rendering**: Unit tests for Bresenham line drawing (verify correct Unicode characters at positions)
- **TUI zoom levels**: Unit tests for risk filtering, neighborhood extraction, cluster grouping
