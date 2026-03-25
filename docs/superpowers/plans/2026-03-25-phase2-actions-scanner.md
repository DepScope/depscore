# Phase 2: GitHub Actions Scanner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add GitHub Actions scanning to depscope — parse workflows, resolve action references through 5 layers (tag→SHA, action.yml, composite steps, bundled code, reusable workflows), detect Docker image refs and curl-pipe-bash patterns, score actions with pinning-aware factors, and output a pinning summary.

**Architecture:** New `internal/actions` package with 8 files handling workflow parsing, ref resolution, composite/bundled/reusable analysis, and scoring. Integrates into the existing scan pipeline via the graph infrastructure (Phase 1). Actions, workflows, Docker images, and script downloads become nodes in the supply chain graph.

**Tech Stack:** Go, `gopkg.in/yaml.v3` (already in go.mod as `go.yaml.in/yaml/v3`), GitHub API for ref resolution, existing manifest parsers for bundled code, `internal/graph` from Phase 1.

**Spec:** `docs/superpowers/specs/2026-03-25-supply-chain-graph-actions-design.md` (Phase 2 section)

**Depends on:** Phase 1 (graph infrastructure) must be merged first.

---

### Task 1: Action types — `internal/actions/types.go`

**Files:**
- Create: `internal/actions/types.go`
- Test: `internal/actions/types_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/actions/types_test.go
package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActionTypeString(t *testing.T) {
	assert.Equal(t, "composite", ActionComposite.String())
	assert.Equal(t, "node", ActionNode.String())
	assert.Equal(t, "docker", ActionDocker.String())
}

func TestParseActionRef(t *testing.T) {
	tests := []struct {
		input string
		want  ActionRef
	}{
		{"actions/checkout@v4", ActionRef{Owner: "actions", Repo: "checkout", Ref: "v4", Path: ""}},
		{"actions/checkout@abc123def", ActionRef{Owner: "actions", Repo: "checkout", Ref: "abc123def", Path: ""}},
		{"org/repo/sub/path@v1", ActionRef{Owner: "org", Repo: "repo", Ref: "v1", Path: "sub/path"}},
		{"docker://alpine:3.19", ActionRef{DockerImage: "alpine:3.19"}},
		{"./local-action", ActionRef{LocalPath: "./local-action"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseActionRef(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestActionRefIsFirstParty(t *testing.T) {
	assert.True(t, ActionRef{Owner: "actions", Repo: "checkout"}.IsFirstParty())
	assert.True(t, ActionRef{Owner: "github", Repo: "codeql-action"}.IsFirstParty())
	assert.False(t, ActionRef{Owner: "some-org", Repo: "deploy"}.IsFirstParty())
}

func TestClassifyPinning(t *testing.T) {
	tests := []struct {
		ref  string
		want PinQuality
	}{
		{"abc123def456abc123def456abc123def456abcdef", PinSHA},
		{"v4.2.0", PinExactVersion},
		{"v4", PinMajorTag},
		{"main", PinBranch},
		{"", PinUnpinned},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifyPinning(tt.ref))
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/actions/ -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Write implementation**

```go
// internal/actions/types.go
package actions

import (
	"regexp"
	"strings"
)

// ActionType identifies the runtime of a GitHub Action.
type ActionType int

const (
	ActionComposite ActionType = iota
	ActionNode                         // JavaScript (node12/16/20)
	ActionDocker
	ActionUnknown
)

func (t ActionType) String() string {
	switch t {
	case ActionComposite:
		return "composite"
	case ActionNode:
		return "node"
	case ActionDocker:
		return "docker"
	default:
		return "unknown"
	}
}

// ActionRef is a parsed reference to a GitHub Action from a workflow uses: field.
type ActionRef struct {
	Owner       string // e.g., "actions"
	Repo        string // e.g., "checkout"
	Ref         string // e.g., "v4", "abc123", "main"
	Path        string // e.g., "sub/path" for actions in subdirectories
	DockerImage string // non-empty if uses: docker://...
	LocalPath   string // non-empty if uses: ./local-action
}

