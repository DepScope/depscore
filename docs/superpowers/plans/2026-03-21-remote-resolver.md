# Remote Resolver Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add remote URL scanning to depscope — fetch manifest files from GitHub/GitLab via their APIs (no clone), fall back to shallow clone for other hosts, and integrate into the existing scan pipeline.

**Architecture:** A new `internal/resolve/` package with a `Resolver` interface and three implementations: `GitHubResolver`, `GitLabResolver`, and `GitCloneResolver`. URL dispatch selects the right resolver. The `manifest.Parser` interface gains a `ParseFiles(map[string][]byte)` method so parsers can accept in-memory content from resolvers. The scan command detects URL vs local path and routes accordingly.

**Tech Stack:** Go 1.22+, `net/http` (GitHub + GitLab APIs), `os/exec` (git clone fallback), `net/http/httptest` (test servers), `github.com/stretchr/testify` (assertions)

**Spec:** `docs/superpowers/specs/2026-03-21-remote-resolver-design.md`

---

## File Map

```
internal/resolve/
├── resolver.go          # ManifestFile struct, Resolver interface
├── filters.go           # IgnoredDirs, ManifestFilenames, IsIgnoredDir(), MatchesManifest()
├── filters_test.go
├── detect.go            # IsRemoteURL(), DetectResolver() → Resolver dispatch
├── detect_test.go
├── github.go            # GitHubResolver — Trees API + Contents API
├── github_test.go
├── gitlab.go            # GitLabResolver — Repository Tree API + Files API
├── gitlab_test.go
├── gitclone.go          # GitCloneResolver — shallow clone fallback
├── gitclone_test.go
└── testdata/
    ├── github/
    │   ├── tree.json            # GET /repos/{owner}/{repo}/git/trees/{ref}?recursive=1
    │   └── contents_gomod.json  # GET /repos/{owner}/{repo}/contents/go.mod
    └── gitlab/
        ├── tree_page1.json      # GET /api/v4/projects/{id}/repository/tree?recursive=true&page=1
        └── gomod_raw.txt        # GET /api/v4/projects/{id}/repository/files/go.mod/raw
```

Changes to existing files:
```
internal/manifest/manifest.go   # Add ParseFiles() to Parser interface, DetectEcosystemFromFiles()
internal/manifest/python.go     # Implement ParseFiles()
internal/manifest/gomod.go      # Implement ParseFiles()
internal/manifest/rust.go       # Implement ParseFiles()
internal/manifest/javascript.go # Implement ParseFiles()
cmd/depscope/scan.go            # URL detection, resolver dispatch, signal handling
```

---

## Task 1: Resolver Types and File Filters

**Files:**
- Create: `internal/resolve/resolver.go`
- Create: `internal/resolve/filters.go`
- Create: `internal/resolve/filters_test.go`

- [ ] **Step 1: Write the filter tests**

`internal/resolve/filters_test.go`:
```go
package resolve_test

import (
	"testing"

	"github.com/depscope/depscope/internal/resolve"
	"github.com/stretchr/testify/assert"
)

func TestIsIgnoredDir(t *testing.T) {
	tests := []struct {
		path    string
		ignored bool
	}{
		{"node_modules/foo/package.json", true},
		{"vendor/github.com/foo/go.mod", true},
		{"target/debug/Cargo.toml", true},
		{".git/config", true},
		{"__pycache__/foo.pyc", true},
		{"dist/bundle.js", true},
		{"build/output.jar", true},
		{"src/go.mod", false},
		{"go.mod", false},
		{"services/api/node_modules/foo/package.json", true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.ignored, resolve.IsIgnoredDir(tt.path), tt.path)
	}
}

func TestMatchesManifest(t *testing.T) {
	tests := []struct {
		path    string
		matches bool
	}{
		{"go.mod", true},
		{"go.sum", true},
		{"requirements.txt", true},
		{"pyproject.toml", true},
		{"poetry.lock", true},
		{"uv.lock", true},
		{"Cargo.toml", true},
		{"Cargo.lock", true},
		{"package.json", true},
		{"package-lock.json", true},
		{"pnpm-lock.yaml", true},
		{"bun.lock", true},
		{"services/api/go.mod", true},
		{"README.md", false},
		{"main.go", false},
		{"node_modules/foo/package.json", false}, // ignored dir takes precedence
	}
	for _, tt := range tests {
		assert.Equal(t, tt.matches, resolve.MatchesManifest(tt.path), tt.path)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/resolve/... -v
```
Expected: compile error — package not found.

- [ ] **Step 3: Implement resolver types and filters**

`internal/resolve/resolver.go`:
```go
package resolve

import "context"

// ManifestFile represents a manifest or lockfile fetched from a remote source.
type ManifestFile struct {
	Path    string // relative path within the repo, e.g. "services/api/go.mod"
	Content []byte
}

// Resolver fetches manifest files from a remote repository.
type Resolver interface {
	Resolve(ctx context.Context, url string) ([]ManifestFile, func(), error)
}
```

