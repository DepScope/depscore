# depscope Server — HTTP API, Web UI, Lambda & Docker

**Date:** 2026-03-22
**Status:** Approved
**Parent spec:** [depscope design spec](2026-03-20-depscope-design.md)

---

## Overview

Add an HTTP server to depscope that serves a web UI for scanning remote repositories. The server renders HTML pages with Go templates, embedded in the binary via `//go:embed`. It runs as a Lambda Function URL on AWS, or locally via Docker.

Users enter a GitHub/GitLab URL on the landing page, get a shareable results URL, and can click through packages to see score breakdowns, dependency trees, and vulnerability details in a slide-in side panel.

---

## Architecture

```
depscope binary
  cmd/depscope/
    ├── main.go         — existing CLI root
    ├── scan.go         — existing CLI scan
    ├── server.go       — NEW: `depscope server` command
    └── ...
  cmd/lambda/
    └── main.go         — Lambda adapter wrapping net/http

  internal/server/
    ├── server.go       — NewServer(), routes, middleware
    ├── handlers.go     — landing, submitScan, scanStatus, results, packageDetail
    └── store/
        ├── store.go    — ScanStore interface
        ├── dynamo.go   — DynamoDB implementation
        └── memory.go   — in-memory (local/Docker)

  internal/web/
    ├── embed.go        — //go:embed templates/* static/*
    ├── templates/
    │   ├── layout.html     — base layout (head, nav, footer)
    │   ├── landing.html    — URL input form
    │   ├── scanning.html   — loading animation + polling JS
    │   └── results.html    — score gauge, package table, side panel
    └── static/
        ├── style.css       — dark theme, risk colors, animations
        └── app.js          — polling, side panel, expand/collapse
```

---

## HTTP Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| `GET` | `/` | `handleLanding` | Landing page with URL input |
| `POST` | `/scan` | `handleSubmitScan` | Creates scan job, redirects to `/scan/{id}` |
| `GET` | `/scan/{id}` | `handleScanPage` | Results page (or scanning animation if in progress) |
| `GET` | `/api/scan/{id}` | `handleScanStatus` | JSON: `{status, result?}` for polling |
| `GET` | `/api/package/{eco}/{name...}/{version}` | `handlePackageDetail` | JSON: single package score breakdown for side panel. Uses Go 1.22+ wildcard `{name...}` to handle scoped npm packages like `@angular/core`. |
| `GET` | `/static/*` | `http.FileServer` | Embedded CSS/JS assets |

---

## Scan Flow

1. User enters a repository URL on the landing page, clicks "Scan"
2. `POST /scan` generates a random scan ID (nanoid), stores `{id, url, status: "queued"}` in the scan store, starts the scan pipeline in a goroutine, and redirects (`303 See Other`) to `/scan/{id}`
3. `/scan/{id}` checks scan status:
   - If `queued` or `running`: renders `scanning.html` with a CSS loading animation. Vanilla JS polls `GET /api/scan/{id}` every 2 seconds.
   - If `complete`: renders `results.html` with the full report.
   - If `failed`: renders error state with the error message.
4. Poll response: `{"status": "complete"}` triggers JS to reload the page, which now renders the full results.
5. Results page is shareable — anyone with the URL sees the cached result.

### Scan Pipeline (reuses existing engine)

```
URL → resolve.DetectResolver → resolver.Resolve
    → manifest.DetectEcosystemFromFiles → parser.ParseFiles
    → registry.FetchAll → core.Score → core.Propagate
    → store.SaveResult
```

The exact same pipeline as the CLI `scan` command, just writing the result to the store instead of stdout.

---

## Web UI Design

### Theme

- **Dark background:** `#0d1117` (dark navy)
- **Surface:** `#161b22` (cards, table rows)
- **Border:** `#30363d`
- **Text:** `#e6edf3` (primary), `#7d8590` (secondary)
- **Accent:** `#58a6ff` (links, interactive elements)

### Risk Colors

| Risk | Color | Hex |
|------|-------|-----|
| LOW | Green | `#3fb950` |
| MEDIUM | Yellow | `#d29922` |
| HIGH | Orange | `#db6d28` |
| CRITICAL | Red | `#f85149` |

### Landing Page

