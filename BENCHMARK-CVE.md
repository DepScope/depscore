# depscope CVE Benchmark

Known vulnerability scan of the **top 100 most-downloaded packages** per ecosystem using [OSV.dev](https://osv.dev). Benchmarked on 2026-03-23.

This complements [BENCHMARK.md](BENCHMARK.md) (reputation scoring) with actual CVE data. Together they answer: *"Is this package healthy AND is it currently vulnerable?"*

## Summary — Latest Versions

CVE scan with **resolved latest versions** (version-specific OSV queries):

| Ecosystem | Scanned | Clean | Vulnerable | Total CVEs |
|-----------|---------|-------|------------|-----------|
| **PyPI** | 100 | 100 | 0 | 0 |
| **npm** | 100 | 98 | 2 | 2 |
| **crates.io** | 100 | 100 | 0 | 0 |
| **Go** | 50 | 49 | 1 | 2 |
| **Packagist** | 100 | 100 | 0 | 0 |
| **Total** | **450** | **447** | **3** | **4** |

**99.3% of top packages are CVE-free on their latest version.**

Only 3 packages have unpatched CVEs: `aws-sdk` (npm), `pm2` (npm), and `github.com/aws/aws-sdk-go` (Go).

## Why Version Matters

The same package can have vastly different CVE counts depending on the version:

| Package | Latest Version | CVEs (latest) | All Versions | CVEs (all) |
|---------|---------------|---------------|-------------|-----------|
| `urllib3` | 2.6.3 | **0** | all | 28 |
| `pillow` | 11.2.1 | **0** | all | 112 |
| `cryptography` | 46.0.5 | **0** | all | 26 |
| `hyper` (Rust) | 1.6.0 | **0** | all | 14 |
| `laravel/framework` | 13.1.1 | **0** | all | 23 |

**depscope queries OSV with the exact resolved version from your lockfile.** If you pin `urllib3==1.26.5`, depscope will find the 9 CVEs affecting that version. If you're on `urllib3==2.6.3`, it correctly reports zero.

This is critical for accurate scanning — reporting all historical CVEs would create noise and false positives.

## All-Version Scan (historical CVE density)

For context, here are the total CVEs across ALL versions. This shows which packages have had the most security issues historically — relevant for assessing a package's security track record.

| Ecosystem | Packages with any historical CVE | Total Historical CVEs | Critical | High | Medium | Low |
|-----------|--------------------------------|----------------------|----------|------|--------|-----|
| **PyPI** | 38 | 376 | 48 | 25 | 270 | 33 |
| **npm** | 2 | 2 | 0 | 0 | 1 | 1 |
| **crates.io** | 28 | 109 | 25 | 9 | 70 | 5 |
| **Go** | 1 | 2 | 0 | 0 | 2 | 0 |
| **Packagist** | 17 | 72 | 15 | 3 | 38 | 16 |

### Most CVE-dense packages (historical)

| # | Package | Ecosystem | Historical CVEs | Critical | Status |
|---|---------|-----------|----------------|----------|--------|
| 1 | `pillow` | PyPI | 112 | 16 | All patched in latest |
| 2 | `aiohttp` | PyPI | 31 | 0 | All patched in latest |
| 3 | `urllib3` | PyPI | 28 | 5 | All patched in latest |
| 4 | `cryptography` | PyPI | 26 | 1 | All patched in latest (mostly bundled OpenSSL) |
| 5 | `laravel/framework` | Packagist | 23 | 4 | All patched in latest |
| 6 | `numpy` | PyPI | 16 | 1 | All patched in latest |
| 7 | `jinja2` | PyPI | 15 | 3 | All patched in latest |
| 8 | `hyper` | crates.io | 14 | 3 | All patched in latest |
| 9 | `requests` | PyPI | 12 | 1 | All patched in latest |
| 10 | `smallvec` | crates.io | 11 | 8 | All patched in latest |

**Insight:** High historical CVE count + zero current CVEs = **active security response**. These packages find and fix vulnerabilities. The real risk is packages that *don't* have CVEs — either because they haven't been audited or because issues aren't being reported.

## Reputation + CVE = The Full Picture

| Scenario | Reputation | CVEs | Risk | Example |
|----------|-----------|------|------|---------|
| Healthy + clean | HIGH (80+) | 0 | Low | Most top packages |
| Healthy + vuln history | MEDIUM (70) | 0 (latest) | Low | `pillow`, `cryptography` |
| Unmaintained + clean | HIGH (50) | 0 | **High** | `colorama`, `bluebird` |
| Unmaintained + vulnerable | HIGH (50) | >0 | **Critical** | Old pinned versions |

**depscope catches all four scenarios.** CVE scanners only catch the last one.

## Reproducing This Benchmark

```bash
# Build the CVE benchmark tool
go build -o bin/benchmark-cve ./cmd/benchmark-cve

# Run for each ecosystem (queries OSV.dev — no API key needed)
# Resolves latest version from registry, then queries OSV with that version
./bin/benchmark-cve pypi packages.txt > cve-pypi.json
./bin/benchmark-cve npm packages.txt > cve-npm.json
./bin/benchmark-cve crates packages.txt > cve-crates.json
./bin/benchmark-cve go packages.txt > cve-go.json
./bin/benchmark-cve packagist packages.txt > cve-packagist.json
```

## Full Results (Latest Version)


### PyPI (Python)

**All 100 packages are clean on their latest version.** No known CVEs affecting the current release.

Latest versions scanned: `boto3@1.42.74`, `packaging@26.0`, `urllib3@2.6.3`, `setuptools@82.0.1`, `certifi@2026.2.25`


### npm (JavaScript)

**Packages with known vulnerabilities on latest version:**

| Package | Version | CVEs | Critical | High | Medium | Low |
|---------|---------|------|----------|------|--------|-----|
| aws-sdk | 2.1693.0 | 1 | 0 | 0 | 1 | 0 |
| pm2 | 6.0.14 | 1 | 0 | 0 | 0 | 1 |

Latest versions scanned: `semver@7.7.4`, `debug@4.4.3`, `chalk@5.6.2`, `commander@14.0.3`, `glob@13.0.6`


### crates.io (Rust)

**All 100 packages are clean on their latest version.** No known CVEs affecting the current release.

Latest versions scanned: `syn@2.0.117`, `hashbrown@0.16.1`, `bitflags@2.11.0`, `getrandom@0.4.2`, `proc-macro2@1.0.106`


### Go

**Packages with known vulnerabilities on latest version:**

| Package | Version | CVEs | Critical | High | Medium | Low |
|---------|---------|------|----------|------|--------|-----|
| github.com/aws/aws-sdk-go | v1.55.8 | 2 | 0 | 0 | 2 | 0 |

Latest versions scanned: `github.com/stretchr/testify@v1.11.1`, `github.com/spf13/cobra@v1.10.2`, `github.com/spf13/viper@v1.21.0`, `github.com/gin-gonic/gin@v1.12.0`, `github.com/gorilla/mux@v1.8.1`


### Packagist (PHP)

**All 100 packages are clean on their latest version.** No known CVEs affecting the current release.

Latest versions scanned: `guzzlehttp/psr7@2.9.0`, `symfony/deprecation-contracts@3.6.0`, `psr/http-message@2.0`, `symfony/polyfill-mbstring@1.33.0`, `psr/log@3.0.2`