`internal/resolve/filters.go`:
```go
package resolve

import (
	"path/filepath"
	"strings"
)

var IgnoredDirs = []string{
	"node_modules",
	"vendor",
	"target",
	".git",
	"__pycache__",
	"dist",
	"build",
}

var ManifestFilenames = []string{
	"go.mod", "go.sum",
	"requirements.txt", "pyproject.toml", "poetry.lock", "uv.lock",
	"Cargo.toml", "Cargo.lock",
	"package.json", "package-lock.json", "pnpm-lock.yaml", "bun.lock",
}

// IsIgnoredDir returns true if the path is under any ignored directory.
func IsIgnoredDir(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts {
		for _, ignored := range IgnoredDirs {
			if part == ignored {
				return true
			}
		}
	}
	return false
}

// MatchesManifest returns true if the path's base name is a known manifest
// file and the path is not under an ignored directory.
func MatchesManifest(path string) bool {
	if IsIgnoredDir(path) {
		return false
	}
	base := filepath.Base(path)
	for _, name := range ManifestFilenames {
		if base == name {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/resolve/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/resolve/
git commit -m "feat: resolve package with ManifestFile type and file filters"
```

---

## Task 2: URL Detection and Dispatch

**Files:**
- Create: `internal/resolve/detect.go`
- Create: `internal/resolve/detect_test.go`

- [ ] **Step 1: Write tests**

`internal/resolve/detect_test.go`:
```go
package resolve_test

import (
	"testing"

	"github.com/depscope/depscope/internal/resolve"
	"github.com/stretchr/testify/assert"
)

func TestIsRemoteURL(t *testing.T) {
	tests := []struct {
		input  string
		remote bool
	}{
		{"https://github.com/psf/requests", true},
		{"http://github.com/psf/requests", true},
		{"ssh://git@github.com/psf/requests.git", true},
		{"git@github.com:psf/requests.git", true},
		{".", false},
		{"/home/user/project", false},
		{"./relative/path", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.remote, resolve.IsRemoteURL(tt.input), tt.input)
	}
}

func TestDetectResolver(t *testing.T) {
	tests := []struct {
		url          string
		expectedType string
	}{
		{"https://github.com/psf/requests", "github"},
		{"https://github.com/psf/requests/tree/v2.31.0", "github"},
		{"https://gitlab.com/org/project", "gitlab"},
		{"https://gitlab.com/group/subgroup/project/-/tree/main", "gitlab"},
		{"https://bitbucket.org/org/repo", "gitclone"},
		{"git@custom.host:org/repo.git", "gitclone"},
		{"ssh://git@example.com/org/repo.git", "gitclone"},
	}
	for _, tt := range tests {
		r := resolve.DetectResolver(tt.url, resolve.DetectOptions{})
		assert.Equal(t, tt.expectedType, r.Type(), tt.url)
	}
}

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		url   string
		owner string
		repo  string
		ref   string
	}{
		{"https://github.com/psf/requests", "psf", "requests", ""},
		{"https://github.com/psf/requests/tree/v2.31.0", "psf", "requests", "v2.31.0"},
		{"https://github.com/psf/requests/tree/feature/branch", "psf", "requests", "feature/branch"},
		{"https://github.com/psf/requests.git", "psf", "requests", ""},
	}
	for _, tt := range tests {
		owner, repo, ref := resolve.ParseGitHubURL(tt.url)
		assert.Equal(t, tt.owner, owner, tt.url)
		assert.Equal(t, tt.repo, repo, tt.url)
		assert.Equal(t, tt.ref, ref, tt.url)
	}
}

func TestParseGitLabURL(t *testing.T) {
	tests := []struct {
		url     string
		project string
		ref     string
	}{
		{"https://gitlab.com/org/project", "org/project", ""},
		{"https://gitlab.com/group/subgroup/project", "group/subgroup/project", ""},
		{"https://gitlab.com/org/project/-/tree/main", "org/project", "main"},
		{"https://gitlab.com/group/sub/project/-/tree/v1.0", "group/sub/project", "v1.0"},
	}
	for _, tt := range tests {
		project, ref := resolve.ParseGitLabURL(tt.url)
		assert.Equal(t, tt.project, project, tt.url)
		assert.Equal(t, tt.ref, ref, tt.url)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/resolve/... -run "TestIsRemote|TestDetect|TestParseGit" -v
```

- [ ] **Step 3: Implement**

`internal/resolve/detect.go`:
```go
package resolve

import (
	"net/url"
	"strings"
)

// DetectOptions configures resolver construction (e.g. tokens).
type DetectOptions struct {
	GitHubToken string
	GitLabToken string
}

// IsRemoteURL returns true if the argument looks like a remote URL rather than a local path.
func IsRemoteURL(arg string) bool {
	for _, prefix := range []string{"http://", "https://", "ssh://", "git@"} {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

// TypedResolver extends Resolver with a Type() method for dispatch testing.
type TypedResolver interface {
	Resolver
	Type() string
}

// DetectResolver selects the appropriate resolver based on the URL.
func DetectResolver(rawURL string, opts DetectOptions) TypedResolver {
	host := extractHost(rawURL)
	switch {
	case strings.Contains(host, "github.com"):
		return NewGitHubResolver(opts.GitHubToken)
	case strings.Contains(host, "gitlab.com"):
		return NewGitLabResolver(opts.GitLabToken)
	default:
		return NewGitCloneResolver()
	}
}

// ParseGitHubURL extracts owner, repo, and optional ref from a GitHub URL.
func ParseGitHubURL(rawURL string) (owner, repo, ref string) {
	rawURL = strings.TrimSuffix(rawURL, ".git")
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return
	}
	owner, repo = parts[0], parts[1]
	// /tree/{ref...} — ref can contain slashes (e.g. feature/branch)
	if len(parts) >= 4 && parts[2] == "tree" {
		ref = strings.Join(parts[3:], "/")
	}
	return
}

// ParseGitLabURL extracts project path and optional ref from a GitLab URL.
func ParseGitLabURL(rawURL string) (project, ref string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	path := strings.Trim(u.Path, "/")
	// GitLab uses /-/tree/{ref} for branch browsing
	if idx := strings.Index(path, "/-/tree/"); idx >= 0 {
		project = path[:idx]
		ref = path[idx+len("/-/tree/"):]
		return
	}
	project = path
	return
}

func extractHost(rawURL string) string {
	// Handle git@ style URLs: git@github.com:owner/repo.git
	if strings.HasPrefix(rawURL, "git@") {
		parts := strings.SplitN(rawURL, ":", 2)
		if len(parts) == 2 {
			return strings.TrimPrefix(parts[0], "git@")
		}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/resolve/... -run "TestIsRemote|TestDetect|TestParseGit" -v
```

