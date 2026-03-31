# Unified Dependency Tree with Project Cache

**Date:** 2026-03-27
**Status:** Draft

## Problem

Depscope currently scans package manifests and GitHub Actions separately, producing a fragmented view of a project's supply chain. The real dependency tree is much larger: pre-commit hooks, git submodules, terraform modules, dev tools, build tools, and script downloads all bring in code that runs on developer machines or in CI. These are invisible today.

Additionally, many dependencies are shared across different parts of the tree (and across scans), but there is no deduplication — the same package gets re-fetched and re-analyzed repeatedly.

## Goal

From a single repo, build a complete dependency tree of **everything that runs on your dev machines or CI**, with:
- Recursive resolution up to depth 25
- Deduplication within a scan (already-seen node → add edge, stop recursing)
- Cross-scan caching at the Project and ProjectVersion level
- CVE integration bridging SHA-pinned deps back to semver
- Company/org trust detection with configurable own-org list
- Web UI (primary), TUI tree walker, and full ASCII tree output

## Scope: What Runs In Your Environment

The tree captures anything that **executes** in your dev machines, CI, or runtime:

| Source | Detection file | Runs where | Recurse into |
|---|---|---|---|
| Package dependencies | go.mod, package.json, pyproject.toml, Cargo.toml, composer.json + lockfiles | App/runtime | Transitive package deps |
| GitHub Actions | `.github/workflows/*.yml` `uses:` | CI | Bundled packages, reusable workflows |
| Pre-commit hooks | `.pre-commit-config.yaml` | Dev machines | Hook repo's package deps |
| Git submodules | `.gitmodules` | App/build | Submodule's manifests, actions, hooks |
| Terraform modules | `*.tf` `source =` blocks | Infra provisioning | Module's own module deps |
| Dev tools | `.tool-versions`, `.mise.toml` | Dev machines | Version identity (leaf) |
| Build tools | Makefile, Taskfile, justfile | Dev/CI | Script downloads they trigger |
| Script downloads | `curl\|sh`, `wget` in shell/CI | CI | Terminal leaf |

**Out of scope (opt-in future flag):** `follow_source_repos: true` — scanning a dependency's source repo CI pipeline to understand how the package was built (supply chain provenance). Not part of the default tree.

## Cache Model

SQLite database replacing the current flat disk cache (`~/.cache/depscope/`). This is a clean break — the existing `DiskCache` (SHA-256 hashed JSON files) is not migrated. First run after upgrade starts with a cold cache.

### `projects` table

One row per unique git remote or registry identity.

```sql
CREATE TABLE projects (
    id               TEXT PRIMARY KEY,  -- normalized: "github.com/actions/checkout" or "pypi/requests"
    type             TEXT NOT NULL,     -- "repo" or "registry"
    source_url       TEXT,              -- full git remote or registry URL
    maintainer_count INTEGER,
    maintainer_names TEXT,              -- JSON array
    stars            INTEGER,
    org_name         TEXT,              -- GitHub/GitLab org, or null
    org_type         TEXT,              -- "corporate", "community", "individual", "own"
    last_fetched     TIMESTAMP NOT NULL
);
```

**TTL: 24 hours.** Metadata (stars, maintainers, org status) changes over time. Re-fetch on next access after expiry.

### `project_versions` table

One row per immutable point-in-time snapshot.

```sql
CREATE TABLE project_versions (
    project_id  TEXT NOT NULL REFERENCES projects(id),
    version_key TEXT NOT NULL,     -- SHA for git deps, "ecosystem/name@version" for registry
    semver      TEXT,              -- resolved semver tag (for CVE lookups), nullable
    dep_types   TEXT,              -- JSON: what was found (manifests, actions, hooks, etc.)
    scanned_at  TIMESTAMP NOT NULL,
    PRIMARY KEY (project_id, version_key)
);

-- Normalized dependency edges (replaces JSON blob for queryability)
CREATE TABLE version_dependencies (
    parent_project_id  TEXT NOT NULL,
    parent_version_key TEXT NOT NULL,
    child_project_id   TEXT NOT NULL,
    child_version_key  TEXT NOT NULL,
    edge_type          TEXT NOT NULL,  -- "depends_on", "uses_action", "uses_hook", etc.
    FOREIGN KEY (parent_project_id, parent_version_key) REFERENCES project_versions(project_id, version_key),
    FOREIGN KEY (child_project_id, child_version_key) REFERENCES project_versions(project_id, version_key)
);

CREATE INDEX idx_vd_parent ON version_dependencies(parent_project_id, parent_version_key);
CREATE INDEX idx_vd_child ON version_dependencies(child_project_id, child_version_key);
```

