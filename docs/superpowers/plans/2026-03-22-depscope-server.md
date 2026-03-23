# depscope Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an HTTP server to depscope with a dark-themed web UI for scanning remote repos, deployable as a Lambda Function URL or Docker container.

**Architecture:** A new `internal/server/` package handles HTTP routing and scan job management. A new `internal/web/` package embeds HTML templates and static assets via `//go:embed`. The scan pipeline is extracted from `cmd/depscope/scan.go` into a shared `internal/scanner/` package so both CLI and server reuse the same code. Storage is pluggable: in-memory for local/Docker, DynamoDB for Lambda.

**Tech Stack:** Go `net/http` (routing, Go 1.22+ patterns), `html/template` (server-rendered pages), `//go:embed` (asset embedding), `github.com/aws/aws-lambda-go` + `aws-lambda-go-api-proxy` (Lambda adapter), `github.com/aws/aws-sdk-go-v2` (DynamoDB), vanilla JS + CSS (dark theme UI)

**Spec:** `docs/superpowers/specs/2026-03-22-depscope-server-design.md`

---

## File Map

```
depscope/
├── cmd/depscope/
│   ├── scan.go              — MODIFY: extract pipeline into internal/scanner
│   └── server_cmd.go        — NEW: `depscope server` cobra command
├── cmd/lambda/
│   └── main.go              — NEW: Lambda adapter
├── internal/scanner/
│   └── scanner.go           — NEW: shared scan pipeline (extracted from scan.go)
├── internal/server/
│   ├── server.go            — NEW: Server struct, routes, NewServer()
│   ├── server_test.go       — NEW: handler tests with httptest
│   ├── handlers.go          — NEW: HTTP handlers
│   ├── validate.go          — NEW: URL validation (SSRF prevention)
│   └── validate_test.go     — NEW: URL validation tests
├── internal/server/store/
│   ├── store.go             — NEW: ScanStore interface, ScanJob, ScanRequest
│   ├── memory.go            — NEW: in-memory implementation
│   ├── memory_test.go       — NEW: store tests
│   └── dynamo.go            — NEW: DynamoDB implementation
├── internal/web/
│   ├── embed.go             — NEW: //go:embed directive
│   ├── templates/
│   │   ├── layout.html      — NEW: base HTML layout
│   │   ├── landing.html     — NEW: URL input form
│   │   ├── scanning.html    — NEW: loading animation
│   │   └── results.html     — NEW: score table + side panel
│   └── static/
│       ├── style.css        — NEW: dark theme CSS
│       └── app.js           — NEW: polling, side panel, interactions
├── infrastructure/
│   └── template.yaml        — NEW: CloudFormation
├── Dockerfile               — NEW
└── Makefile                 — MODIFY: add server/lambda build targets
```

---

## Task 1: Extract Scan Pipeline

**Files:**
- Create: `depscope/internal/scanner/scanner.go`
- Modify: `depscope/cmd/depscope/scan.go`

Extract the URL-resolve → parse → fetch → score → propagate pipeline into a reusable function.

- [ ] **Step 1: Create scanner package**

