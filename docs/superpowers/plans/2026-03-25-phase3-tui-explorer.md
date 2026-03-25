# Phase 3: TUI Graph Explorer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an interactive TUI explorer (`depscope explore`) that lets users navigate the supply chain graph with tree/flat views, fuzzy search, filtering, and node inspection.

**Architecture:** New `internal/tui` package using bubbletea (Elm architecture). The model holds the graph + view state. Views render as tree (expandable nodes) or flat (sorted by risk). The TUI launches after a scan completes.

**Tech Stack:** `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles`

**Spec:** `docs/superpowers/specs/2026-03-25-supply-chain-graph-actions-design.md` (Phase 3 section)

**Depends on:** Phase 1 (graph) and Phase 2 (actions) merged.

---

### Task 1: TUI model and basic tree rendering — `internal/tui/model.go`

**Files:**
- Create: `internal/tui/model.go`
- Create: `internal/tui/tree.go`
- Create: `internal/tui/styles.go`
- Test: `internal/tui/model_test.go`

The bubbletea Model holds the graph, cursor position, expanded nodes, and current view mode.

**model.go:**
```go
package tui

import (
    "github.com/charmbracelet/bubbletea"
    "github.com/depscope/depscope/internal/graph"
)

type viewMode int
const (
    viewTree viewMode = iota
    viewFlat
)

type Model struct {
    graph      *graph.Graph
    roots      []string      // root node IDs (depth-1 packages + workflows)
    expanded   map[string]bool
    cursor     int
    visible    []string      // currently visible node IDs (after expand/collapse)
    mode       viewMode
    width      int
    height     int
    searchMode bool
    searchQuery string
    filterRisk string        // "" = all, "HIGH", "CRITICAL"
    inspecting string        // node ID being inspected (empty = no panel)
}

func NewModel(g *graph.Graph) Model
func (m Model) Init() tea.Cmd
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m Model) View() string
```

**tree.go:** Renders the tree view — walks root nodes, shows children for expanded nodes, color-codes by risk.

**styles.go:** lipgloss styles for risk colors, selected row, header, footer.

Test: verify `NewModel` initializes correctly, `visible` list is built from roots.

- [ ] **Step 1: Write styles.go** — risk color styles (red=CRITICAL, orange=HIGH, yellow=MEDIUM, green=LOW)
- [ ] **Step 2: Write model.go** — Model struct, Init, Update (handle key events), View (delegates to tree/flat)
- [ ] **Step 3: Write tree.go** — renderTree builds visible lines from expanded state
- [ ] **Step 4: Write model_test.go** — test NewModel, expand/collapse, cursor movement
- [ ] **Step 5: Run tests, commit**

```bash
git commit --no-verify -m "feat(tui): add model, tree rendering, and styles"
```

---

### Task 2: Key bindings and navigation

**Files:**
- Modify: `internal/tui/model.go` — Update function handles all keys

Key handling in Update:
```go
case tea.KeyMsg:
    switch msg.String() {
    case "up", "k":
        m.cursor = max(0, m.cursor-1)
    case "down", "j":
        m.cursor = min(len(m.visible)-1, m.cursor+1)
    case "enter":
        m.toggleExpand(m.visible[m.cursor])
    case "tab":
        m.toggleViewMode()
    case "/":
        m.searchMode = true
    case "f":
        m.cycleFilter()
    case "i":
        m.toggleInspect()
    case "p":
        m.showPaths()
    case "q", "ctrl+c":
        return m, tea.Quit
    }
```

- [ ] **Step 1: Implement all key handlers**
- [ ] **Step 2: Add page-up/page-down (ctrl+u/ctrl+d)**
- [ ] **Step 3: Test key handling**
- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "feat(tui): add key bindings and navigation"
```

---

### Task 3: Flat view — `internal/tui/flat.go`

**Files:**
- Create: `internal/tui/flat.go`

Renders all nodes in a flat table sorted by score ascending (worst first). Grouped by node type. Shows: name, version, score, risk, pinning, type.

- [ ] **Step 1: Write flat.go** — renderFlat builds sorted node list
- [ ] **Step 2: Tab toggles between tree and flat view**
- [ ] **Step 3: Test flat view rendering**
- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "feat(tui): add flat sorted-by-risk view"
```

---

