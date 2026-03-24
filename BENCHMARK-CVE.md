# depscope CVE Benchmark

Known vulnerability scan of the **top 100 most-downloaded packages** per ecosystem using [OSV.dev](https://osv.dev). Benchmarked on 2026-03-23.

This complements [BENCHMARK.md](BENCHMARK.md) (reputation scoring) with actual CVE data. Together they answer: *"Is this package healthy AND is it currently vulnerable?"*

## Summary

| Ecosystem | Scanned | Clean | Vulnerable | Total CVEs | Critical | High | Medium | Low |
|-----------|---------|-------|------------|-----------|----------|------|--------|-----|
| **PyPI** | 100 | 62 | 38 | 376 | 48 | 25 | 270 | 33 |
| **npm** | 100 | 98 | 2 | 2 | 0 | 0 | 1 | 1 |
| **crates.io** | 100 | 72 | 28 | 109 | 25 | 9 | 70 | 5 |
| **Go** | 50 | 49 | 1 | 2 | 0 | 0 | 2 | 0 |
| **Packagist** | 100 | 83 | 17 | 72 | 15 | 3 | 38 | 16 |
| **Total** | **450** | **364** | **86** | **561** | **88** | **37** | **381** | **55** |

**19% of top packages have at least one known CVE.**

> **Note:** PyPI, crates.io, and Packagist results show CVEs across ALL versions (not just the latest), because these registries don't return a latest version in their API response when queried without one. npm and Go results are for the latest version only, which is why their CVE counts are lower. This is a known limitation we plan to fix.

## Key Findings

### Most vulnerable packages (by CVE count)

| # | Package | Ecosystem | CVEs | Critical | Details |
|---|---------|-----------|------|----------|---------|
| 1 | **pillow** | PyPI | 112 | 16 | Image processing library — long history of buffer overflows and DoS |
| 2 | **urllib3** | PyPI | 28 | 5 | HTTP client — redirect handling, proxy auth leaks |
| 3 | **aiohttp** | PyPI | 31 | 0 | Async HTTP — request smuggling, CRLF injection |
| 4 | **cryptography** | PyPI | 26 | 1 | Crypto library — mostly from bundled OpenSSL |
| 5 | **laravel/framework** | Packagist | 23 | 4 | PHP framework — XSS, SQL injection, session issues |
| 6 | **numpy** | PyPI | 16 | 1 | Numeric — buffer overflows in array parsing |
| 7 | **jinja2** | PyPI | 15 | 3 | Templating — sandbox escape vulnerabilities |
| 8 | **hyper** | crates.io | 14 | 3 | HTTP library — request smuggling, DoS |
| 9 | **symfony/http-foundation** | Packagist | 13 | 1 | HTTP handling — header injection, session fixation |
| 10 | **requests** | PyPI | 12 | 1 | HTTP client — credential leaks, redirect issues |

### What this tells us

1. **PyPI has the most CVEs** (376 across 38 packages). This reflects Python's maturity — older packages have longer vulnerability histories, not necessarily that they're less secure today.

2. **npm is remarkably clean** — only 2 of 100 top packages have CVEs on their latest version. The npm ecosystem has strong update discipline.

3. **crates.io** shows 28 vulnerable packages, dominated by networking libraries (hyper, h2, mio, tokio). Rust's memory safety helps but doesn't prevent logic bugs.

4. **The high CVE count doesn't mean "don't use these packages."** `pillow` has 112 CVEs but also 112+ patches. Active CVE history means active security response. The real risk is packages with CVEs that *aren't being patched*.

### Reputation + CVE = the full picture

The reputation benchmark ([BENCHMARK.md](BENCHMARK.md)) and CVE benchmark complement each other:

| Package | Reputation Score | CVEs | Interpretation |
|---------|-----------------|------|----------------|
| `pillow` | 71 (MEDIUM) | 112 | High CVE count but actively maintained — patches ship fast |
| `colorama` | 54 (HIGH) | 0 | Zero CVEs but unmaintained for 3+ years — future risk |
| `bluebird` | 54 (HIGH) | 0 | Zero CVEs but abandoned for 6+ years |
| `urllib3` | 77 (MEDIUM) | 28 | Many CVEs but 4 maintainers, active releases |

**Reputation scoring catches what CVE scanning misses** — the package that has no CVEs today but is one maintainer retirement away from becoming an unpatched liability.

## Reproducing This Benchmark

```bash
# Build the CVE benchmark tool
go build -o bin/benchmark-cve ./cmd/benchmark-cve

# Run for each ecosystem (queries OSV.dev — no API key needed)
./bin/benchmark-cve pypi packages.txt > cve-pypi.json
./bin/benchmark-cve npm packages.txt > cve-npm.json
./bin/benchmark-cve crates packages.txt > cve-crates.json
./bin/benchmark-cve go packages.txt > cve-go.json
./bin/benchmark-cve packagist packages.txt > cve-packagist.json
```

## Full Results


### PyPI (Python)

**Packages with known vulnerabilities:**

| Package | Version | CVEs | Critical | High | Medium | Low |
|---------|---------|------|----------|------|--------|-----|
| pillow | all versions | 112 | 16 | 5 | 87 | 4 |
| aiohttp | all versions | 31 | 0 | 1 | 20 | 10 |
| urllib3 | all versions | 28 | 5 | 3 | 15 | 5 |
| cryptography | all versions | 26 | 1 | 3 | 21 | 1 |
| numpy | all versions | 16 | 1 | 1 | 13 | 1 |
| pip | all versions | 16 | 1 | 1 | 13 | 1 |
| jinja2 | all versions | 15 | 3 | 0 | 9 | 3 |
| requests | all versions | 12 | 1 | 2 | 8 | 1 |
| pyyaml | all versions | 8 | 4 | 0 | 4 | 0 |
| pygments | all versions | 8 | 1 | 0 | 7 | 0 |
| setuptools | all versions | 7 | 3 | 0 | 4 | 0 |
| pyarrow | all versions | 7 | 1 | 0 | 6 | 0 |
| starlette | all versions | 7 | 0 | 0 | 5 | 2 |
| certifi | all versions | 6 | 0 | 1 | 5 | 0 |
| protobuf | all versions | 6 | 1 | 0 | 5 | 0 |
| pyjwt | all versions | 6 | 1 | 1 | 4 | 0 |
| rsa | all versions | 6 | 0 | 1 | 4 | 1 |
| grpcio | all versions | 6 | 1 | 0 | 4 | 1 |
| sqlalchemy | all versions | 6 | 3 | 0 | 3 | 0 |
| virtualenv | all versions | 5 | 2 | 0 | 3 | 0 |
| markdown-it-py | all versions | 4 | 0 | 0 | 4 | 0 |
| scipy | all versions | 4 | 1 | 0 | 3 | 0 |
| uvicorn | all versions | 4 | 0 | 0 | 4 | 0 |
| pydantic | all versions | 3 | 0 | 1 | 1 | 1 |
| wheel | all versions | 3 | 0 | 1 | 2 | 0 |
| tqdm | all versions | 3 | 1 | 0 | 1 | 1 |
| fastapi | all versions | 3 | 0 | 0 | 3 | 0 |
| idna | all versions | 2 | 0 | 0 | 2 | 0 |
| filelock | all versions | 2 | 1 | 1 | 0 | 0 |
| httpx | all versions | 2 | 0 | 1 | 1 | 0 |
| pyasn1 | all versions | 2 | 0 | 0 | 2 | 0 |
| oauthlib | all versions | 2 | 0 | 0 | 2 | 0 |
| psutil | all versions | 2 | 0 | 0 | 2 | 0 |
| openpyxl | all versions | 2 | 0 | 1 | 1 | 0 |
| pandas | all versions | 1 | 0 | 0 | 1 | 0 |
| h11 | all versions | 1 | 0 | 1 | 0 | 0 |
| zipp | all versions | 1 | 0 | 0 | 1 | 0 |
| awscli | all versions | 1 | 0 | 0 | 0 | 1 |

**Clean packages (62):** `aiobotocore`, `aiohappyeyeballs`, `aiosignal`, `annotated-types`, `anyio`, `attrs`, `boto3`, `botocore`, `cachetools`, `cffi`, `charset-normalizer`, `click`, `colorama`, `et-xmlfile`, `frozenlist`, `fsspec`, `google-api-core`, `google-auth`, `google-genai`, `googleapis-common-protos`
... and 42 more


### npm (JavaScript)

**Packages with known vulnerabilities:**

| Package | Version | CVEs | Critical | High | Medium | Low |
|---------|---------|------|----------|------|--------|-----|
| aws-sdk | 2.1693.0 | 1 | 0 | 0 | 1 | 0 |
| pm2 | 6.0.14 | 1 | 0 | 0 | 0 | 1 |

**Clean packages (98):** `ajv`, `async`, `axios`, `babel-core`, `bcrypt`, `bluebird`, `body-parser`, `bootstrap`, `chai`, `chalk`, `chart.js`, `cheerio`, `commander`, `compression`, `cookie-parser`, `cors`, `d3`, `date-fns`, `dayjs`, `debug`
... and 78 more


### crates.io (Rust)

**Packages with known vulnerabilities:**

| Package | Version | CVEs | Critical | High | Medium | Low |
|---------|---------|------|----------|------|--------|-----|
| hyper | all versions | 14 | 3 | 2 | 6 | 3 |
| smallvec | all versions | 11 | 8 | 0 | 3 | 0 |
| tokio | all versions | 10 | 1 | 1 | 7 | 1 |
| lock_api | all versions | 6 | 0 | 4 | 2 | 0 |
| h2 | all versions | 6 | 0 | 0 | 5 | 1 |
| rand_core | all versions | 4 | 4 | 0 | 0 | 0 |
| mio | all versions | 4 | 0 | 0 | 4 | 0 |
| http | all versions | 4 | 2 | 0 | 2 | 0 |
| time | all versions | 4 | 0 | 0 | 4 | 0 |
| rustls | all versions | 4 | 0 | 0 | 4 | 0 |
| futures-util | all versions | 4 | 0 | 2 | 2 | 0 |
| futures-task | all versions | 4 | 2 | 0 | 2 | 0 |
| memoffset | all versions | 4 | 0 | 0 | 4 | 0 |
| sha2 | all versions | 3 | 1 | 0 | 2 | 0 |
| nix | all versions | 3 | 1 | 0 | 2 | 0 |
| hashbrown | all versions | 2 | 0 | 0 | 2 | 0 |
| base64 | all versions | 2 | 2 | 0 | 0 | 0 |
| once_cell | all versions | 2 | 0 | 0 | 2 | 0 |
| regex | all versions | 2 | 0 | 0 | 2 | 0 |
| socket2 | all versions | 2 | 0 | 0 | 2 | 0 |
| bytes | all versions | 2 | 0 | 0 | 2 | 0 |
| idna | all versions | 2 | 0 | 0 | 2 | 0 |
| crossbeam-utils | all versions | 2 | 1 | 0 | 1 | 0 |
| slab | all versions | 2 | 0 | 0 | 2 | 0 |
| generic-array | all versions | 2 | 0 | 0 | 2 | 0 |
| tracing | all versions | 2 | 0 | 0 | 2 | 0 |
| rustix | all versions | 1 | 0 | 0 | 1 | 0 |
| chrono | all versions | 1 | 0 | 0 | 1 | 0 |

**Clean packages (72):** `ahash`, `aho-corasick`, `anyhow`, `autocfg`, `bitflags`, `block-buffer`, `byteorder`, `cc`, `cfg-if`, `clap`, `clap_lex`, `digest`, `either`, `fastrand`, `fnv`, `futures-channel`, `futures-core`, `futures-io`, `futures-sink`, `getrandom`
... and 52 more


### Go

**Packages with known vulnerabilities:**

| Package | Version | CVEs | Critical | High | Medium | Low |
|---------|---------|------|----------|------|--------|-----|
| github.com/aws/aws-sdk-go | v1.55.8 | 2 | 0 | 0 | 2 | 0 |

**Clean packages (49):** `github.com/cenkalti/backoff/v4`, `github.com/confluentinc/confluent-kafka-go`, `github.com/docker/docker`, `github.com/fatih/color`, `github.com/gin-gonic/gin`, `github.com/go-chi/chi/v5`, `github.com/go-kit/kit`, `github.com/go-playground/validator`, `github.com/go-sql-driver/mysql`, `github.com/goccy/go-yaml`, `github.com/gofiber/fiber/v2`, `github.com/golang/protobuf`, `github.com/google/uuid`, `github.com/gorilla/mux`, `github.com/grpc-ecosystem/grpc-gateway`, `github.com/hashicorp/consul`, `github.com/hashicorp/terraform`, `github.com/hashicorp/vault`, `github.com/jackc/pgx/v5`, `github.com/jmoiron/sqlx`
... and 29 more


### Packagist (PHP)

**Packages with known vulnerabilities:**

| Package | Version | CVEs | Critical | High | Medium | Low |
|---------|---------|------|----------|------|--------|-----|
| laravel/framework | all versions | 23 | 4 | 1 | 12 | 6 |
| symfony/http-foundation | all versions | 13 | 1 | 1 | 7 | 4 |
| symfony/http-kernel | all versions | 7 | 2 | 1 | 4 | 0 |
| guzzlehttp/guzzle | all versions | 6 | 2 | 0 | 4 | 0 |
| league/commonmark | all versions | 6 | 0 | 0 | 3 | 3 |
| guzzlehttp/psr7 | all versions | 2 | 0 | 0 | 0 | 2 |
| symfony/process | all versions | 2 | 2 | 0 | 0 | 0 |
| symfony/routing | all versions | 2 | 0 | 0 | 2 | 0 |
| phpunit/phpunit | all versions | 2 | 2 | 0 | 0 | 0 |
| symfony/yaml | all versions | 2 | 0 | 0 | 2 | 0 |
| monolog/monolog | all versions | 1 | 0 | 0 | 1 | 0 |
| symfony/mime | all versions | 1 | 0 | 0 | 1 | 0 |
| symfony/translation | all versions | 1 | 0 | 0 | 1 | 0 |
| symfony/error-handler | all versions | 1 | 0 | 0 | 0 | 1 |
| nesbot/carbon | all versions | 1 | 0 | 0 | 1 | 0 |
| league/flysystem | all versions | 1 | 1 | 0 | 0 | 0 |
| psy/psysh | all versions | 1 | 1 | 0 | 0 | 0 |

**Clean packages (83):** `brick/math`, `carbonphp/carbon-doctrine-types`, `composer/pcre`, `composer/semver`, `dflydev/dot-access-data`, `doctrine/deprecations`, `doctrine/inflector`, `doctrine/lexer`, `dragonmantank/cron-expression`, `egulias/email-validator`, `graham-campbell/result-type`, `guzzlehttp/promises`, `laravel/serializable-closure`, `league/flysystem-local`, `league/mime-type-detection`, `myclabs/deep-copy`, `nette/schema`, `nette/utils`, `nikic/php-parser`, `paragonie/constant_time_encoding`
... and 63 more

