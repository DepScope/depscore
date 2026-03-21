# Remote Resolver — Fetch Manifests Without Cloning

**Date:** 2026-03-21
**Status:** Approved
**Parent spec:** [depscope design spec](2026-03-20-depscope-design.md)

---

## Overview

When scanning a remote repository (via CLI argument or the `POST /api/scan` server endpoint), depscope should **not** clone the full repo. Instead, it uses host-specific APIs to list the file tree and fetch only the manifest/lockfiles it needs. This makes remote scans fast and lightweight.

For hosts without a smart-fetch integration, it falls back to a shallow clone.

---

## Resolver Interface

A new `internal/resolve/` package provides a common interface for fetching manifest files from remote sources.

```go
// internal/resolve/resolver.go

type ManifestFile struct {
    Path    string // e.g. "services/api/go.mod"
    Content []byte
}

type Resolver interface {
    // Resolve fetches manifest files from a remote source.
    // Returns the files and a cleanup func (no-op for API-based, rmdir for clone-based).
    Resolve(ctx context.Context, url string) ([]ManifestFile, func(), error)
}
```

All resolvers return `[]ManifestFile` with file content already loaded. The caller invokes the cleanup func via `defer`.

---

## URL Dispatch

`internal/resolve/detect.go` parses the URL and selects the appropriate resolver:

| URL pattern | Resolver |
|---|---|
| `github.com/*` | `GitHubResolver` — Trees API |
| `gitlab.com/*` | `GitLabResolver` — Repository Tree API |
| Everything else (`git@`, other HTTPS hosts) | `GitCloneResolver` — shallow clone |

Detection: if the scan argument starts with `http://`, `https://`, `ssh://`, or `git@`, it's a URL and dispatched to a resolver. Otherwise, it's treated as a local path (existing behavior).

### Ref Resolution

- GitHub: extract everything after `/tree/` in the URL path as the ref. Validate by passing it to the Trees API — if it 404s, fall back to the default branch and warn.
- GitLab: extract ref from `/-/tree/{ref}` path segment.
- If no ref is present in the URL, use the repo's default branch (fetched via the repo metadata API).
- Bare URLs like `github.com/owner/repo` always use the default branch.

---

## File Filtering

All resolvers apply two filters when identifying manifest files:

### 1. Ignored Directories

Skip any path under these directories:

- `node_modules/`
- `vendor/`
- `target/`
- `.git/`
- `__pycache__/`
- `dist/`
- `build/`

### 2. Known Manifest Filenames

Only fetch files matching these names (at any depth in the tree):

| Ecosystem | Filenames |
|---|---|
| Go | `go.mod`, `go.sum` |
| Python | `requirements.txt`, `pyproject.toml`, `poetry.lock`, `uv.lock` |
| Rust | `Cargo.toml`, `Cargo.lock` |
| JavaScript | `package.json`, `package-lock.json`, `pnpm-lock.yaml`, `bun.lock` |

Both filters are shared constants used by all three resolvers.

---

## GitHub Resolver

`internal/resolve/github.go`

### Flow

1. Parse `owner` and `repo` from URL. Extract ref if present.
2. Call `GET /repos/{owner}/{repo}/git/trees/{ref}?recursive=1` — returns the full file tree in a single API call. If the response has `"truncated": true` (repos with >100k entries), log a warning that some manifests may be missed and proceed with the partial tree.
3. Filter the tree entries: skip ignored directories, match known manifest filenames.
4. For each matching entry, call `GET /repos/{owner}/{repo}/contents/{path}?ref={ref}` — returns base64-encoded file content.
5. Decode content, return `[]ManifestFile`. Cleanup func is no-op.

### Authentication

Uses the same `GITHUB_TOKEN` env var already required for VCS scoring. Authenticated rate limit: 5,000 req/hr. A typical resolve uses 1 tree call + 2–6 contents calls — negligible impact.

If no token is set, unauthenticated rate limit (60 req/hr) applies. A warning is emitted, same as the existing VCS scoring behavior.

---

## GitLab Resolver

`internal/resolve/gitlab.go`

### Flow

