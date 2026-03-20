# depscope Core Engine + CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `depscope` CLI that scans a project's dependency manifests, scores each package for supply chain reputation risk, propagates transitive risk through the dependency tree, and exits 0 (pass) or 1 (fail) based on a configurable threshold.

**Architecture:** A shared `internal/` library implements manifest parsing, registry fetching, scoring, and reporting. The `cmd/depscope/` entrypoint wires everything via cobra commands. All external API calls run in parallel (semaphore-limited), are cached to disk, and are stubbed with golden files in tests. No external services are hit during `go test`.

**Tech Stack:** Go 1.22+, `github.com/spf13/cobra` + `github.com/spf13/viper` (CLI/config), `golang.org/x/mod/modfile` (go.mod), `github.com/pelletier/go-toml/v2` (TOML), `github.com/goccy/go-yaml` (YAML), `github.com/google/go-github` (GitHub API), `osv.dev/bindings/go/osvdev` (CVE), `github.com/owenrumney/go-sarif/v3` (SARIF), `github.com/olekukonko/tablewriter` (terminal tables), `golang.org/x/sync/semaphore` (parallel fetch), `github.com/stretchr/testify` (test assertions)

---

## File Map

All files in `internal/core/` use `package core`. No sub-packages — the factor, scorer, and propagator functions are all exported from the same package to keep imports simple.

```
depscope/
├── cmd/depscope/
│   ├── main.go                        # cobra root; wires subcommands
│   ├── scan.go                        # `depscope scan` command
│   ├── package.go                     # `depscope package check` command
│   ├── cache.go                       # `depscope cache status|clear` command
│   └── testdata/                      # E2E test fixture projects
│       └── fixture-python/
│           ├── requirements.txt
│           └── poetry.lock
├── internal/
│   ├── core/                          # package core — all scoring logic
│   │   ├── types.go                   # Score, RiskLevel, Issue, PackageResult, ScanResult
│   │   ├── types_test.go
│   │   ├── factors.go                 # Factor* functions: one per scoring dimension
│   │   ├── factors_test.go
│   │   ├── scorer.go                  # Score() — combines factors with weights
│   │   ├── scorer_test.go
│   │   ├── propagator.go              # Propagate() + EffectiveScore() — transitive risk
│   │   └── propagator_test.go
│   ├── manifest/                      # package manifest
│   │   ├── manifest.go                # Package, Ecosystem, ConstraintType, Parser interface,
│   │   │                              # DetectEcosystem(), ParserFor(), Key(), BuildDepsMap()
│   │   ├── manifest_test.go
│   │   ├── python.go                  # requirements.txt, poetry.lock, uv.lock
│   │   ├── python_test.go
│   │   ├── gomod.go                   # go.mod + go.sum
│   │   ├── gomod_test.go
│   │   ├── rust.go                    # Cargo.toml + Cargo.lock
│   │   ├── rust_test.go
│   │   ├── javascript.go              # package.json + lockfile variants
│   │   ├── javascript_test.go
│   │   └── testdata/
│   │       ├── python/{requirements.txt,poetry.lock,uv.lock}
│   │       ├── go/{go.mod,go.sum}
│   │       ├── rust/{Cargo.toml,Cargo.lock}
│   │       └── javascript/{package.json,package-lock.json,pnpm-lock.yaml,bun.lock}
│   ├── registry/                      # package registry
│   │   ├── registry.go                # PackageInfo, Fetcher interface
│   │   ├── fetcher.go                 # FetchAll() — parallel orchestrator
│   │   ├── fetcher_test.go
│   │   ├── pypi.go + pypi_test.go
│   │   ├── npm.go + npm_test.go
│   │   ├── cratesio.go + cratesio_test.go
│   │   ├── goproxy.go + goproxy_test.go
│   │   └── testdata/{pypi,npm,cratesio,goproxy}/*.json
│   ├── vcs/                           # package vcs
│   │   ├── github.go                  # RepoInfo, GitHubClient
│   │   ├── github_test.go
│   │   └── testdata/*.json
│   ├── vuln/                          # package vuln
│   │   ├── vuln.go                    # Finding, Source interface
│   │   ├── osv.go + osv_test.go
│   │   ├── nvd.go + nvd_test.go
│   │   └── testdata/{osv,nvd}/*.json
│   ├── cache/                         # package cache
│   │   ├── cache.go
│   │   └── cache_test.go
│   ├── config/                        # package config
│   │   ├── config.go                  # Config struct, LoadFile() using Viper, ResolveEnv()
│   │   ├── config_test.go
│   │   └── profiles.go                # Hobby(), OpenSource(), Enterprise() presets
│   └── report/                        # package report
│       ├── report.go                  # shared helpers, sampleScanResult() for tests
│       ├── text.go + text_test.go
│       ├── json.go + json_test.go
│       └── sarif.go + sarif_test.go
├── .goreleaser.yaml
├── Makefile
└── go.mod
```

---

## Task 1: Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `.goreleaser.yaml`
- Create: `cmd/depscope/main.go`

- [ ] **Step 1: Initialize Go module**

```bash
mkdir -p depscope && cd depscope
go mod init github.com/depscope/depscope
```

- [ ] **Step 2: Add all dependencies**

```bash
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest
go get golang.org/x/mod@latest
go get github.com/pelletier/go-toml/v2@latest
go get github.com/goccy/go-yaml@latest
go get github.com/google/go-github/v68@latest
go get golang.org/x/oauth2@latest
go get osv.dev/bindings/go/osvdev@latest
go get github.com/owenrumney/go-sarif/v3@latest
go get github.com/olekukonko/tablewriter@latest
go get golang.org/x/sync@latest
go get github.com/stretchr/testify@latest
go mod tidy
```

- [ ] **Step 3: Create Makefile**

```makefile
.PHONY: build test lint clean

build:
	go build -o bin/depscope ./cmd/depscope

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

clean:
	rm -rf bin/
```

- [ ] **Step 4: Create minimal cobra entrypoint**

`cmd/depscope/main.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "depscope",
	Short: "Supply chain reputation scoring for your dependencies",
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Verify it builds**

```bash
go build ./cmd/depscope && ./depscope --help
```
Expected: usage text, exit 0.

- [ ] **Step 6: Create .goreleaser.yaml**

```yaml
version: 2
project_name: depscope

builds:
  - id: depscope
    main: ./cmd/depscope
    binary: depscope
    env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]

archives:
  - format: tar.gz
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: "checksums.txt"
```

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum Makefile .goreleaser.yaml cmd/
git commit -m "feat: project scaffold with cobra CLI and goreleaser"
```

---

## Task 2: Core Types

**Files:**
- Create: `internal/core/types.go`
- Create: `internal/core/types_test.go`

All scoring logic lives in `package core`. `types.go` defines the shared data types used by every other package.

- [ ] **Step 1: Write the test**

`internal/core/types_test.go`:
```go
package core_test

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestRiskLevelFromScore(t *testing.T) {
	tests := []struct {
		score    int
		expected core.RiskLevel
	}{
		{100, core.RiskLow},
		{80, core.RiskLow},
		{79, core.RiskMedium},
		{60, core.RiskMedium},
		{59, core.RiskHigh},
		{40, core.RiskHigh},
		{39, core.RiskCritical},
		{0, core.RiskCritical},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, core.RiskLevelFromScore(tt.score), "score %d", tt.score)
	}
}

func TestFinalScore(t *testing.T) {
	r := core.PackageResult{OwnScore: 75, TransitiveRiskScore: 45}
	assert.Equal(t, 45, r.FinalScore())

	r2 := core.PackageResult{OwnScore: 45, TransitiveRiskScore: 75}
	assert.Equal(t, 45, r2.FinalScore())
}

func TestScanResultPassed(t *testing.T) {
	passing := core.ScanResult{
		PassThreshold: 70,
		Packages: []core.PackageResult{
			{OwnScore: 80, TransitiveRiskScore: 80},
			{OwnScore: 75, TransitiveRiskScore: 75},
		},
	}
	assert.True(t, passing.Passed())

	failing := core.ScanResult{
		PassThreshold: 70,
		Packages: []core.PackageResult{
			{OwnScore: 80, TransitiveRiskScore: 80},
			{OwnScore: 50, TransitiveRiskScore: 50}, // below threshold
		},
	}
	assert.False(t, failing.Passed())
}
```

- [ ] **Step 2: Run test — expect FAIL**

```bash
go test ./internal/core/... -v
```
Expected: compile error — package not found.

- [ ] **Step 3: Implement types**

`internal/core/types.go`:
```go
package core

type RiskLevel string

const (
	RiskLow      RiskLevel = "LOW"
	RiskMedium   RiskLevel = "MEDIUM"
	RiskHigh     RiskLevel = "HIGH"
	RiskCritical RiskLevel = "CRITICAL"
	RiskUnknown  RiskLevel = "UNKNOWN"
)

func RiskLevelFromScore(score int) RiskLevel {
	switch {
	case score >= 80:
		return RiskLow
	case score >= 60:
		return RiskMedium
	case score >= 40:
		return RiskHigh
	default:
		return RiskCritical
	}
}

type IssueSeverity string

const (
	SeverityHigh   IssueSeverity = "HIGH"
	SeverityMedium IssueSeverity = "MEDIUM"
	SeverityLow    IssueSeverity = "LOW"
	SeverityInfo   IssueSeverity = "INFO"
)

type Issue struct {
	Package  string
	Severity IssueSeverity
	Message  string
}

type PackageResult struct {
	Name                string
	Version             string
	Ecosystem           string
	ConstraintType      string
	Depth               int
	OwnScore            int
	TransitiveRiskScore int
	OwnRisk             RiskLevel
	TransitiveRisk      RiskLevel
	Issues              []Issue
	DependsOnCount      int
	DependedOnCount     int
}

func (r PackageResult) FinalScore() int {
	if r.TransitiveRiskScore < r.OwnScore {
		return r.TransitiveRiskScore
	}
	return r.OwnScore
}

func (r PackageResult) FinalRisk() RiskLevel {
	return RiskLevelFromScore(r.FinalScore())
}

type ScanResult struct {
	Profile         string
	PassThreshold   int
	DirectDeps      int
	TransitiveDeps  int
	MaxDepthReached bool
	Packages        []PackageResult
	AllIssues       []Issue
}

func (s ScanResult) Passed() bool {
	for _, p := range s.Packages {
		if p.FinalScore() < s.PassThreshold {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run test — expect PASS**

```bash
go test ./internal/core/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/core/types.go internal/core/types_test.go
git commit -m "feat: core score types, risk levels, and PackageResult"
```

---

## Task 3: Config System

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/profiles.go`
- Create: `internal/config/config_test.go`
- Create: `internal/config/testdata/depscope.yaml`

Uses Viper for YAML file loading (already in the dependency list), which gives us env var interpolation and type coercion for free.

- [ ] **Step 1: Create testdata fixture**