Note: `TestDetectResolver` will fail because `NewGitHubResolver`, `NewGitLabResolver`, and `NewGitCloneResolver` don't exist yet. Create stub constructors that return minimal structs implementing `TypedResolver`:

```go
// Temporary stubs in detect.go — removed when real implementations land in Tasks 3-5.

type githubResolver struct{ token string }
func NewGitHubResolver(token string) TypedResolver { return &githubResolver{token: token} }
func (r *githubResolver) Type() string { return "github" }
func (r *githubResolver) Resolve(ctx context.Context, url string) ([]ManifestFile, func(), error) {
	return nil, func() {}, nil
}

type gitlabResolver struct{ token string }
func NewGitLabResolver(token string) TypedResolver { return &gitlabResolver{token: token} }
func (r *gitlabResolver) Type() string { return "gitlab" }
func (r *gitlabResolver) Resolve(ctx context.Context, url string) ([]ManifestFile, func(), error) {
	return nil, func() {}, nil
}

type gitCloneResolver struct{}
func NewGitCloneResolver() TypedResolver { return &gitCloneResolver{} }
func (r *gitCloneResolver) Type() string { return "gitclone" }
func (r *gitCloneResolver) Resolve(ctx context.Context, url string) ([]ManifestFile, func(), error) {
	return nil, func() {}, nil
}
```

- [ ] **Step 5: Run full suite — expect PASS**

```bash
go test ./internal/resolve/... -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/resolve/
git commit -m "feat: URL detection and resolver dispatch with GitHub/GitLab URL parsers"
```

---

## Task 3: GitHub Resolver

**Files:**
- Create: `internal/resolve/github.go` (replace stub)
- Create: `internal/resolve/github_test.go`
- Create: `internal/resolve/testdata/github/tree.json`
- Create: `internal/resolve/testdata/github/contents_gomod.json`
- Create: `internal/resolve/testdata/github/contents_gosum.json`

- [ ] **Step 1: Capture golden files**

```bash
mkdir -p internal/resolve/testdata/github

# Tree response for a real repo
curl -s -H "Authorization: Bearer $GITHUB_TOKEN" \
  "https://api.github.com/repos/spf13/cobra/git/trees/main?recursive=1" \
  > internal/resolve/testdata/github/tree.json

# Contents response for go.mod
curl -s -H "Authorization: Bearer $GITHUB_TOKEN" \
  "https://api.github.com/repos/spf13/cobra/contents/go.mod?ref=main" \
  > internal/resolve/testdata/github/contents_gomod.json

# Contents response for go.sum
curl -s -H "Authorization: Bearer $GITHUB_TOKEN" \
  "https://api.github.com/repos/spf13/cobra/contents/go.sum?ref=main" \
  > internal/resolve/testdata/github/contents_gosum.json
```

Verify: `jq '.tree | length' internal/resolve/testdata/github/tree.json` (should be > 0).
Verify: `jq '.name' internal/resolve/testdata/github/contents_gomod.json` (should be "go.mod").

- [ ] **Step 2: Write test**

`internal/resolve/github_test.go`:
```go
package resolve_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/depscope/depscope/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubResolver(t *testing.T) {
	treeData, err := os.ReadFile("testdata/github/tree.json")
	require.NoError(t, err)

	gomodData, err := os.ReadFile("testdata/github/contents_gomod.json")
	require.NoError(t, err)

	gosumData, err := os.ReadFile("testdata/github/contents_gosum.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/git/trees/"):
			w.Write(treeData)
		case strings.HasSuffix(r.URL.Path, "/go.mod"):
			w.Write(gomodData)
		case strings.HasSuffix(r.URL.Path, "/go.sum"):
			w.Write(gosumData)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	resolver := resolve.NewGitHubResolver("fake-token", resolve.WithBaseURL(srv.URL))
	files, cleanup, err := resolver.Resolve(context.Background(), "https://github.com/spf13/cobra")
	defer cleanup()
	require.NoError(t, err)

	// Should find at least go.mod and go.sum
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Path
	}
	assert.Contains(t, names, "go.mod")
	assert.Contains(t, names, "go.sum")

	// Content should be non-empty
	for _, f := range files {
		if f.Path == "go.mod" {
			assert.Contains(t, string(f.Content), "module")
		}
	}
}

func TestGitHubResolverTruncatedTree(t *testing.T) {
	// Simulate a truncated tree response
	treeResp := map[string]interface{}{
		"sha":       "abc123",
		"truncated": true,
		"tree": []map[string]string{
			{"path": "go.mod", "type": "blob"},
		},
	}
	data, _ := json.Marshal(treeResp)

	gomodContent := map[string]interface{}{
		"name":     "go.mod",
		"encoding": "base64",
		"content":  "bW9kdWxlIGV4YW1wbGUuY29t", // "module example.com" in base64
	}
	gomodData, _ := json.Marshal(gomodContent)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/git/trees/") {
			w.Write(data)
		} else {
			w.Write(gomodData)
		}
	}))
	defer srv.Close()

	resolver := resolve.NewGitHubResolver("fake-token", resolve.WithBaseURL(srv.URL))
	files, cleanup, err := resolver.Resolve(context.Background(), "https://github.com/owner/repo")
	defer cleanup()
	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Equal(t, "go.mod", files[0].Path)
}
```

