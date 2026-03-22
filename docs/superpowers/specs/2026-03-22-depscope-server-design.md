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
| `GET` | `/api/package/{eco}/{name}/{version}` | `handlePackageDetail` | JSON: single package score breakdown for side panel |
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
FROM golang:1.22-alpine AS build
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

### Lambda

Each Lambda invocation handles one HTTP request. Scan goroutines run within the same invocation. Lambda may be frozen between invocations — the scan must complete before the response returns.

For the web flow: `POST /scan` starts the scan synchronously and stores the result before redirecting. The scan runs within the Lambda timeout (60s). This is simpler than async — no separate worker needed.

If the scan takes >60s (very large repos), the Lambda times out and the status stays "running". The client sees a timeout error. Future improvement: use Step Functions or SQS for truly async scans.

### Docker/Local

Standard Go HTTP server with `goroutine-per-scan`. The scan runs in a goroutine, the handler returns immediately with the redirect. The polling endpoint checks the in-memory store for completion.

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
