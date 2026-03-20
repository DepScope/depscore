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
- Open-source CVE data by default (OSV.dev, GitHub Advisory); paid sources (NVD, Snyk) optional via config/API key in both CLI and server
- OpenAPI specification for the HTTP API

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
│   │   ├── python.go           # pip (requirements.txt), poetry (pyproject.toml + poetry.lock), uv (uv.lock)
│   │   ├── go.go               # go.mod (constraints) + go.sum (resolved versions)
│   │   ├── rust.go             # Cargo.toml (constraints) + Cargo.lock (resolved versions)
│   │   └── javascript.go       # package.json (constraints) + package-lock.json / pnpm-lock.yaml / bun.lockb
│   ├── registry/
│   │   ├── pypi.go             # PyPI API
│   │   ├── goproxy.go          # pkg.go.dev / proxy.golang.org
│   │   ├── cratesio.go         # crates.io API
│   │   └── npm.go              # npm registry API
│   ├── vcs/
│   │   └── github.go           # GitHub API: contributors, issues, commits, stars, archived
│   ├── vuln/
│   │   ├── osv.go              # OSV.dev (open, always available)
│   │   └── nvd.go              # NVD (optional, requires API key, available in both CLI and server)
│   ├── cache/
│   │   └── cache.go            # TTL-based cache; file-backed for CLI, configurable path for server
│   ├── config/
│   │   ├── config.go           # config loading, merging, env var resolution
│   │   └── profiles.go         # hobby / opensource / enterprise presets
│   └── report/
│       ├── text.go             # human-readable table + issue log
│       ├── json.go             # full structured JSON report
│       └── sarif.go            # SARIF for GitHub Security tab integration
├── api/
│   └── openapi.yaml            # OpenAPI 3.1 spec for the HTTP API
└── web/                        # React frontend (separate repo or submodule)
```

### Manifest Parsing Strategy

Each ecosystem parser reads **two files**:

| Ecosystem | Manifest (constraints) | Lockfile (resolved versions) |
|-----------|------------------------|------------------------------|
| Python (poetry) | `pyproject.toml` | `poetry.lock` |
| Python (pip) | `requirements.txt` | `requirements.txt` (exact pins only) |
| Python (uv) | `pyproject.toml` | `uv.lock` |
| Go | `go.mod` | `go.sum` |
| Rust | `Cargo.toml` | `Cargo.lock` |
| JS/TS (npm) | `package.json` | `package-lock.json` |
| JS/TS (pnpm) | `package.json` | `pnpm-lock.yaml` |
| JS/TS (bun) | `package.json` | `bun.lockb` |

The **manifest** provides constraint type (exact/patch/minor/major) used for version pinning scoring. The **lockfile** provides resolved exact versions used for all other scoring. Both files are always parsed together. If the lockfile is absent, constraints from the manifest are used and a warning is emitted.

### Data Flow

```
depscope scan <path>
      │
      ▼
 Manifest Parser        detects ecosystem, parses manifest + lockfile
      │ []Package{name, resolved_version, constraint_type, depth}
      ▼
 Fetcher (cached)       parallel workers (default: 20 concurrent), rate-limited
      │                 registry APIs + GitHub API
      │                 cache TTL: metadata=24h, CVE=6h
      │                 each unique package fetched exactly once per scan
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
      ├── exit 0  (all package final scores ≥ pass_threshold)
      └── exit 1  (any package final score < pass_threshold)
```

The CLI and HTTP API server share the same `core`, `manifest`, `registry`, `vcs`, `vuln`, `cache`, and `report` packages. No logic is duplicated.

---

## Scoring Model

Each package receives two independent scores:

### Own Score (0–100)
Measures the package's own health and reputation. Higher = safer.

| Factor | Enterprise Default Weight | Notes |
|--------|--------------------------|-------|
| Release recency | 20% | Last release age; >2 years = high risk |
| Maintainer count | 15% | Solo maintainer increases risk |
| Download velocity | 15% | Trend direction matters more than absolute count; see ecosystem notes |
| Open issue ratio | 10% | Open/closed ratio over time |
| Org/company backing | 10% | Verified org vs anonymous individual |
| Version pinning | 15% | Loose constraints declared by this package's dependents |
| Repo health signals | 15% | Commit frequency, star trend, archived flag |

**Ecosystem-specific scoring gaps:**

- **Go:** The Go module proxy does not expose download counts. The `download_velocity` factor (15%) is skipped for Go packages; its weight is redistributed proportionally to the remaining factors.
- **No VCS link:** If a package has no associated source repository, `repo_health` signals are skipped and the `org_backing` and `repo_health` factors receive a 15-point penalty combined. A warning is logged.

**Partial weight overrides:** When a user overrides a subset of weights in config, the remaining weights are scaled proportionally so that all weights always sum to 100%. Example: if the user sets `release_recency: 40` and leaves others unchanged, the remaining 6 factors are scaled down proportionally from their 80% total.

### Transitive Risk Score (0–100)
Reflects the worst-case risk in the package's full dependency subtree, discounted by depth.

**Formula:**

```
transitive_risk_score(P) = min over all descendant packages D at depth d:
    clamp(D.own_score + (d - 1) × 5, 0, 100)
