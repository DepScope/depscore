# Discovery Command Design Spec

## Problem

When a package is compromised or a CVE drops, teams need to quickly answer: "Which of our projects are affected?" Today, depscope scans one project at a time for reputation scoring — there's no way to search across many projects for a specific package at a specific version range.

## Solution

A new `depscope discover` command that walks multiple projects (via filesystem or project list), finds all occurrences of a target package, and classifies each project's exposure against a compromised version range.

## Command Interface

```bash
# Basic — find projects affected by compromised litellm versions
depscope discover litellm --range ">=1.82.7,<1.83.0" /home/me/repos

# From a project list file (paths or URLs, one per line)
depscope discover litellm --range ">=1.82.7,<1.83.0" --list projects.txt

# Also check what version would be installed today
depscope discover litellm --range ">=1.82.7,<1.83.0" --resolve /home/me/repos

# Air-gapped, no network calls
depscope discover litellm --range ">=1.82.7,<1.83.0" --offline /home/me/repos

# JSON output
depscope discover litellm --range ">=1.82.7,<1.83.0" --output json /home/me/repos
```

### Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `package` | Yes | Package name to search for (first positional arg) |
| `path` | No | Start path for filesystem walk (defaults to `.`, last positional arg) |

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--range` | (required) | Compromised version range (semver or PEP 440 syntax) |
| `--list` | - | Path to file containing project paths or URLs (one per line). Used instead of filesystem walk |
| `--resolve` | false | Additionally check current installable version via registry for unresolved constraints |
| `--offline` | false | No network calls. Warns that transitive coverage limited to projects with lockfiles |
| `--output` | `text` | Output format: `text` or `json` |
| `--ecosystem` | - | Filter to specific ecosystem. Accepts: `python`, `npm`, `rust`, `go`, `php` (matching internal `Ecosystem` type constants) |
| `--max-depth` | 10 | Maximum directory depth for filesystem walk |

## Output Classification

Results are classified into four buckets, displayed in priority order:

### CONFIRMED AFFECTED

Lockfile pins a version within the compromised range, or registry resolution confirmed an affected transitive dependency.

```
🔴 CONFIRMED AFFECTED (2 projects)
  /home/me/repos/api-service
    Source: uv.lock
    Installed: litellm 1.82.8
    Depth: transitive (via langchain → litellm)

  /home/me/repos/ml-pipeline
    Source: poetry.lock
    Installed: litellm 1.82.7
    Depth: direct
```

### POTENTIALLY AFFECTED

Manifest constraint allows compromised versions but no lockfile confirms the exact installed version.

```
🟡 POTENTIALLY AFFECTED (1 project)
  /home/me/repos/experiments
    Source: pyproject.toml
    Constraint: litellm >= 1.80
    Reason: constraint allows compromised versions, no lockfile found
    Resolve: litellm 1.82.8 would be installed today (--resolve)
```

### UNRESOLVABLE

Cannot determine exposure — unpinned constraint, no lockfile, and offline mode or registry failure.

```
🔵 UNRESOLVABLE (1 project)
  /home/me/repos/old-tool
    Source: requirements.txt
    Constraint: litellm
    Reason: unpinned, no lockfile, use --resolve for transitive check
```

### SAFE

Package found but version is outside the compromised range.

```
🟢 SAFE (3 projects)
  /home/me/repos/chatbot         litellm 1.83.1  (uv.lock, direct)
  /home/me/repos/data-service    litellm 1.81.0  (poetry.lock, transitive)
  /home/me/repos/web-app         litellm ~=1.84  (pyproject.toml, constraint excludes range)