`internal/config/testdata/depscope.yaml`:
```yaml
profile: enterprise
pass_threshold: 75
depth: 10
registries:
  github_token: ${GITHUB_TOKEN}
vuln_sources:
  osv: true
  nvd: true
  nvd_api_key: ${NVD_KEY}
```

- [ ] **Step 2: Write tests**

`internal/config/config_test.go`:
```go
package config_test

import (
	"os"
	"testing"

	"github.com/depscope/depscope/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseWeightsSumTo100(t *testing.T) {
	cfg := config.Enterprise()
	total := 0
	for _, w := range cfg.Weights {
		total += w
	}
	assert.Equal(t, 100, total, "enterprise weights must sum to 100")
}

func TestAllProfilesWeightsSumTo100(t *testing.T) {
	for _, cfg := range []config.Config{config.Hobby(), config.OpenSource(), config.Enterprise()} {
		total := 0
		for _, w := range cfg.Weights {
			total += w
		}
		assert.Equal(t, 100, total, "profile %s weights must sum to 100", cfg.Profile)
	}
}

func TestPartialWeightOverrideRenormalizes(t *testing.T) {
	base := config.Enterprise()
	merged := base.WithWeights(config.Weights{"release_recency": 40}) // was 20
	total := 0
	for _, w := range merged.Weights {
		total += w
	}
	assert.Equal(t, 100, total, "merged weights must still sum to 100")
	assert.Equal(t, 40, merged.Weights["release_recency"])
}

func TestEnvVarResolution(t *testing.T) {
	os.Setenv("TEST_DEPSCOPE_TOKEN", "secret123")
	defer os.Unsetenv("TEST_DEPSCOPE_TOKEN")
	assert.Equal(t, "secret123", config.ResolveEnv("${TEST_DEPSCOPE_TOKEN}"))
	assert.Equal(t, "literal", config.ResolveEnv("literal"))
}

func TestLoadFile(t *testing.T) {
	os.Setenv("GITHUB_TOKEN", "ghp_test")
	defer os.Unsetenv("GITHUB_TOKEN")

	cfg, err := config.LoadFile("testdata/depscope.yaml")
	require.NoError(t, err)
	assert.Equal(t, 75, cfg.PassThreshold)
	assert.Equal(t, "enterprise", cfg.Profile)
	assert.Equal(t, "ghp_test", cfg.Registries.GitHubToken)
}
```

- [ ] **Step 3: Run — expect FAIL**

```bash
go test ./internal/config/... -v
```

- [ ] **Step 4: Implement profiles**

`internal/config/profiles.go`:
```go
package config

type Weights map[string]int

// All profiles must have exactly these 7 factors summing to 100.
var factorNames = []string{
	"release_recency", "maintainer_count", "download_velocity",
	"open_issue_ratio", "org_backing", "version_pinning", "repo_health",
}

func Hobby() Config {
	return Config{
		Profile: "hobby", PassThreshold: 40, Depth: 10, Concurrency: 20,
		CacheTTL: DefaultCacheTTL(),
		Weights: Weights{
			"release_recency": 15, "maintainer_count": 5, "download_velocity": 15,
			"open_issue_ratio": 10, "org_backing": 5, "version_pinning": 25, "repo_health": 25,
		},
	}
}

func OpenSource() Config {
	return Config{
		Profile: "opensource", PassThreshold: 55, Depth: 10, Concurrency: 20,
		CacheTTL: DefaultCacheTTL(),
		Weights: Weights{
			"release_recency": 18, "maintainer_count": 12, "download_velocity": 15,
			"open_issue_ratio": 10, "org_backing": 8, "version_pinning": 17, "repo_health": 20,
		},
	}
}

func Enterprise() Config {
	return Config{
		Profile: "enterprise", PassThreshold: 70, Depth: 10, Concurrency: 20,
		CacheTTL: DefaultCacheTTL(),
		Weights: Weights{
			"release_recency": 20, "maintainer_count": 15, "download_velocity": 15,
			"open_issue_ratio": 10, "org_backing": 10, "version_pinning": 15, "repo_health": 15,
		},
	}
}

func ProfileByName(name string) Config {
	switch name {
	case "hobby":
		return Hobby()
	case "opensource":
		return OpenSource()
	default:
		return Enterprise()
	}
}
```

- [ ] **Step 5: Implement config**

`internal/config/config.go`:
```go
package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/spf13/viper"
)

type CacheTTL struct {
	Metadata time.Duration
	CVE      time.Duration
}

func DefaultCacheTTL() CacheTTL {
	return CacheTTL{Metadata: 24 * time.Hour, CVE: 6 * time.Hour}
}

type VulnSources struct {
	OSV       bool
	NVD       bool
	NVDAPIKey string
}

type Registries struct {
	GitHubToken string
}

type Config struct {
	Profile       string
	PassThreshold int
	Depth         int
	Concurrency   int
	CacheTTL      CacheTTL
	Weights       Weights
	VulnSources   VulnSources
	Registries    Registries
}

var envVarRe = regexp.MustCompile(`^\$\{(.+)\}$`)

func ResolveEnv(v string) string {
	if m := envVarRe.FindStringSubmatch(v); m != nil {
		return os.Getenv(m[1])
	}
	return v
}

// LoadFile reads a YAML config file via Viper, merges it onto the named profile.
func LoadFile(path string) (Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}

	profileName := v.GetString("profile")
	cfg := ProfileByName(profileName)

	if v.IsSet("pass_threshold") {
		cfg.PassThreshold = v.GetInt("pass_threshold")
	}
	if v.IsSet("depth") {
		cfg.Depth = v.GetInt("depth")
	}
	if v.IsSet("concurrency") {
		cfg.Concurrency = v.GetInt("concurrency")
	}
	if v.IsSet("weights") {
		overrides := Weights{}
		for _, k := range factorNames {
			if v.IsSet("weights." + k) {
				overrides[k] = v.GetInt("weights." + k)
			}
		}
		if len(overrides) > 0 {
			cfg = cfg.WithWeights(overrides)
		}
	}

	cfg.VulnSources = VulnSources{
		OSV:       v.GetBool("vuln_sources.osv"),
		NVD:       v.GetBool("vuln_sources.nvd"),
		NVDAPIKey: ResolveEnv(v.GetString("vuln_sources.nvd_api_key")),
	}
	cfg.Registries = Registries{
		GitHubToken: ResolveEnv(v.GetString("registries.github_token")),
	}
	return cfg, nil
}

// WithWeights applies partial weight overrides, renormalizing so all weights sum to 100.
func (c Config) WithWeights(overrides Weights) Config {
	merged := make(Weights, len(c.Weights))
	for k, v := range overrides {
		merged[k] = v
	}

	overriddenSum := 0
	for _, v := range overrides {
		overriddenSum += v
	}

	baseRemaining := 0
	for k, v := range c.Weights {
		if _, ok := overrides[k]; !ok {
			baseRemaining += v
		}
	}

	remaining := 100 - overriddenSum
	for k, v := range c.Weights {
		if _, ok := overrides[k]; ok {
			continue
		}
		if baseRemaining == 0 {
			merged[k] = 0
		} else {
			merged[k] = int(float64(v) / float64(baseRemaining) * float64(remaining))
		}
	}

	// Fix rounding drift
	total := 0
	for _, v := range merged {
		total += v
	}
	if diff := 100 - total; diff != 0 {
		for k := range c.Weights {
			if _, ok := overrides[k]; !ok {
				merged[k] += diff
				break
			}
		}
	}

	c.Weights = merged
	return c
}
```

- [ ] **Step 6: Run — expect PASS**

```bash
go test ./internal/config/... -v
```

- [ ] **Step 7: Commit**

```bash
git add internal/config/
git commit -m "feat: config system with Viper, profiles, and weight renormalization"
```

---

## Task 4: Cache Layer

**Files:**
- Create: `internal/cache/cache.go`
- Create: `internal/cache/cache_test.go`

- [ ] **Step 1: Write tests**

`internal/cache/cache_test.go`:
```go
package cache_test

import (
	"os"
	"testing"
	"time"

	"github.com/depscope/depscope/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetAndGet(t *testing.T) {
	c := cache.NewDiskCache(t.TempDir())
	require.NoError(t, c.Set("key1", []byte(`{"hello":"world"}`), time.Hour))
	data, ok, err := c.Get("key1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, `{"hello":"world"}`, string(data))
}

func TestExpiredEntryMiss(t *testing.T) {
	c := cache.NewDiskCache(t.TempDir())
	require.NoError(t, c.Set("key2", []byte("data"), -time.Second))
	_, ok, err := c.Get("key2")
	require.NoError(t, err)
	assert.False(t, ok, "expired entry should be a cache miss")
}

func TestMissingKeyMiss(t *testing.T) {
	c := cache.NewDiskCache(t.TempDir())
	_, ok, err := c.Get("nonexistent")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestClearRemovesAllEntries(t *testing.T) {
	dir := t.TempDir()
	c := cache.NewDiskCache(dir)
	require.NoError(t, c.Set("k1", []byte("a"), time.Hour))
	require.NoError(t, c.Set("k2", []byte("b"), time.Hour))
	require.NoError(t, c.Clear())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestStatus(t *testing.T) {
	c := cache.NewDiskCache(t.TempDir())
	require.NoError(t, c.Set("k1", []byte("hello"), time.Hour))
	count, size, err := c.Status()
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Greater(t, size, int64(0))
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/cache/... -v
```

- [ ] **Step 3: Implement**

`internal/cache/cache.go`:
```go
package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type entry struct {
	ExpiresAt time.Time `json:"expires_at"`
	Data      []byte    `json:"data"`
}

type DiskCache struct{ dir string }

func NewDiskCache(dir string) *DiskCache { return &DiskCache{dir: dir} }

func DefaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "depscope")
}

func (c *DiskCache) path(key string) string {
	h := sha256.Sum256([]byte(key))
	return filepath.Join(c.dir, fmt.Sprintf("%x.json", h))
}

func (c *DiskCache) Get(key string) ([]byte, bool, error) {
	data, err := os.ReadFile(c.path(key))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, false, nil
	}
	if time.Now().After(e.ExpiresAt) {
		_ = os.Remove(c.path(key))
		return nil, false, nil
	}
	return e.Data, true, nil
}

func (c *DiskCache) Set(key string, data []byte, ttl time.Duration) error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(entry{ExpiresAt: time.Now().Add(ttl), Data: data})
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(key), b, 0o644)
}

func (c *DiskCache) Clear() error {
	entries, err := os.ReadDir(c.dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		_ = os.Remove(filepath.Join(c.dir, e.Name()))
	}
	return nil
}

func (c *DiskCache) Status() (count int, bytes int64, err error) {
	entries, err := os.ReadDir(c.dir)
	if os.IsNotExist(err) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		if info, _ := e.Info(); info != nil {
			bytes += info.Size()
		}
		count++
	}
	return count, bytes, nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/cache/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/cache/
git commit -m "feat: disk-backed TTL cache with sha256-keyed entries"
```

---