// ParseActionRef parses a workflow uses: value into an ActionRef.
func ParseActionRef(uses string) ActionRef {
	uses = strings.TrimSpace(uses)

	// Docker reference: docker://image:tag
	if strings.HasPrefix(uses, "docker://") {
		return ActionRef{DockerImage: strings.TrimPrefix(uses, "docker://")}
	}

	// Local action: ./path
	if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "../") {
		return ActionRef{LocalPath: uses}
	}

	// GitHub action: owner/repo(/path)@ref
	ref := ActionRef{}
	if atIdx := strings.LastIndex(uses, "@"); atIdx >= 0 {
		ref.Ref = uses[atIdx+1:]
		uses = uses[:atIdx]
	}

	parts := strings.SplitN(uses, "/", 3)
	if len(parts) >= 2 {
		ref.Owner = parts[0]
		ref.Repo = parts[1]
	}
	if len(parts) >= 3 {
		ref.Path = parts[2]
	}

	return ref
}

// IsFirstParty returns true if the action is from GitHub's official orgs.
func (r ActionRef) IsFirstParty() bool {
	return r.Owner == "actions" || r.Owner == "github"
}

// IsDocker returns true if this is a docker:// reference.
func (r ActionRef) IsDocker() bool { return r.DockerImage != "" }

// IsLocal returns true if this is a local action (./path).
func (r ActionRef) IsLocal() bool { return r.LocalPath != "" }

// IsReusableWorkflow returns true if the ref points to a .yml/.yaml file.
func (r ActionRef) IsReusableWorkflow() bool {
	return strings.HasSuffix(r.Path, ".yml") || strings.HasSuffix(r.Path, ".yaml")
}

// FullName returns owner/repo or owner/repo/path.
func (r ActionRef) FullName() string {
	if r.Path != "" {
		return r.Owner + "/" + r.Repo + "/" + r.Path
	}
	return r.Owner + "/" + r.Repo
}

// PinQuality describes how securely an action reference is pinned.
type PinQuality int

const (
	PinSHA          PinQuality = iota // 40-char hex hash
	PinExactVersion                   // vX.Y.Z
	PinMajorTag                       // vX
	PinBranch                         // main, master, etc.
	PinUnpinned                       // no ref at all
)

func (p PinQuality) String() string {
	switch p {
	case PinSHA:
		return "sha"
	case PinExactVersion:
		return "exact_version"
	case PinMajorTag:
		return "major_tag"
	case PinBranch:
		return "branch"
	case PinUnpinned:
		return "unpinned"
	default:
		return "unknown"
	}
}

var shaRegex = regexp.MustCompile(`^[0-9a-f]{40}$`)
var exactVersionRegex = regexp.MustCompile(`^v?\d+\.\d+\.\d+`)
var majorTagRegex = regexp.MustCompile(`^v?\d+$`)

// ClassifyPinning determines the pinning quality of a ref string.
func ClassifyPinning(ref string) PinQuality {
	if ref == "" {
		return PinUnpinned
	}
	if shaRegex.MatchString(ref) {
		return PinSHA
	}
	if exactVersionRegex.MatchString(ref) {
		return PinExactVersion
	}
	if majorTagRegex.MatchString(ref) {
		return PinMajorTag
	}
	return PinBranch
}

// WorkflowFile represents a parsed GitHub Actions workflow.
type WorkflowFile struct {
	Path        string      // e.g., ".github/workflows/ci.yml"
	Actions     []ActionRef // all uses: references
	RunBlocks   []RunBlock  // all run: blocks (for script detection)
	Permissions Permissions // workflow-level permissions
}

// RunBlock is a run: step from a workflow.
type RunBlock struct {
	Content string // the shell script content
	Line    int    // line number in the workflow file (for SARIF)
}