- Centered layout, max-width 600px
- depscope logo (text-based, styled with CSS)
- Large URL input field with "Scan" button
- Subtle tagline: "Supply chain reputation scoring for your dependencies"
- Profile selector (dropdown, default: `enterprise`)
- Below: recent public scans (if any) as clickable cards

### Scanning Page

- Centered pulsing logo animation (CSS `@keyframes`)
- "Scanning {url}..." text
- Animated dots or spinner
- Auto-refreshes when scan completes (JS polling)

### Results Page

**Header section:**
- Repository name + URL
- Overall pass/fail badge
- Profile used (hobby/opensource/enterprise)
- Scan timestamp
- Circular SVG score gauge showing the lowest package score, color-coded by risk level

**Package table:**
- Sortable columns: Package, Version, Score, Risk, Transitive Risk, Constraint
- Each row has a colored risk badge pill
- Rows are clickable — opens the side panel
- Alternating row colors for readability
- Hover highlight

**Issue summary:**
- Below the table: collapsible list of all issues grouped by severity
- Count badges: "3 CRITICAL, 5 HIGH, 12 MEDIUM"

**Side panel (package detail):**
- Slides in from the right, 420px wide, dark overlay on the rest
- Close button (X) and click-outside-to-close
- Package name, version, ecosystem
- Overall score with risk badge
- **7-factor breakdown:** horizontal bars showing each factor's score (0-100), color-coded, with the factor weight percentage
- Direct dependencies list (clickable — loads that package in the panel)
- Vulnerabilities list (if any): CVE ID, severity, summary
- Issues for this package
- Loaded via `GET /api/package/{eco}/{name}/{version}` as JSON, rendered client-side with JS

---

## Storage Interface

```go
type ScanRequest struct {
    URL     string
    Profile string
}

type ScanJob struct {
    ID        string
    URL       string
    Profile   string
    Status    string           // "queued", "running", "complete", "failed"
    Error     string           // set if status == "failed"
    Result    *core.ScanResult // set if status == "complete"
    CreatedAt time.Time
}

type ScanStore interface {
    Create(id string, req ScanRequest) error
    UpdateStatus(id string, status string) error
    SaveResult(id string, result *core.ScanResult) error
    SaveError(id string, errMsg string) error
    Get(id string) (*ScanJob, error)
}
```

### DynamoDB Implementation

- Table: `depscope-scans`
- Partition key: `id` (string)
- Attributes: `url`, `profile`, `status`, `error`, `result` (JSON), `created_at`, `ttl`
- TTL attribute: `ttl` — set to 7 days after creation for auto-expiry
- Result JSON is gzip-compressed before storing (DynamoDB max item size is 400KB; a 1000-package scan can exceed this uncompressed)
- On-demand billing (pay per request, scales to zero)
- Free tier: 25GB storage, 25 WCU, 25 RCU

### In-Memory Implementation

- `sync.Map` storing `ScanJob` values
- No persistence — scan results lost on restart
- Used for local/Docker mode where caching is not critical
- Optional: could add SQLite later for persistent local caching

---

## Lambda Deployment

### Lambda Adapter

`cmd/lambda/main.go` wraps the standard `net/http` server with the AWS Lambda Go adapter:

```go
package main

import (
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
    "github.com/depscope/depscope/internal/server"
)

func main() {
    srv := server.New(server.Options{
        Store: store.NewDynamoStore(os.Getenv("DYNAMODB_TABLE")),
    })
    lambda.Start(httpadapter.NewV2(srv.Handler()).Handler)
}
```

### Lambda Configuration

- Runtime: `provided.al2023` (custom runtime, Go binary)
- Architecture: `arm64` (cheaper, faster)
- Memory: 512MB (enough for concurrent HTTP calls during scan)
- Timeout: 60 seconds (covers most scans; very large repos may need async)
- Function URL: enabled, auth type NONE (public)
- Environment variables:
  - `DEPSCOPE_STORE=dynamo`
  - `DYNAMODB_TABLE=depscope-scans`
  - `GITHUB_TOKEN` (optional, for authenticated GitHub API)

### Security & Rate Limiting

- **URL validation:** only accept `https://` URLs pointing to known public hosts (github.com, gitlab.com, bitbucket.org, etc.). Reject private IP ranges (10.x, 172.16-31.x, 192.168.x), localhost, and non-HTTPS URLs to prevent SSRF.
- **Lambda reserved concurrency:** set to 10 to cap costs and prevent abuse.
- **Rate limiting:** simple in-memory rate limiter for Docker mode (10 scans/IP/minute). Lambda relies on reserved concurrency as its natural rate limit.