- [ ] **Step 3: Run — expect FAIL**

```bash
go test ./internal/resolve/... -run TestGitHub -v
```

- [ ] **Step 4: Implement GitHubResolver**

`internal/resolve/github.go` — replace the stub:
```go
package resolve

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// Option is a functional option for resolvers (e.g. override base URL in tests).
type Option func(*resolverOptions)

type resolverOptions struct {
	baseURL string
}

func WithBaseURL(url string) Option {
	return func(o *resolverOptions) { o.baseURL = url }
}

type GitHubResolver struct {
	token   string
	baseURL string
	client  *http.Client
}

func NewGitHubResolver(token string, opts ...Option) *GitHubResolver {
	o := &resolverOptions{baseURL: "https://api.github.com"}
	for _, opt := range opts {
		opt(o)
	}
	return &GitHubResolver{token: token, baseURL: o.baseURL, client: &http.Client{}}
}

func (r *GitHubResolver) Type() string { return "github" }

func (r *GitHubResolver) Resolve(ctx context.Context, rawURL string) ([]ManifestFile, func(), error) {
	owner, repo, ref := ParseGitHubURL(rawURL)
	if owner == "" || repo == "" {
		return nil, func() {}, fmt.Errorf("invalid GitHub URL: %s", rawURL)
	}

	if ref == "" {
		defaultRef, err := r.fetchDefaultBranch(ctx, owner, repo)
		if err != nil {
			return nil, func() {}, fmt.Errorf("fetch default branch: %w", err)
		}
		ref = defaultRef
	}

	// Step 1: Get the full tree
	treePaths, err := r.fetchTree(ctx, owner, repo, ref)
	if err != nil {
		return nil, func() {}, fmt.Errorf("fetch tree: %w", err)
	}

	// Step 2: Filter for manifests
	var manifestPaths []string
	for _, p := range treePaths {
		if MatchesManifest(p) {
			manifestPaths = append(manifestPaths, p)
		}
	}

	// Step 3: Fetch each manifest file's content
	var files []ManifestFile
	for _, path := range manifestPaths {
		content, err := r.fetchFileContent(ctx, owner, repo, ref, path)
		if err != nil {
			log.Printf("warning: could not fetch %s: %v", path, err)
			continue
		}
		files = append(files, ManifestFile{Path: path, Content: content})
	}

	return files, func() {}, nil
}

func (r *GitHubResolver) fetchDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", r.baseURL, owner, repo)
	var result struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := r.getJSON(ctx, url, &result); err != nil {
		return "", err
	}
	return result.DefaultBranch, nil
}

func (r *GitHubResolver) fetchTree(ctx context.Context, owner, repo, ref string) ([]string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", r.baseURL, owner, repo, ref)
	var result struct {
		Tree      []struct{ Path string `json:"path"`; Type string `json:"type"` } `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := r.getJSON(ctx, url, &result); err != nil {
		return nil, err
	}
	if result.Truncated {
		log.Printf("warning: GitHub tree for %s/%s is truncated (>100k entries); some manifests may be missed", owner, repo)
	}
	var paths []string
	for _, entry := range result.Tree {
		if entry.Type == "blob" {
			paths = append(paths, entry.Path)
		}
	}
	return paths, nil
}

func (r *GitHubResolver) fetchFileContent(ctx context.Context, owner, repo, ref, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", r.baseURL, owner, repo, path, ref)
	var result struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := r.getJSON(ctx, url, &result); err != nil {
		return nil, err
	}
	if result.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected encoding %q for %s", result.Encoding, path)
	}
	// GitHub API returns base64 with embedded newlines; strip them before decoding.
	clean := strings.ReplaceAll(result.Content, "\n", "")
	return base64.StdEncoding.DecodeString(clean)
}

