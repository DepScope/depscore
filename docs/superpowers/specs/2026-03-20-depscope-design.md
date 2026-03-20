# depscope — Supply Chain Reputation & Risk Scoring Tool

**Date:** 2026-03-20
**Status:** Approved
**Domain:** depscope.com

---

## Overview

`depscope` is a supply chain reputation and risk scoring tool that analyzes the dependency tree of a software project and assigns each package a reputation-based risk score. It is not primarily a CVE scanner — its differentiator is reputation scoring based on maintainer health, release activity, download trends, and org backing. CVE data is additive.

It ships as:
1. A **CLI tool** (`depscope`) for local use and CI/CD integration
2. An **HTTP API server** (`depscope server`) for programmatic access
3. A **web UI** at `depscope.com` as a marketing and showcase layer

---

## Goals

- Recursive dependency tree analysis (configurable depth, default 10)
- Reputation scoring per package with transitive risk propagation
- Support for Python (pip/poetry/uv), Go (go.mod), Rust (Cargo.toml), JavaScript/TypeScript (npm/pnpm/bun); Java as future consideration
- CI/CD-friendly: configurable pass threshold, exit codes, SARIF output
- Configurable risk appetite via profiles (hobby / opensource / enterprise)
- Open-source CVE data by default (OSV.dev, GitHub Advisory); paid sources (NVD, Snyk) optional via config/API key

---

## Architecture

### Repository Structure

```
depscope/
├── cmd/
│   ├── depscope/main.go        # CLI entrypoint
│   └── server/main.go          # HTTP API entrypoint
├── internal/
│   ├── core/
│   │   ├── scorer.go           # orchestrates scoring per package
│   │   ├── score.go            # score types, risk levels, thresholds
│   │   └── propagator.go       # transitive risk propagation (bottom-up tree walk)
│   ├── manifest/
│   │   ├── python.go           # pip (requirements.txt), poetry (poetry.lock), uv (uv.lock)
│   │   ├── go.go               # go.mod, go.sum
│   │   ├── rust.go             # Cargo.toml, Cargo.lock
│   │   └── javascript.go       # package.json, package-lock.json, pnpm-lock.yaml, bun.lockb
│   ├── registry/
│   │   ├── pypi.go             # PyPI API
│   │   ├── goproxy.go          # pkg.go.dev / proxy.golang.org
│   │   ├── cratesio.go         # crates.io API
│   │   └── npm.go              # npm registry API
│   ├── vcs/
│   │   └── github.go           # GitHub API: contributors, issues, commits, stars, archived
│   ├── vuln/
│   │   ├── osv.go              # OSV.dev (open, always available)
│   │   └── nvd.go              # NVD (optional, requires API key)
│   ├── cache/
│   │   └── cache.go            # TTL-based, file-backed (~/.cache/depscope/), in-memory for server
│   ├── config/
│   │   ├── config.go           # config loading, merging, env var resolution
│   │   └── profiles.go         # hobby / opensource / enterprise presets
│   └── report/
│       ├── text.go             # human-readable table + issue log
│       ├── json.go             # full structured JSON report
│       └── sarif.go            # SARIF for GitHub Security tab integration
└── web/                        # React frontend (separate repo or submodule)
```

### Data Flow

```
depscope scan <path>
      │
      ▼
 Manifest Parser        detects and parses lockfile/manifest
      │ []Package{name, version, constraint, depth}
      ▼
 Fetcher (cached)       parallel workers, rate-limited
      │                 registry APIs + GitHub API
      │                 cache TTL: metadata=24h, CVE=6h
      │ []PackageMetadata
      ▼
 Scorer                 computes own score per package
      │
      ▼
 Propagator             walks tree bottom-up, attaches transitive risk scores
      │
      ▼
 Reporter               renders text / JSON / SARIF
      │
      ├── exit 0  (all package scores ≥ pass_threshold)
      └── exit 1  (any package score < pass_threshold)
```

The CLI and HTTP API server share the same `core`, `manifest`, `registry`, `vcs`, `vuln`, `cache`, and `report` packages. No logic is duplicated.

---

## Scoring Model

Each package receives two independent scores:

### Own Score (0–100)
Measures the package's own health and reputation.

| Factor | Enterprise Default Weight | Notes |
|--------|--------------------------|-------|
| Release recency | 20% | Last release age; >2 years = high risk |
| Maintainer count | 15% | Solo maintainer increases risk |
| Download velocity | 15% | Trend direction matters more than absolute count |
| Open issue ratio | 10% | Open/closed ratio over time |
| Org/company backing | 10% | Verified org vs anonymous individual |
| Version pinning | 15% | Loose constraints on this package penalize the dependent |
| Repo health signals | 15% | Commit frequency, star trend, archived flag |

### Transitive Risk Score (0–100)
Worst-case risk propagated up from all dependencies, weighted by depth. Depth-1 deps carry more weight than depth-10 deps.

### Risk Levels

| Own Score | Level |
|-----------|-------|
| 80–100 | Low |
| 60–79 | Medium |
| 40–59 | High |
| 0–39 | Critical |

### Risk Propagation Rule
If any package in the dependency tree is rated **High** or **Critical**, all direct dependents on that package are flagged in the issue log and their transitive risk score is elevated accordingly.

### Version Pinning Risk
Loose version constraints directly increase the risk score of the package declaring them. Risk levels by constraint type:

| Constraint | Risk impact |
|------------|-------------|
| Exact (`==1.2.3`, `=1.2.3`) | None |
| Patch (`~=1.2.3`, `~1.2`) | Low |
| Minor (`^1.2`, `>=1.2,<2`) | Medium |
| Major / open (`>=1.0`, `*`) | High |

Loose pinning is both logged as an issue and increases the declaring package's own score penalty.