### Infrastructure (CloudFormation)

A single `infrastructure/template.yaml` CloudFormation template creating:
- Lambda function
- Lambda Function URL
- DynamoDB table
- IAM role for Lambda → DynamoDB access

---

## Docker

### Dockerfile

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /depscope ./cmd/depscope

FROM alpine:3.19
RUN apk add --no-cache git ca-certificates
COPY --from=build /depscope /usr/local/bin/depscope
EXPOSE 8080
ENTRYPOINT ["depscope"]
CMD ["server", "--port", "8080"]
```

### Usage

```bash
# Start web UI
docker run -p 8080:8080 depscope/depscope

# Start web UI with GitHub token for better scoring
docker run -p 8080:8080 -e GITHUB_TOKEN=ghp_xxx depscope/depscope

# CLI scan with mounted project
docker run -v $(pwd):/project depscope/depscope scan /project

# CLI scan remote URL
docker run depscope/depscope scan https://github.com/psf/requests
```

---

## CLI `server` Command

```
depscope server [flags]

Flags:
  --port int        port to listen on (default 8080)
  --store string    storage backend: memory, dynamo (default "memory")
  --table string    DynamoDB table name (when --store=dynamo)
```

---

## Concurrency Model

The server has two execution modes with different concurrency strategies:

### Lambda (synchronous)

Lambda freezes the execution environment after a response is returned — goroutines launched from a handler will not complete. Therefore, **`POST /scan` runs the scan synchronously** within the same request:

1. `POST /scan` stores `status: "running"` in DynamoDB
2. Runs the full scan pipeline inline (blocks the response)
3. Stores the result in DynamoDB
4. Redirects to `/scan/{id}` which renders the complete results page immediately

The scanning animation / polling JS is **not used** in Lambda mode. The user sees the browser's loading indicator during the scan (up to 60s). If the scan completes, they land on the results page. If Lambda times out (>60s), the client gets a 504 and the DynamoDB entry stays in "running" state (cleaned up by TTL).

Future improvement: use SQS + a second Lambda for truly async scans on very large repos.

### Docker/Local (async with goroutines)

Standard Go HTTP server with goroutine-per-scan:

1. `POST /scan` stores `status: "queued"`, launches the scan in a goroutine, redirects immediately to `/scan/{id}`
2. `/scan/{id}` renders the scanning animation page
3. JS polls `GET /api/scan/{id}` every 2 seconds
4. When the goroutine completes, it updates the store to `status: "complete"`
5. Next poll triggers a page reload showing the full results

The `handleSubmitScan` handler checks `server.Mode` (lambda vs local) to decide which path to take.

---

## File Map

```
cmd/depscope/
  └── server.go              — `depscope server` cobra command

cmd/lambda/
  └── main.go                — Lambda adapter

internal/server/
  ├── server.go              — Server struct, NewServer(), routes
  ├── handlers.go            — HTTP handlers
  └── store/
      ├── store.go           — ScanStore interface, ScanJob, ScanRequest
      ├── dynamo.go          — DynamoDB implementation
      └── memory.go          — in-memory implementation

internal/web/
  ├── embed.go               — //go:embed
  ├── templates/
  │   ├── layout.html
  │   ├── landing.html
  │   ├── scanning.html
  │   └── results.html
  └── static/
      ├── style.css
      └── app.js

infrastructure/
  └── template.yaml          — CloudFormation (Lambda + DynamoDB)

Dockerfile
```

---

## Testing

- **Handlers:** `httptest.NewServer` with in-memory store, verify HTML responses contain expected elements, verify JSON API responses
- **Store:** unit tests for memory store; integration test for DynamoDB using `aws-sdk-go-v2` mock or localstack
- **Templates:** render templates with sample data, verify HTML structure
- **Lambda adapter:** test with mock Lambda event, verify response format
- **E2E:** `docker build` + `docker run` + curl to verify the full flow locally

---

## Later Phase: File Upload

Not in this spec. When added:
- `POST /scan` accepts `multipart/form-data` with manifest files
- Files are parsed in-memory (no disk needed)
- Same scoring pipeline, same results page
- New landing page section: "Or upload your manifest files"