1. Parse project path from URL (supports `gitlab.com/group/subgroup/project` format).
2. Call `GET /api/v4/projects/{url-encoded-path}/repository/tree?recursive=true&ref={ref}&per_page=100`. Paginate using the `page` query parameter (GitLab's default offset pagination). Cap at 10 pages (1,000 entries) to prevent runaway requests on massive repos; log a warning if the cap is hit.
3. Filter tree entries with the same ignore-dirs + manifest-filename filters.
4. For each match, call `GET /api/v4/projects/{id}/repository/files/{url-encoded-path}/raw?ref={ref}`.
5. Return `[]ManifestFile`. Cleanup func is no-op.

### Authentication

Uses `GITLAB_TOKEN` env var. Required for private repos. Public repos work without a token but are subject to lower rate limits.

---

## Git Clone Fallback

`internal/resolve/gitclone.go`

### Flow

1. Create a temp directory via `os.MkdirTemp("", "depscope-clone-*")`.
2. Run `git clone --depth=1 {url} {tmpdir}` via `exec.CommandContext(ctx, ...)` so the clone respects context cancellation and timeouts. The scan command sets a 60-second timeout on the clone context.
3. Walk the temp directory, applying the same ignore-dirs + manifest-filename filters.
4. Read matching files into `[]ManifestFile`.
5. Return cleanup func that calls `os.RemoveAll(tmpdir)`.

### Error Handling

- If `git` is not on PATH, return a clear error: "git is required for scanning non-GitHub/GitLab URLs".
- If clone fails (auth, network, invalid URL), return the git error message.
- Cleanup func is always returned (even on error) and is safe to call on an empty/partial temp dir.

### Signal Handling

The scan command registers a `signal.Notify` handler for SIGINT/SIGTERM that calls the cleanup func before exiting. Go's `defer` does **not** run on signal-based termination, so explicit signal handling is required. If cleanup fails or is skipped (e.g., SIGKILL), orphaned temp dirs use the `depscope-clone-*` prefix so they can be identified and cleaned up manually.

---

## CLI Integration

The `depscope scan` command accepts either a local path or a URL:

```
depscope scan .                                    # local directory
depscope scan https://github.com/psf/requests      # GitHub smart fetch
depscope scan https://gitlab.com/org/project        # GitLab smart fetch
depscope scan https://bitbucket.org/org/repo        # clone fallback
depscope scan git@custom-host.com:org/repo.git      # clone fallback
```

### Scan Pipeline Integration

The scan command has two paths depending on input type:

**Local path (existing):** `DetectEcosystem(dir)` → `ParserFor(ecosystem)` → `parser.Parse(dir)` → scoring pipeline.

**Remote URL (new):**
1. `DetectResolver(url)` → resolver
2. `resolver.Resolve(ctx, url)` → `[]ManifestFile`
3. Group `ManifestFile`s by directory (e.g., `services/api/go.mod` and `services/api/go.sum` belong together)
4. For each group, detect ecosystem from the filenames present (same logic as `DetectEcosystem`, but operating on filename strings instead of stat calls)
5. Call `parser.ParseFiles(files map[string][]byte)` — a new method on the `Parser` interface that accepts a map of `filename → content`
6. The rest of the pipeline (registry fetch, scoring, propagation, reporting) is unchanged

**Parser interface change:**

```go
type Parser interface {
    Parse(dir string) ([]Package, error)             // local: reads from disk
    ParseFiles(files map[string][]byte) ([]Package, error) // remote: reads from memory
    Ecosystem() Ecosystem
}
```

`Parse(dir)` becomes a thin wrapper: read the relevant files from disk into a `map[string][]byte`, then call `ParseFiles`. This keeps the parsing logic in one place.

**Multi-ecosystem repos:** A remote repo may contain manifests for multiple ecosystems (e.g., `go.mod` at root + `frontend/package.json`). Each group is parsed independently and the results are merged into a single scan, same as local scanning of a monorepo.

---

## Caching

Resolved manifests are **not** cached. They are cheap to fetch (a few small API calls) and should always reflect the current state of the repo. Downstream package scores (registry, VCS, vuln data) continue to use the existing disk cache.

---

## Testing

- **GitHub/GitLab resolvers:** Golden-file tests using `httptest.Server`, same pattern as the existing registry and VCS clients. Captured API responses in `internal/resolve/testdata/`.
- **Clone fallback:** Test with a local bare git repo created in `TestMain`, avoiding network calls.
- **URL detection:** Table-driven tests for the dispatch logic in `detect.go`.
- **File filtering:** Unit tests for the ignore-dir and manifest-name filters.

---

## File Map

```
internal/resolve/
├── resolver.go          # ManifestFile, Resolver interface, shared filter constants
├── detect.go            # DetectResolver(url) → Resolver dispatch
├── detect_test.go
├── filters.go           # IgnoredDirs, ManifestFilenames, MatchesManifest(), IsIgnoredDir()
├── filters_test.go
├── github.go            # GitHubResolver
├── github_test.go
├── gitlab.go            # GitLabResolver
├── gitlab_test.go
├── gitclone.go          # GitCloneResolver
├── gitclone_test.go
└── testdata/
    ├── github/          # golden tree + contents responses
    └── gitlab/          # golden tree + file responses
```
