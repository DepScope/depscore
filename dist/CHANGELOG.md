## Changelog

### v3.0.0 — Supply Chain Index & Compromised Package Scanner

**Major new features:**

- **`depscope index`** — Walk any directory tree and catalog every package manifest across all ecosystems (npm, Go, Python, Rust, PHP). Incremental mtime-based re-indexing, global package dedup, full transitive dependency tree extraction from lockfiles (package-lock.json, Cargo.lock, poetry.lock, composer.lock).
  - `depscope index status` — Show stats, ecosystem breakdown, top packages
  - `depscope index search <name>` — Find all manifests referencing a package, with "depends on" and "depended on by"
  - `depscope index list [--ecosystem npm]` — List all indexed manifests with package counts
  - `depscope index explore` — Interactive TUI browser with live search, score/risk display, and dependency chain drill-down
  - `depscope index enrich` — Add reputation scores (7-factor via registry + VCS) and CVE data (via OSV.dev) to every indexed package. Resumable, concurrent, uses CVE cache.
  - `depscope index report [--ecosystem npm]` — Comprehensive risk report with distribution charts, CVE summaries, top risky packages, most vulnerable packages, ecosystem breakdown, and most exposed manifests.

- **`depscope compromised`** — Scan for known-bad packages in dependency trees.
  - `--packages "axios@1.14.1,axios@0.30.4"` or `--file compromised.txt` — inline or file-based targets with semver range support (^, ~, >=, <, compound, wildcard)
  - `--from-index` — Instant query from the SQLite index instead of walking the filesystem
  - Dependency chain tracing — shows the full path from root to compromised package
  - Classifies findings as DIRECT (in manifest) or INDIRECT (transitive)
  - Logs all findings to SQLite `compromised_findings` table

- **Web UI enhancements:**
  - `/search` — Index browser with stats dashboard, risk distribution bars, package search, and compromised check tab
  - Search results include reputation scores, risk levels, and CVE counts from enrichment data

- **Unified dependency tree** (merged from v2.x):
  - BFS crawler with 8 resolvers (package, action, pre-commit, submodule, terraform, tool, script, buildtool)
  - SQLite cache with projects, versions, dependencies, CVE cache, ref resolutions
  - `depscope tree` command for ASCII dependency tree output
  - Interactive TUI walker for dependency browsing
  - Web graph with subtree highlighting, filtering, depth control
  - Reputation scoring pass with registry + VCS lookups
  - CVE post-processing pass via OSV.dev

**SQLite schema:** 10 tables — projects, project_versions, version_dependencies, index_manifests, manifest_packages, index_runs, compromised_findings, ref_resolutions, cve_cache, (+ existing scan store tables)

**npm semver matching:** Full support for ^, ~, >=, >, <=, <, compound ranges, wildcard, ^0.x.y and ^0.0.x edge cases.

**Lockfile dependency tree extraction:**
- package-lock.json (npm v2/v3) — full transitive with devDependencies
- Cargo.lock (Rust) — full transitive
- poetry.lock (Python) — full transitive via TOML parsing
- composer.lock (PHP) — full transitive, skips php/ext-*/lib-* entries

### v2.x

* feat: add web graph visualization with D3.js + SQLite (#17)
* fix: add SQLite db files to gitignore