## Task 5: Manifest Types, Detector, and Utilities

**Files:**
- Create: `internal/manifest/manifest.go`
- Create: `internal/manifest/manifest_test.go`

This file defines: `Package` (with `Key()` method), `Ecosystem`, `ConstraintType`, `Parser` interface, `DetectEcosystem()`, `ParserFor()`, and `BuildDepsMap()`.

**Note on dependency graphs:** Lockfiles vary in how much graph structure they expose. `Cargo.lock` and `poetry.lock` have explicit dependency lists per package (used to populate `Package.Parents`). `go.mod` and `package-lock.json` do not have the full graph — for those, all indirect deps are treated as depth-2 and `BuildDepsMap` creates a flat two-level graph (all direct deps depend on all indirect deps). This is conservative but never misses risk.

- [ ] **Step 1: Write tests**

`internal/manifest/manifest_test.go`:
```go
package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectEcosystem(t *testing.T) {
	tests := []struct {
		file     string
		expected manifest.Ecosystem
	}{
		{"go.mod", manifest.EcosystemGo},
		{"Cargo.toml", manifest.EcosystemRust},
		{"package.json", manifest.EcosystemNPM},
		{"requirements.txt", manifest.EcosystemPython},
		{"poetry.lock", manifest.EcosystemPython},
		{"uv.lock", manifest.EcosystemPython},
	}
	for _, tt := range tests {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, tt.file), []byte(""), 0o644))
		eco, err := manifest.DetectEcosystem(dir)
		require.NoError(t, err, tt.file)
		assert.Equal(t, tt.expected, eco, tt.file)
	}
}

func TestParseConstraintType(t *testing.T) {
	tests := []struct {
		constraint string
		expected   manifest.ConstraintType
	}{
		{"==1.2.3", manifest.ConstraintExact},
		{"=1.2.3", manifest.ConstraintExact},
		{"~=1.2.3", manifest.ConstraintPatch},
		{"~1.2", manifest.ConstraintPatch},
		{"^1.2.3", manifest.ConstraintMinor},
		{">=1.2,<2.0", manifest.ConstraintMinor},
		{">=1.0.0", manifest.ConstraintMajor},
		{"*", manifest.ConstraintMajor},
		{"latest", manifest.ConstraintMajor},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, manifest.ParseConstraintType(tt.constraint), tt.constraint)
	}
}

func TestPackageKey(t *testing.T) {
	p := manifest.Package{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython}
	assert.Equal(t, "python/requests@2.31.0", p.Key())
}

func TestBuildDepsMap(t *testing.T) {
	pkgs := []manifest.Package{
		{Name: "a", Depth: 1, Ecosystem: manifest.EcosystemGo},
		{Name: "b", Depth: 2, Ecosystem: manifest.EcosystemGo, Parents: []string{"a"}},
		{Name: "c", Depth: 2, Ecosystem: manifest.EcosystemGo, Parents: []string{"a"}},
	}
	deps := manifest.BuildDepsMap(pkgs)
	assert.ElementsMatch(t, []string{"b", "c"}, deps["a"])
}

func TestBuildDepsMapFallback(t *testing.T) {
	// When Parents is empty (e.g. go.mod flat list), all depth-1 get all depth-2 as children.
	pkgs := []manifest.Package{
		{Name: "direct1", Depth: 1, Ecosystem: manifest.EcosystemGo},
		{Name: "indirect1", Depth: 2, Ecosystem: manifest.EcosystemGo},
	}
	deps := manifest.BuildDepsMap(pkgs)
	assert.Contains(t, deps["direct1"], "indirect1")
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/manifest/... -run TestDetect -run TestParse -run TestPackageKey -run TestBuildDeps -v
```

- [ ] **Step 3: Implement**

`internal/manifest/manifest.go`:
```go
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Ecosystem string

const (
	EcosystemPython Ecosystem = "python"
	EcosystemGo     Ecosystem = "go"
	EcosystemRust   Ecosystem = "rust"
	EcosystemNPM    Ecosystem = "npm"
)

type ConstraintType string

const (
	ConstraintExact ConstraintType = "exact"
	ConstraintPatch ConstraintType = "patch"
	ConstraintMinor ConstraintType = "minor"
	ConstraintMajor ConstraintType = "major"
)

// Package is one dependency entry, merged from manifest (constraints) and lockfile (resolved version).
type Package struct {
	Name            string
	ResolvedVersion string
	Constraint      string
	ConstraintType  ConstraintType
	Ecosystem       Ecosystem
	Depth           int      // 1 = direct dep, 2+ = transitive
	Parents         []string // names of packages that directly depend on this one
}

// Key returns a unique string identifier for this package within a scan.
func (p Package) Key() string {
	return string(p.Ecosystem) + "/" + p.Name + "@" + p.ResolvedVersion
}

// Parser reads a project directory and returns all packages (direct + transitive).
type Parser interface {
	Parse(dir string) ([]Package, error)
	Ecosystem() Ecosystem
}

var ecosystemFiles = []struct {
	file      string
	ecosystem Ecosystem
}{
	{"go.mod", EcosystemGo},
	{"Cargo.toml", EcosystemRust},
	{"package.json", EcosystemNPM},
	{"uv.lock", EcosystemPython},
	{"poetry.lock", EcosystemPython},
	{"requirements.txt", EcosystemPython},
}

// DetectEcosystem scans dir for known manifest files and returns the ecosystem.
func DetectEcosystem(dir string) (Ecosystem, error) {
	for _, ef := range ecosystemFiles {
		if _, err := os.Stat(filepath.Join(dir, ef.file)); err == nil {
			return ef.ecosystem, nil
		}
	}
	return "", fmt.Errorf("no recognized manifest found in %s", dir)
}

// ParserFor returns the concrete parser for the given ecosystem.
func ParserFor(eco Ecosystem) Parser {
	switch eco {
	case EcosystemPython:
		return NewPythonParser()
	case EcosystemGo:
		return NewGoModParser()
	case EcosystemRust:
		return NewRustParser()
	case EcosystemNPM:
		return NewJavaScriptParser()
	default:
		panic("unknown ecosystem: " + string(eco))
	}
}

// BuildDepsMap builds a map of package name → list of direct dependency names.
// Uses Package.Parents when available. Falls back to a flat two-level structure
// (all depth-1 packages depend on all depth-2 packages) when Parents is empty.
func BuildDepsMap(pkgs []Package) map[string][]string {
	deps := make(map[string][]string)

	// Check if any package has Parents populated (richer graph available).
	hasParentInfo := false
	for _, p := range pkgs {
		if len(p.Parents) > 0 {
			hasParentInfo = true
			break
		}
	}

	if hasParentInfo {
		// Build from Parent relationships: parent → children
		for _, p := range pkgs {
			for _, parent := range p.Parents {
				deps[parent] = append(deps[parent], p.Name)
			}
		}
		return deps
	}

	// Fallback: flat two-level (all direct deps depend on all indirect deps)
	var direct, indirect []string
	for _, p := range pkgs {
		if p.Depth <= 1 {
			direct = append(direct, p.Name)
		} else {
			indirect = append(indirect, p.Name)
		}
	}
	for _, d := range direct {
		deps[d] = append(deps[d], indirect...)
	}
	return deps
}

// ParseConstraintType classifies a raw version constraint string.
func ParseConstraintType(constraint string) ConstraintType {
	c := strings.TrimSpace(constraint)
	switch {
	case strings.HasPrefix(c, "==") || (strings.HasPrefix(c, "=") && !strings.HasPrefix(c, "=>")):
		return ConstraintExact
	case strings.HasPrefix(c, "~=") || strings.HasPrefix(c, "~"):
		return ConstraintPatch
	case strings.HasPrefix(c, "^") || (strings.HasPrefix(c, ">=") && strings.Contains(c, "<")):
		return ConstraintMinor
	default:
		return ConstraintMajor
	}
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/manifest/... -run TestDetect -run TestParse -run TestPackageKey -run TestBuildDeps -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/manifest/manifest.go internal/manifest/manifest_test.go
git commit -m "feat: manifest types, detector, Key(), ParserFor(), BuildDepsMap()"
```

---

## Task 6: Go Manifest Parser

**Files:**
- Create: `internal/manifest/gomod.go`
- Create: `internal/manifest/gomod_test.go`
- Create: `internal/manifest/testdata/go/go.mod`
- Create: `internal/manifest/testdata/go/go.sum`

- [ ] **Step 1: Create fixtures**

`internal/manifest/testdata/go/go.mod`:
```
module example.com/myapp

go 1.22

require (
	github.com/spf13/cobra v1.8.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/sync v0.6.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)
```

`internal/manifest/testdata/go/go.sum` — minimal valid content (hashes not verified in tests):
```
github.com/spf13/cobra v1.8.0 h1:7aJaZx1B85qltLMc546zn58bgVOqr/8x7rmKnMOKHo=
github.com/spf13/cobra v1.8.0/go.mod h1:wHxEcudfqmLYa8iTfL+OuZPbBZkmvliBWKIoKoEqu0=
```

- [ ] **Step 2: Write test**

`internal/manifest/gomod_test.go`:
```go
package manifest_test

import (
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoModParser(t *testing.T) {
	p := manifest.NewGoModParser()
	pkgs, err := p.Parse("testdata/go")
	require.NoError(t, err)
	assert.Len(t, pkgs, 5) // 3 direct + 2 indirect

	m := pkgMap(pkgs)
	require.Contains(t, m, "github.com/spf13/cobra")
	assert.Equal(t, "v1.8.0", m["github.com/spf13/cobra"].ResolvedVersion)
	assert.Equal(t, manifest.ConstraintExact, m["github.com/spf13/cobra"].ConstraintType)
	assert.Equal(t, 1, m["github.com/spf13/cobra"].Depth)
	assert.Equal(t, 2, m["github.com/inconshreveable/mousetrap"].Depth)
}

func pkgMap(pkgs []manifest.Package) map[string]manifest.Package {
	m := make(map[string]manifest.Package, len(pkgs))
	for _, p := range pkgs {
		m[p.Name] = p
	}
	return m
}
```

- [ ] **Step 3: Run — expect FAIL**

```bash
go test ./internal/manifest/... -run TestGoMod -v
```

- [ ] **Step 4: Implement**

`internal/manifest/gomod.go`:
```go
package manifest

import (
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

type GoModParser struct{}

func NewGoModParser() *GoModParser { return &GoModParser{} }

func (p *GoModParser) Ecosystem() Ecosystem { return EcosystemGo }

func (p *GoModParser) Parse(dir string) ([]Package, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return nil, err
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, req := range f.Require {
		depth := 1
		if req.Indirect {
			depth = 2
		}
		pkgs = append(pkgs, Package{
			Name:            req.Mod.Path,
			ResolvedVersion: req.Mod.Version,
			Constraint:      req.Mod.Version,
			ConstraintType:  ConstraintExact, // go.mod pins exact versions
			Ecosystem:       EcosystemGo,
			Depth:           depth,
			// Parents not populated: go.mod has no graph info
		})
	}
	return pkgs, nil
}
```

