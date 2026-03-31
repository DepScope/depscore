# Supply Chain Indexer — Phase 1 Design Spec

## Goal

Add a `depscope index` command that walks an entire directory tree (home folder, computer, any path), discovers every package manifest across all ecosystems, parses dependencies, deduplicates globally, and stores everything in SQLite for fast querying. Supports incremental re-indexing via mtime tracking.

## Command Interface

```
depscope index <path>                          # incremental index (default: local scope)
depscope index <path> --force                  # full rescan, ignore mtime cache
depscope index <path> --scope local            # parse disk only (default)
depscope index <path> --scope deps             # + resolve transitive deps from lockfiles
depscope index <path> --scope supply-chain     # + network: repos, actions, CVEs
depscope index status                          # show index statistics
depscope index status --ecosystem npm          # filter stats by ecosystem
```

**Flags:**
- `--force` — ignore mtime cache, re-parse all manifests
- `--scope` — indexing depth: `local` (default), `deps`, `supply-chain`
- `--db` — SQLite path (default: `~/.cache/depscope/depscope-cache.db`)

## Architecture

### Walk & Discover

Walk from `<path>` recursively. Include hidden directories (`.vscode`, `.config`, `.tools`, etc.). Skip only `.git` directories. Discover every recognized manifest file:

**Ecosystem manifest files:**
- **npm**: `package.json`, `package-lock.json`, `pnpm-lock.yaml`, `bun.lock`
- **Python**: `pyproject.toml`, `requirements.txt`, `poetry.lock`, `uv.lock`
- **Go**: `go.mod`, `go.sum`
- **Rust**: `Cargo.toml`, `Cargo.lock`
- **PHP**: `composer.json`, `composer.lock`
- **Actions**: `.github/workflows/*.yml`
- **Config**: `.pre-commit-config.yaml`, `.gitmodules`, `.tool-versions`, `.mise.toml`
- **Terraform**: `*.tf`
- **Build**: `Makefile`, `Taskfile.yml`, `Taskfile.yaml`, `justfile`

Unlike the existing `buildFileTree` in `crawl.go`, the indexer does NOT skip `node_modules` or `vendor` — it walks into them to catalog every installed package.

### Incremental Indexing

For each discovered manifest:
1. Compute absolute path
2. Check `index_manifests` table for existing entry
3. Compare file mtime (and optionally sha256 checksum)
4. If unchanged → skip, print `[skip]`
5. If changed or new → parse, upsert packages, update mtime/checksum
6. `--force` bypasses the mtime check entirely

Deleted manifests (in DB but no longer on disk) are detected at the end of a run and pruned from the index along with orphaned `manifest_packages` links.

### Parse & Dedup

Use existing ecosystem parsers (`manifest.ParserFor(eco).ParseFiles(files)`) for each manifest. For `node_modules/*/package.json`, parse directly for name + version (no lockfile needed — the installed version IS the resolved version).

**Global dedup:** Each unique `project_id` (e.g., `npm/axios`) gets one row in `projects`. Each unique `version_key` (e.g., `npm/axios@1.14.1`) gets one row in `project_versions`. The `manifest_packages` junction table links a manifest to its packages. If 50 projects use `axios@1.14.1`, there's one `project_versions` row and 50 `manifest_packages` rows.

### Scope Levels

**`local` (default):**
- Parse manifests on disk only
- No network calls
- Fast: hundreds of manifests per second
- Sufficient for compromised package search

**`deps`:**
- Additionally resolve transitive dependencies from lockfiles
- Uses existing BFS crawler in offline mode (no network)
- Populates `version_dependencies` for the full transitive graph
- Enables "who transitively depends on X?" queries

**`supply-chain`:**
- Additionally follow GitHub/registry APIs
- Resolve repos, Actions dependencies, score packages, fetch CVEs
- Uses existing BFS crawler with full network access
- Slowest but richest data (rate-limited by GitHub API)

### SQLite Schema Additions

Added to the existing `CacheDB.migrate()`:

```sql
CREATE TABLE IF NOT EXISTS index_manifests (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    abs_path     TEXT UNIQUE NOT NULL,
    rel_path     TEXT NOT NULL,
    root_path    TEXT NOT NULL,
    ecosystem    TEXT NOT NULL,
    mtime        INTEGER NOT NULL,
    checksum     TEXT NOT NULL DEFAULT '',
    last_indexed DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_manifests_root ON index_manifests(root_path);
CREATE INDEX IF NOT EXISTS idx_manifests_eco ON index_manifests(ecosystem);

CREATE TABLE IF NOT EXISTS manifest_packages (
    manifest_id  INTEGER NOT NULL,
    project_id   TEXT NOT NULL,
    version_key  TEXT NOT NULL,
    constraint_  TEXT NOT NULL DEFAULT '',
    dep_scope    TEXT NOT NULL DEFAULT 'direct',
    UNIQUE(manifest_id, project_id, version_key),
    FOREIGN KEY (manifest_id) REFERENCES index_manifests(id) ON DELETE CASCADE,
    FOREIGN KEY (project_id) REFERENCES projects(id)
);

CREATE INDEX IF NOT EXISTS idx_mp_project ON manifest_packages(project_id);
CREATE INDEX IF NOT EXISTS idx_mp_version ON manifest_packages(version_key);

CREATE TABLE IF NOT EXISTS index_runs (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    root_path         TEXT NOT NULL,
    scope             TEXT NOT NULL,
    started_at        DATETIME NOT NULL,
    finished_at       DATETIME,
    manifests_found   INTEGER NOT NULL DEFAULT 0,
    manifests_updated INTEGER NOT NULL DEFAULT 0,
    manifests_skipped INTEGER NOT NULL DEFAULT 0,
    packages_total    INTEGER NOT NULL DEFAULT 0,
    packages_new      INTEGER NOT NULL DEFAULT 0,
    errors            INTEGER NOT NULL DEFAULT 0
);
```