```

- A depth-1 dependency's own score is used directly (no discount)
- Each additional depth level adds 5 points (making deeper packages less severe in propagation)
- Example: a Critical package (own score 25) at depth 1 → effective score 25 (Critical); at depth 5 → 45 (High); at depth 10 → 70 (Medium)
- The transitive risk level is derived from this score using the same thresholds as own score

### Final Score (used for pass/fail)
```
final_score(P) = min(own_score(P), transitive_risk_score(P))
```

The `Score` column in the text output table is the **final score**. Pass/fail threshold is applied to the final score.

### Risk Levels

| Score | Level |
|-------|-------|
| 80–100 | Low |
| 60–79 | Medium |
| 40–59 | High |
| 0–39 | Critical |

### Risk Propagation Rule
If any package in the dependency tree is rated **High** or **Critical**, all direct dependents on that package are flagged in the issue log. Their transitive risk score is updated per the formula above.

### Version Pinning Risk
Loose version constraints directly reduce the version pinning factor score of the **package that declares them** (i.e., the dependent, not the dependency). Parsed from the manifest file, not the lockfile.

| Constraint type | Pinning factor score | Example |
|-----------------|----------------------|---------|
| Exact | 100 (no penalty) | `==1.2.3`, `=1.2.3` |
| Patch | 75 | `~=1.2.3`, `~1.2`, `1.2.*` |
| Minor | 50 | `^1.2`, `>=1.2,<2.0` |
| Major / open | 25 | `>=1.0`, `*`, `latest` |

Loose pinning is both logged as an issue and factors into the declaring package's own score via the version pinning weight.

---

## Risk Profiles

Profiles are named presets for risk appetite. A user config file always overrides any profile value. When a user partially overrides weights, remaining weights are renormalized to sum to 100%.

**Default profile: enterprise.**

| Profile | pass_threshold | Intended audience |
|---------|---------------|-------------------|
| hobby | 40 | Personal/hobby projects |
| opensource | 55 | Open source projects |
| enterprise | 70 | Production/enterprise code |

Full weight tables for each profile are defined in `internal/config/profiles.go`. Key differences:
- `hobby`: lower weight on `maintainer_count` (5%) and `org_backing` (5%), tolerates solo maintainers
- `opensource`: balanced weights, moderate thresholds
- `enterprise`: weights as per table above, strict threshold

---

## CLI Interface

### Valid Ecosystem Identifiers

Used with `depscope package check --ecosystem <value>`:

| Value | Ecosystem |
|-------|-----------|
| `python` | PyPI (pip/poetry/uv) |
| `go` | Go modules (pkg.go.dev) |
| `rust` | crates.io |
| `npm` | npm registry (JS/TS) |

### Commands

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

# Verbose: full package metadata in addition to issue log
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
  # Partial overrides are valid; remaining weights are renormalized to sum to 100%

vuln_sources:
  osv: true               # OSV.dev, always available
  nvd: true               # NVD, requires api_key
  nvd_api_key: ${NVD_KEY} # resolved from environment variable

registries:
  github_token: ${GITHUB_TOKEN}  # strongly recommended; without it, GitHub API limit is 60 req/hr
```

**GitHub token note:** Without a GitHub token, the unauthenticated GitHub API rate limit (60 req/hr) will be exceeded on any scan with more than ~60 packages. A warning is emitted if no token is configured. Fallback: VCS signals are skipped and a penalty is applied to the `repo_health` factor.

### CI/CD Integration (GitHub Actions example)

```yaml
- name: depscope scan
  run: depscope scan . --profile enterprise --output sarif > depscope.sarif
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    NVD_KEY: ${{ secrets.NVD_KEY }}     # optional

- uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: depscope.sarif
```

---

## Output Format

### Text (default)

The `Score` column is the **final score** = min(own score, transitive risk score). This is the value compared against the pass threshold.

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

Every issue is always logged in full output. Default output shows the table + issue list. `--verbose` adds full package metadata (download counts, last release date, contributor list, etc.) per package.

### JSON
Full structured report: dependency graph, per-package scores (own + transitive + final), issue list, metadata, scan configuration used.

### SARIF
Standard static analysis interchange format. Uploads directly to GitHub Security tab.

---

## Web UI & API

### OpenAPI Specification
The HTTP API is defined in `api/openapi.yaml` (OpenAPI 3.1). This serves as the contract between the server and web UI, enables client SDK generation, and is published at `https://depscope.com/api/docs`.

### HTTP API Endpoints

```
POST /api/scan
  body: { "github_url": "https://github.com/owner/repo", "profile": "enterprise" }
  returns: { "job_id": "abc123", "status": "queued" }

GET  /api/scan/:jobId
  returns: job status ("queued" | "running" | "complete" | "failed") + full JSON report when complete

GET  /api/package/:ecosystem/:name/:version
  path params: ecosystem = python | go | rust | npm
  returns: single package score + issues

GET  /api/health

GET  /api/docs
  returns: OpenAPI spec (Swagger UI)
```