### Task 4: Search — `internal/tui/search.go`

**Files:**
- Create: `internal/tui/search.go`

Fuzzy search across all node names. When `/` is pressed, show search input at top. As user types, filter visible nodes to matches. Enter to jump to first match. Esc to cancel search.

Uses `bubbles/textinput` for the search field.

- [ ] **Step 1: Write search.go** — search model + fuzzy matching
- [ ] **Step 2: Wire into Update/View**
- [ ] **Step 3: Test search filtering**
- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "feat(tui): add fuzzy search"
```

---

### Task 5: Filter by risk — `internal/tui/filter.go`

**Files:**
- Create: `internal/tui/filter.go`

`f` key cycles through filter states: All → HIGH+ → CRITICAL → All. Filters the visible node list.

- [ ] **Step 1: Write filter.go**
- [ ] **Step 2: Wire into Update/View**
- [ ] **Step 3: Test filter cycling**
- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "feat(tui): add risk-level filtering"
```

---

### Task 6: Inspect panel — `internal/tui/inspect.go`

**Files:**
- Create: `internal/tui/inspect.go`

`i` key opens a side panel showing full node details:
- All metadata fields
- Score breakdown (for actions: each factor)
- All incoming/outgoing edges
- For actions: pinning quality, resolved SHA, action type
- For packages: CVE list, maintainer info

Renders as a right-side panel taking ~40% width.

- [ ] **Step 1: Write inspect.go** — renderInspectPanel
- [ ] **Step 2: Wire into View (split layout: tree left, panel right)**
- [ ] **Step 3: Test inspect rendering**
- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "feat(tui): add inspect panel"
```

---

### Task 7: Path tracing — `internal/tui/paths.go`

**Files:**
- Create: `internal/tui/paths.go`

`p` key shows all paths from root to the selected node. Uses `graph.FindPaths()`. Displayed as a temporary overlay or replacing the main view.

- [ ] **Step 1: Write paths.go**
- [ ] **Step 2: Wire into Update/View**
- [ ] **Step 3: Test path display**
- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "feat(tui): add path tracing view"
```

---

### Task 8: CLI command — `cmd/depscope/explore_cmd.go`

**Files:**
- Create: `cmd/depscope/explore_cmd.go`

```bash
depscope explore .                    # scan then launch TUI
depscope explore . --only actions     # scan actions only, then TUI
depscope explore . --no-cve           # skip CVEs, launch TUI
```

The command runs a scan (reusing scanner.ScanDir/ScanURL), extracts the graph, and launches the TUI.

Also add `--explore` flag to the `scan` command as shorthand.

- [ ] **Step 1: Write explore_cmd.go** — Cobra command, runs scan, launches bubbletea program
- [ ] **Step 2: Add --explore flag to scan command**
- [ ] **Step 3: Test command registration**
- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "feat(tui): add explore command and --explore flag"
```

---

### Task 9: Footer help bar + polish

**Files:**
- Modify: `internal/tui/model.go` — add help bar at bottom
- Modify: `internal/tui/styles.go` — polish colors

Footer: `[↑↓] navigate [enter] expand [/] search [f] filter [i] inspect [p] path [Tab] tree/flat [q] quit`

- [ ] **Step 1: Add footer rendering**
- [ ] **Step 2: Add header with project name + node counts**
- [ ] **Step 3: Polish risk colors, selected highlight, borders**
- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "feat(tui): add footer help bar and visual polish"
```

---

### Task 10: Build and smoke test

- [ ] **Step 1: Build binary**

Run: `go build -o depscope ./cmd/depscope/`

- [ ] **Step 2: Test explore command**

Run: `./depscope explore . --no-cve`
Expected: TUI launches with tree view of this project's graph

- [ ] **Step 3: Test keyboard interactions**
- Arrow keys navigate, Enter expands/collapses
- `/` opens search, typing filters
- `f` cycles risk filter
- `i` opens inspect panel
- `Tab` switches to flat view
- `p` shows paths
- `q` quits

- [ ] **Step 4: Test --explore flag on scan**

Run: `./depscope scan . --explore --no-cve`
Expected: Same TUI launches

- [ ] **Step 5: Full test suite**

Run: `go test ./... -count=1`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git commit --no-verify -m "fix(tui): fixes from smoke testing"
```