- [ ] **Step 5: Run — expect PASS**

```bash
go test ./internal/manifest/... -run TestGoMod -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/manifest/gomod.go internal/manifest/gomod_test.go internal/manifest/testdata/go/
git commit -m "feat: go.mod manifest parser using golang.org/x/mod"
```

---

## Task 7: Python Manifest Parser

**Files:**
- Create: `internal/manifest/python.go`
- Create: `internal/manifest/python_test.go`
- Create: `internal/manifest/testdata/python/{requirements.txt,poetry.lock,uv.lock}`

- [ ] **Step 1: Create fixtures**

`internal/manifest/testdata/python/requirements.txt`:
```
requests==2.31.0
urllib3>=1.26.0
cryptography>=3.0
pytest~=7.0
```

`internal/manifest/testdata/python/poetry.lock`:
```toml
[[package]]
name = "requests"
version = "2.31.0"

[package.dependencies]
urllib3 = ">=1.21.1,<3"
certifi = ">=2017.4.17"

[[package]]
name = "urllib3"
version = "2.0.7"

[[package]]
name = "certifi"
version = "2023.11.17"
```

`internal/manifest/testdata/python/uv.lock`:
```toml
version = 1

[[package]]
name = "requests"
version = "2.31.0"
source = { registry = "https://pypi.org/simple" }

[[package]]
name = "urllib3"
version = "2.0.7"
source = { registry = "https://pypi.org/simple" }
```

- [ ] **Step 2: Write tests**

`internal/manifest/python_test.go`:
```go
package manifest_test

import (
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequirementsTxt(t *testing.T) {
	p := manifest.NewPythonParser()
	pkgs, err := p.ParseFile("testdata/python/requirements.txt")
	require.NoError(t, err)
	m := pkgMap(pkgs)
	assert.Equal(t, manifest.ConstraintExact, m["requests"].ConstraintType)
	assert.Equal(t, manifest.ConstraintMajor, m["urllib3"].ConstraintType)
	assert.Equal(t, manifest.ConstraintPatch, m["pytest"].ConstraintType)
}

func TestPoetryLock(t *testing.T) {
	p := manifest.NewPythonParser()
	pkgs, err := p.ParseFile("testdata/python/poetry.lock")
	require.NoError(t, err)
	m := pkgMap(pkgs)
	assert.Equal(t, "2.31.0", m["requests"].ResolvedVersion)
	// urllib3 should list requests as a parent
	assert.Contains(t, m["urllib3"].Parents, "requests")
}

func TestUVLock(t *testing.T) {
	p := manifest.NewPythonParser()
	pkgs, err := p.ParseFile("testdata/python/uv.lock")
	require.NoError(t, err)
	assert.Len(t, pkgs, 2)
}
```

- [ ] **Step 3: Run — expect FAIL**
- [ ] **Step 4: Implement**

`internal/manifest/python.go`:
```go
package manifest

import (
	"bufio"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type PythonParser struct{}

func NewPythonParser() *PythonParser { return &PythonParser{} }
func (p *PythonParser) Ecosystem() Ecosystem { return EcosystemPython }

func (p *PythonParser) Parse(dir string) ([]Package, error) {
	for _, try := range []struct {
		file string
	}{
		{"uv.lock"}, {"poetry.lock"}, {"requirements.txt"},
	} {
		path := dir + "/" + try.file
		if _, err := os.Stat(path); err == nil {
			return p.ParseFile(path)
		}
	}
	return nil, os.ErrNotExist
}

func (p *PythonParser) ParseFile(path string) ([]Package, error) {
	if strings.HasSuffix(path, "requirements.txt") {
		return p.parseRequirements(path)
	}
	if strings.HasSuffix(path, "poetry.lock") {
		return p.parsePoetryLock(path)
	}
	return p.parseUVLock(path)
}

func (p *PythonParser) parseRequirements(path string) ([]Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var pkgs []Package
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, constraint := splitRequirement(line)
		pkgs = append(pkgs, Package{
			Name: name, Constraint: constraint,
			ConstraintType: ParseConstraintType(constraint),
			Ecosystem: EcosystemPython, Depth: 1,
		})
	}
	return pkgs, scanner.Err()
}

func splitRequirement(line string) (name, constraint string) {
	for i, ch := range line {
		if ch == '=' || ch == '>' || ch == '<' || ch == '~' || ch == '!' {
			return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i:])
		}
	}
	return strings.TrimSpace(line), ""
}

func (p *PythonParser) parsePoetryLock(path string) ([]Package, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock struct {
		Package []struct {
			Name         string            `toml:"name"`
			Version      string            `toml:"version"`
			Dependencies map[string]any    `toml:"dependencies"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	// Build name→children map from dependencies section
	childrenOf := make(map[string][]string)
	for _, pkg := range lock.Package {
		for depName := range pkg.Dependencies {
			childrenOf[pkg.Name] = append(childrenOf[pkg.Name], strings.ToLower(depName))
		}
	}
	// Build parent map: invert childrenOf
	parentsOf := make(map[string][]string)
	for parent, children := range childrenOf {
		for _, child := range children {
			parentsOf[child] = append(parentsOf[child], parent)
		}
	}

	var pkgs []Package
	for _, pkg := range lock.Package {
		pkgs = append(pkgs, Package{
			Name: pkg.Name, ResolvedVersion: pkg.Version,
			Constraint: "==" + pkg.Version, ConstraintType: ConstraintExact,
			Ecosystem: EcosystemPython, Depth: 1,
			Parents: parentsOf[pkg.Name],
		})
	}
	return pkgs, nil
}