func (r *GitHubResolver) getJSON(ctx context.Context, url string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API %s: %d %s", url, resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(target)
}
```

Remove the `githubResolver` stub from `detect.go`. `DetectResolver` keeps its `TypedResolver` return type — `*GitHubResolver` satisfies it via its `Type()` method, so no changes to `DetectResolver`'s body are needed.

- [ ] **Step 5: Run — expect PASS**

```bash
go test ./internal/resolve/... -run TestGitHub -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/resolve/
git commit -m "feat: GitHub resolver using Trees + Contents API with golden-file tests"
```

---

## Task 4: GitLab Resolver

**Files:**
- Create: `internal/resolve/gitlab.go` (replace stub)
- Create: `internal/resolve/gitlab_test.go`
- Create: `internal/resolve/testdata/gitlab/tree_page1.json`
- Create: `internal/resolve/testdata/gitlab/gomod_raw.txt`

- [ ] **Step 1: Create golden files manually**

Since GitLab's public API requires no auth for public projects, capture from a known public project:

```bash
mkdir -p internal/resolve/testdata/gitlab

# Tree response
curl -s "https://gitlab.com/api/v4/projects/gnachman%2Fiterm2/repository/tree?recursive=true&per_page=100&page=1" \
  > internal/resolve/testdata/gitlab/tree_page1.json
```

If the above project doesn't have manifests we need, create synthetic golden files:

`internal/resolve/testdata/gitlab/tree_page1.json`:
```json
[
  {"id": "a1", "name": "go.mod", "type": "blob", "path": "go.mod", "mode": "100644"},
  {"id": "a2", "name": "go.sum", "type": "blob", "path": "go.sum", "mode": "100644"},
  {"id": "a3", "name": "main.go", "type": "blob", "path": "cmd/main.go", "mode": "100644"},
  {"id": "a4", "name": "README.md", "type": "blob", "path": "README.md", "mode": "100644"}
]
```

`internal/resolve/testdata/gitlab/gomod_raw.txt`:
```
module example.com/myproject

go 1.22

require github.com/spf13/cobra v1.8.0
```

- [ ] **Step 2: Write test**

`internal/resolve/gitlab_test.go`:
```go
package resolve_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/depscope/depscope/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitLabResolver(t *testing.T) {
	treeData, err := os.ReadFile("testdata/gitlab/tree_page1.json")
	require.NoError(t, err)

	gomodData, err := os.ReadFile("testdata/gitlab/gomod_raw.txt")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/repository/tree"):
			w.Header().Set("Content-Type", "application/json")
			w.Write(treeData)
		case strings.Contains(r.URL.Path, "/repository/files/") && strings.HasSuffix(r.URL.Path, "/raw"):
			w.Header().Set("Content-Type", "text/plain")
			w.Write(gomodData)
		default:
			// Default branch lookup
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"default_branch": "main"}`))
		}
	}))
	defer srv.Close()

	resolver := resolve.NewGitLabResolver("fake-token", resolve.WithBaseURL(srv.URL))
	files, cleanup, err := resolver.Resolve(context.Background(), "https://gitlab.com/org/project")
	defer cleanup()
	require.NoError(t, err)

	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Path
	}
	assert.Contains(t, names, "go.mod")
	assert.Contains(t, names, "go.sum")
	// Should NOT contain non-manifest files
	assert.NotContains(t, names, "cmd/main.go")
	assert.NotContains(t, names, "README.md")

	// Verify content
	for _, f := range files {
		if f.Path == "go.mod" {
			assert.Contains(t, string(f.Content), "module")
		}
	}
}
```

- [ ] **Step 3: Run — expect FAIL**

```bash
go test ./internal/resolve/... -run TestGitLab -v
```

- [ ] **Step 4: Implement GitLabResolver**

`internal/resolve/gitlab.go` — replace the stub:
```go
package resolve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

type GitLabResolver struct {
	token   string
	baseURL string
	client  *http.Client
}

func NewGitLabResolver(token string, opts ...Option) *GitLabResolver {
	o := &resolverOptions{baseURL: "https://gitlab.com"}
	for _, opt := range opts {
		opt(o)
	}
	return &GitLabResolver{token: token, baseURL: o.baseURL, client: &http.Client{}}
}

func (r *GitLabResolver) Type() string { return "gitlab" }

func (r *GitLabResolver) Resolve(ctx context.Context, rawURL string) ([]ManifestFile, func(), error) {
	project, ref := ParseGitLabURL(rawURL)
	if project == "" {
		return nil, func() {}, fmt.Errorf("invalid GitLab URL: %s", rawURL)
	}

	encodedProject := url.PathEscape(project)

	if ref == "" {
		defaultRef, err := r.fetchDefaultBranch(ctx, encodedProject)
		if err != nil {
			return nil, func() {}, fmt.Errorf("fetch default branch: %w", err)
		}
		ref = defaultRef
	}

	// Step 1: Paginated tree listing (capped at 10 pages)
	var allPaths []string
	for page := 1; page <= 10; page++ {
		paths, hasMore, err := r.fetchTreePage(ctx, encodedProject, ref, page)
		if err != nil {
			return nil, func() {}, fmt.Errorf("fetch tree page %d: %w", page, err)
		}
		allPaths = append(allPaths, paths...)
		if !hasMore {
			break
		}
		if page == 10 {
			log.Printf("warning: GitLab tree for %s capped at 1000 entries; some manifests may be missed", project)
		}
	}

	// Step 2: Filter for manifests
	var manifestPaths []string
	for _, p := range allPaths {
		if MatchesManifest(p) {
			manifestPaths = append(manifestPaths, p)
		}
	}

	// Step 3: Fetch file contents
	var files []ManifestFile
	for _, path := range manifestPaths {
		content, err := r.fetchFileRaw(ctx, encodedProject, ref, path)
		if err != nil {
			log.Printf("warning: could not fetch %s: %v", path, err)
			continue
		}
		files = append(files, ManifestFile{Path: path, Content: content})
	}

	return files, func() {}, nil
}

func (r *GitLabResolver) fetchDefaultBranch(ctx context.Context, encodedProject string) (string, error) {
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s", r.baseURL, encodedProject)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	r.setAuth(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitLab API %s: %d", apiURL, resp.StatusCode)
	}

	var result struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.DefaultBranch, nil
}

func (r *GitLabResolver) fetchTreePage(ctx context.Context, encodedProject, ref string, page int) ([]string, bool, error) {
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/tree?recursive=true&ref=%s&per_page=100&page=%d",
		r.baseURL, encodedProject, url.QueryEscape(ref), page)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, false, err
	}
	r.setAuth(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("GitLab API tree: %d", resp.StatusCode)
	}

	var entries []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, false, err
	}

	var paths []string
	for _, e := range entries {
		if e.Type == "blob" {
			paths = append(paths, e.Path)
		}
	}

	// If we got a full page, there might be more
	hasMore := len(entries) == 100
	return paths, hasMore, nil
}

func (r *GitLabResolver) fetchFileRaw(ctx context.Context, encodedProject, ref, filePath string) ([]byte, error) {
	encodedPath := url.PathEscape(filePath)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/%s/raw?ref=%s",
		r.baseURL, encodedProject, encodedPath, url.QueryEscape(ref))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	r.setAuth(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API file %s: %d", filePath, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (r *GitLabResolver) setAuth(req *http.Request) {
	if r.token != "" {
		req.Header.Set("PRIVATE-TOKEN", r.token)
	}
}
```