### CacheDB Methods

```go
// Index manifests
UpsertIndexManifest(m *IndexManifest) error
GetIndexManifest(absPath string) (*IndexManifest, error)
ListIndexManifests(rootPath string) ([]IndexManifest, error)
DeleteIndexManifest(absPath string) error
PruneDeletedManifests(rootPath string, currentPaths map[string]bool) (int, error)

// Manifest-package links
AddManifestPackage(mp *ManifestPackage) error
GetManifestPackages(manifestID int64) ([]ManifestPackage, error)
FindPackageManifests(projectID string) ([]ManifestPackage, error)  // reverse lookup
ClearManifestPackages(manifestID int64) error  // before re-indexing a manifest

// Index runs
StartIndexRun(rootPath, scope string) (*IndexRun, error)
FinishIndexRun(run *IndexRun) error
GetLastIndexRun(rootPath string) (*IndexRun, error)
GetIndexRuns(rootPath string, limit int) ([]IndexRun, error)

// Statistics
IndexStats(rootPath string) (*IndexStatus, error)  // counts by ecosystem, top packages
```

### Output Format

Real-time progress during indexing:
```
Indexing /Users/jj ...
  [npm]     src/webapp/package.json                      87 packages (3 new)
  [npm]     src/webapp/node_modules/axios/package.json   12 packages (0 new)
  [python]  src/ml-service/pyproject.toml                34 packages (34 new)
  [go]      src/api/go.mod                               45 packages (12 new)
  [skip]    src/old-project/package.json                 (unchanged)
  ...

Done: 342 manifests scanned, 2,847 packages indexed (1,203 new), 4 errors
Index: /Users/jj/.cache/depscope/depscope-cache.db
```

Status subcommand:
```
Index: /Users/jj/.cache/depscope/depscope-cache.db
Last run: 2026-03-31 10:15:00 (scope: local)
Root: /Users/jj

Manifests:  342
Packages:   2,847 unique
  npm:      1,832
  python:     456
  go:         312
  rust:       198
  php:         49

Top packages: lodash (127 manifests), axios (89), express (67)
```

### Error Handling

- **Unreadable files/dirs**: skip, count as error, continue. A home folder will have permission-denied paths.
- **Malformed manifests**: skip, count as error, continue. Log the path.
- **DB errors**: fatal. Can't index without storage.
- **Network errors** (supply-chain scope): warn, fall back to local data for that package. Don't block the entire index.

### File Structure

| File | Responsibility |
|------|---------------|
| `internal/cache/index.go` | IndexManifest, ManifestPackage, IndexRun types + table migration + CRUD methods |
| `internal/cache/index_test.go` | Unit tests for index DB operations |
| `internal/scanner/indexer.go` | Core indexer: walk, discover, incremental mtime check, parse, dedup, store, progress |
| `internal/scanner/indexer_test.go` | Unit + integration tests for indexer |
| `cmd/depscope/index_cmd.go` | CLI: `depscope index <path>`, `depscope index status`, flags |

### node_modules Handling

When the walker encounters `node_modules/*/package.json`, it parses the package name and version directly from the JSON (the `name` and `version` fields). No lockfile resolution needed — the installed version is the truth. This handles:
- Top-level: `node_modules/axios/package.json` → `axios@1.14.1`
- Scoped: `node_modules/@scope/pkg/package.json` → `@scope/pkg@2.0.0`
- Nested: `node_modules/foo/node_modules/bar/package.json` → `bar@3.0.0`

The parent manifest (the project's own `package.json`) is linked to these via `manifest_packages`. The dependency scope is determined by whether the package appears in `dependencies`, `devDependencies`, or only transitively.

### Future Integration (Phase 2 & 3)

- **Phase 2**: `depscope compromised --from-index` queries `manifest_packages` joined to `projects` — instant results without filesystem walking
- **Phase 3**: Web UI adds `/api/index/stats` and `/api/index/search` endpoints; statistics over time from `index_runs`
- **Statistics**: ecosystem distribution trends, new packages per run, most-duplicated packages, packages with no lockfile pinning

### Testing Strategy

1. **DB operations** (`cache/index_test.go`): CRUD for all three new tables, dedup behavior, cascade deletes, pruning
2. **Indexer unit tests** (`scanner/indexer_test.go`): fixture directories with multiple ecosystems, incremental skip behavior, node_modules parsing, force mode
3. **Integration test**: multi-ecosystem directory with hidden dirs, node_modules, verify correct counts and dedup