func (p *PythonParser) parseUVLock(path string) ([]Package, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock struct {
		Package []struct {
			Name    string `toml:"name"`
			Version string `toml:"version"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, pkg := range lock.Package {
		pkgs = append(pkgs, Package{
			Name: pkg.Name, ResolvedVersion: pkg.Version,
			Constraint: "==" + pkg.Version, ConstraintType: ConstraintExact,
			Ecosystem: EcosystemPython, Depth: 1,
		})
	}
	return pkgs, nil
}
```

- [ ] **Step 5: Run — expect PASS**

```bash
go test ./internal/manifest/... -run TestRequirements -run TestPoetry -run TestUV -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/manifest/python.go internal/manifest/python_test.go internal/manifest/testdata/python/
git commit -m "feat: Python manifest parser (requirements.txt, poetry.lock, uv.lock)"
```

---

## Task 8: Rust + JavaScript Manifest Parsers

**Files:**
- Create: `internal/manifest/rust.go` + `rust_test.go` + `testdata/rust/`
- Create: `internal/manifest/javascript.go` + `javascript_test.go` + `testdata/javascript/`

Follow the same pattern as Task 7.

**Rust notes:**
- `Cargo.toml` uses bare version strings like `"1.0"` (major constraint) and `"=0.11"` (exact). Cargo constraint rules: bare `"X.Y"` = `>=X.Y, <X+1` (minor), `"=X.Y.Z"` = exact, `"~X.Y"` = patch. Call `ParseConstraintType` after normalizing bare Cargo constraints to the Python/semver form.
- `Cargo.lock` has `[[package]]` entries with `dependencies = ["name version", ...]` — use these to populate `Parents`.

`testdata/rust/Cargo.toml`:
```toml
[dependencies]
serde = { version = "1.0", features = ["derive"] }
tokio = { version = "^1.35", features = ["full"] }
reqwest = "=0.11.23"
```

`testdata/rust/Cargo.lock`:
```toml
[[package]]
name = "myapp"
version = "0.1.0"
dependencies = ["serde 1.0.196", "tokio 1.35.1", "reqwest 0.11.23"]

[[package]]
name = "serde"
version = "1.0.196"

[[package]]
name = "tokio"
version = "1.35.1"

[[package]]
name = "reqwest"
version = "0.11.23"
dependencies = ["tokio 1.35.1"]
```

**JavaScript notes:**
- `package.json` constraints: `^1.x` = minor, `~1.x.x` = patch, `1.2.3` = exact, `>=1.0` = major.
- `package-lock.json` v3 format: `packages["node_modules/name"].version` = resolved version.
- `pnpm-lock.yaml`: parse with `goccy/go-yaml`.
- `bun.lock` (JSONC text, Bun v1.2+): strip comments and parse as JSON.

`testdata/javascript/package.json`:
```json
{
  "dependencies": {
    "express": "^4.18.2",
    "lodash": "4.17.21",
    "axios": ">=1.0.0"
  }
}
```

`testdata/javascript/package-lock.json`:
```json
{
  "lockfileVersion": 3,
  "packages": {
    "node_modules/express": { "version": "4.18.2" },
    "node_modules/lodash":  { "version": "4.17.21" },
    "node_modules/axios":   { "version": "1.6.5" }
  }
}
```

Tests to write for each:
- Package count matches fixture
- Constraint types parsed correctly (exact/patch/minor/major)
- Resolved versions come from lockfile
- Parent relationships populated for Rust (from Cargo.lock dependencies)

- [ ] **Step 1: Create fixture files** (as above)
- [ ] **Step 2: Write tests**
- [ ] **Step 3: Run — expect FAIL**
- [ ] **Step 4: Implement Rust parser**
- [ ] **Step 5: Implement JavaScript parser**
- [ ] **Step 6: Run — expect PASS** (`go test ./internal/manifest/... -v`)
- [ ] **Step 7: Commit** `"feat: Rust and JavaScript manifest parsers"`

---

## Task 9: Registry Client Interface + PyPI Client

**Files:**
- Create: `internal/registry/registry.go`
- Create: `internal/registry/pypi.go` + `pypi_test.go`
- Create: `internal/registry/testdata/pypi/requests.json`

- [ ] **Step 1: Capture golden file**

```bash
mkdir -p internal/registry/testdata/pypi
curl -s https://pypi.org/pypi/requests/json > internal/registry/testdata/pypi/requests.json
```

Verify the file has content: `jq '.info.name' internal/registry/testdata/pypi/requests.json`

- [ ] **Step 2: Define shared types**

`internal/registry/registry.go`:
```go
package registry

import "time"

type PackageInfo struct {
	Name             string
	Version          string
	Ecosystem        string
	TotalDownloads   int64
	MonthlyDownloads int64
	DownloadTrend    float64   // positive = growing
	LastReleaseAt    time.Time
	FirstReleaseAt   time.Time
	ReleaseCount     int
	MaintainerCount  int
	HasOrgBacking    bool
	SourceRepoURL    string
	IsDeprecated     bool
}

// Fetcher retrieves metadata for a single package.
type Fetcher interface {
	Fetch(name, version string) (*PackageInfo, error)
	Ecosystem() string
}

// Option is a functional option for registry clients (e.g. override base URL in tests).
type Option func(*clientOptions)

type clientOptions struct {
	baseURL    string
	httpClient interface{} // *http.Client
}

func WithBaseURL(url string) Option {
	return func(o *clientOptions) { o.baseURL = url }
}
```

- [ ] **Step 3: Write test**

`internal/registry/pypi_test.go`:
```go
package registry_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/depscope/depscope/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPyPIFetch(t *testing.T) {
	data, err := os.ReadFile("testdata/pypi/requests.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv.Close()

	client := registry.NewPyPIClient(registry.WithBaseURL(srv.URL))
	info, err := client.Fetch("requests", "2.31.0")
	require.NoError(t, err)
	assert.Equal(t, "requests", info.Name)
	assert.NotZero(t, info.LastReleaseAt)
	assert.Greater(t, info.MaintainerCount, 0)
}
```

- [ ] **Step 4: Run — expect FAIL**

```bash
go test ./internal/registry/... -run TestPyPI -v
```

- [ ] **Step 5: Implement PyPI client**

`internal/registry/pypi.go` — call `{baseURL}/pypi/{name}/json`, unmarshal the response:
- `info.name` → `Name`
- `info.maintainer` (comma-separated) + `info.author` → `MaintainerCount` (count unique)
- `releases[version][0].upload_time` → `LastReleaseAt`
- `info.project_urls.Source` or `info.home_page` → `SourceRepoURL`
- `info.classifiers` containing `"Development Status :: 7 - Inactive"` → `IsDeprecated = true`

- [ ] **Step 6: Run — expect PASS**

```bash
go test ./internal/registry/... -run TestPyPI -v
```

- [ ] **Step 7: Commit**

```bash
git add internal/registry/
git commit -m "feat: registry Fetcher interface and PyPI client with golden-file tests"
```

---

## Task 10: npm, crates.io, and Go Proxy Clients

Follow the exact same pattern as Task 9 for each registry.

**npm** (`https://registry.npmjs.org/{name}/{version}`):
- `maintainers` array → `MaintainerCount`
- `time[version]` → `LastReleaseAt`
- Download count: `https://api.npmjs.org/downloads/point/last-month/{name}` (separate request)

**crates.io** (`https://crates.io/api/v1/crates/{name}`):
- `crate.downloads` → `TotalDownloads`
- `versions[0].updated_at` → `LastReleaseAt`
- No maintainer count in public API → use GitHub contributor count if `SourceRepoURL` is known (defer to VCS client)

**Go proxy** (`https://proxy.golang.org/{module}/@v/{version}.info`):
- `Time` field → `LastReleaseAt`
- No download count available (skip `download_velocity` factor for Go — per spec)
- Contributor count via GitHub API on source repo (defer to VCS client)
- Note: Go proxy doesn't have maintainer info; `MaintainerCount` remains 0 until VCS resolves it

For each client:
- [ ] Capture golden file with curl, commit to `testdata/`
- [ ] Write test with `httptest.NewServer` serving the golden file
- [ ] Implement client
- [ ] Run — expect PASS
- [ ] Commit: `"feat: {npm|cratesio|goproxy} registry client"`

---

## Task 11: GitHub VCS Client

**Files:**
- Create: `internal/vcs/github.go`
- Create: `internal/vcs/github_test.go`
- Create: `internal/vcs/testdata/`

- [ ] **Step 1: Capture golden files**

```bash
mkdir -p internal/vcs/testdata
curl -s "https://api.github.com/repos/psf/requests" > internal/vcs/testdata/repo_requests.json
curl -s "https://api.github.com/repos/psf/requests/contributors?per_page=1" \
  > internal/vcs/testdata/contributors_requests.json
```

- [ ] **Step 2: Define types and write test**

`internal/vcs/github.go` will export:
```go
type RepoInfo struct {
	Owner            string
	Repo             string
	ContributorCount int
	OpenIssueCount   int
	ClosedIssueCount int
	StarCount        int
	LastCommitAt     time.Time
	IsArchived       bool
	HasOrgBacking    bool   // true if owner is a GitHub org (type == "Organization")
}

type Client interface {
	FetchRepo(owner, repo string) (*RepoInfo, error)
	// RepoFromURL parses a GitHub URL and calls FetchRepo.
	RepoFromURL(sourceURL string) (*RepoInfo, error)
}
```

Test assertions:
```go
assert.Greater(t, info.ContributorCount, 10)
assert.False(t, info.IsArchived)
assert.True(t, info.HasOrgBacking)
assert.NotZero(t, info.LastCommitAt)
```

- [ ] **Step 3: Implement using `go-github`**

Use `github.com/google/go-github/v68/github`. Inject `*http.Client` for testing (the `httptest.NewServer` approach). Attach `GITHUB_TOKEN` via `golang.org/x/oauth2` when available. GitHub `X-Total-Count` header gives contributor count (more efficient than paginating all contributors).

- [ ] **Step 4: Run — expect PASS**
- [ ] **Step 5: Commit** `"feat: GitHub VCS client for repo health signals"`

---

## Task 12: Vulnerability Clients

**Files:**
- Create: `internal/vuln/vuln.go`
- Create: `internal/vuln/osv.go` + `osv_test.go`
- Create: `internal/vuln/nvd.go` + `nvd_test.go`
- Create: `internal/vuln/testdata/`

- [ ] **Step 1: Define shared types**

`internal/vuln/vuln.go`:
```go
package vuln

type Severity string
const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
)

type Finding struct {
	ID       string
	Summary  string
	Severity Severity
	FixedIn  []string
	Source   string // "osv" or "nvd"
}

type Source interface {
	Query(ecosystem, name, version string) ([]Finding, error)
}
```

- [ ] **Step 2: Capture golden files**

```bash
curl -s -X POST https://api.osv.dev/v1/query \
  -d '{"version":"2.28.2","package":{"name":"requests","ecosystem":"PyPI"}}' \
  > internal/vuln/testdata/osv_requests.json
```

- [ ] **Step 3: Write tests for OSV and NVD** (httptest.NewServer pattern)

OSV test: assert that `requests 2.28.2` (older version with known CVEs) returns findings. Use golden file.

NVD test: assert that the client returns empty results when no API key is configured (graceful skip).

- [ ] **Step 4: Implement OSV client**

Use `osv.dev/bindings/go/osvdev`. Map `osvdev.Vulnerability` → `Finding`. Severity comes from `CVSS` or `database_specific.severity` fields.

- [ ] **Step 5: Implement NVD client**

`GET https://services.nvd.nist.gov/rest/json/cves/2.0?keywordSearch={name}&apiKey={key}`. Return empty when key is absent (log a debug message).

- [ ] **Step 6: Run — expect PASS**

```bash
go test ./internal/vuln/... -v
```

- [ ] **Step 7: Commit** `"feat: OSV.dev and NVD vulnerability clients"`

---

## Task 13: Parallel Fetcher Orchestrator

**Files:**
- Create: `internal/registry/fetcher.go`
- Create: `internal/registry/fetcher_test.go`

Takes `[]manifest.Package`, deduplicates, runs parallel fetches (registry + VCS + vuln), returns `map[string]*FetchResult` keyed by `pkg.Key()`.

- [ ] **Step 1: Write tests**

```go
func TestFetchAllDeduplicates(t *testing.T) {
	pkgs := []manifest.Package{
		{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython},
		{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython}, // dup
		{Name: "urllib3",  ResolvedVersion: "2.0.7",  Ecosystem: manifest.EcosystemPython},
	}
	fetchCount := 0
	stub := &stubFetcher{fn: func(name, version string) (*registry.PackageInfo, error) {
		fetchCount++
		return &registry.PackageInfo{Name: name}, nil
	}}
	results, err := registry.FetchAll(context.Background(), pkgs, stub, nil, nil,
		registry.FetchOptions{Concurrency: 5})
	require.NoError(t, err)
	assert.Equal(t, 2, fetchCount, "duplicate package should be fetched only once")
	assert.Len(t, results, 2)
}

func TestFetchAllContinuesOnError(t *testing.T) {
	pkgs := []manifest.Package{
		{Name: "good", ResolvedVersion: "1.0.0", Ecosystem: manifest.EcosystemPython},
		{Name: "bad",  ResolvedVersion: "1.0.0", Ecosystem: manifest.EcosystemPython},
	}
	stub := &stubFetcher{fn: func(name, version string) (*registry.PackageInfo, error) {
		if name == "bad" {
			return nil, errors.New("registry unavailable")
		}
		return &registry.PackageInfo{Name: name}, nil
	}}
	results, err := registry.FetchAll(context.Background(), pkgs, stub, nil, nil,
		registry.FetchOptions{Concurrency: 5})
	require.NoError(t, err, "FetchAll should not fail when one package errors")
	assert.NotNil(t, results["python/good@1.0.0"].Info)
	assert.NotNil(t, results["python/bad@1.0.0"].Err)
}
```

- [ ] **Step 2: Run — expect FAIL**
- [ ] **Step 3: Implement**

`internal/registry/fetcher.go`:
```go
package registry

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/vcs"
	"github.com/depscope/depscope/internal/vuln"
)

type FetchResult struct {
	Info     *PackageInfo
	RepoInfo *vcs.RepoInfo
	Vulns    []vuln.Finding
	Err      error
}

type FetchOptions struct {
	Concurrency int
}

func FetchAll(
	ctx context.Context,
	pkgs []manifest.Package,
	reg Fetcher,
	vcsClient vcs.Client,
	vulnSources []vuln.Source,
	opts FetchOptions,
) (map[string]*FetchResult, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 20
	}
	sem := semaphore.NewWeighted(int64(opts.Concurrency))

	// Deduplicate by Key()
	unique := make(map[string]manifest.Package)
	for _, p := range pkgs {
		unique[p.Key()] = p
	}

	results := make(map[string]*FetchResult, len(unique))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for key, pkg := range unique {
		key, pkg := key, pkg
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sem.Acquire(ctx, 1); err != nil {
				mu.Lock()
				results[key] = &FetchResult{Err: err}
				mu.Unlock()
				return
			}
			defer sem.Release(1)

			res := &FetchResult{}
			info, err := reg.Fetch(pkg.Name, pkg.ResolvedVersion)
			if err != nil {
				res.Err = err
			} else {
				res.Info = info
				if vcsClient != nil && info.SourceRepoURL != "" {
					if repoInfo, err := vcsClient.RepoFromURL(info.SourceRepoURL); err == nil {
						res.RepoInfo = repoInfo
					}
				}
				for _, vs := range vulnSources {
					if findings, err := vs.Query(pkg.Ecosystem.String(), pkg.Name, pkg.ResolvedVersion); err == nil {
						res.Vulns = append(res.Vulns, findings...)
					}
				}
			}

			mu.Lock()
			results[key] = res
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results, nil
}
```

Note: `pkg.Ecosystem.String()` — add a `String()` method to `Ecosystem` in `manifest.go` that returns the OSV-compatible ecosystem name (PyPI, Go, crates.io, npm).

- [ ] **Step 4: Run — expect PASS**
- [ ] **Step 5: Commit** `"feat: parallel fetch orchestrator with dedup and semaphore"`

---

## Task 14: Individual Factor Scorers

**Files:**
- Create: `internal/core/factors.go`
- Create: `internal/core/factors_test.go`

All in `package core`. Each factor is an exported function returning `(int, []Issue)`. The `int` is the raw factor score 0–100.

- [ ] **Step 1: Write tests**

`internal/core/factors_test.go`:
```go
package core_test

import (
	"testing"
	"time"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
	"github.com/stretchr/testify/assert"
)

func TestFactorReleaseRecency(t *testing.T) {
	fresh := &registry.PackageInfo{LastReleaseAt: time.Now().Add(-3 * 30 * 24 * time.Hour)}
	score, issues := core.FactorReleaseRecency(fresh)
	assert.Greater(t, score, 70)
	assert.Empty(t, issues)

	old := &registry.PackageInfo{LastReleaseAt: time.Now().Add(-3 * 365 * 24 * time.Hour)}
	score2, issues2 := core.FactorReleaseRecency(old)
	assert.Less(t, score2, 40)
	assert.NotEmpty(t, issues2)
}

func TestFactorMaintainerCount(t *testing.T) {
	solo := &registry.PackageInfo{MaintainerCount: 1}
	score, issues := core.FactorMaintainerCount(solo)
	assert.Less(t, score, 40)
	assert.NotEmpty(t, issues)

	team := &registry.PackageInfo{MaintainerCount: 5}
	score2, _ := core.FactorMaintainerCount(team)
	assert.GreaterOrEqual(t, score2, 80)
}

func TestFactorVersionPinning(t *testing.T) {
	cases := []struct { ct manifest.ConstraintType; expected int }{
		{manifest.ConstraintExact, 100},
		{manifest.ConstraintPatch, 75},
		{manifest.ConstraintMinor, 50},
		{manifest.ConstraintMajor, 25},
	}
	for _, c := range cases {
		score, _ := core.FactorVersionPinning(c.ct)
		assert.Equal(t, c.expected, score, string(c.ct))
	}
}

func TestFactorVersionPinningLooseEmitsIssue(t *testing.T) {
	_, issues := core.FactorVersionPinning(manifest.ConstraintMajor)
	assert.NotEmpty(t, issues)
	assert.Equal(t, core.SeverityHigh, issues[0].Severity)
}

func TestFactorRepoHealthArchivedRepo(t *testing.T) {
	archived := &vcs.RepoInfo{IsArchived: true}
	score, issues := core.FactorRepoHealth(archived)
	assert.Equal(t, 0, score)
	assert.NotEmpty(t, issues)
}
```

- [ ] **Step 2: Run — expect FAIL**
- [ ] **Step 3: Implement all 7 factors**

`internal/core/factors.go`:
```go
package core

import (
	"time"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
)

// FactorReleaseRecency: <6mo=100, <1yr=80, <2yr=60, <3yr=40, ≥3yr=20
func FactorReleaseRecency(info *registry.PackageInfo) (int, []Issue) {
	if info == nil || info.LastReleaseAt.IsZero() {
		return 0, []Issue{{Severity: SeverityHigh, Message: "no release information available"}}
	}
	age := time.Since(info.LastReleaseAt)
	switch {
	case age < 6*30*24*time.Hour:
		return 100, nil
	case age < 365*24*time.Hour:
		return 80, nil
	case age < 2*365*24*time.Hour:
		return 60, nil
	case age < 3*365*24*time.Hour:
		return 40, []Issue{{Severity: SeverityMedium, Message: "last release over 2 years ago"}}
	default:
		return 20, []Issue{{Severity: SeverityHigh, Message: "last release over 3 years ago — possibly abandoned"}}
	}
}

// FactorMaintainerCount: 1=20, 2=50, 3-4=75, 5+=100
func FactorMaintainerCount(info *registry.PackageInfo) (int, []Issue) {
	if info == nil {
		return 0, nil
	}
	switch {
	case info.MaintainerCount <= 0:
		return 0, []Issue{{Severity: SeverityMedium, Message: "no maintainer information"}}
	case info.MaintainerCount == 1:
		return 20, []Issue{{Severity: SeverityHigh, Message: "solo maintainer — bus factor risk"}}
	case info.MaintainerCount == 2:
		return 50, []Issue{{Severity: SeverityMedium, Message: "only 2 maintainers"}}
	case info.MaintainerCount <= 4:
		return 75, nil
	default:
		return 100, nil
	}
}

// FactorDownloadVelocity: returns 0,nil for Go packages (no data available).
func FactorDownloadVelocity(info *registry.PackageInfo) (int, []Issue) {
	if info == nil || info.MonthlyDownloads == 0 {
		return 0, nil // caller must skip this factor (redistribute weight)
	}
	switch {
	case info.MonthlyDownloads > 1_000_000:
		return 100, nil
	case info.MonthlyDownloads > 100_000:
		return 80, nil
	case info.MonthlyDownloads > 10_000:
		return 60, nil
	case info.MonthlyDownloads > 1_000:
		return 40, nil
	default:
		return 20, []Issue{{Severity: SeverityLow, Message: "very low download volume"}}
	}
}

// FactorOpenIssueRatio: penalizes high open/total ratio.
func FactorOpenIssueRatio(repo *vcs.RepoInfo) (int, []Issue) {
	if repo == nil {
		return 50, nil // neutral when no data
	}
	total := repo.OpenIssueCount + repo.ClosedIssueCount
	if total == 0 {
		return 80, nil
	}
	ratio := float64(repo.OpenIssueCount) / float64(total)
	switch {
	case ratio < 0.1:
		return 100, nil
	case ratio < 0.25:
		return 75, nil
	case ratio < 0.5:
		return 50, []Issue{{Severity: SeverityMedium, Message: "high open issue ratio"}}
	default:
		return 20, []Issue{{Severity: SeverityHigh, Message: "very high open issue ratio — low maintenance activity"}}
	}
}

// FactorOrgBacking: org-backed packages score higher than solo-individual packages.
func FactorOrgBacking(repo *vcs.RepoInfo, info *registry.PackageInfo) (int, []Issue) {
	backed := (repo != nil && repo.HasOrgBacking) || (info != nil && info.HasOrgBacking)
	if backed {
		return 100, nil
	}
	return 30, []Issue{{Severity: SeverityLow, Message: "no org/company backing — individual maintainer"}}
}

// FactorVersionPinning scores the constraint type declared in the manifest.
func FactorVersionPinning(ct manifest.ConstraintType) (int, []Issue) {
	switch ct {
	case manifest.ConstraintExact:
		return 100, nil
	case manifest.ConstraintPatch:
		return 75, []Issue{{Severity: SeverityLow, Message: "patch-level version constraint"}}
	case manifest.ConstraintMinor:
		return 50, []Issue{{Severity: SeverityMedium, Message: "minor-level constraint — supply chain risk"}}
	default:
		return 25, []Issue{{Severity: SeverityHigh, Message: "open/major version constraint — supply chain attack surface"}}
	}
}

// FactorRepoHealth: commit frequency, archived status, star trend.
func FactorRepoHealth(repo *vcs.RepoInfo) (int, []Issue) {
	if repo == nil {
		return 35, []Issue{{Severity: SeverityMedium, Message: "no source repository found"}}
	}
	if repo.IsArchived {
		return 0, []Issue{{Severity: SeverityHigh, Message: "source repository is archived — no further development"}}
	}
	age := time.Since(repo.LastCommitAt)
	switch {
	case age < 90*24*time.Hour:
		return 100, nil
	case age < 365*24*time.Hour:
		return 70, nil
	case age < 2*365*24*time.Hour:
		return 40, []Issue{{Severity: SeverityMedium, Message: "no commits in over a year"}}
	default:
		return 10, []Issue{{Severity: SeverityHigh, Message: "no commits in over 2 years"}}
	}
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/core/... -run TestFactor -v
```

- [ ] **Step 5: Commit** `"feat: individual factor scorers with issue generation"`

---

## Task 15: Scorer Orchestrator

**Files:**
- Create: `internal/core/scorer.go`
- Create: `internal/core/scorer_test.go`

`Score()` takes a `manifest.Package`, its `*registry.FetchResult`, and `config.Weights`, runs all 7 factors, redistributes weight for unavailable factors (Go download velocity), and returns a `PackageResult` with `OwnScore` set.

- [ ] **Step 1: Write tests**

`internal/core/scorer_test.go`:
```go
package core_test

import (
	"testing"
	"time"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
)

func TestScorerHealthyPackage(t *testing.T) {
	pkg := manifest.Package{
		Name: "requests", ResolvedVersion: "2.31.0",
		ConstraintType: manifest.ConstraintExact, Ecosystem: manifest.EcosystemPython, Depth: 1,
	}
	fr := &registry.FetchResult{
		Info: &registry.PackageInfo{
			MaintainerCount: 3, HasOrgBacking: true,
			LastReleaseAt: time.Now().Add(-3 * 30 * 24 * time.Hour),
			MonthlyDownloads: 5_000_000,
		},
		RepoInfo: &vcs.RepoInfo{
			ContributorCount: 50, OpenIssueCount: 10, ClosedIssueCount: 200,
			IsArchived: false, HasOrgBacking: true,
			LastCommitAt: time.Now().Add(-7 * 24 * time.Hour),
		},
	}
	result := core.Score(pkg, fr, config.Enterprise().Weights)
	assert.Greater(t, result.OwnScore, 75, "healthy package should exceed enterprise threshold")
}

func TestScorerAbandonedPackage(t *testing.T) {
	pkg := manifest.Package{
		Name: "abandoned", ConstraintType: manifest.ConstraintMajor,
		Ecosystem: manifest.EcosystemNPM, Depth: 1,
	}
	fr := &registry.FetchResult{
		Info: &registry.PackageInfo{
			MaintainerCount: 1,
			LastReleaseAt:   time.Now().Add(-4 * 365 * 24 * time.Hour),
		},
		RepoInfo: &vcs.RepoInfo{IsArchived: true},
	}
	result := core.Score(pkg, fr, config.Enterprise().Weights)
	assert.Less(t, result.OwnScore, 40, "abandoned package should be Critical")
	assert.NotEmpty(t, result.Issues)
}

func TestScorerGoPackageSkipsDownloadVelocity(t *testing.T) {
	pkg := manifest.Package{
		Name: "golang.org/x/sync", ResolvedVersion: "v0.6.0",
		ConstraintType: manifest.ConstraintExact, Ecosystem: manifest.EcosystemGo, Depth: 1,
	}
	fr := &registry.FetchResult{
		Info: &registry.PackageInfo{
			MaintainerCount: 5, LastReleaseAt: time.Now().Add(-30 * 24 * time.Hour),
			MonthlyDownloads: 0, // not available for Go
		},
	}
	weights := config.Enterprise().Weights
	result := core.Score(pkg, fr, weights)
	// Should not panic; download_velocity weight redistributed
	assert.Greater(t, result.OwnScore, 0)
}
```

- [ ] **Step 2: Run — expect FAIL**
- [ ] **Step 3: Implement**

`internal/core/scorer.go`:
```go
package core

import (
	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
)

type factorScore struct {
	score     int
	issues    []Issue
	available bool // false = skip this factor (redistribute weight)
}

// Score computes OwnScore for one package. TransitiveRiskScore is set by Propagate() later.
func Score(pkg manifest.Package, fr *registry.FetchResult, weights config.Weights) PackageResult {
	var info *registry.PackageInfo
	var repoInfo *vcs.RepoInfo // imported from internal/vcs
	if fr != nil {
		info = fr.Info
		repoInfo = fr.RepoInfo
	}

	factors := map[string]factorScore{}

	s, issues := FactorReleaseRecency(info)
	factors["release_recency"] = factorScore{score: s, issues: issues, available: true}

	s, issues = FactorMaintainerCount(info)
	factors["maintainer_count"] = factorScore{score: s, issues: issues, available: true}

	s, issues = FactorDownloadVelocity(info)
	available := info != nil && info.MonthlyDownloads > 0
	factors["download_velocity"] = factorScore{score: s, issues: issues, available: available}

	s, issues = FactorOpenIssueRatio(repoInfo)
	factors["open_issue_ratio"] = factorScore{score: s, issues: issues, available: true}

	s, issues = FactorOrgBacking(repoInfo, info)
	factors["org_backing"] = factorScore{score: s, issues: issues, available: true}

	s, issues = FactorVersionPinning(pkg.ConstraintType)
	factors["version_pinning"] = factorScore{score: s, issues: issues, available: true}

	s, issues = FactorRepoHealth(repoInfo)
	factors["repo_health"] = factorScore{score: s, issues: issues, available: true}

	// Compute active weights: redistribute unavailable factor weights proportionally
	activeWeights := redistributeUnavailable(weights, factors)

	total := 0
	for name, fs := range factors {
		if w, ok := activeWeights[name]; ok && fs.available {
			total += fs.score * w
		}
	}
	ownScore := clamp(total/100, 0, 100)

	var allIssues []Issue
	for _, fs := range factors {
		allIssues = append(allIssues, fs.issues...)
	}

	return PackageResult{
		Name: pkg.Name, Version: pkg.ResolvedVersion,
		Ecosystem: string(pkg.Ecosystem), ConstraintType: string(pkg.ConstraintType),
		Depth: pkg.Depth,
		OwnScore: ownScore, OwnRisk: RiskLevelFromScore(ownScore),
		TransitiveRiskScore: 100, // default: no transitive risk until Propagate() runs
		TransitiveRisk:      RiskLow,
		Issues: allIssues,
	}
}

func clamp(v, min, max int) int {
	if v < min { return min }
	if v > max { return max }
	return v
}

// redistributeUnavailable returns a new weight map with unavailable factors' weights
// distributed proportionally to the remaining available factors.
func redistributeUnavailable(weights config.Weights, factors map[string]factorScore) config.Weights {
	unavailableWeight := 0
	availableBaseWeight := 0
	for name, fs := range factors {
		w := weights[name]
		if !fs.available {
			unavailableWeight += w
		} else {
			availableBaseWeight += w
		}
	}
	if unavailableWeight == 0 {
		return weights
	}
	result := make(config.Weights)
	for name, fs := range factors {
		if !fs.available {
			result[name] = 0
			continue
		}
		w := weights[name]
		extra := int(float64(w) / float64(availableBaseWeight) * float64(unavailableWeight))
		result[name] = w + extra
	}
	// Fix rounding
	total := 0
	for _, v := range result { total += v }
	if diff := 100 - total; diff != 0 {
		for name := range factors {
			if factors[name].available {
				result[name] += diff
				break
			}
		}
	}
	return result
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/core/... -run TestScorer -v
```

- [ ] **Step 5: Commit** `"feat: scorer orchestrator with weight redistribution for unavailable factors"`

---

## Task 16: Transitive Risk Propagator

**Files:**
- Create: `internal/core/propagator.go`
- Create: `internal/core/propagator_test.go`

**Key design:** `EffectiveScore(ownScore, depth int)` uses the package's absolute depth (as stored in `Package.Depth` / `PackageResult.Depth`). Depth 1 = direct dep, no discount. Each additional depth level adds 5 points (makes deeper risks less severe from the root's perspective).

Formula: `effective = clamp(ownScore + (depth-1)*5, 0, 100)`

For package P, `TransitiveRiskScore = min over all descendants D: EffectiveScore(D.OwnScore, D.Depth)`.

"Descendants of P" = all packages reachable from P in the dependency graph (using `deps` map).

- [ ] **Step 1: Write tests**

`internal/core/propagator_test.go`:
```go
package core_test

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestEffectiveScore(t *testing.T) {
	// Depth 1 (direct dep): no discount
	assert.Equal(t, 25, core.EffectiveScore(25, 1))
	assert.Equal(t, core.RiskCritical, core.RiskLevelFromScore(core.EffectiveScore(25, 1)))

	// Depth 5: +20 discount → 45 (High)
	assert.Equal(t, 45, core.EffectiveScore(25, 5))
	assert.Equal(t, core.RiskHigh, core.RiskLevelFromScore(core.EffectiveScore(25, 5)))

	// Depth 10: +45 discount → 70 (Medium)
	assert.Equal(t, 70, core.EffectiveScore(25, 10))
	assert.Equal(t, core.RiskMedium, core.RiskLevelFromScore(core.EffectiveScore(25, 10)))

	// Clamp: score 90 at depth 5 → 110 → clamped to 100
	assert.Equal(t, 100, core.EffectiveScore(90, 5))
}

func TestPropagate(t *testing.T) {
	// root (depth 1) → middle (depth 2) → bad (depth 3, own score 25)
	results := []core.PackageResult{
		{Name: "root",   OwnScore: 90, Depth: 1, TransitiveRiskScore: 100},
		{Name: "middle", OwnScore: 75, Depth: 2, TransitiveRiskScore: 100},
		{Name: "bad",    OwnScore: 25, Depth: 3, TransitiveRiskScore: 100},
	}
	deps := map[string][]string{
		"root":   {"middle"},
		"middle": {"bad"},
	}
	propagated := core.Propagate(results, deps)

	byName := make(map[string]core.PackageResult)
	for _, r := range propagated {
		byName[r.Name] = r
	}

	// bad has no deps → TransitiveRiskScore = 100 (no transitive risk from itself)
	assert.Equal(t, 100, byName["bad"].TransitiveRiskScore)

	// middle depends on bad@depth3 → effective = 25 + (3-1)*5 = 35
	assert.Equal(t, 35, byName["middle"].TransitiveRiskScore)
	assert.Equal(t, core.RiskCritical, byName["middle"].TransitiveRisk)

	// root depends on middle (depth2=40) and bad via middle (depth3=35) → min = 35
	assert.Equal(t, 35, byName["root"].TransitiveRiskScore)
}

func TestPropagateNoRisk(t *testing.T) {
	results := []core.PackageResult{
		{Name: "a", OwnScore: 85, Depth: 1, TransitiveRiskScore: 100},
		{Name: "b", OwnScore: 90, Depth: 2, TransitiveRiskScore: 100},
	}
	deps := map[string][]string{"a": {"b"}}
	propagated := core.Propagate(results, deps)
	for _, r := range propagated {
		if r.Name == "a" {
			// b at depth 2 → effective = 90 + 5 = 95, still Low
			assert.Equal(t, core.RiskLow, r.TransitiveRisk)
		}
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/core/... -run TestEffective -run TestPropagate -v
```

- [ ] **Step 3: Implement**

`internal/core/propagator.go`:
```go
package core

// EffectiveScore discounts a package's own score based on its absolute depth.
// Depth 1 (direct dep) = no discount. Each additional level adds 5 points.
// clamp to [0, 100].
func EffectiveScore(ownScore, depth int) int {
	discounted := ownScore + (depth-1)*5
	if discounted > 100 {
		return 100
	}
	if discounted < 0 {
		return 0
	}
	return discounted
}

// Propagate computes TransitiveRiskScore for each package in results.
// deps maps package name → list of direct dependency names.
// Uses each dependency's Depth field (absolute depth from the scan root) for discounting.
func Propagate(results []PackageResult, deps map[string][]string) []PackageResult {
	byName := make(map[string]*PackageResult, len(results))
	for i := range results {
		byName[results[i].Name] = &results[i]
	}

	for i := range results {
		minEffective := 100
		visited := make(map[string]bool)
		var walk func(name string)
		walk = func(name string) {
			for _, depName := range deps[name] {
				if visited[depName] {
					continue
				}
				visited[depName] = true
				if dep, ok := byName[depName]; ok {
					eff := EffectiveScore(dep.OwnScore, dep.Depth)
					if eff < minEffective {
						minEffective = eff
					}
					walk(depName)
				}
			}
		}
		walk(results[i].Name)

		results[i].TransitiveRiskScore = minEffective
		results[i].TransitiveRisk = RiskLevelFromScore(minEffective)
	}
	return results
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/core/... -run TestEffective -run TestPropagate -v
```

- [ ] **Step 5: Commit** `"feat: transitive risk propagator using absolute depth discounting"`

---

## Task 17: Report Formatters

**Files:**
- Create: `internal/report/report.go`
- Create: `internal/report/text.go` + `text_test.go`
- Create: `internal/report/json.go` + `json_test.go`
- Create: `internal/report/sarif.go` + `sarif_test.go`

- [ ] **Step 1: Define shared helpers and test fixture**

`internal/report/report.go`:
```go
package report

import (
	"github.com/depscope/depscope/internal/core"
)

// SampleScanResult returns a predictable ScanResult for use in formatter tests.
func SampleScanResult() core.ScanResult {
	return core.ScanResult{
		Profile: "enterprise", PassThreshold: 70,
		DirectDeps: 2, TransitiveDeps: 5,
		Packages: []core.PackageResult{
			{
				Name: "requests", Version: "2.31.0", Ecosystem: "python",
				ConstraintType: "exact", Depth: 1,
				OwnScore: 82, TransitiveRiskScore: 82,
				OwnRisk: core.RiskLow, TransitiveRisk: core.RiskLow,
			},
			{
				Name: "urllib3", Version: "2.0.7", Ecosystem: "python",
				ConstraintType: "minor", Depth: 1,
				OwnScore: 40, TransitiveRiskScore: 40,
				OwnRisk: core.RiskHigh, TransitiveRisk: core.RiskHigh,
				Issues: []core.Issue{
					{Package: "urllib3", Severity: core.SeverityHigh, Message: "solo maintainer"},
				},
			},
		},
		AllIssues: []core.Issue{
			{Package: "urllib3", Severity: core.SeverityHigh, Message: "solo maintainer"},
		},
	}
}
```

- [ ] **Step 2: Write tests**

`internal/report/text_test.go`:
```go
package report_test

import (
	"bytes"
	"testing"

	"github.com/depscope/depscope/internal/report"
	"github.com/stretchr/testify/assert"
)

func TestTextReportContainsPackageNames(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	report.WriteText(&buf, result)
	out := buf.String()
	assert.Contains(t, out, "requests")
	assert.Contains(t, out, "urllib3")
	assert.Contains(t, out, "LOW")
	assert.Contains(t, out, "HIGH")
}

func TestTextReportShowsFailWhenBelowThreshold(t *testing.T) {
	result := report.SampleScanResult() // urllib3 score 40 < threshold 70
	var buf bytes.Buffer
	report.WriteText(&buf, result)
	assert.Contains(t, buf.String(), "FAIL")
}

func TestTextReportShowsPassWhenAboveThreshold(t *testing.T) {
	result := report.SampleScanResult()
	result.Packages[1].OwnScore = 80
	result.Packages[1].TransitiveRiskScore = 80
	result.Packages[1].OwnRisk = "LOW"
	var buf bytes.Buffer
	report.WriteText(&buf, result)
	assert.Contains(t, buf.String(), "PASS")
}
```

`internal/report/json_test.go`:
```go
package report_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/depscope/depscope/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONReportValid(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, report.WriteJSON(&buf, report.SampleScanResult()))
	var out map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Contains(t, out, "packages")
	assert.Contains(t, out, "passed")
	assert.Contains(t, out, "profile")
}
```

`internal/report/sarif_test.go`:
```go
package report_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/depscope/depscope/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSARIFVersion(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, report.WriteSARIF(&buf, report.SampleScanResult()))
	var out map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "2.1.0", out["version"])
}
```

- [ ] **Step 3: Run — expect FAIL**
- [ ] **Step 4: Implement text reporter** — use `tablewriter` for the package table; iterate `AllIssues` for the issue log; print `Result: PASS` or `Result: FAIL`.
- [ ] **Step 5: Implement JSON reporter** — `json.NewEncoder(w).Encode(struct{ ...ScanResult; Passed bool }{...})`
- [ ] **Step 6: Implement SARIF reporter** — use `go-sarif/v3`; map each `core.Issue` with severity HIGH/CRITICAL to a SARIF result; one SARIF rule per unique message.
- [ ] **Step 7: Run — expect PASS**

```bash
go test ./internal/report/... -v
```

- [ ] **Step 8: Commit** `"feat: text, JSON, and SARIF report formatters"`

---

## Task 18: CLI Scan Command

**Files:**
- Create: `cmd/depscope/scan.go`
- Create: `cmd/depscope/run.go` (shared `runE` wrapper for testable exit codes)

The `scan` command must return an exit code without calling `os.Exit()` directly, so it can be tested. Use a `RunE` that returns a sentinel error for non-zero exits, and have `main()` translate that to an exit code.

- [ ] **Step 1: Add exit code handling to main**

`cmd/depscope/main.go` — update to handle a special exit-code error:
```go
type exitError struct{ code int }
func (e exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

func main() {
	if err := rootCmd.Execute(); err != nil {
		var ee exitError
		if errors.As(err, &ee) {
			os.Exit(ee.code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Write integration test**

`cmd/depscope/scan_test.go`:
```go
package main_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanCommandOutputsTable(t *testing.T) {
	// Uses testdata/fixture-python/ with stubbed registry clients
	var stdout bytes.Buffer
	code, err := runScanCommand(&stdout, "testdata/fixture-python", "--profile", "hobby", "--output", "text")
	require.NoError(t, err)
	out := stdout.String()
	assert.Contains(t, out, "Scanned")
	// exit code 0 or 1 depending on fixture scores — just verify it ran
	assert.True(t, code == 0 || code == 1)
}

func TestScanCommandJSON(t *testing.T) {
	var stdout bytes.Buffer
	_, err := runScanCommand(&stdout, "testdata/fixture-python", "--output", "json")
	require.NoError(t, err)
	// Verify output is valid JSON
	assert.True(t, json.Valid(stdout.Bytes()))
}
```

`runScanCommand` is a test helper defined in `scan_test.go` that creates a fresh cobra command, injects a stub registry fetcher, runs it, and captures stdout + exit code. This avoids `os.Exit` being called in tests.

- [ ] **Step 3: Run — expect FAIL**

```bash
go test ./cmd/depscope/... -v
```

- [ ] **Step 4: Create test fixture**

`cmd/depscope/testdata/fixture-python/requirements.txt`:
```
requests==2.31.0
urllib3==2.0.7
```

- [ ] **Step 5: Implement scan command**

`cmd/depscope/scan.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/report"
)

var scanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Scan a project's dependencies for supply chain risk",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runScan,
}