**TTL: never expires.** A SHA is immutable. `npm/lodash@4.17.21` always has the same deps.

The `version_dependencies` junction table replaces a JSON `dependencies` blob — this makes `discover` queries fast via indexed lookups on the child columns instead of fragile `LIKE` queries on JSON text.

### Cache size management

`project_versions` never expires, so the database grows over time. Mitigation:
- `depscope cache status` reports database size and row counts
- `depscope cache prune --older-than 90d` removes project_versions not accessed in N days (tracks `last_accessed` timestamp, updated on cache hit)
- SQLite `VACUUM` runs after prune to reclaim space
- No automatic eviction — the user controls when to prune

### `cve_cache` table

```sql
CREATE TABLE cve_cache (
    ecosystem  TEXT NOT NULL,
    name       TEXT NOT NULL,
    version    TEXT NOT NULL,
    findings   TEXT NOT NULL,      -- JSON array of CVE records
    fetched_at TIMESTAMP NOT NULL,
    PRIMARY KEY (ecosystem, name, version)
);
```

**TTL: 6 hours.** New CVEs get published frequently.

### Ref resolution TTLs

Mutable refs must be re-resolved periodically:

| Lookup | TTL | Reason |
|---|---|---|
| Tag → SHA | 1h | Tags can be moved (especially major tags like `v4`) |
| Branch → SHA | 15min | Branches move constantly |

Once resolved to a SHA, the `project_version` for that SHA is permanent.

## Graph Model

### Node types

Existing (unchanged):
- `NodePackage` — versioned software dependency
- `NodeRepo` — source code repository
- `NodeAction` — CI/CD action reference
- `NodeWorkflow` — workflow file
- `NodeDockerImage` — container base image
- `NodeScriptDownload` — curl/wget in CI steps

New:
- `NodePrecommitHook` — `.pre-commit-config.yaml` hook
- `NodeTerraformModule` — Terraform/OpenTofu module
- `NodeGitSubmodule` — `.gitmodules` entry
- `NodeDevTool` — `.tool-versions`, `.mise.toml` entry
- `NodeBuildTool` — Makefile/Taskfile that installs things

### Edge types

Existing (unchanged):
- `EdgeDependsOn` — package → package
- `EdgeHostedAt` — package → repo
- `EdgeUsesAction` — workflow → action
- `EdgeBundles` — action → package
- `EdgeTriggers` — workflow → workflow
- `EdgeResolvesTo` — action → repo (tag→SHA)
- `EdgePullsImage` — workflow/action → docker_image
- `EdgeDownloads` — workflow → script_download

New:
- `EdgeUsesHook` — repo → precommit hook
- `EdgeUsesModule` — repo → terraform module
- `EdgeIncludesSubmodule` — repo → git submodule
- `EdgeUsesTool` — repo → dev tool
- `EdgeBuiltWith` — repo → build tool dependency

### Node structure

The existing `graph.Node` struct gains two new fields to link back to the cache. This is a **breaking change** to the struct — all existing code that constructs `Node` values must be updated.

```go
type Node struct {
    ID         string
    Type       NodeType
    Name       string
    Version    string         // resolved version or SHA
    Ref        string         // original reference (tag, branch, constraint)
    Score      int
    Risk       core.RiskLevel
    Pinning    PinningQuality
    Metadata   map[string]any
    ProjectID  string         // FK → projects.id (new)
    VersionKey string         // FK → project_versions.version_key (new)
}
```

### Pinning quality extension

Add `PinningSemverRange` between `PinningExactVersion` and `PinningMajorTag` to cover lockfile-resolved semver ranges:

```go
const (
    PinningSHA          PinningQuality = iota
    PinningDigest
    PinningExactVersion
    PinningSemverRange    // new: ^4.0.0, ~1.2.3 (mutable at publish time)
    PinningMajorTag
    PinningBranch
    PinningUnpinned
    PinningNA
)
```

### Scoring for new node types

Existing scoring (`core.Score`) is designed for registry packages. New node types use a simplified model based on the metadata available:

| Node type | Scoring factors | Notes |
|---|---|---|
| `NodePrecommitHook` | Repo health, maintainer count, org backing, pinning quality | Same as actions — these are git repos |
| `NodeTerraformModule` | Repo health, maintainer count, org backing, pinning quality, download count (Terraform Registry) | Terraform Registry provides download stats |
| `NodeGitSubmodule` | Repo health, maintainer count, org backing | No pinning factor (submodules are always SHA-pinned in git) |
| `NodeDevTool` | Release recency, org backing | Minimal — these are version identifiers, leaf nodes |
| `NodeBuildTool` | Pinning quality of scripts detected within | Heuristic — score reflects how many unpinned downloads it triggers |

## Crawler & Resolver Architecture

### FileTree type

A `FileTree` is an in-memory map of file paths to their contents, representing a directory's files. For the root scan this is built from disk; for resolved dependencies it comes from git fetch or registry API.

```go
// FileTree represents a set of files, keyed by relative path.
// For large repos, only manifest/config files are loaded (not all source).
type FileTree map[string][]byte // path → content
```

Memory management: resolvers only fetch files they need to detect dependencies (manifests, configs, workflow files), not full source trees. A resolved npm package fetches `package.json` from the registry, not the entire tarball. A resolved git repo fetches via GitHub Trees API with path filtering (already implemented in `internal/resolve/`). This keeps each `FileTree` small (typically <100 files, <1MB) even at depth 25.

For local directories (root scan), `FileTree` is populated by walking the filesystem with the same filtering — only known manifest/config filenames are read.

### Crawler

Central BFS engine with dedup. Replaces `scanner.ScanDir` / `scanner.ScanURL`.

```go
type Crawler struct {
    cache     *CacheDB
    resolvers map[DepSourceType]Resolver
    graph     *graph.Graph
    seen      map[string]bool  // version_keys seen in THIS scan (mutex-protected)
    maxDepth  int              // default 25
    ownOrgs   []string         // configured trusted orgs
    mu        sync.Mutex       // protects seen + graph
}
```

**Output type:** The Crawler produces a `*graph.Graph` (the unified dependency tree) plus a `CrawlStats` struct with counts and errors. The existing `core.ScanResult` is built from the graph in a post-processing step (scoring, CVE, risk propagation), preserving backward compatibility with report formatters.

```go
type CrawlResult struct {
    Graph    *graph.Graph
    Stats    CrawlStats
    Errors   []CrawlError  // partial failures
}

type CrawlStats struct {
    TotalNodes    int
    TotalEdges    int
    MaxDepthHit   int
    CacheHits     int
    CacheMisses   int
    ByType        map[graph.NodeType]int
}
```

### Resolver interface

Each dependency source implements:

```go
type Resolver interface {
    // Detect finds dependency references in file contents.
    // For monorepos with multiple manifests, returns refs from all of them.
    Detect(ctx context.Context, contents FileTree) ([]DepRef, error)

    // Resolve takes a reference and returns the project identity, version,
    // and a FileTree of its contents (for recursive scanning).
    Resolve(ctx context.Context, ref DepRef) (*ResolvedDep, error)
}

type DepRef struct {
    Source    DepSourceType  // package, action, precommit, terraform, submodule, tool, script
    Name     string
    Ref      string
    Ecosystem string
    Pinning  graph.PinningQuality
}

type ResolvedDep struct {
    ProjectID  string
    VersionKey string
    Semver     string       // for CVE lookups, nullable
    Contents   FileTree     // for recursive scanning (nil for leaf nodes)
    Metadata   ProjectMeta
}
```

### 7 resolvers

| Resolver | Detects from | Resolves via | Reuses existing code |
|---|---|---|---|
| `PackageResolver` | Manifest/lockfiles | Registry APIs | `internal/manifest/`, `internal/registry/` |
| `ActionResolver` | `.github/workflows/*.yml` | GitHub API | `internal/actions/` |
| `PrecommitResolver` | `.pre-commit-config.yaml` | Git clone/fetch | New |
| `TerraformResolver` | `*.tf` `source =` blocks | Registry + git | New |
| `SubmoduleResolver` | `.gitmodules` | Git fetch | New |
| `ToolResolver` | `.tool-versions`, `.mise.toml` | Version DB lookup | New |
| `ScriptResolver` | `curl\|sh`, `wget` in shell/CI | URL fetch | Partially exists |

