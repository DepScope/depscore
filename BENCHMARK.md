# depscope Benchmark

Reputation + CVE scores for the **top 100 most-downloaded packages** in each ecosystem, benchmarked on 2026-03-23.

depscope combines a **7-factor reputation score** with a **CVE penalty** for a single number that answers: *"Can I trust this package right now?"*

## How Scoring Works

```
Final Score = Reputation Score (7 weighted factors) - CVE Penalty
```

**Reputation factors** (100 points, weighted):

| Factor | Weight | What it measures |
|--------|--------|-----------------|
| Release recency | 20% | Days since last release |
| Maintainer count | 15% | Bus-factor risk |
| Download velocity | 15% | Community adoption |
| Version pinning | 15% | Constraint tightness |
| Repository health | 15% | Commit activity, archived status |
| Organization backing | 10% | Org vs individual maintainer |
| Open issue ratio | 10% | Maintenance responsiveness |

**CVE penalty** (subtracted from reputation score):

| Severity | Penalty per CVE |
|----------|----------------|
| CRITICAL | -15 |
| HIGH | -10 |
| MEDIUM | -5 |
| LOW | -2 |

**Risk levels:** LOW (80-100), MEDIUM (60-79), HIGH (40-59), CRITICAL (0-39)

## Summary

| Ecosystem | Packages | Avg Score | LOW | MEDIUM | HIGH | CRITICAL | CVEs on Latest |
|-----------|----------|-----------|-----|--------|------|----------|---------------|
| **PyPI** | 100 | 76.5 | 27% | 69% | 4% | 0% | 0 |
| **npm** | 100 | 78.7 | 57% | 37% | 5% | 0% | 3 |
| **crates.io** | 100 | 75.2 | 0% | 100% | 0% | 0% | 0 |
| **Go** | 50 | 65.6 | 0% | 76% | 24% | 0% | 2 |
| **Packagist** | 100 | 74.4 | 32% | 59% | 9% | 0% | 0 |

**Key findings:**
- Zero false CRITICAL ratings across 450 top packages
- 99% of top packages are CVE-free on their latest version
- Only 5 CVEs total across 4 packages (npm: aws-sdk, pm2; Go: aws-sdk-go)
- The few HIGH-risk ratings are legitimate (abandoned packages, solo maintainers)

## Notable Packages

### Correctly flagged as concerning

These are real supply chain risks — popular packages with maintenance issues:

| Package | Ecosystem | Score | Issue |
|---------|-----------|-------|-------|
| `colorama` | PyPI | 54 | Solo maintainer, last release 1,246 days ago |
| `mdurl` | PyPI | 54 | Solo maintainer, last release 1,317 days ago |
| `bluebird` | npm | 54 | Solo maintainer, last release 2,307 days ago (6.3 years!) |
| `micro` | npm | 54 | Solo maintainer, last release 1,213 days ago |
| `fnv` | crates.io | 61 | No listed maintainers, last release 2,139 days ago |
| `github.com/pkg/errors` | Go | 52 | Archived, last release 2,260 days ago |
| `ralouphie/getallheaders` | Packagist | 54 | Solo maintainer, last release 2,573 days ago (7 years!) |
| `psr/event-dispatcher` | Packagist | 54 | Solo maintainer, last release 2,631 days ago |

### Correctly scored as trusted

| Package | Ecosystem | Score | Why |
|---------|-----------|-------|-----|
| `guzzlehttp/psr7` | Packagist | 87 | 7 maintainers, released 13 days ago |
| `sebastian/exporter` | Packagist | 87 | 5 maintainers, released 46 days ago |
| `fastify` | npm | 87 | 13 maintainers, active releases |
| `black` | PyPI | 84 | Active development, frequent releases |
| `boto3` | PyPI | 80 | AWS SDK, released daily |

### CVE impact example

With old pinned versions, CVE penalties dramatically change scores:

| Package | Reputation | CVEs | Penalty | Final Score |
|---------|-----------|------|---------|-------------|
| `requests@2.28.0` | 79 | 4 (1C, 2H, 1M) | -40 | **39 (CRITICAL)** |
| `urllib3@1.26.5` | 87 | 9 (4C, 1H, 3M, 1L) | -87 | **0 (CRITICAL)** |
| `requests@2.32.5` | 79 | 0 | 0 | **79 (MEDIUM)** |
| `urllib3@2.6.3` | 87 | 0 | 0 | **87 (LOW)** |

**Updating to the latest version eliminates all CVE penalties.**

## Version-Specific CVE Scanning