---

## Risk Profiles

Profiles are named presets for risk appetite. A user config file always overrides any profile value.

```yaml
profiles:
  hobby:
    pass_threshold: 40
    weights:
      maintainer_count: 5
      org_backing: 5
      release_recency: 15
      # ...

  opensource:
    pass_threshold: 55
    # balanced weights

  enterprise:
    pass_threshold: 70
    # weights as per table above (default)
```

**Default profile: enterprise.**

---

## CLI Interface

```bash
# Auto-detect manifest in current directory
depscope scan .

# Explicit manifest file
depscope scan --manifest poetry.lock

# Profile selection
depscope scan . --profile opensource

# Config file (overrides profile)
depscope scan . --profile enterprise --config depscope.yaml

# Output formats
depscope scan . --output json > report.json
depscope scan . --output sarif > depscope.sarif

# Depth limit
depscope scan . --depth 5

# Verbose: full issue log
depscope scan . --verbose

# Single package lookup
depscope package check requests==2.28.0 --ecosystem python

# Cache management
depscope cache status
depscope cache clear
```

### Config File (`depscope.yaml`)

```yaml
profile: enterprise
pass_threshold: 75        # overrides profile default
depth: 10
cache_ttl:
  metadata: 24h
  cve: 6h
concurrency: 20

weights:
  release_recency: 20
  maintainer_count: 15
  download_velocity: 15
  open_issue_ratio: 10
  org_backing: 10
  version_pinning: 15
  repo_health: 15

vuln_sources:
  osv: true
  nvd: true
  nvd_api_key: ${NVD_KEY}

registries:
  github_token: ${GITHUB_TOKEN}
```

### CI/CD Integration (GitHub Actions example)

```yaml
- name: depscope scan
  run: depscope scan . --profile enterprise --output sarif > depscope.sarif
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

- uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: depscope.sarif
```

---

## Output Format

### Text (default)

```
depscope scan . — enterprise profile

Scanned 3 direct deps, 47 transitive deps (depth: 10)

┌─────────────────────┬───────┬────────────┬──────────────────┬────────┐
│ Package             │ Score │ Own Risk   │ Transitive Risk  │ Pinned │
├─────────────────────┼───────┼────────────┼──────────────────┼────────┤
│ requests 2.28.0     │  82   │ LOW        │ LOW              │ exact  │
│ urllib3 ^1.26       │  71   │ MEDIUM     │ MEDIUM           │ minor  │ ◄ loose pin
│ cryptography >=3.0  │  41   │ HIGH       │ CRITICAL         │ major  │ ◄ loose pin
└─────────────────────┴───────┴────────────┴──────────────────┴────────┘

Issues (8 total):
  [HIGH]     cryptography: solo maintainer (1 contributor)
  [HIGH]     cryptography: last release 18 months ago
  [MEDIUM]   cryptography: 43 open issues, low close rate
  [MEDIUM]   urllib3: loose version constraint (^1.26) — supply chain risk
  [MEDIUM]   cryptography ← urllib3: transitive risk propagated
  [LOW]      cryptography: no org/company backing
  [INFO]     requests: depends on 12 transitive packages (depth 3)
  [INFO]     urllib3: 3 packages depend on this

Result: FAIL (1 package below threshold 70)
Exit code: 1
```

Every issue is always logged in full output. Summary mode (default) shows the table + issue list. `--verbose` adds full metadata per package.

### JSON
Full structured report: dependency graph, per-package scores, issue list, metadata, scan configuration used.

### SARIF
Standard static analysis interchange format. Uploads directly to GitHub Security tab.

---

## Web UI & API

### HTTP API

```
POST /api/scan
  body: { "github_url": "...", "profile": "enterprise" }
  returns: { "job_id": "abc123", "status": "queued" }

GET  /api/scan/:jobId
  returns: job status + full JSON report when complete

GET  /api/package/:ecosystem/:name/:version
  returns: single package score + issues

GET  /api/health
```

Scans are async (job queue) because deep recursive scans can take 10–30 seconds.

### Web UI (depscope.com)

- Landing page with GitHub URL input
- Live progress indicator (SSE or polling)
- Interactive dependency tree, color-coded by risk level
- Expandable nodes showing transitive deps and issue log
- Shareable result URLs: `depscope.com/scan/abc123`
- README badge generator: `![depscope score](depscope.com/badge/owner/repo.svg)`

The badge is a key marketing mechanism — repos using it link back to depscope.com.

---

## Caching

- **File-backed** (`~/.cache/depscope/`) for CLI — persists across runs
- **In-memory + disk** for the server
- TTL defaults: registry metadata = 24h, CVE data = 6h
- Both TTLs are configurable
- A package seen multiple times in the tree is fetched exactly once per scan

---

## Error Handling

- Registry API failures: log as a warning, mark package score as `unknown`, do not fail the whole scan
- Missing GitHub token: repo health signals are skipped, score penalized slightly, warning logged
- Depth limit reached: logged as `[INFO]` with count of packages not scanned beyond the limit
- CVE source unavailable: logged as warning, scan continues without that source

---

## Testing Strategy

- Unit tests for each scorer factor in isolation (mock registry responses)
- Integration tests for each manifest parser against real lockfile fixtures
- Integration tests for registry clients against recorded HTTP responses (golden files)
- End-to-end CLI test: run `depscope scan` against a known fixture project, assert exit code and JSON output
- Profile tests: verify that the same package scores differently across profiles

---

## Out of Scope (v1)

- Java / Maven support (future)
- Private registry support (future)
- Historical trend tracking / score over time (future)
- GitHub App / PR comment integration (future)
- Paid CVE sources in CLI (web/API only in v1, then unlocked via config key)