Remove the `gitlabResolver` stub from `detect.go`. Same as GitHub — `*GitLabResolver` satisfies `TypedResolver` via `Type()`.

- [ ] **Step 5: Run — expect PASS**

```bash
go test ./internal/resolve/... -run TestGitLab -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/resolve/
git commit -m "feat: GitLab resolver using Repository Tree + Files API with golden-file tests"
```

---

## Task 5: Git Clone Fallback

**Files:**
- Create: `internal/resolve/gitclone.go` (replace stub)
- Create: `internal/resolve/gitclone_test.go`

- [ ] **Step 1: Write test**

`internal/resolve/gitclone_test.go`:
```go
package resolve_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/depscope/depscope/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitCloneResolver uses a local bare git repo to avoid network calls.
func TestGitCloneResolver(t *testing.T) {
	// Create a bare git repo with a go.mod file
	bareDir := t.TempDir()
	workDir := t.TempDir()

	// Init bare repo
	run(t, bareDir, "git", "init", "--bare", bareDir)

	// Clone it, add a file, push
	run(t, workDir, "git", "clone", bareDir, workDir)
	run(t, workDir, "git", "config", "user.email", "test@test.com")
	run(t, workDir, "git", "config", "user.name", "Test")

	require.NoError(t, os.WriteFile(filepath.Join(workDir, "go.mod"), []byte("module example.com\n\ngo 1.22\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "main.go"), []byte("package main\n"), 0o644))
	// Also create a node_modules dir that should be ignored
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, "node_modules", "foo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "node_modules", "foo", "package.json"), []byte("{}"), 0o644))

	run(t, workDir, "git", "add", "-A")
	run(t, workDir, "git", "commit", "-m", "init")
	run(t, workDir, "git", "push", "origin", "HEAD")

	// Now test the resolver against the bare repo
	resolver := resolve.NewGitCloneResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	files, cleanup, err := resolver.Resolve(ctx, bareDir)
	require.NoError(t, err)
	defer cleanup()

	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Path
	}

	assert.Contains(t, names, "go.mod")
	assert.NotContains(t, names, "main.go")     // not a manifest
	// node_modules should be filtered out
	for _, name := range names {
		assert.NotContains(t, name, "node_modules")
	}
}

func TestGitCloneResolverTimeout(t *testing.T) {
	resolver := resolve.NewGitCloneResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	_, cleanup, err := resolver.Resolve(ctx, "https://example.com/nonexistent.git")
	defer cleanup()
	assert.Error(t, err)
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "cmd %s %v failed: %s", name, args, string(out))
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/resolve/... -run TestGitClone -v
```

- [ ] **Step 3: Implement GitCloneResolver**

`internal/resolve/gitclone.go` — replace the stub:
```go
package resolve

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type GitCloneResolver struct{}

func NewGitCloneResolver() *GitCloneResolver {
	return &GitCloneResolver{}
}

func (r *GitCloneResolver) Type() string { return "gitclone" }

func (r *GitCloneResolver) Resolve(ctx context.Context, url string) ([]ManifestFile, func(), error) {
	tmpDir, err := os.MkdirTemp("", "depscope-clone-*")
	if err != nil {
		return nil, func() {}, fmt.Errorf("create temp dir: %w", err)
	}

	cleanup := func() { os.RemoveAll(tmpDir) }

	// Check git is available
	if _, err := exec.LookPath("git"); err != nil {
		return nil, cleanup, fmt.Errorf("git is required for scanning non-GitHub/GitLab URLs: %w", err)
	}

	// Shallow clone with context timeout
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", url, tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, cleanup, fmt.Errorf("git clone failed: %w\n%s", err, string(output))
	}

	// Walk and collect manifest files
	var files []ManifestFile
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			return nil
		}

		// Get path relative to the clone root
		rel, err := filepath.Rel(tmpDir, path)
		if err != nil {
			return nil
		}

		if !MatchesManifest(rel) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		files = append(files, ManifestFile{Path: rel, Content: content})
		return nil
	})
	if err != nil {
		return nil, cleanup, fmt.Errorf("walk clone dir: %w", err)
	}

	return files, cleanup, nil
}
```