**`BuildToolResolver` scope:** Detection in Makefiles/Taskfiles/justfiles is best-effort heuristic only. It regex-scans for `curl`, `wget`, `go install`, `pip install`, `npm install -g` patterns in target bodies. It will miss obfuscated or variable-interpolated URLs and may false-positive on commented-out code. This is explicitly a best-effort resolver — it catches the obvious cases and flags the file for manual review.

### Authentication

Resolvers that access git hosts need tokens. Configuration extends the existing pattern:

```yaml
auth:
  github_token: ${GITHUB_TOKEN}     # existing, from env
  gitlab_token: ${GITLAB_TOKEN}     # existing, from env
  terraform_token: ${TF_TOKEN}      # Terraform Cloud/Enterprise
  bitbucket_token: ${BB_TOKEN}      # Bitbucket
```

Resolvers receive an `AuthProvider` that returns the appropriate token for a given host. Private repos that fail authentication produce an error node (see Error Handling below), not a crash.

### Concurrency model

The BFS loop processes each depth level concurrently:

```go
// Per-resolver concurrency limits (configurable)
type ConcurrencyConfig struct {
    RegistryWorkers  int  // default 10 (fast API calls)
    GitCloneWorkers  int  // default 3  (slow, heavy)
    GitHubAPIWorkers int  // default 5  (rate-limited)
}
```