Scans are async (job queue) because deep recursive scans can take 10–30 seconds.

### Web UI (depscope.com)

- Landing page with GitHub URL input
- Live progress indicator (SSE or polling)
- Interactive dependency tree, color-coded by risk level
- Expandable nodes showing transitive deps and issue log
- Shareable result URLs: `https://depscope.com/scan/abc123`
- README badge generator: `![depscope score](https://depscope.com/badge/owner/repo.svg)`

The badge is a key marketing mechanism — repos using it link back to depscope.com.

---

## Caching

### CLI
- File-backed at `~/.cache/depscope/`
- Persists across CLI runs
- TTL defaults: registry metadata = 24h, CVE data = 6h

### Server
- Disk-backed at a configurable path (default: OS temp dir + `/depscope-cache/`)
- Maximum cache size: 10 GB (LRU eviction when limit is reached)
- Same TTL defaults as CLI; configurable per-deployment

### Shared rules
- Both TTLs are configurable in `depscope.yaml`
- A package seen multiple times in the tree is fetched exactly once per scan (in-memory deduplication within a single scan run)

---

## Error Handling

- **Registry API failure:** log warning, mark package score as `unknown`, do not fail the whole scan
- **No GitHub token:** warn that large scans will hit the unauthenticated rate limit (60 req/hr); VCS signals are skipped and `repo_health` factor takes a 15-point penalty
- **GitHub API rate limit hit mid-scan:** remaining packages skip VCS signals with a warning; scan completes with reduced accuracy noted in output
- **No VCS link for a package:** skip `repo_health` signals; apply combined 15-point penalty across `org_backing` and `repo_health` factors; log `[INFO]`
- **Depth limit reached:** log `[INFO]` with count of packages not scanned beyond the limit
- **CVE source unavailable:** log warning, scan continues without that source
- **Missing lockfile (manifest only):** log warning, use manifest constraints for version pinning scoring, resolved versions are unknown

---

## Testing Strategy

- Unit tests for each scorer factor in isolation (mock registry responses)
- Unit tests for the transitive propagation formula with known trees
- Unit tests for partial weight override normalization
- Integration tests for each manifest parser against real lockfile + manifest fixture pairs
- Integration tests for registry clients against recorded HTTP responses (golden files)
- End-to-end CLI test: run `depscope scan` against a known fixture project, assert exit code and JSON output match expected values
- Profile tests: verify that the same package scores differently across all three profiles

---

## Dependencies & Libraries

| Purpose | Library | Notes |
|---------|---------|-------|
| CLI commands & config | `github.com/spf13/cobra` + `github.com/spf13/viper` | Industry standard |
| go.mod parsing | `golang.org/x/mod/modfile` | Official Go toolchain package |
| TOML (Cargo.toml, Cargo.lock, poetry.lock, uv.lock) | `github.com/pelletier/go-toml/v2` | TOML 1.0.0 compliant |
| YAML (pnpm-lock.yaml) | `github.com/goccy/go-yaml` | Better error reporting than go-yaml/yaml |
| GitHub API | `github.com/google/go-github` | Official Google client |
| OSV.dev CVE data | `osv.dev/bindings/go/osvdev` | Official OSV bindings |
| SARIF output | `github.com/owenrumney/go-sarif/v3` | SARIF 2.1.0 |
| Concurrency / semaphore | `golang.org/x/sync/semaphore` | Rate-limit parallel fetches |
| Terminal table output | `github.com/olekukonko/tablewriter` | ASCII/Unicode tables |
| HTTP server | stdlib `net/http` (Go 1.22+) | Built-in routing is sufficient |

**Built from scratch** (using stdlib + above libs for decode only):
- Registry API clients: PyPI, npm, crates.io, Go proxy, NVD — thin `net/http` wrappers
- Scoring engine, propagator, cache layer, report formatters

**bun.lockb:** Bun v1.2+ uses a text-based `bun.lock` (JSONC). Support that format; fall back to `package.json` if absent. Binary `bun.lockb` is not supported in v1.

## Build & Release

- **Go version:** 1.22+ (required for stdlib HTTP routing)
- **Releases:** `goreleaser` — cross-platform binaries (Linux/macOS/Windows, amd64/arm64), published to GitHub Releases
- **Server job storage:** In-memory map for v1; jobs expire after 24h, no persistence required for the marketing demo site

## Test Data Strategy

- **Golden files** for all registry API clients — recorded HTTP responses in `testdata/`, tests never hit live APIs
- **Real lockfile fixtures** sourced from well-known open source projects for each ecosystem
- Fixture set includes both healthy and high-risk examples to validate scoring behavior

---

## Out of Scope (v1)

- Java / Maven support (future)
- Private registry support (future)
- Historical trend tracking / score over time (future)
- GitHub App / PR comment integration (future)
