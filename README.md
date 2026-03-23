# depscope

Supply chain reputation scoring for your dependencies.

depscope analyzes your project's dependency tree and scores each package on reputation factors — not just CVEs, but maintainer count, release freshness, org backing, version pinning, download velocity, and repository health. It propagates risk through the full transitive dependency graph so a deeply nested risky package surfaces at the top level.

```
depscope scan — profile: enterprise (threshold: 70)

├── requests 2.31.0 [Score: 85 | Risk: LOW]
│   ├── urllib3 2.0.7 [Score: 42 | Risk: HIGH | 2 CVE] (minor)
│   └── certifi 2023.11.17 [Score: 90 | Risk: LOW]
├── flask 3.0.0 [Score: 78 | Risk: MEDIUM]
│   ├── werkzeug 3.0.1 [Score: 82 | Risk: LOW]
│   └── jinja2 3.1.3 [Score: 71 | Risk: MEDIUM]
│       └── markupsafe 2.1.4 [Score: 88 | Risk: LOW]
└── click 8.1.7 [Score: 76 | Risk: MEDIUM]

Issues:
  [HIGH] urllib3: solo maintainer — bus factor risk
  [HIGH] urllib3: CVE CVE-2023-45803: urllib3 request body not stripped... (HIGH)
  [MEDIUM] flask: minor-level constraint — supply chain risk

Scanned 5 direct + 8 transitive dependencies
Result: FAIL
```

## Install

```bash
go install github.com/depscope/depscope/cmd/depscope@latest
```

Or download a binary from [Releases](https://github.com/DepScope/depscore/releases).

## Quick Start

```bash
# Scan current directory
depscope scan .

# Scan a GitHub repository
depscope scan https://github.com/pallets/flask

# Use a relaxed profile for hobby projects
depscope scan . --profile hobby

# JSON output for CI pipelines
depscope scan . --output json

# SARIF output for GitHub Security tab
depscope scan . --output sarif

# Check a single package
depscope package check requests==2.31.0 --ecosystem python
```

## Supported Ecosystems

| Ecosystem | Manifest | Lockfile | Registry |
|-----------|----------|----------|----------|
| Python | `requirements.txt` | `poetry.lock`, `uv.lock` | PyPI |
| Go | `go.mod` | `go.sum` | proxy.golang.org |
| Rust | `Cargo.toml` | `Cargo.lock` | crates.io |
| JavaScript/TypeScript | `package.json` | `package-lock.json` | npm |

When both a manifest and lockfile exist, depscope uses the **lockfile's resolved version** for reputation scoring but reads the **manifest's constraint** to flag wide version ranges.

## Scoring Model

Each package gets two scores:

- **Own Score** (0-100): Weighted average of 7 reputation factors + CVE penalty
- **Transitive Risk Score**: Minimum effective score across all transitive dependencies, discounted by depth

The **final score** is `min(own_score, transitive_risk_score)`.

### Reputation Factors

| Factor | What it measures |
|--------|-----------------|
| Release Recency | How recently the package was updated |
| Maintainer Count | Bus factor — solo vs team maintenance |
| Download Velocity | Community adoption and usage |
| Open Issue Ratio | Maintenance responsiveness |
| Org Backing | Individual vs organization ownership |
| Version Pinning | How tightly the dependency is constrained |
| Repo Health | Commit activity, archived status |

### CVE Scoring

Vulnerability findings from OSV.dev are blended into the reputation score:
- **No CVEs**: Full reputation score
- **CRITICAL CVE**: Heavy penalty (score blended 70/30 with vuln score of 10)
- **HIGH CVE**: Moderate penalty (vuln score 30)
- **MEDIUM CVE**: Light penalty (vuln score 60)

### Risk Profiles

| Profile | Threshold | Use Case |
|---------|-----------|----------|
| `hobby` | 40 | Personal projects, experiments |
| `opensource` | 55 | Open source libraries |
| `enterprise` | 70 | Production applications |

```bash
depscope scan . --profile enterprise  # strict
depscope scan . --profile hobby       # relaxed
```

### Custom Configuration

Create a `depscope.yaml` to override any profile setting:

```yaml
profile: enterprise
pass_threshold: 75
max_depth: 8
weights:
  release_recency: 25
  maintainer_count: 20
vuln:
  osv: true
  nvd: false
```

```bash
depscope scan . --config depscope.yaml
```

## CI/CD Integration

depscope exits with code **0** (pass) or **1** (fail) based on the profile threshold.

### GitHub Actions

```yaml
- name: Supply chain check
  run: |
    go install github.com/depscope/depscope/cmd/depscope@latest
    depscope scan . --profile enterprise --output sarif > depscope.sarif
  continue-on-error: true

- name: Upload SARIF
  uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: depscope.sarif
```

### GitLab CI

```yaml
supply-chain-check:
  script:
    - go install github.com/depscope/depscope/cmd/depscope@latest
    - depscope scan . --profile enterprise --output json > depscope.json
  artifacts:
    reports:
      depscope: depscope.json
```

## Cache

Registry responses are cached locally to avoid repeated API calls:

```bash
depscope cache status   # show cache size
depscope cache clear    # clear all cached data
```

Cache location: `~/.cache/depscope/` (TTL: 24h for metadata, 6h for CVE data).

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `GITHUB_TOKEN` | GitHub API access for repo health, contributor count, org backing |
| `NVD_API_KEY` | NVD vulnerability database (optional, falls back to OSV.dev) |

Setting `GITHUB_TOKEN` significantly improves scoring accuracy by enabling repository health checks, contributor counting, and organization detection.

## Architecture

```
cmd/depscope/          CLI entry point (cobra commands)
internal/
  manifest/            Ecosystem detection + parsers (Go/Python/Rust/JS)
  registry/            PyPI, npm, crates.io, Go proxy clients + cached fetcher
  vcs/                 GitHub API client for repo health signals
  vuln/                OSV.dev + NVD vulnerability lookups
  core/                Factor scorers, weighted scorer, transitive risk propagator
  config/              Risk profiles, YAML config, weight management
  cache/               SHA256-keyed TTL disk cache
  report/              Text tree, JSON, SARIF formatters
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).
