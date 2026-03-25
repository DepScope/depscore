# Supply Chain Graph + GitHub Actions Scanner Design Spec

## Problem

depscope currently scans package dependencies as a flat list. But the real supply chain is a graph: your app depends on packages, which live in repos, which have CI workflows, which use actions, which bundle their own packages, which depend on more packages. A compromised action in a transitive dependency's CI pipeline is just as dangerous as a compromised package — but nobody sees it today.

Additionally, GitHub Actions references are often pinned to mutable tags (`@v4`) instead of immutable SHAs, creating a supply chain risk that package managers solved years ago. Pre-commit hooks, Docker base images, and curl-pipe-bash patterns in CI have the same problem.

## Solution

Three-phase enhancement to depscope:

1. **Graph infrastructure** — refactor the scan pipeline to produce a dependency graph instead of a flat list
2. **Actions scanner** — parse GitHub Actions workflows, resolve all 5 layers of action dependencies, score them
3. **Graph visualization** — interactive TUI explorer for navigating the full supply chain graph

## Graph Data Model

### Node Types

| Node type | Description | Phase |
|-----------|-------------|-------|
| `package` | Versioned software dependency | Now (exists) |
| `repo` | Source code repository | Phase 1 |
| `action` | CI/CD action reference | Phase 2 |
| `workflow` | Workflow file | Phase 2 |
| `docker_image` | Container base image | Phase 2 |
| `script_download` | curl/wget binary in CI steps | Phase 2 |
| `hook` | Pre-commit/husky hook | Future |
| `terraform_module` | IaC module reference | Future |
| `git_submodule` | .gitmodules reference | Future |
| `build_tool` | Compiler/runtime version | Future |
| `os_package` | apt/apk/yum in Dockerfile | Future |
| `vendored_code` | Copied/vendored code | Future |

### Edge Types

| Edge type | From → To | Description | Phase |
|-----------|-----------|-------------|-------|
| `depends_on` | package → package | Package dependency | Now (exists) |
| `hosted_at` | package → repo | Package source repo | Phase 1 |
| `uses_action` | workflow → action | Workflow uses action | Phase 2 |
| `bundles` | action → package | Action bundles packages | Phase 2 |
| `triggers` | workflow → workflow | Calls reusable workflow | Phase 2 |
| `resolves_to` | action → repo | Tag/ref resolves to SHA | Phase 2 |
| `pulls_image` | workflow/action → docker_image | FROM or container reference | Phase 2 |
| `downloads` | workflow → script_download | curl/wget in run step | Phase 2 |
| `uses_hook` | hook_config → hook | Pre-commit config uses hook | Future |
| `uses_module` | terraform_config → terraform_module | Module source reference | Future |
| `includes_submodule` | repo → repo | Git submodule | Future |
| `built_with` | repo → build_tool | Runtime/compiler version | Future |
| `installs_os_pkg` | docker_image → os_package | apt/apk install | Future |
| `vendors` | repo → vendored_code | Copied code in repo | Future |
| `attests` | package/action → signing | Provenance attestation | Future |

### Node Identity

Each node has a unique ID following the pattern `{type}:{ecosystem}/{name}@{version}`:

- `package:python/litellm@1.82.8`
- `repo:github.com/BerriAI/litellm`
- `action:actions/checkout@v4`
- `action:actions/checkout@abc123def456` (SHA-pinned)
- `workflow:github.com/org/repo/.github/workflows/ci.yml`
- `docker_image:docker.io/python@3.12-slim`
- `script_download:https://install.example.com/setup.sh`

**Mapping from existing Package.Key():** The current `Package.Key()` returns `{ecosystem}/{name}@{version}` (e.g., `python/litellm@1.82.8`). The graph builder prepends the node type prefix: `package:` + `Package.Key()`. The ecosystem string in the key uses the internal constant value (`python`, `go`, `npm`, `rust`, `php`) not the display string (`PyPI`, `Go`, `crates.io`). No changes to `Package.Key()` itself.