`depscope/internal/scanner/scanner.go`:
```go
package scanner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/resolve"
)

type Options struct {
	Profile  string
	MaxFiles int
}

// ScanURL resolves a remote URL and runs the full scan pipeline.
func ScanURL(ctx context.Context, url string, opts Options) (*core.ScanResult, error) {
	cfg := config.ProfileByName(opts.Profile)

	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 5000
	}

	resolver := resolve.DetectResolver(url, resolve.DetectOptions{
		GitHubToken: os.Getenv("GITHUB_TOKEN"),
		GitLabToken: os.Getenv("GITLAB_TOKEN"),
		MaxFiles:    maxFiles,
	})

	files, cleanup, err := resolver.Resolve(ctx, url)
	defer cleanup()
	if err != nil {
		return nil, fmt.Errorf("resolve: %w", err)
	}

	var pkgs []manifest.Package
	groups := groupByDirectory(files)
	for dir, group := range groups {
		filenames := make([]string, 0, len(group))
		fileMap := make(map[string][]byte)
		for _, f := range group {
			name := filepath.Base(f.Path)
			filenames = append(filenames, name)
			fileMap[name] = f.Content
		}
		eco, err := manifest.DetectEcosystemFromFiles(filenames)
		if err != nil {
			log.Printf("warning: skipping %s: %v", dir, err)
			continue
		}
		parsed, err := manifest.ParserFor(eco).ParseFiles(fileMap)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", dir, err)
		}
		pkgs = append(pkgs, parsed...)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found in %s", url)
	}

	return scorePipeline(pkgs, cfg)
}

// ScanDir runs the scan pipeline on a local directory.
func ScanDir(dir string, opts Options) (*core.ScanResult, error) {
	cfg := config.ProfileByName(opts.Profile)

	eco, err := manifest.DetectEcosystem(dir)
	if err != nil {
		return nil, fmt.Errorf("detect ecosystem: %w", err)
	}
	pkgs, err := manifest.ParserFor(eco).Parse(dir)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return scorePipeline(pkgs, cfg)
}

func scorePipeline(pkgs []manifest.Package, cfg config.Config) (*core.ScanResult, error) {
	fetchers := registry.FetchersByEcosystem{
		"PyPI":      registry.NewPyPIClient(),
		"npm":       registry.NewNPMClient(),
		"crates.io": registry.NewCratesClient(),
		"Go":        registry.NewGoProxyClient(),
	}

	fetchResults := registry.FetchAll(pkgs, fetchers, int64(cfg.Concurrency))

	scored := make([]core.PackageResult, 0, len(pkgs))
	for _, pkg := range pkgs {
		fr := fetchResults[pkg.Key()]
		scored = append(scored, core.Score(pkg, fr, cfg.Weights))
	}

	depsMap := manifest.BuildDepsMap(pkgs)
	scored = core.Propagate(scored, depsMap)

	directCount, transitiveCount := 0, 0
	for _, pkg := range pkgs {
		if pkg.Depth <= 1 {
			directCount++
		} else {
			transitiveCount++
		}
	}

	var allIssues []core.Issue
	for _, r := range scored {
		allIssues = append(allIssues, r.Issues...)
	}

	result := &core.ScanResult{
		Profile:        cfg.Profile,
		PassThreshold:  cfg.PassThreshold,
		DirectDeps:     directCount,
		TransitiveDeps: transitiveCount,
		Packages:       scored,
		AllIssues:      allIssues,
	}
	return result, nil
}

func groupByDirectory(files []resolve.ManifestFile) map[string][]resolve.ManifestFile {
	groups := make(map[string][]resolve.ManifestFile)
	for _, f := range files {
		dir := filepath.Dir(f.Path)
		groups[dir] = append(groups[dir], f)
	}
	return groups
}
```

- [ ] **Step 2: Refactor scan.go to use scanner package**

Replace the inline pipeline in `cmd/depscope/scan.go` with calls to `scanner.ScanURL` and `scanner.ScanDir`. Keep the CLI flags, config loading, and output formatting in `scan.go`.

- [ ] **Step 3: Run all tests — expect PASS**

```bash
go test ./... -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/scanner/ cmd/depscope/scan.go
git commit -m "refactor: extract scan pipeline into internal/scanner for reuse"
```

---

## Task 2: Storage Interface + Memory Store

**Files:**
- Create: `depscope/internal/server/store/store.go`
- Create: `depscope/internal/server/store/memory.go`
- Create: `depscope/internal/server/store/memory_test.go`

- [ ] **Step 1: Write tests**

