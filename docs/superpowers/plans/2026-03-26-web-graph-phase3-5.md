# Web Graph — Phases 3-5: Blast Radius, Simulation, Gap Analysis

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add blast radius analysis (CVE + package mode), zero-day simulation, and gap analysis to the web graph page.

**Architecture:** Three new API endpoints + sidebar UI sections. Backend logic operates on the in-memory graph. Frontend highlights affected nodes/paths in the D3 visualization.

**Tech Stack:** Go backend, vanilla JS frontend, existing D3 graph page, existing graph package.

**Spec:** `docs/superpowers/specs/2026-03-26-web-graph-visualization-design.md`

---

### Task 1: Blast radius backend — `internal/server/handlers.go`

**Files:**
- Create: `internal/server/blast.go` — blast radius + simulation logic
- Test: `internal/server/blast_test.go`

New endpoint: `POST /api/scan/{id}/blast-radius`

Request (CVE mode):
```json
{"mode": "cve", "cve_id": "CVE-2024-99999"}
```

Request (package mode):
```json
{"mode": "package", "package": "litellm", "range": ">=1.82.7,<1.83.0"}
```

Response:
```json
{
  "affected_nodes": ["package:python/litellm@1.82.8"],
  "paths": [["workflow:ci.yml", "action:deploy", "package:python/litellm@1.82.8"]],
  "total_affected": 1,
  "blast_depth": 2
}
```

Logic:
- CVE mode: query OSV.dev API with the CVE ID to get affected package + version range, then find matching nodes in graph
- Package mode: use `discover.ParseRange` + `discover.VersionInRange` to find matching package nodes
- For each affected node: use `graph.FindPaths` from each root to find exposure paths
- Also find nodes that transitively depend on affected nodes (reverse BFS)

- [ ] **Step 1: Write blast radius logic**
- [ ] **Step 2: Write tests with fixture graph**
- [ ] **Step 3: Register route, commit**

```bash
git commit --no-verify -m "feat(server): add blast radius API endpoint"
```

---

### Task 2: Zero-day simulation — `internal/server/blast.go`

New endpoint: `POST /api/scan/{id}/simulate`

Request:
```json
{"package": "lodash", "range": ">=4.17.0,<4.17.22", "severity": "critical", "description": "Simulated RCE"}
```

Uses the same blast radius logic but with a user-provided package + range instead of a CVE lookup. NOT persisted.

- [ ] **Step 1: Add simulate handler (reuses blast radius logic)**
- [ ] **Step 2: Write test, register route, commit**

```bash
git commit --no-verify -m "feat(server): add zero-day simulation API endpoint"
```

---

### Task 3: Gap analysis — `internal/server/gaps.go`

New endpoint: `GET /api/scan/{id}/gaps`

Response:
```json
{
  "gaps": [
    {"type": "unpinned_action", "node_id": "action:org/deploy@main", "detail": "branch-pinned"},
    {"type": "broad_permissions", "node_id": "workflow:release.yml", "detail": "no permissions block"}
  ],
  "summary": {"unpinned_action": 3, "broad_permissions": 1, "script_download": 0}
}
```

Gap types:
- `unpinned_action`: action nodes with pinning quality worse than SHA
- `unpinned_docker`: docker image nodes not digest-pinned
- `no_lockfile`: package nodes with no resolved version (constraint only)
- `broad_permissions`: workflow nodes with broad/missing permissions
- `script_download`: script download nodes (always a gap)

- [ ] **Step 1: Write gap analysis logic**
- [ ] **Step 2: Write test, register route, commit**

```bash
git commit --no-verify -m "feat(server): add gap analysis API endpoint"
```

---

### Task 4: Blast radius UI — sidebar + graph highlighting

**Files:**
- Modify: `internal/web/templates/graph.html` — add blast radius section to sidebar
- Modify: `internal/web/static/graph.js` — add blast radius fetch + highlighting
- Modify: `internal/web/static/graph.css` — pulse animation, dimming styles

Add to sidebar:
```html
<div class="sidebar-section">
  <h3>Blast Radius</h3>
  <div class="tab-toggle">
    <button class="tab active" data-tab="cve">CVE</button>
    <button class="tab" data-tab="package">Package</button>
  </div>
  <div id="tab-cve" class="tab-content active">
    <input type="text" id="blast-cve" placeholder="CVE-2024-99999">
  </div>
  <div id="tab-package" class="tab-content">
    <input type="text" id="blast-pkg" placeholder="Package name">
    <input type="text" id="blast-range" placeholder=">=1.82.7,<1.83.0">
  </div>
  <button id="btn-blast" class="btn-danger">Analyze</button>
  <button id="btn-blast-reset" class="btn-secondary" style="display:none">Reset</button>
  <div id="blast-results"></div>
</div>
```

JS: fetch POST, highlight affected nodes (red pulse + glow), dim unaffected, show paths in sidebar.

- [ ] **Step 1: Add sidebar HTML**
- [ ] **Step 2: Add JS for blast radius (fetch, highlight, reset)**
- [ ] **Step 3: Add CSS for pulse/glow animations**
- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "feat(web): add blast radius UI with graph highlighting"
```

---

### Task 5: Simulation UI + Gap analysis UI

**Files:**
- Modify: `internal/web/templates/graph.html` — simulation form + gap analysis button
- Modify: `internal/web/static/graph.js` — simulation + gap handlers

Simulation form in sidebar (below blast radius):
```html
<div class="sidebar-section">
  <h3>Zero-Day Simulation ⚠</h3>
  <input type="text" id="sim-pkg" placeholder="Package name">
  <input type="text" id="sim-range" placeholder="Affected version range">
  <select id="sim-severity">
    <option value="critical">Critical</option>
    <option value="high">High</option>
    <option value="medium">Medium</option>
  </select>
  <textarea id="sim-desc" placeholder="Description (optional)" rows="2"></textarea>
  <button id="btn-simulate" class="btn-danger">Simulate Zero-Day</button>
  <div id="sim-results"></div>
</div>
```

Gap analysis button:
```html
<div class="sidebar-section">
  <h3>Gap Analysis</h3>
  <button id="btn-gaps" class="btn-primary">Analyze Gaps</button>
  <div id="gap-results"></div>
</div>
```

JS: fetch endpoints, highlight nodes by gap type color, list gaps in sidebar.

- [ ] **Step 1: Add simulation + gap HTML**
- [ ] **Step 2: Add JS handlers**
- [ ] **Step 3: Commit**

```bash
git commit --no-verify -m "feat(web): add zero-day simulation and gap analysis UI"
```

---

### Task 6: Smoke test

- [ ] **Step 1: Build + start server**
- [ ] **Step 2: Scan a project, open graph**
- [ ] **Step 3: Test blast radius (package mode with a package in the graph)**
- [ ] **Step 4: Test simulation (inject hypothetical CVE)**
- [ ] **Step 5: Test gap analysis (verify unpinned actions highlighted)**
- [ ] **Step 6: Test reset button**
- [ ] **Step 7: Full test suite**

```bash
go test ./... -race -count=1
```