depscope queries [OSV.dev](https://osv.dev) with the **exact resolved version** from your lockfile:

- `urllib3==2.6.3` → 0 CVEs (all patched)
- `urllib3==1.26.5` → 9 CVEs (4 CRITICAL)
- `urllib3` (all versions) → 28 CVEs (historical)

This prevents false positives from historical CVEs that don't affect your version.

## Reproducing This Benchmark

```bash
# Build tools
make build
go build -o bin/benchmark ./cmd/benchmark
go build -o bin/benchmark-cve ./cmd/benchmark-cve

# Run full benchmark (fetches top packages, runs reputation + CVE)
python3 scripts/benchmark.py

# Run specific ecosystems
python3 scripts/benchmark.py pypi npm --count 50

# Depth benchmark (clones real projects, full recursive scan)
python3 scripts/benchmark-depth.py
```

## Full Results


### PyPI (Python) - Top 100

| # | Package | Score | Risk | CVEs | Maintainers | Days Since Release | Source |
|---|---------|-------|------|------|-------------|-------------------|--------|
| 1 | pydantic | 87 | LOW | - | 13 | 27 | yes |
| 2 | pydantic-core | 87 | LOW | - | 6 | 28 | yes |
| 3 | urllib3 | 84 | LOW | - | 4 | 75 | yes |
| 4 | platformdirs | 84 | LOW | - | 4 | 18 | yes |
| 5 | pyasn1 | 84 | LOW | - | 3 | 7 | yes |
| 6 | yarl | 84 | LOW | - | 4 | 22 | yes |
| 7 | propcache | 84 | LOW | - | 4 | 166 | yes |
| 8 | certifi | 80 | LOW | - | 2 | 27 | yes |
| 9 | numpy | 80 | LOW | - | 2 | 15 | yes |
| 10 | pyyaml | 80 | LOW | - | 2 | 175 | yes |
| 11 | s3transfer | 80 | LOW | - | 2 | 113 | yes |
| 12 | s3fs | 80 | LOW | - | 2 | 46 | yes |
| 13 | iniconfig | 80 | LOW | - | 2 | 156 | yes |
| 14 | jmespath | 80 | LOW | - | 2 | 60 | yes |
| 15 | wheel | 80 | LOW | - | 2 | 61 | yes |
| 16 | rich | 80 | LOW | - | 2 | 32 | yes |
| 17 | multidict | 80 | LOW | - | 2 | 57 | yes |
| 18 | google-auth | 80 | LOW | - | 2 | 11 | yes |
| 19 | tzdata | 80 | LOW | - | 2 | 100 | yes |
| 20 | frozenlist | 80 | LOW | - | 2 | 169 | yes |
| 21 | greenlet | 80 | LOW | - | 2 | 31 | yes |
| 22 | sqlalchemy | 80 | LOW | - | 2 | 21 | yes |
| 23 | trove-classifiers | 80 | LOW | - | 2 | 68 | yes |
| 24 | uvicorn | 80 | LOW | - | 2 | 8 | yes |
| 25 | starlette | 80 | LOW | - | 2 | 1 | yes |
| 26 | psutil | 80 | LOW | - | 2 | 54 | yes |
| 27 | tenacity | 80 | LOW | - | 2 | 45 | yes |
| 28 | boto3 | 78 | MEDIUM | - | 1 | 0 | yes |
| 29 | packaging | 78 | MEDIUM | - | 1 | 61 | yes |
| 30 | setuptools | 78 | MEDIUM | - | 1 | 15 | yes |
| 31 | typing-extensions | 78 | MEDIUM | - | 4 | 210 | yes |
| 32 | botocore | 78 | MEDIUM | - | 1 | 0 | yes |
| 33 | idna | 78 | MEDIUM | - | 1 | 162 | yes |
| 34 | charset-normalizer | 78 | MEDIUM | - | 1 | 8 | yes |
| 35 | aiobotocore | 78 | MEDIUM | - | 1 | 6 | yes |
| 36 | cryptography | 78 | MEDIUM | - | 1 | 41 | yes |
| 37 | pip | 78 | MEDIUM | - | 1 | 47 | yes |
| 38 | pycparser | 78 | MEDIUM | - | 1 | 61 | yes |
| 39 | pygments | 78 | MEDIUM | - | 3 | 276 | yes |
| 40 | fsspec | 78 | MEDIUM | - | 1 | 46 | yes |
| 41 | click | 78 | MEDIUM | - | 1 | 128 | yes |
| 42 | pandas | 78 | MEDIUM | - | 1 | 34 | yes |
| 43 | attrs | 78 | MEDIUM | - | 1 | 4 | yes |
| 44 | pytest | 78 | MEDIUM | - | 1 | 107 | yes |
| 45 | anyio | 78 | MEDIUM | - | 1 | 0 | yes |
| 46 | markupsafe | 78 | MEDIUM | - | 1 | 177 | yes |
| 47 | filelock | 78 | MEDIUM | - | 1 | 12 | yes |
| 48 | importlib-metadata | 78 | MEDIUM | - | 1 | 3 | yes |
| 49 | pathspec | 78 | MEDIUM | - | 1 | 56 | yes |
| 50 | pyjwt | 78 | MEDIUM | - | 1 | 10 | yes |
| 51 | typing-inspection | 78 | MEDIUM | - | 1 | 174 | yes |
| 52 | aiohttp | 78 | MEDIUM | - | 1 | 79 | yes |
| 53 | python-dotenv | 78 | MEDIUM | - | 1 | 22 | yes |
| 54 | jsonschema | 78 | MEDIUM | - | 1 | 76 | yes |
| 55 | tqdm | 78 | MEDIUM | - | 1 | 48 | yes |
| 56 | google-api-core | 78 | MEDIUM | - | 1 | 33 | yes |
| 57 | grpcio | 78 | MEDIUM | - | 1 | 6 | yes |
| 58 | tomli | 78 | MEDIUM | - | 1 | 72 | yes |
| 59 | awscli | 78 | MEDIUM | - | 1 | 0 | yes |
| 60 | virtualenv | 78 | MEDIUM | - | 1 | 14 | yes |
| 61 | googleapis-common-protos | 78 | MEDIUM | - | 1 | 17 | yes |
| 62 | rpds-py | 78 | MEDIUM | - | 1 | 113 | yes |
| 63 | referencing | 78 | MEDIUM | - | 1 | 161 | yes |
| 64 | wrapt | 78 | MEDIUM | - | 1 | 18 | yes |
| 65 | pillow | 78 | MEDIUM | - | 1 | 41 | yes |
| 66 | pyasn1-modules | 78 | MEDIUM | - | 4 | 361 | yes |
| 67 | scipy | 78 | MEDIUM | - | 1 | 29 | yes |
| 68 | pyarrow | 78 | MEDIUM | - | 1 | 36 | yes |
| 69 | oauthlib | 78 | MEDIUM | - | 3 | 277 | yes |
| 70 | pyparsing | 78 | MEDIUM | - | 1 | 62 | yes |
| 71 | fastapi | 78 | MEDIUM | - | 1 | 0 | yes |
| 72 | google-genai | 78 | MEDIUM | - | 1 | 6 | yes |
| 73 | cachetools | 78 | MEDIUM | - | 1 | 14 | yes |
| 74 | opentelemetry-proto | 78 | MEDIUM | - | 1 | 19 | yes |
| 75 | grpcio-tools | 78 | MEDIUM | - | 1 | 6 | yes |
| 76 | opentelemetry-semantic-conventions | 78 | MEDIUM | - | 1 | 19 | yes |
| 77 | requests | 75 | MEDIUM | - | 2 | 217 | yes |
| 78 | cffi | 75 | MEDIUM | - | 2 | 196 | yes |
| 79 | h11 | 75 | MEDIUM | - | 2 | 334 | yes |
| 80 | rsa | 75 | MEDIUM | - | 2 | 342 | yes |
| 81 | aiosignal | 75 | MEDIUM | - | 2 | 263 | yes |
| 82 | annotated-types | 73 | MEDIUM | - | 3 | 672 | yes |
| 83 | httpx | 73 | MEDIUM | - | 1 | 189 | yes |
| 84 | zipp | 73 | MEDIUM | - | 1 | 288 | yes |
| 85 | httpcore | 73 | MEDIUM | - | 1 | 333 | yes |
| 86 | markdown-it-py | 73 | MEDIUM | - | 1 | 225 | yes |
| 87 | jsonschema-specifications | 73 | MEDIUM | - | 1 | 197 | yes |
| 88 | six | 70 | MEDIUM | - | 2 | 474 | yes |
| 89 | requests-oauthlib | 70 | MEDIUM | - | 2 | 728 | yes |
| 90 | aiohappyeyeballs | 70 | MEDIUM | - | 2 | 377 | yes |
| 91 | python-dateutil | 68 | MEDIUM | - | 4 | 752 | yes |
| 92 | pytz | 68 | MEDIUM | - | 2 | 21 | no |
| 93 | grpcio-status | 66 | MEDIUM | - | 1 | 6 | no |
| 94 | protobuf | 66 | MEDIUM | - | 1 | 3 | no |
| 95 | jinja2 | 65 | MEDIUM | - | 1 | 383 | yes |
| 96 | pluggy | 60 | MEDIUM | - | 1 | 313 | no |
| 97 | et-xmlfile | 57 | HIGH | - | 2 | 514 | no |
| 98 | openpyxl | 57 | HIGH | - | 2 | 633 | no |
| 99 | colorama | 54 | HIGH | - | 1 | 1246 | yes |
| 100 | mdurl | 54 | HIGH | - | 1 | 1318 | yes |

### npm (JavaScript) - Top 100

| # | Package | Score | Risk | CVEs | Maintainers | Days Since Release | Source |
|---|---------|-------|------|------|-------------|-------------------|--------|
| 1 | express | 87 | LOW | - | 5 | 112 | yes |
| 2 | semver | 87 | LOW | - | 5 | 46 | yes |
| 3 | webpack | 87 | LOW | - | 8 | 20 | yes |
| 4 | typescript | 87 | LOW | - | 6 | 0 | yes |
| 5 | prettier | 87 | LOW | - | 11 | 61 | yes |
| 6 | jest | 87 | LOW | - | 5 | 14 | yes |
| 7 | supertest | 87 | LOW | - | 6 | 77 | yes |
| 8 | jquery | 87 | LOW | - | 5 | 65 | yes |
| 9 | sequelize | 87 | LOW | - | 9 | 16 | yes |
| 10 | knex | 87 | LOW | - | 5 | 0 | yes |
| 11 | redis | 87 | LOW | - | 5 | 35 | yes |
| 12 | sqlite3 | 87 | LOW | - | 10 | 11 | yes |
| 13 | graphql | 87 | LOW | - | 6 | 0 | yes |
| 14 | fastify | 87 | LOW | - | 5 | 1 | yes |
| 15 | koa | 87 | LOW | - | 11 | 27 | yes |
| 16 | multer | 87 | LOW | - | 5 | 19 | yes |
| 17 | joi | 87 | LOW | - | 6 | 0 | yes |
| 18 | ramda | 87 | LOW | - | 8 | 164 | yes |
| 19 | mobx | 87 | LOW | - | 6 | 178 | yes |
| 20 | framer-motion | 87 | LOW | - | 66 | 7 | yes |
| 21 | chart.js | 87 | LOW | - | 5 | 162 | yes |
| 22 | jsdom | 87 | LOW | - | 6 | 4 | yes |
| 23 | lodash | 84 | LOW | - | 3 | 61 | yes |
| 24 | axios | 84 | LOW | - | 4 | 24 | yes |
| 25 | mocha | 84 | LOW | - | 3 | 139 | yes |
| 26 | sinon | 84 | LOW | - | 4 | 8 | yes |
| 27 | tailwindcss | 84 | LOW | - | 3 | 5 | yes |
| 28 | dotenv | 84 | LOW | - | 4 | 39 | yes |
| 29 | cors | 84 | LOW | - | 3 | 60 | yes |
| 30 | body-parser | 84 | LOW | - | 3 | 76 | yes |
| 31 | jsonwebtoken | 84 | LOW | - | 4 | 110 | yes |
| 32 | mongoose | 84 | LOW | - | 4 | 0 | yes |
| 33 | firebase | 84 | LOW | - | 4 | 5 | yes |
| 34 | serve-static | 84 | LOW | - | 3 | 98 | yes |
| 35 | zustand | 84 | LOW | - | 3 | 8 | yes |
| 36 | playwright | 84 | LOW | - | 4 | 45 | yes |
| 37 | marked | 84 | LOW | - | 4 | 3 | yes |
| 38 | pixi.js | 84 | LOW | - | 4 | 8 | yes |
| 39 | bcrypt | 82 | LOW | - | 5 | 316 | yes |
| 40 | echarts | 82 | LOW | - | 9 | 237 | yes |
| 41 | react | 80 | LOW | - | 2 | 56 | yes |
| 42 | commander | 80 | LOW | - | 2 | 52 | yes |
| 43 | underscore | 80 | LOW | - | 2 | 33 | yes |
| 44 | eslint | 80 | LOW | - | 2 | 3 | yes |
| 45 | next | 80 | LOW | - | 2 | 3 | yes |
| 46 | vue | 80 | LOW | - | 2 | 15 | yes |
| 47 | sass | 80 | LOW | - | 2 | 13 | yes |
| 48 | aws-sdk | 80 | LOW | 1 | 2 | 104 | yes |
| 49 | socket.io | 80 | LOW | - | 2 | 90 | yes |
| 50 | prisma | 80 | LOW | - | 2 | 12 | yes |
| 51 | ajv | 80 | LOW | - | 2 | 37 | yes |
| 52 | immutable | 80 | LOW | - | 2 | 20 | yes |
| 53 | immer | 80 | LOW | - | 2 | 42 | yes |
| 54 | three | 80 | LOW | - | 2 | 24 | yes |
| 55 | puppeteer | 80 | LOW | - | 2 | 5 | yes |
| 56 | cheerio | 80 | LOW | - | 2 | 59 | yes |
| 57 | jotai | 80 | LOW | - | 2 | 0 | yes |
| 58 | glob | 78 | MEDIUM | - | 1 | 32 | yes |
| 59 | rimraf | 78 | MEDIUM | - | 1 | 36 | yes |
| 60 | chai | 78 | MEDIUM | - | 1 | 91 | yes |
| 61 | nodemon | 78 | MEDIUM | - | 1 | 31 | yes |
| 62 | pm2 | 78 | MEDIUM | 1 | 1 | 118 | yes |
| 63 | bootstrap | 78 | MEDIUM | - | 3 | 209 | yes |
| 64 | postcss | 78 | MEDIUM | - | 1 | 21 | yes |
| 65 | pg | 78 | MEDIUM | - | 1 | 19 | yes |
| 66 | mysql2 | 78 | MEDIUM | - | 1 | 8 | yes |
| 67 | compression | 78 | MEDIUM | - | 3 | 249 | yes |
| 68 | zod | 78 | MEDIUM | - | 1 | 60 | yes |
| 69 | dayjs | 78 | MEDIUM | - | 1 | 12 | yes |
| 70 | sharp | 78 | MEDIUM | - | 1 | 137 | yes |
| 71 | gsap | 78 | MEDIUM | - | 1 | 101 | yes |
| 72 | lottie-web | 78 | MEDIUM | - | 4 | 306 | yes |
| 73 | async | 76 | MEDIUM | - | 5 | 581 | yes |
| 74 | prismjs | 76 | MEDIUM | - | 8 | 378 | yes |
| 75 | debug | 75 | MEDIUM | - | 2 | 191 | yes |
| 76 | uuid | 75 | MEDIUM | - | 2 | 196 | yes |
| 77 | yargs | 75 | MEDIUM | - | 2 | 300 | yes |
| 78 | morgan | 75 | MEDIUM | - | 2 | 249 | yes |
| 79 | chalk | 73 | MEDIUM | - | 1 | 196 | yes |
| 80 | cookie-parser | 73 | MEDIUM | - | 3 | 531 | yes |
| 81 | rxjs | 73 | MEDIUM | - | 3 | 395 | yes |
| 82 | highlight.js | 73 | MEDIUM | - | 4 | 453 | yes |
| 83 | fp-ts | 73 | MEDIUM | - | 1 | 217 | yes |
| 84 | sirv | 73 | MEDIUM | - | 1 | 201 | yes |
| 85 | moment | 71 | MEDIUM | - | 5 | 818 | yes |
| 86 | redux | 71 | MEDIUM | - | 6 | 821 | yes |
| 87 | helmet | 70 | MEDIUM | - | 2 | 371 | yes |
| 88 | restify | 66 | MEDIUM | - | 15 | 1123 | yes |
| 89 | date-fns | 65 | MEDIUM | - | 1 | 553 | yes |
| 90 | d3 | 64 | MEDIUM | - | 2 | 741 | yes |
| 91 | minimist | 62 | MEDIUM | - | 3 | 1138 | yes |
| 92 | recoil | 62 | MEDIUM | - | 3 | 1118 | yes |
| 93 | hapi | 62 | MEDIUM | 1 | 4 | 2604 | yes |
| 94 | mkdirp | 59 | HIGH | - | 1 | 1064 | yes |
| 95 | passport | 59 | HIGH | - | 1 | 847 | yes |
| 96 | bluebird | 54 | HIGH | - | 1 | 2307 | yes |
| 97 | micro | 54 | HIGH | - | 1 | 1213 | yes |
| 98 | polka | 54 | HIGH | - | 1 | 2600 | yes |

### crates.io (Rust) - Top 100

| # | Package | Score | Risk | CVEs | Maintainers | Days Since Release | Source |
|---|---------|-------|------|------|-------------|-------------------|--------|
| 1 | syn | 77 | MEDIUM | - | 0 | 32 | yes |
| 2 | hashbrown | 77 | MEDIUM | - | 0 | 123 | yes |
| 3 | bitflags | 77 | MEDIUM | - | 0 | 38 | yes |
| 4 | getrandom | 77 | MEDIUM | - | 0 | 21 | yes |
| 5 | proc-macro2 | 77 | MEDIUM | - | 0 | 61 | yes |
| 6 | libc | 77 | MEDIUM | - | 0 | 16 | yes |
| 7 | rand_core | 77 | MEDIUM | - | 0 | 51 | yes |
| 8 | quote | 77 | MEDIUM | - | 0 | 20 | yes |
| 9 | rand | 77 | MEDIUM | - | 0 | 44 | yes |
| 10 | regex-syntax | 77 | MEDIUM | - | 0 | 27 | yes |
| 11 | indexmap | 77 | MEDIUM | - | 0 | 75 | yes |
| 12 | cfg-if | 77 | MEDIUM | - | 0 | 160 | yes |
| 13 | serde | 77 | MEDIUM | - | 0 | 177 | yes |
| 14 | rand_chacha | 77 | MEDIUM | - | 0 | 50 | yes |
| 15 | thiserror-impl | 77 | MEDIUM | - | 0 | 64 | yes |
| 16 | thiserror | 77 | MEDIUM | - | 0 | 64 | yes |
| 17 | memchr | 77 | MEDIUM | - | 0 | 45 | yes |
| 18 | serde_derive | 77 | MEDIUM | - | 0 | 177 | yes |
| 19 | unicode-ident | 77 | MEDIUM | - | 0 | 36 | yes |
| 20 | itoa | 77 | MEDIUM | - | 0 | 4 | yes |
| 21 | windows-sys | 77 | MEDIUM | - | 0 | 168 | yes |
| 22 | serde_json | 77 | MEDIUM | - | 0 | 76 | yes |
| 23 | regex-automata | 77 | MEDIUM | - | 0 | 49 | yes |
| 24 | log | 77 | MEDIUM | - | 0 | 111 | yes |
| 25 | once_cell | 77 | MEDIUM | - | 0 | 12 | yes |
| 26 | cc | 77 | MEDIUM | - | 0 | 10 | yes |
| 27 | regex | 77 | MEDIUM | - | 0 | 49 | yes |
| 28 | ryu | 77 | MEDIUM | - | 0 | 43 | yes |
| 29 | clap | 77 | MEDIUM | - | 0 | 11 | yes |
| 30 | smallvec | 77 | MEDIUM | - | 0 | 127 | yes |
| 31 | aho-corasick | 77 | MEDIUM | - | 0 | 146 | yes |
| 32 | parking_lot_core | 77 | MEDIUM | - | 0 | 171 | yes |
| 33 | socket2 | 77 | MEDIUM | - | 0 | 18 | yes |
| 34 | parking_lot | 77 | MEDIUM | - | 0 | 171 | yes |
| 35 | rustix | 77 | MEDIUM | - | 0 | 29 | yes |
| 36 | windows_x86_64_msvc | 77 | MEDIUM | - | 0 | 168 | yes |
| 37 | linux-raw-sys | 77 | MEDIUM | - | 0 | 91 | yes |
| 38 | bytes | 77 | MEDIUM | - | 0 | 48 | yes |
| 39 | lock_api | 77 | MEDIUM | - | 0 | 171 | yes |
| 40 | windows_x86_64_gnu | 77 | MEDIUM | - | 0 | 168 | yes |
| 41 | mio | 77 | MEDIUM | - | 0 | 110 | yes |
| 42 | windows_aarch64_msvc | 77 | MEDIUM | - | 0 | 168 | yes |
| 43 | windows_i686_msvc | 77 | MEDIUM | - | 0 | 168 | yes |
| 44 | windows_i686_gnu | 77 | MEDIUM | - | 0 | 168 | yes |
| 45 | pin-project-lite | 77 | MEDIUM | - | 0 | 24 | yes |
| 46 | digest | 77 | MEDIUM | - | 0 | 10 | yes |
| 47 | http | 77 | MEDIUM | - | 0 | 119 | yes |
| 48 | windows-targets | 77 | MEDIUM | - | 0 | 168 | yes |
| 49 | anyhow | 77 | MEDIUM | - | 0 | 32 | yes |
| 50 | time | 77 | MEDIUM | - | 0 | 47 | yes |
| 51 | block-buffer | 77 | MEDIUM | - | 0 | 28 | yes |
| 52 | miniz_oxide | 77 | MEDIUM | - | 0 | 11 | yes |
| 53 | tokio | 77 | MEDIUM | - | 0 | 21 | yes |
| 54 | windows_x86_64_gnullvm | 77 | MEDIUM | - | 0 | 168 | yes |
| 55 | windows_aarch64_gnullvm | 77 | MEDIUM | - | 0 | 168 | yes |
| 56 | hyper | 77 | MEDIUM | - | 0 | 130 | yes |
| 57 | rustls | 77 | MEDIUM | - | 0 | 27 | yes |
| 58 | url | 77 | MEDIUM | - | 0 | 77 | yes |
| 59 | slab | 77 | MEDIUM | - | 0 | 52 | yes |
| 60 | toml | 77 | MEDIUM | - | 0 | 0 | yes |
| 61 | generic-array | 77 | MEDIUM | - | 0 | 150 | yes |
| 62 | futures-core | 77 | MEDIUM | - | 0 | 37 | yes |
| 63 | futures-util | 77 | MEDIUM | - | 0 | 37 | yes |
| 64 | futures-task | 77 | MEDIUM | - | 0 | 37 | yes |
| 65 | tracing-core | 77 | MEDIUM | - | 0 | 95 | yes |
| 66 | sha2 | 77 | MEDIUM | - | 0 | 49 | yes |
| 67 | clap_lex | 77 | MEDIUM | - | 0 | 11 | yes |
| 68 | typenum | 77 | MEDIUM | - | 0 | 173 | yes |
| 69 | futures-sink | 77 | MEDIUM | - | 0 | 37 | yes |
| 70 | tracing | 77 | MEDIUM | - | 0 | 95 | yes |
| 71 | unicode-width | 77 | MEDIUM | - | 0 | 168 | yes |
| 72 | futures-channel | 77 | MEDIUM | - | 0 | 37 | yes |
| 73 | h2 | 77 | MEDIUM | - | 0 | 77 | yes |
| 74 | chrono | 77 | MEDIUM | - | 0 | 29 | yes |
| 75 | tempfile | 77 | MEDIUM | - | 0 | 13 | yes |
| 76 | futures-io | 77 | MEDIUM | - | 0 | 37 | yes |
| 77 | tokio-util | 77 | MEDIUM | - | 0 | 79 | yes |
| 78 | nix | 77 | MEDIUM | - | 0 | 24 | yes |
| 79 | autocfg | 73 | MEDIUM | - | 0 | 279 | yes |
| 80 | semver | 73 | MEDIUM | - | 0 | 190 | yes |
| 81 | idna | 73 | MEDIUM | - | 0 | 215 | yes |
| 82 | percent-encoding | 73 | MEDIUM | - | 0 | 215 | yes |
| 83 | ahash | 73 | MEDIUM | - | 0 | 320 | yes |
| 84 | base64 | 69 | MEDIUM | - | 0 | 692 | yes |
| 85 | itertools | 69 | MEDIUM | - | 0 | 448 | yes |
| 86 | strsim | 69 | MEDIUM | - | 0 | 720 | yes |
| 87 | lazy_static | 69 | MEDIUM | - | 0 | 640 | yes |
| 88 | num-traits | 69 | MEDIUM | - | 0 | 689 | yes |
| 89 | version_check | 69 | MEDIUM | - | 0 | 606 | yes |
| 90 | either | 69 | MEDIUM | - | 0 | 383 | yes |
| 91 | ppv-lite86 | 69 | MEDIUM | - | 0 | 379 | yes |
| 92 | crossbeam-utils | 69 | MEDIUM | - | 0 | 463 | yes |
| 93 | fastrand | 69 | MEDIUM | - | 0 | 470 | yes |
| 94 | http-body | 69 | MEDIUM | - | 0 | 619 | yes |
| 95 | memoffset | 69 | MEDIUM | - | 0 | 727 | yes |
| 96 | heck | 65 | MEDIUM | - | 0 | 741 | yes |
| 97 | scopeguard | 65 | MEDIUM | - | 0 | 981 | yes |
| 98 | byteorder | 65 | MEDIUM | - | 0 | 900 | yes |
| 99 | fnv | 61 | MEDIUM | - | 0 | 2139 | yes |
| 100 | pin-utils | 61 | MEDIUM | - | 0 | 2161 | yes |

### Go Proxy - Top 50

| # | Package | Score | Risk | CVEs | Maintainers | Days Since Release | Source |
|---|---------|-------|------|------|-------------|-------------------|--------|
| 1 | github.com/spf13/cobra | 74 | MEDIUM | - | 0 | 110 | yes |
| 2 | github.com/gin-gonic/gin | 74 | MEDIUM | - | 0 | 24 | yes |
| 3 | github.com/sirupsen/logrus | 74 | MEDIUM | - | 0 | 152 | yes |
| 4 | github.com/lib/pq | 74 | MEDIUM | - | 0 | 5 | yes |
| 5 | github.com/redis/go-redis/v9 | 74 | MEDIUM | - | 0 | 35 | yes |
| 6 | github.com/labstack/echo/v4 | 74 | MEDIUM | - | 0 | 30 | yes |
| 7 | github.com/gofiber/fiber/v2 | 74 | MEDIUM | - | 0 | 27 | yes |
| 8 | github.com/jackc/pgx/v5 | 74 | MEDIUM | - | 0 | 1 | yes |
| 9 | github.com/go-chi/chi/v5 | 74 | MEDIUM | - | 0 | 47 | yes |
| 10 | github.com/fatih/color | 74 | MEDIUM | - | 0 | 4 | yes |
| 11 | github.com/hashicorp/consul | 74 | MEDIUM | - | 0 | 26 | yes |
| 12 | github.com/hashicorp/vault | 74 | MEDIUM | - | 0 | 19 | yes |
| 13 | github.com/pelletier/go-toml/v2 | 74 | MEDIUM | - | 0 | 0 | yes |
| 14 | github.com/uber-go/zap | 74 | MEDIUM | - | 0 | 124 | yes |
| 15 | github.com/nats-io/nats.go | 74 | MEDIUM | - | 0 | 28 | yes |
| 16 | github.com/segmentio/kafka-go | 74 | MEDIUM | - | 0 | 67 | yes |
| 17 | github.com/goccy/go-yaml | 74 | MEDIUM | - | 0 | 75 | yes |
| 18 | github.com/docker/docker | 74 | MEDIUM | - | 0 | 138 | yes |
| 19 | github.com/hashicorp/terraform | 74 | MEDIUM | - | 0 | 13 | yes |
| 20 | github.com/kubernetes/kubernetes | 74 | MEDIUM | - | 0 | 5 | yes |
| 21 | github.com/stretchr/testify | 68 | MEDIUM | - | 0 | 209 | yes |
| 22 | github.com/spf13/viper | 68 | MEDIUM | - | 0 | 196 | yes |
| 23 | github.com/go-sql-driver/mysql | 68 | MEDIUM | - | 0 | 284 | yes |
| 24 | github.com/aws/aws-sdk-go | 68 | MEDIUM | 2 | 0 | 235 | yes |
| 25 | github.com/prometheus/client_golang | 68 | MEDIUM | - | 0 | 199 | yes |
| 26 | github.com/urfave/cli/v2 | 68 | MEDIUM | - | 0 | 283 | yes |
| 27 | github.com/jmoiron/sqlx | 63 | MEDIUM | - | 0 | 708 | yes |
| 28 | github.com/rs/zerolog | 63 | MEDIUM | - | 0 | 368 | yes |
| 29 | google.golang.org/grpc | 61 | MEDIUM | - | 0 | 6 | no |
| 30 | google.golang.org/protobuf | 61 | MEDIUM | - | 0 | 102 | no |
| 31 | golang.org/x/sync | 61 | MEDIUM | - | 0 | 28 | no |
| 32 | golang.org/x/net | 61 | MEDIUM | - | 0 | 12 | no |
| 33 | golang.org/x/text | 61 | MEDIUM | - | 0 | 14 | no |
| 34 | golang.org/x/crypto | 61 | MEDIUM | - | 0 | 12 | no |
| 35 | golang.org/x/sys | 61 | MEDIUM | - | 0 | 21 | no |
| 36 | golang.org/x/oauth2 | 61 | MEDIUM | - | 0 | 40 | no |
| 37 | golang.org/x/mod | 61 | MEDIUM | - | 0 | 14 | no |
| 38 | golang.org/x/tools | 61 | MEDIUM | - | 0 | 12 | no |
| 39 | github.com/gorilla/mux | 57 | HIGH | - | 0 | 888 | yes |
| 40 | github.com/google/uuid | 57 | HIGH | - | 0 | 790 | yes |
| 41 | github.com/cenkalti/backoff/v4 | 57 | HIGH | - | 0 | 811 | yes |
| 42 | github.com/go-kit/kit | 57 | HIGH | - | 0 | 1029 | yes |
| 43 | github.com/streadway/amqp | 57 | HIGH | - | 0 | 1009 | yes |
| 44 | github.com/golang/protobuf | 57 | HIGH | - | 0 | 748 | yes |
| 45 | github.com/go-playground/validator | 52 | HIGH | - | 0 | 2281 | yes |
| 46 | github.com/pkg/errors | 52 | HIGH | - | 0 | 2260 | yes |
| 47 | github.com/mitchellh/mapstructure | 52 | HIGH | - | 0 | 1433 | yes |
| 48 | github.com/olivere/elastic/v7 | 52 | HIGH | - | 0 | 1466 | yes |
| 49 | github.com/grpc-ecosystem/grpc-gateway | 52 | HIGH | - | 0 | 1973 | yes |
| 50 | github.com/confluentinc/confluent-kafka-go | 52 | HIGH | - | 0 | 1329 | yes |

### Packagist (PHP) - Top 100

| # | Package | Score | Risk | CVEs | Maintainers | Days Since Release | Source |
|---|---------|-------|------|------|-------------|-------------------|--------|
| 1 | guzzlehttp/psr7 | 87 | LOW | - | 7 | 13 | yes |
| 2 | sebastian/exporter | 87 | LOW | - | 5 | 46 | yes |
| 3 | sebastian/comparator | 84 | LOW | - | 4 | 46 | yes |
| 4 | sebastian/recursion-context | 84 | LOW | - | 3 | 46 | yes |
| 5 | symfony/css-selector | 84 | LOW | - | 3 | 35 | yes |
| 6 | symfony/uid | 84 | LOW | - | 3 | 79 | yes |
| 7 | guzzlehttp/guzzle | 82 | LOW | - | 7 | 212 | yes |
| 8 | doctrine/inflector | 82 | LOW | - | 5 | 225 | yes |
| 9 | symfony/console | 80 | LOW | - | 2 | 17 | yes |
| 10 | symfony/finder | 80 | LOW | - | 2 | 54 | yes |
| 11 | symfony/string | 80 | LOW | - | 2 | 43 | yes |
| 12 | symfony/process | 80 | LOW | - | 2 | 56 | yes |
| 13 | symfony/event-dispatcher | 80 | LOW | - | 2 | 78 | yes |
| 14 | symfony/var-dumper | 80 | LOW | - | 2 | 37 | yes |
| 15 | symfony/http-foundation | 80 | LOW | - | 2 | 18 | yes |
| 16 | symfony/mime | 80 | LOW | - | 2 | 18 | yes |
| 17 | symfony/translation | 80 | LOW | - | 2 | 35 | yes |
| 18 | symfony/http-kernel | 80 | LOW | - | 2 | 17 | yes |
| 19 | symfony/error-handler | 80 | LOW | - | 2 | 60 | yes |
| 20 | symfony/routing | 80 | LOW | - | 2 | 26 | yes |
| 21 | sebastian/diff | 80 | LOW | - | 2 | 46 | yes |
| 22 | symfony/yaml | 80 | LOW | - | 2 | 43 | yes |
| 23 | symfony/filesystem | 80 | LOW | - | 2 | 26 | yes |
| 24 | nesbot/carbon | 80 | LOW | - | 2 | 12 | yes |
| 25 | symfony/mailer | 80 | LOW | - | 2 | 26 | yes |
| 26 | vlucas/phpdotenv | 80 | LOW | - | 2 | 86 | yes |
| 27 | phpoption/phpoption | 80 | LOW | - | 2 | 86 | yes |
| 28 | webmozart/assert | 80 | LOW | - | 2 | 25 | yes |
| 29 | nette/utils | 80 | LOW | - | 2 | 39 | yes |
| 30 | symfony/clock | 80 | LOW | - | 2 | 131 | yes |
| 31 | laravel/serializable-closure | 80 | LOW | - | 2 | 31 | yes |
| 32 | nette/schema | 80 | LOW | - | 2 | 29 | yes |
| 33 | guzzlehttp/promises | 78 | MEDIUM | - | 4 | 213 | yes |
| 34 | nikic/php-parser | 78 | MEDIUM | - | 1 | 108 | yes |
| 35 | monolog/monolog | 78 | MEDIUM | - | 1 | 81 | yes |
| 36 | phpunit/phpunit | 78 | MEDIUM | - | 1 | 34 | yes |
| 37 | phpunit/php-file-iterator | 78 | MEDIUM | - | 1 | 46 | yes |
| 38 | phpunit/php-code-coverage | 78 | MEDIUM | - | 1 | 46 | yes |
| 39 | sebastian/environment | 78 | MEDIUM | - | 1 | 2 | yes |
| 40 | theseer/tokenizer | 78 | MEDIUM | - | 1 | 106 | yes |
| 41 | sebastian/global-state | 78 | MEDIUM | - | 1 | 46 | yes |
| 42 | sebastian/version | 78 | MEDIUM | - | 1 | 46 | yes |
| 43 | phpunit/php-timer | 78 | MEDIUM | - | 1 | 46 | yes |
| 44 | phpunit/php-text-template | 78 | MEDIUM | - | 1 | 46 | yes |
| 45 | sebastian/object-enumerator | 78 | MEDIUM | - | 1 | 46 | yes |
| 46 | sebastian/object-reflector | 78 | MEDIUM | - | 1 | 46 | yes |
| 47 | sebastian/type | 78 | MEDIUM | - | 1 | 46 | yes |
| 48 | sebastian/cli-parser | 78 | MEDIUM | - | 1 | 46 | yes |
| 49 | sebastian/complexity | 78 | MEDIUM | - | 1 | 46 | yes |
| 50 | sebastian/lines-of-code | 78 | MEDIUM | - | 1 | 46 | yes |
| 51 | phpunit/php-invoker | 78 | MEDIUM | - | 1 | 46 | yes |
| 52 | league/flysystem | 78 | MEDIUM | - | 1 | 26 | yes |
| 53 | dragonmantank/cron-expression | 78 | MEDIUM | - | 1 | 143 | yes |
| 54 | psy/psysh | 78 | MEDIUM | - | 1 | 1 | yes |
| 55 | graham-campbell/result-type | 78 | MEDIUM | - | 1 | 86 | yes |
| 56 | league/flysystem-local | 78 | MEDIUM | - | 1 | 59 | yes |
| 57 | league/commonmark | 78 | MEDIUM | - | 1 | 5 | yes |
| 58 | tijsverkoyen/css-to-inline-styles | 78 | MEDIUM | - | 1 | 112 | yes |
| 59 | composer/semver | 78 | MEDIUM | - | 3 | 215 | yes |
| 60 | laravel/framework | 78 | MEDIUM | - | 1 | 5 | yes |
| 61 | symfony/service-contracts | 75 | MEDIUM | - | 2 | 252 | yes |
| 62 | symfony/polyfill-intl-grapheme | 75 | MEDIUM | - | 2 | 270 | yes |
| 63 | symfony/translation-contracts | 75 | MEDIUM | - | 2 | 252 | yes |
| 64 | symfony/polyfill-php83 | 75 | MEDIUM | - | 2 | 259 | yes |
| 65 | symfony/polyfill-php84 | 75 | MEDIUM | - | 2 | 273 | yes |
| 66 | paragonie/constant_time_encoding | 75 | MEDIUM | - | 2 | 180 | yes |
| 67 | ramsey/uuid | 74 | MEDIUM | - | 0 | 100 | yes |
| 68 | brick/math | 74 | MEDIUM | - | 0 | 7 | yes |
| 69 | doctrine/deprecations | 74 | MEDIUM | - | 0 | 45 | yes |
| 70 | symfony/polyfill-php80 | 73 | MEDIUM | - | 3 | 446 | yes |
| 71 | symfony/polyfill-intl-idn | 73 | MEDIUM | - | 3 | 559 | yes |
| 72 | dflydev/dot-access-data | 73 | MEDIUM | - | 4 | 624 | yes |
| 73 | symfony/deprecation-contracts | 70 | MEDIUM | - | 2 | 544 | yes |
| 74 | symfony/polyfill-mbstring | 70 | MEDIUM | - | 2 | 456 | yes |
| 75 | symfony/polyfill-intl-normalizer | 70 | MEDIUM | - | 2 | 561 | yes |
| 76 | symfony/polyfill-ctype | 70 | MEDIUM | - | 2 | 561 | yes |
| 77 | symfony/event-dispatcher-contracts | 70 | MEDIUM | - | 2 | 544 | yes |
| 78 | symfony/polyfill-uuid | 70 | MEDIUM | - | 2 | 561 | yes |
| 79 | doctrine/lexer | 68 | MEDIUM | - | 3 | 778 | yes |
| 80 | myclabs/deep-copy | 68 | MEDIUM | - | 0 | 235 | yes |
| 81 | phar-io/manifest | 68 | MEDIUM | - | 3 | 751 | yes |
| 82 | psr/log | 65 | MEDIUM | - | 1 | 559 | yes |
| 83 | psr/http-factory | 65 | MEDIUM | - | 1 | 708 | yes |
| 84 | egulias/email-validator | 65 | MEDIUM | - | 1 | 382 | yes |
| 85 | ramsey/collection | 65 | MEDIUM | - | 1 | 367 | yes |
| 86 | league/mime-type-detection | 65 | MEDIUM | - | 1 | 549 | yes |
| 87 | sebastian/code-unit-reverse-lookup | 65 | MEDIUM | - | 1 | 629 | yes |
| 88 | sebastian/code-unit | 65 | MEDIUM | - | 1 | 370 | yes |
| 89 | voku/portable-ascii | 65 | MEDIUM | - | 1 | 488 | yes |
| 90 | composer/pcre | 65 | MEDIUM | - | 1 | 496 | yes |
| 91 | phar-io/version | 62 | MEDIUM | - | 3 | 1492 | yes |
| 92 | psr/http-message | 59 | HIGH | - | 1 | 1085 | yes |
| 93 | psr/http-client | 59 | HIGH | - | 1 | 912 | yes |
| 94 | carbonphp/carbon-doctrine-types | 59 | HIGH | - | 1 | 773 | yes |
| 95 | ralouphie/getallheaders | 54 | HIGH | - | 1 | 2573 | yes |
| 96 | psr/container | 54 | HIGH | - | 1 | 1599 | yes |
| 97 | psr/event-dispatcher | 54 | HIGH | - | 1 | 2631 | yes |
| 98 | psr/simple-cache | 54 | HIGH | - | 1 | 1607 | yes |
| 99 | psr/clock | 54 | HIGH | - | 1 | 1214 | yes |
| 100 | psr/cache | 54 | HIGH | - | 1 | 1874 | yes |