- **Within a BFS level:** All `Detect` calls for a given FileTree run sequentially (they're fast, CPU-only). All `Resolve` calls for discovered refs run concurrently via a worker pool, partitioned by resolver type.
- **Across levels:** The next level starts only when all resolutions in the current level complete (ensures `seen` map is fully updated before next level).
- **Rate limiting:** GitHub API calls go through a shared rate-limiter (respects `X-RateLimit-Remaining` headers). Registry APIs use per-host semaphores.
- **Global timeout:** Configurable per-scan timeout (default 10 minutes). Context cancellation propagates to all resolvers.

### Error handling

Partial failures are expected in a depth-25 crawl. Strategy: **continue on error, mark failed nodes.**

```go
type CrawlError struct {
    DepRef   DepRef
    Depth    int
    Err      error
    Resolver DepSourceType
}
```

- **Resolver fails** (rate limit, auth failure, network error): Create a node with `Risk = CRITICAL` and `Metadata["error"] = "resolution failed: ..."`. Add to `CrawlResult.Errors`. Continue with other refs.
- **Corrupt cache entry:** Delete the entry, re-resolve from source. Log a warning.
- **Private repo / no access:** Create node with `Metadata["error"] = "access denied"`. Still appears in the tree as an unresolved dependency — visible risk.
- **SQLite locked:** Use WAL mode (already used in existing code). Concurrent reads are fine. Writes use a mutex. If still blocked, retry with backoff (3 attempts, then fail the write but continue the scan).

The final report includes an "Unresolved Dependencies" section listing all error nodes.

### BFS algorithm

The queue carries two types of items: a `FileTree` to scan fresh, or a cached `version_key` whose children are already known.

```go
type queueItem struct {
    // Exactly one of these is set:
    Contents   FileTree  // fresh scan needed
    CachedDeps []CachedChild // children already known from cache

    Depth      int
    ParentVK   string    // for edge construction
}

type CachedChild struct {
    ProjectID  string
    VersionKey string
    EdgeType   string
}
```

```
queue = [queueItem{Contents: root_dir, Depth: 0}]

while queue not empty:
    item = queue.pop()
    if item.Depth >= maxDepth: continue

    if item.Contents != nil:
        // FRESH SCAN: detect + resolve
        for each resolver:
            refs = resolver.Detect(item.Contents)
            resolve refs concurrently:
                for each ref:
                    resolved = resolver.Resolve(ref)  // may return error node
                    vk = resolved.VersionKey

                    add node + edge to graph

                    if vk in seen:
                        continue  // dedup: edge added, stop recursing

                    seen[vk] = true

                    if cached = cache.GetDeps(vk):
                        queue.enqueue(queueItem{CachedDeps: cached, Depth: item.Depth+1, ParentVK: vk})
                    else:
                        queue.enqueue(queueItem{Contents: resolved.Contents, Depth: item.Depth+1, ParentVK: vk})

    else if item.CachedDeps != nil:
        // CACHE HIT: children are known, just build graph nodes/edges
        for each child in item.CachedDeps:
            add node + edge to graph (from item.ParentVK → child)

            if child.VersionKey in seen:
                continue  // dedup

            seen[child.VersionKey] = true

            // Recurse into this child's own deps (also from cache or resolve)
            if cached = cache.GetDeps(child.VersionKey):
                queue.enqueue(queueItem{CachedDeps: cached, Depth: item.Depth+1, ParentVK: child.VersionKey})
            else:
                // Cache miss for a child — need to resolve it fresh
                resolved = resolveByKey(child.ProjectID, child.VersionKey)
                queue.enqueue(queueItem{Contents: resolved.Contents, Depth: item.Depth+1, ParentVK: child.VersionKey})
```

Key behaviors:
- Two queue item types: fresh `FileTree` to scan, or `CachedDeps` to expand. Avoids the ambiguity of mixing them.
- `seen` is updated **before** enqueuing, preventing duplicate processing of shared children.
- Cache hits skip content fetching but still recurse to build the full graph.
- Cache misses for cached children (e.g., child was pruned from cache) fall back to fresh resolution.

## Mutable Ref Risk Detection

Dependencies pinned to mutable refs (branches, movable tags) are a **critical security risk**. The dependency can change under you without any action on your part. This is a proven attack vector — the `tj-actions/changed-files` incident (2025) exploited exactly this: a tag was repointed to a malicious commit, compromising every workflow that used it without SHA pinning.

### Risk classification

| Ref type | Mutability | Risk | Example |
|---|---|---|---|
| SHA | Immutable | None | `actions/checkout@abc123def456` |
| Digest | Immutable | None | `python@sha256:abc123` |
| Exact version tag | Low mutability | Low | `v4.2.1` (rarely moved, but possible) |
| Semver range | Mutable at publish | Medium | `^4.0.0`, `~1.2.3` (new versions auto-resolve) |
| Major tag | Mutable | High | `v4` (regularly repointed to latest minor) |
| Branch | Constantly mutable | Critical | `main`, `master`, `latest` |
| Unpinned | No ref at all | Critical | `uses: actions/checkout` (no version) |

### Detection across all dependency sources

Every resolver reports the pinning quality of each ref it finds:

| Source | Immutable example | Mutable example |
|---|---|---|
| GitHub Actions | `uses: x@sha` | `uses: x@main`, `uses: x@v4` |
| Pre-commit hooks | `rev: abc123sha` | `rev: main`, `rev: v1` |
| Terraform modules | `version = "1.2.3"` | `ref = "main"` |
| Git submodules | Specific SHA in `.gitmodules` | Branch tracking |
| Docker images | `image@sha256:abc` | `image:latest` |
| Dev tools | `.tool-versions: python 3.12.1` | No pinning / `latest` |
| Package deps | Lockfile with exact versions | Manifest-only with ranges |

### Reporting

Mutable refs get prominent treatment in all outputs:

- **Web UI:** Dedicated "Mutable Refs" tab showing all non-immutable pins, sorted by risk. Red highlight on critical (branch/unpinned).
- **ASCII tree:** Mutable refs get a `⚡` marker: `[action] actions/checkout@v4 ⚡MUTABLE`
- **TUI walker:** Filter mode to show only mutable refs
- **Summary stats:** "X of Y dependencies use mutable refs (Z critical)"

### Scoring impact

Mutable refs affect the "Version Pinning" factor in the reputation score:

- SHA/Digest → 100 (full score)
- Exact version → 85
- Semver range (with lockfile) → 70
- Major tag → 40
- Branch → 20
- Unpinned → 0

For actions and pre-commit hooks specifically (where tag-repointing attacks are proven), major tag pinning gets an additional risk flag beyond the score penalty — it's called out as a specific actionable finding: "Pin to SHA `X` instead of tag `vN`."

## CVE Integration

Post-processing pass after the crawler finishes, not part of the BFS loop.

### Three scenarios

**Registry packages** — direct lookup:
- Have `ecosystem/name@version` → query OSV/NVD directly

**Git-pinned deps with semver tags** — SHA → tag bridge:
- Look up git tags pointing at the SHA via GitHub API
- If tag matches semver → store in `project_versions.semver`
- Query CVE databases with repo identity + semver

**Git-pinned deps without semver** — unchecked:
- Mark as `cve_status = "unchecked"` in metadata
- Flag in report as "no CVE coverage — no semver mapping"

### Pipeline

1. Crawler builds full graph
2. Walk all nodes with a semver → batch query OSV
3. Attach CVE findings to node metadata + update score
4. Cache CVE results in `cve_cache` table (6h TTL)

## Company/Org Detection & Trust

### Configuration

```yaml
trusted_orgs:
  - github.com/my-company
  - github.com/my-other-org
  - gitlab.com/my-team
```

### Own org detection

When resolving a dependency's `ProjectID`, check if it matches any `trusted_orgs` prefix. If yes → `org_type = "own"` → risk score gets a floor of 80 (configurable). Applies transitively.

### Corporate org detection

- GitHub API returns org type (`Organization` vs `User`)
- Heuristic: org with 10+ public repos, 5+ members, company field set → `org_type = "corporate"`
- Individual maintainer → `org_type = "individual"`
- Hardcoded seed list for major orgs (google, microsoft, hashicorp, etc.) as fallback

### Scoring impact

- `own` → score floor of 80 (configurable)
- `corporate` → boost to "Organization Backing" factor (existing scoring)
- `individual` → no change
- `unknown` → no change

Stored on `projects` table — computed once per project, reused across versions and scans.

## Discover Command

With the new cache, `discover` becomes a cache query for previously scanned projects:

```
depscope discover lodash --range "<4.17.21"
```

1. Query `version_dependencies` table: `SELECT DISTINCT parent_project_id FROM version_dependencies WHERE child_project_id = 'npm/lodash'` — fast indexed lookup
2. Join with `project_versions` to get the child's semver, check if it falls in vulnerable range
3. Walk the `version_dependencies` edges backward to reconstruct the full dependency path from root to the affected package

Falls back to file-walk + parse for unscanned projects, populating the cache as it goes.

## Output & Visualization

### Web UI (primary)

Enhanced D3 graph:
- Force-directed layout with clustering by node type
- Semantic zoom: clusters as colored bubbles (zoomed out) → individual nodes (zoomed in)
- Filter panel: toggle node types, filter by risk level, org type, search by name
- Node detail sidebar: project metadata, version info, CVEs, pinning, score breakdown, children
- Depth slider: control render depth (start at 3, slide deeper)
- Highlight paths: click leaf → highlight path to root

### TUI tree walker (secondary)

Interactive directory-browser navigation:
- Start at root, show direct children with type badges and score bars
- Enter to drill into children, Backspace to go up
- `i` for info panel, `/` to search, `f` to filter
- Color-coded by risk level

### ASCII tree command

```
depscope tree [PATH] [--depth N] [--type action,package,...] [--risk high,critical]
```

Full Unicode tree output showing the entire dependency chain:
- `--depth N` to limit depth
- `--type` to filter node types
- `--risk` to show only nodes at/above risk threshold
- `--collapse N` to auto-collapse deep subtrees
- `--json` for machine-readable output
- Piping support: `depscope tree > deps.txt`

Summary line at bottom: total nodes, risk counts, max depth reached.

## Migration

This is a **replace** of the existing scanning infrastructure:

1. New `internal/crawler/` package with `Crawler`, `Resolver` interface, BFS engine, `CrawlResult` output type
2. New `internal/cache/db.go` SQLite cache with `projects`, `project_versions`, `version_dependencies`, `cve_cache` tables. Replaces `internal/cache/cache.go` disk cache entirely (clean break, no migration)
3. New resolver packages: `internal/crawler/precommit/`, `internal/crawler/terraform/`, `internal/crawler/submodule/`, `internal/crawler/tool/`, `internal/crawler/buildtool/`
4. Existing code wrapped as resolvers: `PackageResolver` wraps `internal/manifest/` + `internal/registry/`, `ActionResolver` wraps `internal/actions/`
5. `scanner.ScanDir` and `scanner.ScanURL` refactored to use `Crawler`. They now return `CrawlResult`, with a post-processing step that builds `core.ScanResult` for backward compatibility with report formatters
6. `graph.Node` struct extended with `ProjectID` and `VersionKey` fields (breaking change — all Node construction sites updated)
7. `PinningSemverRange` added to `PinningQuality` enum
8. `config.Config` extended with `TrustedOrgs []string`, `Auth` (multi-host tokens), `Concurrency` (per-resolver limits)
9. `discover` command queries `version_dependencies` table first, falls back to walk
10. Graph types extended with 5 new node types and 5 new edge types
11. Web UI reworked for larger graphs with clustering, filtering, depth control
12. TUI tree walker added alongside existing views
13. New `depscope tree` subcommand for full ASCII/Unicode tree output
14. `depscope cache prune` subcommand for cache size management