### Node Properties

```go
type Node struct {
    ID       string
    Type     NodeType
    Name     string
    Version  string            // resolved version or SHA
    Ref      string            // original reference (tag, branch, constraint)
    Score    int               // 0-100 reputation score
    Risk     RiskLevel         // LOW, MEDIUM, HIGH, CRITICAL
    Pinning  PinningQuality    // SHA, ExactVersion, MajorTag, Branch, Unpinned, NA
    Metadata map[string]any    // ecosystem-specific data
}

type Edge struct {
    From  string     // NodeID
    To    string     // NodeID
    Type  EdgeType
    Depth int        // distance from root
}
```

## Phase 1: Graph Infrastructure

### Refactoring the Scan Pipeline

**Current flow:**
```
manifests → parse → []Package → fetch registry → score → propagate → flat report
```

**New flow:**
```
manifests + workflows + Dockerfiles → parse → Graph → enrich → score nodes → propagate edges → report
```

### New Package: `internal/graph`

| File | Responsibility |
|------|---------------|
| `types.go` | `Graph`, `Node`, `Edge`, `NodeType`, `EdgeType`, `PinningQuality` enums |
| `graph.go` | Graph construction, node/edge addition, querying (neighbors, paths, subgraph) |
| `builder.go` | Converts `[]manifest.Package` into a graph (bridge from existing pipeline) |
| `propagator.go` | Risk propagation over graph edges (replaces current `core/propagator.go` logic) |

### Backward Compatibility

`core.ScanResult` becomes a view over the graph:
- `ScanResult.Packages` → all nodes where `Type == Package`
- `ScanResult.RiskPaths` → computed from graph path queries
- Existing text/JSON/SARIF output unchanged
- All existing tests continue to pass

### Multi-Tier Caching

Extend `internal/cache` with tiered TTLs:

| Cache key pattern | TTL | Description |
|-------------------|-----|-------------|
| `registry:{ecosystem}:{name}:{version}` | 24h | Package registry metadata (already exists) |
| `cve:{ecosystem}:{name}:{version}` | 6h | CVE data (already exists) |
| `repo:{owner}/{repo}` | 12h | Repo metadata: stars, maintainers, archived |
| `repo:{owner}/{repo}:{sha}` | 87600h (10 years) | Immutable: file content at specific SHA. Use max TTL as "forever" — content at a SHA never changes. |
| `action:{owner}/{repo}:{ref}` | 1h | Tag → SHA resolution (tags can move) |
| `docker:{image}:{tag}` | 6h | Docker Hub metadata, tag → digest |

### CLI Changes (Phase 1)

`--only` flag on `depscope scan`:

```bash
depscope scan .                        # everything (all detected ecosystems)
depscope scan . --only python          # just Python deps
depscope scan . --only python,go       # Python + Go
depscope scan . --only actions         # just GitHub Actions (Phase 2)
```

Accepts: `python`, `go`, `rust`, `npm`, `php`, `actions`, `docker` (last two enabled in Phase 2).

## Phase 2: GitHub Actions Scanner

### New Package: `internal/actions`

| File | Responsibility |
|------|---------------|
| `parser.go` | Parse `.github/workflows/*.yml` — extract all `uses:`, `run:`, `container:` references |
| `resolver.go` | Resolve action refs: tag → SHA via GitHub API, fetch action.yml, determine action type |
| `composite.go` | Parse composite action steps, recurse into transitive action deps |
| `bundled.go` | Fetch bundled code (package.json, requirements.txt, Dockerfile) from action repos |
| `dockerfile.go` | Parse Dockerfile FROM lines and detect pip/npm installs |
| `scriptdetect.go` | Detect curl/wget pipe-to-shell patterns in `run:` blocks |
| `scorer.go` | Action-specific scoring factors |
| `types.go` | Action-specific types: WorkflowFile, ActionRef, ActionType |