`depscope/internal/server/store/memory_test.go`:
```go
package store_test

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStoreCreateAndGet(t *testing.T) {
	s := store.NewMemoryStore()
	require.NoError(t, s.Create("abc", store.ScanRequest{URL: "https://github.com/a/b", Profile: "enterprise"}))
	job, err := s.Get("abc")
	require.NoError(t, err)
	assert.Equal(t, "abc", job.ID)
	assert.Equal(t, "queued", job.Status)
	assert.Equal(t, "https://github.com/a/b", job.URL)
}

func TestMemoryStoreUpdateStatus(t *testing.T) {
	s := store.NewMemoryStore()
	require.NoError(t, s.Create("abc", store.ScanRequest{URL: "https://github.com/a/b"}))
	require.NoError(t, s.UpdateStatus("abc", "running"))
	job, _ := s.Get("abc")
	assert.Equal(t, "running", job.Status)
}

func TestMemoryStoreSaveResult(t *testing.T) {
	s := store.NewMemoryStore()
	require.NoError(t, s.Create("abc", store.ScanRequest{URL: "https://github.com/a/b"}))
	result := &core.ScanResult{Profile: "enterprise", PassThreshold: 70}
	require.NoError(t, s.SaveResult("abc", result))
	job, _ := s.Get("abc")
	assert.Equal(t, "complete", job.Status)
	assert.NotNil(t, job.Result)
}

func TestMemoryStoreSaveError(t *testing.T) {
	s := store.NewMemoryStore()
	require.NoError(t, s.Create("abc", store.ScanRequest{URL: "https://github.com/a/b"}))
	require.NoError(t, s.SaveError("abc", "something broke"))
	job, _ := s.Get("abc")
	assert.Equal(t, "failed", job.Status)
	assert.Equal(t, "something broke", job.Error)
}

func TestMemoryStoreGetNotFound(t *testing.T) {
	s := store.NewMemoryStore()
	_, err := s.Get("nonexistent")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Implement store interface and memory store**

`store.go` — `ScanStore` interface, `ScanJob`, `ScanRequest` types.
`memory.go` — `sync.Map`-based implementation.

- [ ] **Step 3: Run — expect PASS**

```bash
go test ./internal/server/store/... -v
```

- [ ] **Step 4: Commit**

```bash
git commit -m "feat: ScanStore interface and in-memory implementation"
```

---

## Task 3: URL Validation

**Files:**
- Create: `depscope/internal/server/validate.go`
- Create: `depscope/internal/server/validate_test.go`

- [ ] **Step 1: Write tests**

```go
func TestValidateURL(t *testing.T) {
	valid := []string{
		"https://github.com/psf/requests",
		"https://gitlab.com/org/project",
		"https://bitbucket.org/org/repo",
	}
	for _, u := range valid {
		assert.NoError(t, server.ValidateScanURL(u), u)
	}

	invalid := []string{
		"http://github.com/psf/requests",  // not HTTPS
		"https://localhost/foo",            // localhost
		"https://10.0.0.1/repo",           // private IP
		"https://192.168.1.1/repo",        // private IP
		"ftp://github.com/foo",            // wrong scheme
		"not-a-url",                       // garbage
		"",                                // empty
	}
	for _, u := range invalid {
		assert.Error(t, server.ValidateScanURL(u), u)
	}
}
```

- [ ] **Step 2: Implement ValidateScanURL**

Check: HTTPS scheme, non-empty host, not a private IP range, not localhost.

- [ ] **Step 3: Run — expect PASS**
- [ ] **Step 4: Commit** `"feat: URL validation for SSRF prevention"`

---

## Task 4: Server Skeleton + Handlers

**Files:**
- Create: `depscope/internal/server/server.go`
- Create: `depscope/internal/server/handlers.go`
- Create: `depscope/internal/server/server_test.go`

- [ ] **Step 1: Write handler tests**

```go
func TestLandingPage(t *testing.T) {
	srv := server.New(server.Options{Store: store.NewMemoryStore(), Mode: server.ModeLocal})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "depscope")
	assert.Contains(t, string(body), "Scan")
}

