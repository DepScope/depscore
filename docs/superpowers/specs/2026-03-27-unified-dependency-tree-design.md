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

SQLite database replacing the current flat disk cache (`~/.cache/depscope/`).

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
    dependencies TEXT,             -- JSON array of child version_keys
    dep_types   TEXT,              -- JSON: what was found (manifests, actions, hooks, etc.)
    scanned_at  TIMESTAMP NOT NULL,
    PRIMARY KEY (project_id, version_key)
);
```

**TTL: never expires.** A SHA is immutable. `npm/lodash@4.17.21` always has the same deps.

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

Every node links back to the cache:

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
    ProjectID  string         // FK → projects.id
    VersionKey string         // FK → project_versions.version_key
}
```

## Crawler & Resolver Architecture

### Crawler

Central BFS engine with dedup. Replaces `scanner.ScanDir` / `scanner.ScanURL`.

```go
type Crawler struct {
    cache     *CacheDB
    resolvers map[DepSourceType]Resolver
    graph     *graph.Graph
    seen      map[string]bool  // version_keys seen in THIS scan
    maxDepth  int              // default 25
    ownOrgs   []string         // configured trusted orgs
}
```

### Resolver interface

Each dependency source implements:

```go
type Resolver interface {
    Detect(ctx context.Context, contents FileTree) ([]DepRef, error)
    Resolve(ctx context.Context, ref DepRef) (*ResolvedDep, error)
}

type DepRef struct {
    Source    DepSourceType  // package, action, precommit, terraform, submodule, tool, script
    Name     string
    Ref      string
    Ecosystem string
}

type ResolvedDep struct {
    ProjectID  string
    VersionKey string
    Semver     string       // for CVE lookups, nullable
    Contents   FileTree     // for recursive scanning
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

### BFS algorithm

```
queue = [(root_dir_contents, depth=0)]
while queue not empty:
    contents, depth = queue.pop()
    if depth >= maxDepth: continue

    for each resolver:
        refs = resolver.Detect(contents)
        for each ref:
            resolved = resolver.Resolve(ref)
            vk = resolved.VersionKey

            add node + edge to graph (linked to ProjectID, VersionKey)

            if vk in seen:
                continue  // already in this scan's tree → edge added, stop

            seen[vk] = true

            if cached_deps = cache.GetDeps(vk):
                // cache knows the children — enqueue them for graph building
                // but skip re-fetching contents
                for each child_vk in cached_deps:
                    queue.enqueue(child_vk, depth+1)
            else:
                // scan resolved contents for its own deps
                queue.enqueue(resolved.Contents, depth+1)
                cache.StoreDeps(vk, discovered_children)
```

Key: when cache has the dependency list, we skip re-fetching/re-parsing but still walk children to build this scan's graph edges.

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

1. Query `project_versions` for entries whose `dependencies` JSON contains `npm/lodash@*`
2. Check if matched semver falls in vulnerable range
3. Return affected projects with full dependency path

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

1. New `internal/crawler/` package with `Crawler`, `Resolver` interface, BFS engine
2. New `internal/cache/db.go` SQLite cache (replaces `internal/cache/cache.go` disk cache)
3. New resolver packages: `internal/crawler/precommit/`, `internal/crawler/terraform/`, `internal/crawler/submodule/`, `internal/crawler/tool/`, `internal/crawler/buildtool/`
4. Existing code wrapped as resolvers: `PackageResolver` wraps `internal/manifest/` + `internal/registry/`, `ActionResolver` wraps `internal/actions/`
5. `scanner.ScanDir` and `scanner.ScanURL` refactored to use `Crawler`
6. `discover` command queries cache first, falls back to walk
7. Graph types extended with new node/edge types
8. Web UI reworked for larger graphs
9. TUI tree walker added alongside existing views
10. New `depscope tree` subcommand