### 5-Layer Resolution Pipeline

**Layer 1: Workflow parsing**

Parse `.github/workflows/*.yml` — extract:
- `jobs.*.steps[].uses` → action references
- `jobs.*.uses` → reusable workflow references
- `jobs.*.container.image` → Docker image references
- `jobs.*.steps[].run` → scan for script download patterns
- `docker://image:tag` in `uses` → Docker image references
- `./local-action` in `uses` → resolve within the repo

**Layer 2: Ref resolution**

For each action reference, resolve to a concrete SHA:
- `actions/checkout@v4` → GitHub API: `GET /repos/actions/checkout/git/ref/tags/v4` → SHA
- Cache tag→SHA (1h TTL, tags can move)
- Cache SHA→content (forever, immutable)

**Layer 3: action.yml parsing**

Fetch `action.yml` (or `action.yaml`) at the resolved SHA. Determine action type:
- `runs.using: composite` → parse `runs.steps[].uses` for transitive action deps, recurse
- `runs.using: node20` (or node16, node12) → JS action
- `runs.using: docker` → Docker action, `runs.image` points to Dockerfile or image

**Layer 4: Bundled code analysis**

For JS actions: fetch `package.json` and `package-lock.json` from action repo at resolved SHA. Feed into existing npm manifest parser → package nodes connected via `bundles` edges.

For Docker actions: fetch Dockerfile. Extract `FROM` → docker_image node. Detect `pip install`, `COPY requirements.txt`, `npm install` → package nodes.

For Python actions (composite with pip): detect `pip install` in `run:` steps → package refs.

**Layer 5: Reusable workflows**

`uses: org/repo/.github/workflows/build.yml@main` — fetch the workflow file from the referenced repo at the specified ref. Parse it as a workflow (back to Layer 1). All its actions become transitive deps.

**Depth control:** `--depth` flag limits recursion across all layers. Default 10. Cycle detection via visited set on NodeIDs.

### Pinning Quality Classification

| Quality | Example | Score impact |
|---------|---------|-------------|
| SHA | `actions/checkout@abc123def` | Best — immutable |
| ExactVersion | `actions/checkout@v4.2.0` | Good — conventionally stable |
| MajorTag | `actions/checkout@v4` | Moderate — mutable, common practice |
| Branch | `some-org/action@main` | Poor — constantly changing |
| Unpinned | `some-org/action` (no ref) | Critical — anything goes |
| Digest | `docker://alpine@sha256:abc` | Best for Docker images |

**First-party context:** Actions from `actions/*` and `github/*` orgs get reduced pinning penalty since GitHub controls tag integrity. Third-party actions are scored strictly.

### Action Scoring Factors