func TestSubmitScanRedirects(t *testing.T) {
	srv := server.New(server.Options{Store: store.NewMemoryStore(), Mode: server.ModeLocal})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.PostForm(ts.URL+"/scan", url.Values{"url": {"https://github.com/psf/requests"}, "profile": {"enterprise"}})
	require.NoError(t, err)
	assert.Equal(t, 303, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Location"), "/scan/")
}

func TestScanStatusJSON(t *testing.T) {
	s := store.NewMemoryStore()
	s.Create("test123", store.ScanRequest{URL: "https://github.com/a/b"})
	srv := server.New(server.Options{Store: s, Mode: server.ModeLocal})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/scan/test123")
	assert.Equal(t, 200, resp.StatusCode)
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "queued", result["status"])
}
```

- [ ] **Step 2: Implement server.go**

`server.go` — `Server` struct with `Options` (Store, Mode). `Handler()` returns `http.Handler` with routes:
- `GET /` → handleLanding
- `POST /scan` → handleSubmitScan
- `GET /scan/{id}` → handleScanPage
- `GET /api/scan/{id}` → handleScanStatus
- `GET /api/package/{eco}/{name...}/{version}` → handlePackageDetail (placeholder for now)
- `GET /static/` → embedded file server

Mode is `ModeLocal` (async goroutine) or `ModeLambda` (synchronous).

- [ ] **Step 3: Implement handlers.go**

`handleLanding` — renders landing.html template.
`handleSubmitScan` — validates URL, generates ID (use `crypto/rand` hex), creates scan job, launches scan (goroutine or inline based on mode), redirects.
`handleScanPage` — gets job from store, renders scanning.html or results.html based on status.
`handleScanStatus` — returns JSON `{status, error?}`.

- [ ] **Step 4: Run — expect PASS**
- [ ] **Step 5: Commit** `"feat: HTTP server with scan submission and status handlers"`

---

## Task 5: HTML Templates + CSS

**Files:**
- Create: `depscope/internal/web/embed.go`
- Create: `depscope/internal/web/templates/layout.html`
- Create: `depscope/internal/web/templates/landing.html`
- Create: `depscope/internal/web/templates/scanning.html`
- Create: `depscope/internal/web/templates/results.html`
- Create: `depscope/internal/web/static/style.css`
- Create: `depscope/internal/web/static/app.js`

- [ ] **Step 1: Create embed.go**

```go
package web

import "embed"

//go:embed templates/* static/*
var Assets embed.FS
```

- [ ] **Step 2: Create layout.html**

Base template with dark theme, nav bar, footer. Uses `{{template "content" .}}` for page content.

- [ ] **Step 3: Create landing.html**

Centered URL input form, profile dropdown (hobby/opensource/enterprise), "Scan" button. depscope logo in CSS text. Tagline underneath.

- [ ] **Step 4: Create scanning.html**

Pulsing logo animation, "Scanning {url}..." text, animated dots. JS polling script (inline or from app.js).

- [ ] **Step 5: Create results.html**

Header: repo name, pass/fail badge, SVG score gauge.
Package table: sortable, clickable rows with risk badge pills.
Issue summary: collapsible, grouped by severity.
Side panel placeholder: `<div id="panel">` that app.js populates.

- [ ] **Step 6: Create style.css**

Full dark theme with:
- Colors from spec (background #0d1117, surface #161b22, risk colors)
- Score gauge (SVG circle with stroke-dasharray animation)
- Package table styling (alternating rows, hover, risk pills)
- Side panel (fixed position, slide-in from right, 420px wide, overlay)
- Scanning animation (@keyframes pulse)
- Responsive layout
- Typography (system font stack)

- [ ] **Step 7: Create app.js**

Vanilla JS for:
- Poll `/api/scan/{id}` every 2s, reload on complete
- Side panel: click row → fetch `/api/package/{eco}/{name}/{version}` → render 7-factor bars
- Close panel on X or click outside
- Table sorting (click column header)

- [ ] **Step 8: Verify templates render**

```bash
go build ./cmd/depscope
```

- [ ] **Step 9: Commit** `"feat: dark-themed HTML templates and CSS with embedded assets"`

---

## Task 6: Package Detail API

**Files:**
- Modify: `depscope/internal/server/handlers.go`

- [ ] **Step 1: Implement handlePackageDetail**

`GET /api/package/{eco}/{name...}/{version}` — finds the package in the most recent scan result (look up by ecosystem + name + version in the store), returns JSON with score breakdown, issues, deps.

For now, scan results are the source of truth. The handler searches all stored scans for a matching package.

- [ ] **Step 2: Write test**

Create a scan with known result, then `GET /api/package/python/requests/2.31.0`, assert JSON has `score`, `factors`, `issues` keys.

- [ ] **Step 3: Run — expect PASS**
- [ ] **Step 4: Commit** `"feat: package detail API endpoint for side panel"`

---

## Task 7: CLI `server` Command

**Files:**
- Create: `depscope/cmd/depscope/server_cmd.go`

- [ ] **Step 1: Implement server command**

```go
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the depscope web server",
	RunE:  runServer,
}