Remove the `gitCloneResolver` stub from `detect.go`.

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/resolve/... -run TestGitClone -v
```

- [ ] **Step 5: Run full resolve package tests**

```bash
go test ./internal/resolve/... -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/resolve/
git commit -m "feat: git clone fallback resolver with shallow clone and timeout"
```

---

## Task 6: Parser Interface — Add ParseFiles()

**Files:**
- Modify: `internal/manifest/manifest.go`
- Modify: `internal/manifest/manifest_test.go`
- Modify: `internal/manifest/python.go`
- Modify: `internal/manifest/gomod.go`
- Modify: `internal/manifest/rust.go`
- Modify: `internal/manifest/javascript.go`

This task adds `ParseFiles(files map[string][]byte)` to the `Parser` interface and `DetectEcosystemFromFiles()` for remote file groups. Each ecosystem parser's existing `Parse(dir)` becomes a thin wrapper that reads files from disk and delegates to `ParseFiles`.

**Note:** This task modifies files created in the core CLI plan (Tasks 5-8). If those tasks haven't been implemented yet, this task creates the interface with `ParseFiles` from the start so the parsers can be built to support both paths from day one.

- [ ] **Step 1: Write test for DetectEcosystemFromFiles**

Add to `internal/manifest/manifest_test.go`:
```go
func TestDetectEcosystemFromFiles(t *testing.T) {
	tests := []struct {
		filenames []string
		expected  manifest.Ecosystem
	}{
		{[]string{"go.mod", "go.sum"}, manifest.EcosystemGo},
		{[]string{"Cargo.toml", "Cargo.lock"}, manifest.EcosystemRust},
		{[]string{"package.json", "package-lock.json"}, manifest.EcosystemNPM},
		{[]string{"requirements.txt"}, manifest.EcosystemPython},
		{[]string{"pyproject.toml", "poetry.lock"}, manifest.EcosystemPython},
		{[]string{"uv.lock"}, manifest.EcosystemPython},
	}
	for _, tt := range tests {
		eco, err := manifest.DetectEcosystemFromFiles(tt.filenames)
		require.NoError(t, err, "%v", tt.filenames)
		assert.Equal(t, tt.expected, eco, "%v", tt.filenames)
	}

	_, err := manifest.DetectEcosystemFromFiles([]string{"README.md"})
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/manifest/... -run TestDetectEcosystemFromFiles -v
```

- [ ] **Step 3: Implement DetectEcosystemFromFiles and update Parser interface**

Add to `internal/manifest/manifest.go`:
```go
// DetectEcosystemFromFiles detects the ecosystem from a list of filenames
// (used for remote-resolved files where we don't have a directory to stat).
func DetectEcosystemFromFiles(filenames []string) (Ecosystem, error) {
	nameSet := make(map[string]bool)
	for _, f := range filenames {
		nameSet[filepath.Base(f)] = true
	}
	for _, ef := range ecosystemFiles {
		if nameSet[ef.file] {
			return ef.ecosystem, nil
		}
	}
	return "", fmt.Errorf("no recognized manifest in files: %v", filenames)
}
```

Update the `Parser` interface:
```go
type Parser interface {
	Parse(dir string) ([]Package, error)
	ParseFiles(files map[string][]byte) ([]Package, error)
	Ecosystem() Ecosystem
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/manifest/... -run TestDetectEcosystemFromFiles -v
```

- [ ] **Step 5: Add ParseFiles() to all four parsers**

Each parser must implement `ParseFiles(files map[string][]byte) ([]Package, error)`. The existing `Parse(dir)` method becomes a thin wrapper that reads files from disk and calls `ParseFiles`. Example for the Go parser:

`internal/manifest/gomod.go`:
```go
func (p *GoModParser) ParseFiles(files map[string][]byte) ([]Package, error) {
	goModData, ok := files["go.mod"]
	if !ok {
		return nil, fmt.Errorf("go.mod not found in files")
	}
	goSumData := files["go.sum"] // optional

	// ... existing parsing logic, but operating on goModData/goSumData bytes
	// instead of reading from disk ...
}

func (p *GoModParser) Parse(dir string) ([]Package, error) {
	files := make(map[string][]byte)
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && name == "go.mod" {
			return nil, err
		}
		if err == nil {
			files[name] = data
		}
	}
	return p.ParseFiles(files)
}
```

Apply the same pattern to `python.go` (reads `requirements.txt`/`pyproject.toml`/`poetry.lock`/`uv.lock`), `rust.go` (reads `Cargo.toml`/`Cargo.lock`), and `javascript.go` (reads `package.json` + lockfile variants). The key is: move the byte-parsing logic into `ParseFiles`, make `Parse` a disk-reading wrapper.

Run all manifest tests to verify no regressions:

```bash
go test ./internal/manifest/... -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/manifest/
git commit -m "feat: add ParseFiles() to Parser interface and DetectEcosystemFromFiles()"
```

---

## Task 7: CLI Scan — URL Detection and Resolver Integration

**Files:**
- Modify: `cmd/depscope/scan.go`

This modifies the scan command to detect URL arguments and route through the resolver pipeline. Also adds signal handling for cleanup.

**Note:** This task modifies `scan.go` from the core CLI plan (Task 18). If not yet implemented, see the core plan for the full scan command structure — this task adds the URL branch to `runScan`.

- [ ] **Step 1: Write integration test for URL scanning**

Add to `cmd/depscope/scan_test.go`:
```go
func TestScanCommandWithURL(t *testing.T) {
	// Setup: serve a fake GitHub API
	treeResp := `{"tree": [{"path": "requirements.txt", "type": "blob"}], "truncated": false}`
	contentsResp := `{"name": "requirements.txt", "encoding": "base64", "content": "cmVxdWVzdHM9PTIuMzEuMA=="}`
	// base64 of "requests==2.31.0"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/git/trees/"):
			fmt.Fprint(w, treeResp)
		case strings.Contains(r.URL.Path, "/contents/"):
			fmt.Fprint(w, contentsResp)
		default:
			fmt.Fprint(w, `{"default_branch": "main"}`)
		}
	}))
	defer srv.Close()

	// Run scan command pointing at the fake GitHub
	var stdout bytes.Buffer
	code, err := runScanCommand(&stdout,
		"https://github.com/test/repo",
		"--profile", "hobby",
		"--output", "text",
	)
	// This test validates the URL path is wired; full scoring depends on registry stubs
	require.NoError(t, err)
	assert.True(t, code == 0 || code == 1)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./cmd/depscope/... -run TestScanCommandWithURL -v