// Permissions from the workflow's permissions: block.
type Permissions struct {
	Defined  bool              // true if permissions: block exists
	Scopes   map[string]string // e.g., {"contents": "write", "id-token": "write"}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/actions/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/actions/types.go internal/actions/types_test.go
git commit --no-verify -m "feat(actions): add action types, ref parser, and pinning classifier"
```

---

### Task 2: Workflow YAML parser — `internal/actions/parser.go`

**Files:**
- Create: `internal/actions/parser.go`
- Test: `internal/actions/parser_test.go`

This is Layer 1: parse `.github/workflows/*.yml` files and extract all references.

- [ ] **Step 1: Write the failing test**

```go
// internal/actions/parser_test.go
package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorkflow(t *testing.T) {
	yaml := `
name: CI
on: push
permissions:
  contents: read
  id-token: write
jobs:
  build:
    runs-on: ubuntu-latest
    container:
      image: node:20-alpine
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4.2.0
      - uses: some-org/deploy@abc123def456abc123def456abc123def456abcdef
      - uses: docker://alpine:3.19
      - uses: ./local-action
      - run: |
          curl -sSL https://install.example.com/setup.sh | bash
      - run: npm test
  reusable:
    uses: org/shared/.github/workflows/lint.yml@main
`
	wf, err := ParseWorkflow([]byte(yaml), ".github/workflows/ci.yml")
	require.NoError(t, err)

	assert.Equal(t, ".github/workflows/ci.yml", wf.Path)

	// Should find 6 action refs (5 steps + 1 reusable workflow)
	assert.Len(t, wf.Actions, 6)

	// Check pinning types
	assert.Equal(t, "actions", wf.Actions[0].Owner)
	assert.Equal(t, "v4", wf.Actions[0].Ref)

	// Docker ref
	assert.Equal(t, "alpine:3.19", wf.Actions[3].DockerImage)

	// Local ref
	assert.Equal(t, "./local-action", wf.Actions[4].LocalPath)

	// Reusable workflow
	assert.True(t, wf.Actions[5].IsReusableWorkflow())

	// Container image should be detected
	// (stored as a docker ref in Actions list)
	hasContainerImage := false
	for _, a := range wf.Actions {
		if a.DockerImage == "node:20-alpine" {
			hasContainerImage = true
		}
	}
	assert.True(t, hasContainerImage)

	// Permissions
	assert.True(t, wf.Permissions.Defined)
	assert.Equal(t, "read", wf.Permissions.Scopes["contents"])
	assert.Equal(t, "write", wf.Permissions.Scopes["id-token"])

	// Run blocks
	assert.GreaterOrEqual(t, len(wf.RunBlocks), 2)
}

func TestParseWorkflowNoPermissions(t *testing.T) {
	yaml := `
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`
	wf, err := ParseWorkflow([]byte(yaml), "ci.yml")
	require.NoError(t, err)
	assert.False(t, wf.Permissions.Defined)
}

func TestParseWorkflowDir(t *testing.T) {
	// Test ParseWorkflowDir with a temp dir
	// (creates .github/workflows/ with fixture files)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/actions/ -run TestParseWorkflow -v`
Expected: FAIL

- [ ] **Step 3: Write implementation**

Parse the YAML using `go.yaml.in/yaml/v3`. Extract `jobs.*.steps[].uses`, `jobs.*.uses`, `jobs.*.container.image`, and all `run:` blocks. Handle both string and map permission formats.

Also implement `ParseWorkflowDir(dir string) ([]WorkflowFile, error)` that globs `.github/workflows/*.yml` and `.github/workflows/*.yaml` and parses each.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/actions/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/actions/parser.go internal/actions/parser_test.go
git commit --no-verify -m "feat(actions): add workflow YAML parser (Layer 1)"
```

---

### Task 3: Script download detection — `internal/actions/scriptdetect.go`

**Files:**
- Create: `internal/actions/scriptdetect.go`
- Test: `internal/actions/scriptdetect_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/actions/scriptdetect_test.go
package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectScriptDownloads(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		want    int // number of downloads detected
	}{
		{"curl pipe bash", "curl -sSL https://example.com/install.sh | bash", 1},
		{"wget pipe sh", "wget -O- https://example.com/setup.sh | sh", 1},
		{"curl pipe python", "curl https://example.com/script.py | python3", 1},
		{"download then execute", "curl -o install.sh https://example.com/install.sh\nsh install.sh", 1},
		{"safe curl", "curl -o output.json https://api.example.com/data", 0},
		{"no downloads", "echo hello\nnpm test", 0},
		{"multiple", "curl https://a.com/x | bash\nwget https://b.com/y | sh", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectScriptDownloads(tt.script)
			assert.Len(t, got, tt.want)
		})
	}
}

func TestScriptDownloadURL(t *testing.T) {
	downloads := DetectScriptDownloads("curl -sSL https://install.example.com/setup.sh | bash")
	assert.Len(t, downloads, 1)
	assert.Equal(t, "https://install.example.com/setup.sh", downloads[0].URL)
}
```

- [ ] **Step 2: Run test, write implementation**

```go
// internal/actions/scriptdetect.go
package actions

import (
	"regexp"
	"strings"
)

// ScriptDownload represents a detected download-and-execute pattern.
type ScriptDownload struct {
	URL     string // the URL being downloaded
	Pattern string // e.g., "curl|bash", "wget|sh"
	Line    string // the matched line
}

// DetectScriptDownloads scans a run: block for dangerous download-and-execute patterns.
func DetectScriptDownloads(script string) []ScriptDownload {
	var results []ScriptDownload
	// Match patterns like: curl ... | bash, wget ... | sh, curl ... | python
	// Also: curl -o file ... && sh file
	// Extract URLs from the commands
	// ...implementation...
	return results
}
```

Use regex patterns to match:
- `(curl|wget)\s+.*\|\s*(bash|sh|zsh|python|python3)`
- `(curl|wget)\s+.*-o\s+(\S+).*\n.*\b(sh|bash|python)\s+\2`

Extract URLs with a URL-matching regex.

- [ ] **Step 3: Run tests, commit**

```bash
git commit --no-verify -m "feat(actions): add script download detection for run blocks"
```

---

### Task 4: Dockerfile parser — `internal/actions/dockerfile.go`

**Files:**
- Create: `internal/actions/dockerfile.go`
- Test: `internal/actions/dockerfile_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/actions/dockerfile_test.go
package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDockerfile(t *testing.T) {
	content := `FROM python:3.12-slim AS builder
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt
RUN npm install
FROM node:20-alpine
COPY --from=builder /app /app
`
	result, err := ParseDockerfile([]byte(content))
	require.NoError(t, err)

	// Two FROM images
	assert.Len(t, result.BaseImages, 2)
	assert.Equal(t, "python", result.BaseImages[0].Image)
	assert.Equal(t, "3.12-slim", result.BaseImages[0].Tag)
	assert.Equal(t, "node", result.BaseImages[1].Image)
	assert.Equal(t, "20-alpine", result.BaseImages[1].Tag)

	// Detects pip install and npm install
	assert.True(t, result.HasPipInstall)
	assert.True(t, result.HasNpmInstall)
}

func TestParseDockerfileDigest(t *testing.T) {
	content := `FROM alpine@sha256:abc123def456`
	result, err := ParseDockerfile([]byte(content))
	require.NoError(t, err)
	assert.Equal(t, "alpine", result.BaseImages[0].Image)
	assert.Equal(t, "sha256:abc123def456", result.BaseImages[0].Digest)
}
```

- [ ] **Step 2: Write implementation**

Parse FROM lines with regex. Detect `pip install`, `npm install`, `COPY requirements.txt`, `COPY package.json` patterns in RUN/COPY instructions.

- [ ] **Step 3: Run tests, commit**

```bash
git commit --no-verify -m "feat(actions): add Dockerfile FROM parser"
```

---

### Task 5: Action ref resolver — `internal/actions/resolver.go`

**Files:**
- Create: `internal/actions/resolver.go`
- Test: `internal/actions/resolver_test.go`

This is Layer 2: resolve tag/ref → SHA via GitHub API, then fetch action.yml (Layer 3).

- [ ] **Step 1: Write the failing test**

Use httptest to mock GitHub API responses. Test:
- Tag resolution: `GET /repos/owner/repo/git/ref/tags/v4` → SHA
- action.yml fetch: `GET /repos/owner/repo/contents/action.yml?ref=SHA` → content
- Composite action detection
- JS action detection
- Docker action detection
- Caching behavior (second call for same ref should not hit API)

- [ ] **Step 2: Write implementation**

```go
// internal/actions/resolver.go
package actions

// Resolver resolves action references to their concrete SHA and action type.
type Resolver struct {
	githubToken string
	httpClient  *http.Client
	cache       *cache.DiskCache
	baseURL     string // for testing
}

// ResolvedAction is the result of resolving an ActionRef.
type ResolvedAction struct {
	Ref        ActionRef
	SHA        string      // resolved commit SHA
	Type       ActionType  // composite, node, docker
	Pinning    PinQuality
	ActionYAML *ActionYAML // parsed action.yml contents (nil if fetch failed)
}

// ActionYAML represents the parsed action.yml/action.yaml file.
type ActionYAML struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Runs        ActionYAMLRuns `yaml:"runs"`
}

type ActionYAMLRuns struct {
	Using string   `yaml:"using"` // "composite", "node20", "docker"
	Main  string   `yaml:"main"`  // JS entry point
	Image string   `yaml:"image"` // Docker image or Dockerfile path
	Steps []struct {
		Uses string `yaml:"uses"`
		Run  string `yaml:"run"`
	} `yaml:"steps"` // composite steps
}

// Resolve resolves an ActionRef to a ResolvedAction.
func (r *Resolver) Resolve(ctx context.Context, ref ActionRef) (*ResolvedAction, error) {
	// 1. Resolve tag/ref to SHA via GitHub API (with cache)
	// 2. Fetch action.yml at that SHA (with cache)
	// 3. Parse action.yml to determine type
	// 4. Return ResolvedAction
}
```

- [ ] **Step 3: Run tests, commit**

```bash
git commit --no-verify -m "feat(actions): add action ref resolver (Layers 2+3)"
```

---

### Task 6: Composite + bundled code analysis — `internal/actions/composite.go` + `internal/actions/bundled.go`

**Files:**
- Create: `internal/actions/composite.go`
- Create: `internal/actions/bundled.go`
- Test: `internal/actions/composite_test.go`
- Test: `internal/actions/bundled_test.go`

Layer 3 (composite transitive deps) and Layer 4 (bundled code).

**composite.go:** Parse composite action steps, extract `uses:` for transitive action deps. Recurse by calling resolver again for each.

**bundled.go:** For JS actions, fetch `package.json`/`package-lock.json` from the action repo at resolved SHA. Feed into `manifest.NewJavaScriptParser().ParseFiles()`. For Docker actions, fetch Dockerfile and parse with `ParseDockerfile()`.

- [ ] **Step 1: Write tests with mock data**
- [ ] **Step 2: Write implementations**
- [ ] **Step 3: Run tests, commit**

```bash
git commit --no-verify -m "feat(actions): add composite action and bundled code analysis (Layers 3+4)"
```

---

### Task 7: Action scoring — `internal/actions/scorer.go`

**Files:**
- Create: `internal/actions/scorer.go`
- Test: `internal/actions/scorer_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestScoreAction(t *testing.T) {
	tests := []struct {
		name    string
		action  ScoringInput
		wantMin int
		wantMax int
	}{
		{"sha-pinned first-party", ScoringInput{Pinning: PinSHA, FirstParty: true, RepoStars: 10000}, 85, 100},
		{"major-tag third-party", ScoringInput{Pinning: PinMajorTag, FirstParty: false, RepoStars: 100}, 40, 70},
		{"branch-pinned unknown", ScoringInput{Pinning: PinBranch, FirstParty: false, RepoStars: 0}, 0, 40},
		{"script download", ScoringInput{IsScriptDownload: true}, 0, 0},
	}
	// ...
}
```

- [ ] **Step 2: Write implementation**

7 weighted factors: pinning quality (25%), first-party (15%), repo health (15%), maintainer count (10%), release recency (10%), bundled dep risk (15%), permissions scope (10%).

Script downloads always score 0.

Docker image scoring: 5 factors — pinning quality (30%), official status (20%), image age (20%), base image chain (15%), vulnerability count (15%).

- [ ] **Step 3: Run tests, commit**

```bash
git commit --no-verify -m "feat(actions): add action and Docker image scoring"
```

---

### Task 8: Wire actions into scan pipeline

**Files:**
- Modify: `internal/manifest/manifest.go` — add `EcosystemActions` constant, detect `.github/workflows/`
- Modify: `internal/scanner/scanner.go` — call actions parser/resolver when actions ecosystem detected
- Modify: `internal/resolve/filters.go` — add `Dockerfile` to ManifestFilenames
- Create: `internal/actions/scan.go` — orchestrator that ties all layers together and produces graph nodes

- [ ] **Step 1: Add EcosystemActions**

In `manifest.go`, add:
```go
EcosystemActions Ecosystem = "actions"
```

Update `ecosystemFiles` to detect `.github/workflows/` (directory check, not file check).

- [ ] **Step 2: Create actions scan orchestrator**

`internal/actions/scan.go` — `ScanWorkflows(dir string, resolver *Resolver, g *graph.Graph)` that:
1. Finds all workflow YAML files
2. Parses each (Layer 1)
3. Resolves each action ref (Layer 2+3)
4. Analyzes bundled code (Layer 4)
5. Handles reusable workflows (Layer 5)
6. Adds all nodes and edges to the graph
7. Scores each node

- [ ] **Step 3: Wire into scanner.ScanDir**

When `EcosystemActions` is detected (and not filtered by `--only`), call `actions.ScanWorkflows` and merge nodes into the graph.

- [ ] **Step 4: Run all tests**

Run: `go test ./... -count=1`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git commit --no-verify -m "feat(actions): wire actions scanner into scan pipeline"
```

---

### Task 9: Pinning summary output

**Files:**
- Modify: `internal/report/text.go` — add pinning summary section
- Modify: `internal/report/json.go` — add `pinning_summary` to JSON output
- Create: `internal/actions/summary.go` — compute pinning statistics from graph
- Test: `internal/actions/summary_test.go`

- [ ] **Step 1: Write summary computation**

```go
// internal/actions/summary.go
type PinningSummary struct {
	SHAPinned     int
	ExactVersion  int
	MajorTag      int
	Branch        int
	Unpinned      int
	FirstParty    int
	ThirdParty    int
	ScriptDownloads int
	Total         int
}

func ComputePinningSummary(g *graph.Graph) PinningSummary {
	// Iterate action nodes, count by pinning quality
}
```

- [ ] **Step 2: Add to text and JSON output**
- [ ] **Step 3: Run tests, commit**

```bash
git commit --no-verify -m "feat(report): add pinning summary for GitHub Actions"
```

---

### Task 10: `--org` flag for scan

**Files:**
- Modify: `cmd/depscope/scan.go` — add `--org` flag
- Modify: `internal/scanner/scanner.go` — add `ScanOrg(ctx, org, opts)` function
- Test: `internal/scanner/org_test.go`

- [ ] **Step 1: Add ScanOrg**

Uses GitHub API `GET /orgs/{org}/repos` (paginated) to list repos, then calls `ScanURL` for each.

- [ ] **Step 2: Add --org flag to CLI**
- [ ] **Step 3: Run tests, commit**

```bash
git commit --no-verify -m "feat(scan): add --org flag for org-wide scanning"
```

---

### Task 11: Integration test + smoke test

**Files:**
- Create: `internal/actions/integration_test.go`
- Build and manually test

- [ ] **Step 1: Write integration test**

Create a temp dir with `.github/workflows/ci.yml` fixture containing various action refs. Run the full pipeline. Verify graph contains action nodes, workflow nodes, and edges.

- [ ] **Step 2: Build and smoke test**

```bash
go build -o depscope ./cmd/depscope/
./depscope scan . --only actions --no-cve
./depscope scan . --output json --no-cve | python3 -c "import sys,json; d=json.load(sys.stdin); print([n['type'] for n in d.get('graph',{}).get('nodes',[])])"
```

- [ ] **Step 3: Full regression**

Run: `go test ./... -race -count=1`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git commit --no-verify -m "test(actions): add integration tests and verify full pipeline"
```