func init() {
	scanCmd.Flags().String("profile", "enterprise", "Risk profile: hobby|opensource|enterprise")
	scanCmd.Flags().String("config", "", "Path to depscope.yaml config file")
	scanCmd.Flags().String("output", "text", "Output format: text|json|sarif")
	scanCmd.Flags().Int("depth", 0, "Max dependency depth (0 = profile default)")
	scanCmd.Flags().Bool("verbose", false, "Show full package metadata")
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 { dir = args[0] }

	cfg, err := loadConfig(cmd)
	if err != nil { return err }

	eco, err := manifest.DetectEcosystem(dir)
	if err != nil { return fmt.Errorf("manifest detection: %w", err) }

	pkgs, err := manifest.ParserFor(eco).Parse(dir)
	if err != nil { return fmt.Errorf("manifest parse: %w", err) }

	ch := cache.NewDiskCache(cache.DefaultDir())
	reg := registry.NewCachedFetcher(buildRegistryFetcher(eco), ch, cfg.CacheTTL.Metadata)
	vcsClient := vcs.NewGitHubClient(cfg.Registries.GitHubToken)
	vulnSources := buildVulnSources(cfg)

	fetchResults, err := registry.FetchAll(cmd.Context(), pkgs, reg, vcsClient, vulnSources,
		registry.FetchOptions{Concurrency: cfg.Concurrency})
	if err != nil { return err }

	var results []core.PackageResult
	for _, pkg := range pkgs {
		fr := fetchResults[pkg.Key()]
		results = append(results, core.Score(pkg, fr, cfg.Weights))
	}

	deps := manifest.BuildDepsMap(pkgs)
	results = core.Propagate(results, deps)

	scanResult := core.ScanResult{
		Profile: cfg.Profile, PassThreshold: cfg.PassThreshold,
		Packages: results,
	}
	for _, r := range results {
		scanResult.AllIssues = append(scanResult.AllIssues, r.Issues...)
	}

	outputFmt, _ := cmd.Flags().GetString("output")
	switch outputFmt {
	case "json":
		report.WriteJSON(os.Stdout, scanResult)
	case "sarif":
		report.WriteSARIF(os.Stdout, scanResult)
	default:
		report.WriteText(os.Stdout, scanResult)
	}

	if !scanResult.Passed() {
		return exitError{1}
	}
	return nil
}
```

Also implement:
- `loadConfig(cmd)` — reads `--config` flag if set, else uses `--profile` flag
- `buildRegistryFetcher(eco)` — returns the right registry client for the ecosystem
- `buildVulnSources(cfg)` — builds OSV and optionally NVD clients based on config

- [ ] **Step 6: Run — expect PASS**

```bash
go test ./cmd/depscope/... -v
make build
```

- [ ] **Step 7: Commit** `"feat: scan command wiring manifest/fetch/score/propagate/report pipeline"`

---

## Task 19: CLI Package + Cache Commands

**Files:**
- Create: `cmd/depscope/package.go`
- Create: `cmd/depscope/cache.go`

- [ ] **Step 1: Write tests**

`cmd/depscope/package_test.go`:
```go
func TestPackageCheckCommand(t *testing.T) {
	// Stub registry fetcher returns a predictable PackageInfo
	stub := &stubFetcher{fn: func(name, ver string) (*registry.PackageInfo, error) {
		return &registry.PackageInfo{
			Name: name, MaintainerCount: 3,
			LastReleaseAt: time.Now().Add(-30 * 24 * time.Hour),
		}, nil
	}}
	var stdout bytes.Buffer
	code, err := runPackageCheck(&stdout, stub, "requests==2.31.0", "--ecosystem", "python")
	require.NoError(t, err)
	assert.Equal(t, 0, code) // healthy package should pass hobby profile default
	assert.Contains(t, stdout.String(), "requests")
	assert.Contains(t, stdout.String(), "Score")
}
```

`cmd/depscope/cache_test.go`:
```go
func TestCacheStatusNoCache(t *testing.T) {
	var stdout bytes.Buffer
	// Point at empty temp dir
	runCacheStatus(&stdout, t.TempDir())
	assert.Contains(t, stdout.String(), "0 entries")
}
```

- [ ] **Step 2: Run — expect FAIL**
- [ ] **Step 3: Implement package command**

`cmd/depscope/package.go` — parse `name==version` arg, call registry fetch directly, score, print single-row table + issues.

- [ ] **Step 4: Implement cache command**

`cmd/depscope/cache.go` — `cache status` prints count + size; `cache clear` calls `DiskCache.Clear()`.

- [ ] **Step 5: Run — expect PASS**

```bash
go test ./cmd/depscope/... -run TestPackage -run TestCache -v
```

- [ ] **Step 6: Commit** `"feat: package check and cache management commands"`

---

## Task 20: Full Test Suite + CI

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Run full test suite with race detector**

```bash
go test ./... -race -count=1 -v 2>&1 | tail -30
```
Expected: all PASS, no race conditions.

- [ ] **Step 2: Build both binaries**

```bash
make build
./bin/depscope --help
./bin/depscope scan --help
./bin/depscope package check --help
./bin/depscope cache status
```

- [ ] **Step 3: Create GitHub Actions CI**

`.github/workflows/ci.yml`:
```yaml
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.22" }
      - run: go test ./... -race -count=1
      - run: go build ./cmd/depscope
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.22" }
      - uses: golangci/golangci-lint-action@v6
```

- [ ] **Step 4: Verify goreleaser snapshot**

```bash
goreleaser release --snapshot --clean
ls dist/
```
Expected: binaries for linux/darwin/windows × amd64/arm64.

- [ ] **Step 5: Commit**

```bash
git add .github/
git commit -m "chore: GitHub Actions CI and goreleaser snapshot verification"
```

---

## Done

After Task 20, `depscope scan` is a fully functional CLI that:
- Auto-detects ecosystem from the project directory
- Parses all supported manifest + lockfile formats (Go, Python, Rust, JS/TS)
- Fetches registry metadata, GitHub VCS signals, and CVE data in parallel with caching
- Scores each package with 7 weighted factors, handling missing data gracefully
- Propagates transitive risk with depth discounting
- Outputs text table, JSON, or SARIF
- Exits 0/1 based on a configurable profile threshold

**Next plans:**
- `2026-03-20-depscope-server.md` — HTTP API server wrapping the same core engine
- `2026-03-20-depscope-web.md` — React web UI for depscope.com