| Factor | Weight | What it measures |
|--------|--------|-----------------|
| Pinning quality | 25% | SHA > exact version > major tag > branch |
| First-party status | 15% | actions/*, github/* get a boost |
| Repository health | 15% | Stars, recent commits, archived status |
| Maintainer count | 10% | Bus factor |
| Release recency | 10% | Last release/tag age |
| Bundled dep risk | 15% | Worst score among bundled packages |
| Permissions scope | 10% | Workflow-level `permissions:` block grants broad access (e.g., `contents: write`, `id-token: write`). Scored per-workflow, inherited by all actions within it. If no `permissions:` block is defined, the workflow gets default (broad) permissions — higher risk. This factor is on the workflow node, propagated to child action nodes. |

### Docker Image Scoring Factors

| Factor | Weight | What it measures |
|--------|--------|-----------------|
| Pinning quality | 30% | Digest > exact tag > latest |
| Official status | 20% | Docker Official Image or Verified Publisher |
| Image age | 20% | Last pushed date |
| Base image chain | 15% | What it's built FROM |
| Vulnerability count | 15% | Known CVEs in image layers. Phase 2 uses Docker Hub API metadata (if available) rather than full image scanning. Full CVE scanning (Trivy/Grype integration) deferred to future enhancement. |

### Script Download Scoring

Any `curl|bash`, `wget|sh`, `curl -sSL url | python` pattern detected in `run:` blocks = CRITICAL risk, score 0. No version pinning, no integrity check, no audit trail.

Detection patterns:
- `curl ... | sh`, `curl ... | bash`
- `wget ... | sh`, `wget ... | bash`
- `curl ... | python`, `wget ... | python`
- `curl -o script.sh ... && sh script.sh` (download then execute)

### CLI Changes (Phase 2)

**`--org` flag** on `scan`:

```bash
depscope scan --org my-org                          # scan all repos in org
depscope scan --org my-org --only actions           # just actions across org
```

Implementation: GitHub API `GET /orgs/{org}/repos` (paginated) to list repos, then scan each repo's workflows via the existing remote resolver (GitHub Trees API + Contents API, no clone needed).

**`--org` flag** on `discover` (deferred to follow-up):

`discover --org` would search all org repos for a specific compromised action. This requires different resolution than the current filesystem-based discover (which walks local dirs). Deferring to a follow-up spec to keep Phase 2 focused. Users can achieve the same result with `depscope discover actions/checkout --range ">=4.0.0" --list org-repos.txt` where `org-repos.txt` lists cloned repo paths.

**Output changes:**

Text output adds a "Pinning Summary" section:

```
Pinning Summary (GitHub Actions):
  SHA-pinned:     12 (48%)
  Exact version:   8 (32%)
  Major tag:        4 (16%)  ⚠
  Branch:           1 (4%)   ⚠⚠

  First-party:    15    Third-party: 10
  Script downloads: 2   ⚠⚠ (curl|bash detected)
```

JSON output adds `pinning_summary` object and `graph` object (nodes + edges) alongside existing `packages` array.

SARIF output maps issues to workflow file locations (line numbers of `uses:` references).

### Manifest Detection

`.github/workflows/*.yml` files are added to the existing manifest detection:
- `resolve/filters.go`: `ManifestFilenames` gains `Dockerfile` (Docker image scanning)
- `manifest/manifest.go`: New `EcosystemActions` and `EcosystemDocker` constants added to `Ecosystem` type. `ecosystemFiles` table gains entries for `.github/workflows/` directory detection. `DetectAllEcosystems` (which lives in `manifest/manifest.go`, not `resolve/filters.go`) recognizes `EcosystemActions` when `.github/workflows/` exists.
- Note: workflow files are detected at the directory level (`.github/workflows/` exists), not as individual manifest filenames, since there can be multiple workflow files per project.

## Phase 3: Graph Visualization (TUI)

### New Package: `internal/tui`

Built with `bubbletea` (Go TUI framework) and `lipgloss` (styling).

### New Command: `depscope explore`

```bash
depscope explore .                    # scan then launch TUI
depscope explore --org my-org         # org-wide then TUI
depscope scan . --explore             # shorthand
```

### TUI Layout

```
┌─ depscope explore ─────────────────────────────────────────────┐
│ Search: lit_                                          [/] find │
├────────────────────────────────────────────────────────────────┤
│ ▼ your-app (root)                                             │
│   ├── litellm@1.82.8 [HIGH ●] score:45                       │
│   │   ├── hosted: github.com/BerriAI/litellm                 │
│   │   │   └── .github/workflows/ci.yml                        │
│   │   │       ├── actions/checkout@v4 [SHA ✓]                 │
│   │   │       └── some-org/deploy@main [BRANCH ⚠⚠]           │
│   │   │           └── bundles: lodash@4.17.21 score:82        │
│   │   └── depends: openai@1.30.0, httpx@0.27.0               │
│   ├── flask@3.2.0 score:81                                    │
│   └── .github/workflows/ci.yml                                │
│       ├── actions/checkout@abc123 [SHA ✓] score:95            │
│       └── docker://python:3.12 [EXACT ✓] score:78            │
├────────────────────────────────────────────────────────────────┤
│ [↑↓] navigate [enter] expand [/] search [f] filter [q] quit  │
└────────────────────────────────────────────────────────────────┘
```

### Key Interactions

| Key | Action |
|-----|--------|
| `↑↓` | Navigate nodes |
| `enter` | Expand/collapse node, show children |
| `/` | Fuzzy search across all node names |
| `f` | Filter by risk level, node type, pinning quality |
| `i` | Inspect: detail panel with full metadata, score breakdown, all edges |
| `p` | Path: show all paths from root to selected node |
| `Tab` | Switch between tree view and flat sorted-by-risk view |
| `q` | Quit |

### Views

**Tree view** (default): Hierarchical expansion from root project. Nodes colored by risk. Expand to see children.

**Flat view** (Tab toggle): All nodes sorted by score ascending (worst first). Grouped by type. Quick way to find the weakest links.

**Inspect panel** (i key): Side panel showing:
- Full node metadata (registry data, API data)
- Score breakdown (factor-by-factor)
- All incoming and outgoing edges
- For actions: pinning quality, resolved SHA, action type
- For packages: CVE list, maintainer info

**Path view** (p key): Shows all dependency paths from root to selected node. Like the current risk paths but interactive — click any node in the path to jump to it.

## Implementation Order

| Phase | What | Depends on |
|-------|------|-----------|
| Phase 1 | Graph infrastructure, refactor scan pipeline, --only flag, multi-tier cache | Nothing |
| Phase 2 | Actions scanner (5 layers), Docker/script detection, --org flag, pinning summary, action/docker scoring | Phase 1 |
| Phase 3 | TUI explorer, tree/flat views, search/filter/inspect/path | Phase 1 (Phase 2 makes it more useful) |

Phase 2 and 3 can be parallelized after Phase 1 completes.

Each phase gets its own implementation plan (via writing-plans skill).

## Error Handling

- **GitHub API rate limiting:** Respect `X-RateLimit-Remaining`, back off when low. Cache aggressively. Surface rate limit warnings to user.
- **Private repos:** Actions in private repos require appropriate token scope. Warn and skip if inaccessible.
- **Invalid workflow YAML:** Log warning, skip the file, continue scanning other workflows.
- **Action repo not found:** Score as CRITICAL (action references a nonexistent repo — possibly deleted/compromised).
- **Circular dependencies:** Cycle detection via visited set. Break cycle, log warning.
- **Network failures:** Degrade gracefully per-node. Mark as unresolvable, continue with other nodes.
- **Docker Hub API limits:** Anonymous pulls limited to 100/6h. Cache image metadata aggressively.
- **Org scanning rate:** Paginate repo list, respect rate limits, show progress.

## Testing Strategy

### Phase 1
- Unit tests for graph operations (add node, add edge, query paths, subgraph)
- Unit tests for builder ([]Package → Graph conversion)
- Unit tests for graph-based risk propagation
- Integration test: existing scan produces identical output via graph pipeline
- Regression: all existing tests pass unchanged

### Phase 2
- Unit tests for workflow YAML parsing (fixture files for various workflow patterns)
- Unit tests for action ref resolution (mock GitHub API responses)
- Unit tests for action.yml parsing (composite, JS, Docker types)
- Unit tests for Dockerfile FROM extraction
- Unit tests for curl|bash detection in run blocks
- Unit tests for action and Docker scoring factors
- Integration test: scan a repo with workflows, verify graph contains action nodes
- Mock-based tests for --org scanning

### Phase 3
- Unit tests for TUI model state transitions (expand, collapse, search, filter)
- Unit tests for tree rendering from graph
- Unit tests for path computation
- Manual testing for interactive TUI (hard to automate)