```

- [ ] **Step 3: Update runScan to handle URLs**

Modify `cmd/depscope/scan.go` — add URL detection at the top of `runScan`:

```go
func runScan(cmd *cobra.Command, args []string) error {
	target := "."
	if len(args) > 0 {
		target = args[0]
	}

	var pkgs []manifest.Package

	if resolve.IsRemoteURL(target) {
		// Remote URL path
		resolver := resolve.DetectResolver(target, resolve.DetectOptions{
			GitHubToken: os.Getenv("GITHUB_TOKEN"),
			GitLabToken: os.Getenv("GITLAB_TOKEN"),
		})

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		// Register signal handler BEFORE Resolve() so cleanup works during slow clones.
		// Use sync.Once to ensure cleanup runs exactly once regardless of signal vs defer.
		var cleanupOnce sync.Once
		var cleanupFn func()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			cleanupOnce.Do(func() {
				if cleanupFn != nil {
					cleanupFn()
				}
			})
			os.Exit(1)
		}()
		defer func() {
			signal.Stop(sigCh)
			cleanupOnce.Do(func() {
				if cleanupFn != nil {
					cleanupFn()
				}
			})
		}()

		files, cleanup, err := resolver.Resolve(ctx, target)
		cleanupFn = cleanup

		if err != nil {
			return fmt.Errorf("resolve remote: %w", err)
		}

		// Group files by directory and parse each group
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
				return fmt.Errorf("parse %s: %w", dir, err)
			}
			pkgs = append(pkgs, parsed...)
		}
	} else {
		// Local path (existing behavior)
		eco, err := manifest.DetectEcosystem(target)
		if err != nil {
			return fmt.Errorf("manifest detection: %w", err)
		}
		pkgs, err = manifest.ParserFor(eco).Parse(target)
		if err != nil {
			return fmt.Errorf("manifest parse: %w", err)
		}
	}

	// ... rest of scoring pipeline unchanged ...
}

// groupByDirectory groups ManifestFiles by their parent directory.
func groupByDirectory(files []resolve.ManifestFile) map[string][]resolve.ManifestFile {
	groups := make(map[string][]resolve.ManifestFile)
	for _, f := range files {
		dir := filepath.Dir(f.Path)
		groups[dir] = append(groups[dir], f)
	}
	return groups
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./cmd/depscope/... -run TestScanCommandWithURL -v
```

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -v
```

- [ ] **Step 6: Commit**

```bash
git add cmd/depscope/scan.go internal/resolve/
git commit -m "feat: scan command accepts remote URLs via resolver dispatch"
```

---

## Task 8: Update Scan Command Usage and Help

**Files:**
- Modify: `cmd/depscope/scan.go`

- [ ] **Step 1: Update command usage string**

```go
var scanCmd = &cobra.Command{
	Use:   "scan [path-or-url]",
	Short: "Scan a project's dependencies for supply chain risk",
	Long: `Scan a local directory or remote repository for dependency risk.

Examples:
  depscope scan .
  depscope scan /path/to/project
  depscope scan https://github.com/owner/repo
  depscope scan https://gitlab.com/group/project
  depscope scan git@custom-host.com:org/repo.git`,
	Args: cobra.MaximumNArgs(1),
	RunE: runScan,
}
```

- [ ] **Step 2: Verify help text**

```bash
go build -o bin/depscope ./cmd/depscope && ./bin/depscope scan --help
```
Expected: usage shows `[path-or-url]` with examples.

- [ ] **Step 3: Commit**

```bash
git add cmd/depscope/scan.go
git commit -m "docs: update scan command usage to show URL examples"
```