```

### JSON Output

```json
{
  "package": "litellm",
  "range": ">=1.82.7,<1.83.0",
  "results": [
    {
      "status": "confirmed",
      "project": "/home/me/repos/api-service",
      "source": "uv.lock",
      "version": "1.82.8",
      "depth": "transitive",
      "dependency_path": ["langchain", "litellm"]
    }
  ],
  "summary": {
    "confirmed": 2,
    "potentially": 1,
    "unresolvable": 1,
    "safe": 3,
    "total": 7
  }
}
```

## Architecture

### Two-Phase Pipeline

**Phase 1: Fast Discovery**

1. Enumerate project roots via filesystem walk or project list file
2. For filesystem walk: find directories containing manifest/lockfile files, skip ignored dirs (`node_modules`, `vendor`, `target`, `.git`, `__pycache__`, `dist`, `build`)
3. Text search (`strings.Contains`) each manifest/lockfile for the package name
4. Collect matched projects and matched files; skip non-matches immediately

**Phase 2: Precise Classification**

For each matched project:

1. **Lockfile exists:** Parse lockfile using existing `internal/manifest` parsers. Extract resolved version and dependency path. Classify with `VersionInRange(resolved, compromisedRange)`.

2. **No lockfile, default mode:** Resolve transitive dependency tree via registry APIs. Each ecosystem exposes dependency data differently — PyPI via `requires_dist` in JSON API, npm via `dependencies` in package metadata, crates.io via `/crates/{name}/{version}/dependencies`, Go proxy via `go.mod` at `/{module}/@v/{version}.mod`, Packagist via `require` in package metadata. New `FetchDependencies(name, version)` methods must be added to each registry client to support this. Walk the tree recursively until the target package is found or the tree is exhausted. If found, extract version and classify. If not found, mark SAFE.

3. **No lockfile, `--offline` mode:** Parse manifest constraint only. If constraint can be evaluated for overlap with compromised range, classify as POTENTIALLY or SAFE. Otherwise, UNRESOLVABLE.

4. **`--resolve` flag (additive):** For manifest-only constraints, also query registry for current installable version and include in output.

### New Package: `internal/discover`

| File | Responsibility |
|------|---------------|
| `types.go` | `DiscoverResult`, `ProjectMatch`, `Status` enum (confirmed/potentially/unresolvable/safe) |
| `discover.go` | Orchestrator: accepts config, runs both phases, returns classified results |
| `walker.go` | Filesystem walker + project list reader. Reuses ignore patterns from `internal/resolve/filters.go` |
| `matcher.go` | Phase 1 text search across manifest/lockfile files |
| `classifier.go` | Phase 2 classification logic: version range comparison + bucket assignment |
| `version.go` | `VersionInRange()`, `ConstraintOverlaps()`, range parsing. Supports semver and PEP 440 |
| `resolve.go` | Transitive tree resolution for lockfile-less projects. Uses new `FetchDependencies` methods on registry clients to walk dependency trees per-ecosystem |

### New CLI Command: `cmd/depscope/discover_cmd.go`

Cobra command definition. Flag parsing, validation, calls into `internal/discover`. Output formatting reuses/extends `internal/report`.

### Reused Existing Code

| Package | What's reused |
|---------|--------------|
| `internal/manifest` | All ecosystem parsers for phase 2 lockfile/manifest parsing |
| `internal/registry` | Registry clients for `--resolve` checks. Extended with new `FetchDependencies()` method per ecosystem for transitive tree resolution |
| `internal/resolve/filters.go` | `MatchesManifest()`, ignored directory list |
| `internal/cache` | Disk-backed TTL cache for registry and CVE data |
| `internal/report` | Extended with discover-specific text and JSON formatters |

## Version Range Matching

### Input Format

Standard constraint syntax per ecosystem:
- `>=1.82.7,<1.83.0` — range
- `==1.82.8` — exact version
- `>=1.82.7` — open-ended
- `<2.0.0` — upper bound

### Classification Logic

| Project State | Logic | Result |
|---------------|-------|--------|
| Lockfile with resolved version | `VersionInRange(resolved, compromised)` → true | CONFIRMED |
| Lockfile with resolved version | `VersionInRange(resolved, compromised)` → false | SAFE |
| Manifest constraint, no lockfile, range evaluable | Constraint overlaps compromised range | POTENTIALLY |
| Manifest constraint, no lockfile, no overlap | Constraint excludes compromised range | SAFE |
| Manifest constraint, no lockfile, `--resolve` | Resolve current installable → `VersionInRange()` → if in range: CONFIRMED; if not in range but constraint allows it: POTENTIALLY |
| Transitive dep found via registry resolution | `VersionInRange(resolved, compromised)` → true | CONFIRMED |
| Transitive dep found via registry resolution | `VersionInRange(resolved, compromised)` → false | SAFE |
| Unpinned constraint, no lockfile, `--offline` | Cannot determine | UNRESOLVABLE |

### Constraint Overlap

For "potentially affected" classification: does the manifest constraint allow any version in the compromised range? This is interval intersection on version ranges. E.g., `>=1.80` overlaps `>=1.82.7,<1.83.0` but `>=1.84` does not.

## Three Operating Modes

| Mode | Network | Transitive coverage | Use case |
|------|---------|-------------------|----------|
| **Default** | Yes (for projects without lockfiles) | Full: lockfile tree + registry resolution | Standard incident response |
| **`--resolve`** | Yes (extended) | Full + checks current installable version | Higher confidence on "potentially affected" |
| **`--offline`** | None | Lockfile-only (warns about limitations) | Air-gapped environments |

## Error Handling

- **Permission denied on directories:** Log warning, continue walking
- **Symlink loops:** Don't follow symlinks
- **Invalid paths in `--list`:** Log warning, include as error status, continue
- **URLs in `--list`:** Use existing remote resolvers (GitHub/GitLab API, git clone)
- **Unparseable versions:** Classify as UNRESOLVABLE with reason
- **Pre-release versions:** Include in range matching (PEP 440 and semver define ordering)
- **Multiple version specs in one project:** Report all, flag conflicts
- **Network failures during resolution:** Degrade per-project to UNRESOLVABLE with reason, continue
- **Rate limiting:** Respect existing concurrency settings, reuse cache
- **Package not in registry:** SAFE (can't be installed from this registry)
- **Empty/comment lines in `--list`:** Skip (`#` prefix for comments)

## Testing Strategy

### Unit Tests (`internal/discover/*_test.go`)

- `walker_test.go` — filesystem walk with nested projects, ignore dirs, symlinks, permission errors
- `matcher_test.go` — text search against fixture manifest/lockfile files for various ecosystems
- `classifier_test.go` — classification for each combination of lockfile presence, constraint type, and mode flags
- `version_test.go` — range matching: semver and PEP 440, overlap detection, pre-release, open-ended ranges, unparseable input
- `resolve_test.go` — transitive tree resolution with mocked registry clients

### Integration Tests

- Temp directory tree with multiple projects containing different manifest/lockfile combinations
- Full discover pipeline execution
- Assert correct classification across all four buckets
- Test all three modes: default, `--resolve`, `--offline`

### CLI Tests (`cmd/depscope/discover_test.go`)

- Flag parsing and validation (missing `--range`, invalid range syntax)
- `--list` file parsing (valid paths, invalid paths, URLs, comments)
- Output format switching (text vs json)

### Fixtures

- Minimal real-world manifest/lockfile snippets per ecosystem with target package at various versions
- Reuse existing test fixtures from `internal/manifest` where applicable