func init() {
	serverCmd.Flags().Int("port", 8080, "port to listen on")
	serverCmd.Flags().String("store", "memory", "storage backend: memory, dynamo")
	serverCmd.Flags().String("table", "depscope-scans", "DynamoDB table name")
	rootCmd.AddCommand(serverCmd)
}
```

`runServer` creates the store (memory or dynamo based on flag), creates the server, and calls `http.ListenAndServe`.

- [ ] **Step 2: Test manually**

```bash
go build -o bin/depscope ./cmd/depscope
./bin/depscope server --port 8080
# Open http://localhost:8080 in browser
```

- [ ] **Step 3: Commit** `"feat: depscope server CLI command"`

---

## Task 8: DynamoDB Store

**Files:**
- Create: `depscope/internal/server/store/dynamo.go`

- [ ] **Step 1: Implement DynamoDB store**

Uses `github.com/aws/aws-sdk-go-v2`. Compress result JSON with gzip before storing. Decompress on Get. Set TTL to 7 days.

- [ ] **Step 2: Add aws-sdk-go-v2 dependency**

```bash
go get github.com/aws/aws-sdk-go-v2/config@latest
go get github.com/aws/aws-sdk-go-v2/service/dynamodb@latest
go get github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue@latest
```

- [ ] **Step 3: Commit** `"feat: DynamoDB scan store with gzip-compressed results"`

---

## Task 9: Lambda Adapter

**Files:**
- Create: `depscope/cmd/lambda/main.go`
- Modify: `depscope/Makefile` — add `build-lambda` target

- [ ] **Step 1: Add Lambda dependencies**

```bash
go get github.com/aws/aws-lambda-go@latest
go get github.com/awslabs/aws-lambda-go-api-proxy@latest
```

- [ ] **Step 2: Create Lambda main**

`cmd/lambda/main.go` — creates server with `ModeLambda` + DynamoDB store, wraps with `httpadapter.NewV2`.

- [ ] **Step 3: Add Makefile targets**

```makefile
build-lambda:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap ./cmd/lambda
	zip lambda.zip bootstrap

build-server:
	go build -o bin/depscope-server ./cmd/depscope
```

- [ ] **Step 4: Verify build**

```bash
make build-lambda
ls -la lambda.zip
```

- [ ] **Step 5: Commit** `"feat: Lambda adapter with Function URL support"`

---

## Task 10: CloudFormation + Dockerfile

**Files:**
- Create: `depscope/infrastructure/template.yaml`
- Create: `depscope/Dockerfile`

- [ ] **Step 1: Create CloudFormation template**

Lambda function (arm64, 512MB, 60s timeout), Function URL (auth NONE), DynamoDB table (on-demand, TTL enabled), IAM role.

- [ ] **Step 2: Create Dockerfile**

Multi-stage build: Go 1.26-alpine → alpine:3.19 with git + ca-certificates.

- [ ] **Step 3: Test Docker build**

```bash
docker build -t depscope .
docker run -p 8080:8080 depscope
# curl http://localhost:8080
```

- [ ] **Step 4: Commit** `"feat: Dockerfile and CloudFormation infrastructure"`

---

## Task 11: Integration Test + Polish

**Files:**
- Modify: various — fix issues found during manual testing

- [ ] **Step 1: Run full test suite**

```bash
go test ./... -race -count=1
```

- [ ] **Step 2: Manual E2E test**

1. `./bin/depscope server` → open browser → submit `https://github.com/psf/requests`
2. Verify scanning animation appears
3. Verify results page renders with package table
4. Click a package → verify side panel opens with score breakdown
5. Verify shareable URL works (open in incognito)

- [ ] **Step 3: Fix any issues found**
- [ ] **Step 4: Commit** `"fix: integration test fixes and UI polish"`
