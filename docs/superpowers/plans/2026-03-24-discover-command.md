# Discover Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `depscope discover` command that finds all projects affected by a compromised package across a filesystem or project list, classifying exposure into confirmed/potentially/unresolvable/safe buckets.

**Architecture:** Two-phase pipeline — Phase 1 does fast text search across manifest/lockfile files to eliminate non-matches, Phase 2 does precise version parsing and classification using existing manifest parsers and registry clients. New `internal/discover` package with 7 files, new Cobra command in `cmd/depscope/discover_cmd.go`.

**Tech Stack:** Go, Cobra CLI, `golang.org/x/mod/semver` for semver comparison, existing manifest parsers and registry clients.

**Spec:** `docs/superpowers/specs/2026-03-24-discover-command-design.md`

---

### Task 1: Types — `internal/discover/types.go`

**Files:**
- Create: `internal/discover/types.go`
- Test: `internal/discover/types_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/discover/types_test.go
package discover

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	assert.Equal(t, "confirmed", StatusConfirmed.String())
	assert.Equal(t, "potentially", StatusPotentially.String())
	assert.Equal(t, "unresolvable", StatusUnresolvable.String())
	assert.Equal(t, "safe", StatusSafe.String())
}

func TestDiscoverResultSummary(t *testing.T) {
	result := &DiscoverResult{
		Package: "litellm",
		Range:   ">=1.82.7,<1.83.0",
		Matches: []ProjectMatch{
			{Project: "/a", Status: StatusConfirmed},
			{Project: "/b", Status: StatusConfirmed},
			{Project: "/c", Status: StatusPotentially},
			{Project: "/d", Status: StatusSafe},
		},
	}
	s := result.Summary()
	assert.Equal(t, 2, s.Confirmed)
	assert.Equal(t, 1, s.Potentially)
	assert.Equal(t, 0, s.Unresolvable)
	assert.Equal(t, 1, s.Safe)
	assert.Equal(t, 4, s.Total)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/discover/ -run TestStatus -v`
Expected: FAIL — package does not exist yet

- [ ] **Step 3: Write minimal implementation**

```go
// internal/discover/types.go
package discover

// Status represents the exposure classification of a project match.
type Status int

const (
	StatusConfirmed    Status = iota // lockfile confirms affected version
	StatusPotentially                // constraint allows affected versions
	StatusUnresolvable               // cannot determine exposure
	StatusSafe                       // version outside compromised range
)

func (s Status) String() string {
	switch s {
	case StatusConfirmed:
		return "confirmed"
	case StatusPotentially:
		return "potentially"
	case StatusUnresolvable:
		return "unresolvable"
	case StatusSafe:
		return "safe"
	default:
		return "unknown"
	}
}

// ProjectMatch represents a single project's exposure to the target package.
type ProjectMatch struct {
	Project        string   // filesystem path to the project root
	Status         Status
	Source         string   // file where the package was found (e.g., "uv.lock")
	Version        string   // resolved version (from lockfile) or empty
	Constraint     string   // version constraint (from manifest) or empty
	Depth          string   // "direct" or "transitive"
	DependencyPath []string // dependency chain for transitive deps
	Reason         string   // human-readable explanation for the classification
}

// DiscoverResult holds the complete output of a discover run.
type DiscoverResult struct {
	Package string
	Range   string
	Matches []ProjectMatch
}

// Summary returns counts per status bucket.
type ResultSummary struct {
	Confirmed    int `json:"confirmed"`
	Potentially  int `json:"potentially"`
	Unresolvable int `json:"unresolvable"`
	Safe         int `json:"safe"`
	Total        int `json:"total"`
}

func (r *DiscoverResult) Summary() ResultSummary {
	var s ResultSummary
	for _, m := range r.Matches {
		switch m.Status {
		case StatusConfirmed:
			s.Confirmed++
		case StatusPotentially:
			s.Potentially++
		case StatusUnresolvable:
			s.Unresolvable++
		case StatusSafe:
			s.Safe++
		}
	}
	s.Total = len(r.Matches)
	return s
}

// Config holds the settings for a discover run.
type Config struct {
	Package    string   // target package name
	Range      string   // compromised version range string
	StartPath  string   // filesystem walk start path (empty if using ListFile)
	ListFile   string   // path to project list file (empty if using StartPath)
	Ecosystem  string   // optional ecosystem filter (e.g., "python")
	MaxDepth   int      // max directory depth for filesystem walk
	Resolve    bool     // check current installable version via registry
	Offline    bool     // no network calls
	Output     string   // output format: "text" or "json"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/discover/ -run TestStatus -v && go test ./internal/discover/ -run TestDiscoverResult -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discover/types.go internal/discover/types_test.go
git commit -m "feat(discover): add core types for discover command"
```

---

### Task 2: Version range parsing and matching — `internal/discover/version.go`

**Files:**
- Create: `internal/discover/version.go`
- Test: `internal/discover/version_test.go`

This is the most critical piece — parsing version range queries and comparing them against resolved versions and constraints.

- [ ] **Step 1: Write the failing tests**

```go
// internal/discover/version_test.go
package discover

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  Version
	}{
		{"1.82.7", Version{Major: 1, Minor: 82, Patch: 7}},
		{"0.1.0", Version{Major: 0, Minor: 1, Patch: 0}},
		{"2.0.0", Version{Major: 2, Minor: 0, Patch: 0}},
		{"1.82.7rc1", Version{Major: 1, Minor: 82, Patch: 7, Pre: "rc1"}},
		{"v1.2.3", Version{Major: 1, Minor: 2, Patch: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want.Major, got.Major)
			assert.Equal(t, tt.want.Minor, got.Minor)
			assert.Equal(t, tt.want.Patch, got.Patch)
		})
	}
}

func TestParseVersionInvalid(t *testing.T) {
	_, err := ParseVersion("not-a-version")
	assert.Error(t, err)

	_, err = ParseVersion("")
	assert.Error(t, err)
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int // -1, 0, 1
	}{
		{"1.0.0", "2.0.0", -1},
		{"1.82.7", "1.82.7", 0},
		{"1.82.8", "1.82.7", 1},
		{"1.83.0", "1.82.9", 1},
		{"0.1.0", "0.2.0", -1},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			a, _ := ParseVersion(tt.a)
			b, _ := ParseVersion(tt.b)
			assert.Equal(t, tt.want, a.Compare(b))
		})
	}
}

func TestParseRange(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{">=1.82.7,<1.83.0", false},
		{"==1.82.8", false},
		{">=1.82.7", false},
		{"<2.0.0", false},
		{">=1.0,<2.0,!=1.5.0", false},
		{"", true},
		{"invalid", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := ParseRange(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVersionInRange(t *testing.T) {
	tests := []struct {
		version string
		rng     string
		want    bool
	}{
		{"1.82.7", ">=1.82.7,<1.83.0", true},
		{"1.82.9", ">=1.82.7,<1.83.0", true},
		{"1.83.0", ">=1.82.7,<1.83.0", false},
		{"1.82.6", ">=1.82.7,<1.83.0", false},
		{"1.82.8", "==1.82.8", true},
		{"1.82.7", "==1.82.8", false},
		{"1.0.0", ">=1.82.7", false},
		{"2.0.0", ">=1.82.7", true},
		{"1.99.0", "<2.0.0", true},
		{"2.0.0", "<2.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.version+"_in_"+tt.rng, func(t *testing.T) {
			r, err := ParseRange(tt.rng)
			require.NoError(t, err)
			v, err := ParseVersion(tt.version)
			require.NoError(t, err)
			assert.Equal(t, tt.want, r.Contains(v))
		})
	}
}

func TestConstraintOverlaps(t *testing.T) {
	tests := []struct {
		constraint string
		rng        string
		want       bool
	}{
		{">=1.80", ">=1.82.7,<1.83.0", true},    // allows versions in range
		{">=1.84", ">=1.82.7,<1.83.0", false},   // starts above range
		{"<1.82.7", ">=1.82.7,<1.83.0", false},  // ends before range
		{">=1.82.7,<1.82.9", ">=1.82.7,<1.83.0", true}, // subset
		{"==1.82.8", ">=1.82.7,<1.83.0", true},  // exact match in range
		{"==1.84.0", ">=1.82.7,<1.83.0", false},  // exact match outside
		{">=1.0", ">=1.82.7,<1.83.0", true},      // wide open
		{"~=1.82.0", ">=1.82.7,<1.83.0", true},   // compatible release overlaps
	}
	for _, tt := range tests {
		t.Run(tt.constraint+"_overlaps_"+tt.rng, func(t *testing.T) {
			r, err := ParseRange(tt.rng)
			require.NoError(t, err)
			got, err := ConstraintOverlaps(tt.constraint, r)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/discover/ -run "TestParseVersion|TestVersionCompare|TestParseRange|TestVersionInRange|TestConstraintOverlaps" -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/discover/version.go
package discover

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a parsed semantic version.
type Version struct {
	Major int
	Minor int
	Patch int
	Pre   string // pre-release identifier (e.g., "rc1", "beta2")
}

// ParseVersion parses a version string like "1.82.7" or "v1.2.3".
func ParseVersion(s string) (Version, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return Version{}, fmt.Errorf("empty version string")
	}

	// Split off pre-release: "1.82.7rc1" → "1.82.7", "rc1"
	var pre string
	for i, c := range s {
		if c != '.' && (c < '0' || c > '9') {
			pre = s[i:]
			s = s[:i]
			break
		}
	}

	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return Version{}, fmt.Errorf("invalid version: %q", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version: %w", err)
	}
	patch := 0
	if len(parts) >= 3 && parts[2] != "" {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return Version{}, fmt.Errorf("invalid patch version: %w", err)
		}
	}

	return Version{Major: major, Minor: minor, Patch: patch, Pre: pre}, nil
}

// Compare returns -1, 0, or 1.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return cmpInt(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return cmpInt(v.Minor, other.Minor)
	}
	if v.Patch != other.Patch {
		return cmpInt(v.Patch, other.Patch)
	}
	// Pre-release versions have lower precedence than release
	if v.Pre == "" && other.Pre != "" {
		return 1
	}
	if v.Pre != "" && other.Pre == "" {
		return -1
	}
	if v.Pre < other.Pre {
		return -1
	}
	if v.Pre > other.Pre {
		return 1
	}
	return 0
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	return 1
}

// Constraint is a single version constraint like ">=1.82.7" or "<1.83.0".
type Constraint struct {
	Op      string  // ">=", "<=", ">", "<", "==", "!=", "~="
	Version Version
}

// Range is a set of constraints that must all be satisfied.
type Range struct {
	Constraints []Constraint
}

// ParseRange parses a range string like ">=1.82.7,<1.83.0".
func ParseRange(s string) (Range, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Range{}, fmt.Errorf("empty range")
	}

	parts := strings.Split(s, ",")
	var constraints []Constraint
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		c, err := parseConstraint(part)
		if err != nil {
			return Range{}, fmt.Errorf("parsing constraint %q: %w", part, err)
		}
		constraints = append(constraints, c)
	}

	if len(constraints) == 0 {
		return Range{}, fmt.Errorf("no valid constraints in range %q", s)
	}

	return Range{Constraints: constraints}, nil
}

func parseConstraint(s string) (Constraint, error) {
	ops := []string{"~=", "===", "!==", "==", "!=", ">=", "<=", ">", "<"}
	for _, op := range ops {
		if strings.HasPrefix(s, op) {
			ver, err := ParseVersion(strings.TrimSpace(s[len(op):]))
			if err != nil {
				return Constraint{}, err
			}
			return Constraint{Op: op, Version: ver}, nil
		}
	}
	return Constraint{}, fmt.Errorf("no operator found in constraint %q", s)
}

// Contains returns true if the version satisfies all constraints in the range.
func (r Range) Contains(v Version) bool {
	for _, c := range r.Constraints {
		if !c.matches(v) {
			return false
		}
	}
	return true
}

func (c Constraint) matches(v Version) bool {
	cmp := v.Compare(c.Version)
	switch c.Op {
	case "==", "===":
		return cmp == 0
	case "!=", "!==":
		return cmp != 0
	case ">=":
		return cmp >= 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case "<":
		return cmp < 0
	case "~=":
		// Compatible release: ~=1.82.0 means >=1.82.0,<1.83.0
		if cmp < 0 {
			return false
		}
		return v.Major == c.Version.Major && v.Minor == c.Version.Minor
	default:
		return false
	}
}

// ConstraintOverlaps checks if a manifest constraint string allows any version
// that falls within the given range. Used for "potentially affected" classification.
func ConstraintOverlaps(constraint string, r Range) (bool, error) {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		// Unpinned — could be anything, so it overlaps
		return true, nil
	}

	// Parse the constraint as a Range itself
	cr, err := ParseRange(constraint)
	if err != nil {
		// Try as a single constraint with implicit ==
		if v, verr := ParseVersion(constraint); verr == nil {
			return r.Contains(v), nil
		}
		return false, err
	}

	// Compute the effective bounds of both ranges and check for intersection.
	// We use a sampling approach: check if the range boundaries could overlap.
	rLow, rHigh := r.bounds()
	cLow, cHigh := cr.bounds()

	// If constraint's upper bound is below range's lower bound → no overlap
	if cHigh != nil && rLow != nil && cHigh.Compare(*rLow) < 0 {
		return false, nil
	}
	// If constraint's lower bound is above range's upper bound → no overlap
	if cLow != nil && rHigh != nil && cLow.Compare(*rHigh) >= 0 {
		// Check if rHigh is inclusive
		if !r.highInclusive() {
			return false, nil
		}
		if cLow.Compare(*rHigh) > 0 {
			return false, nil
		}
	}

	// If we get here, the ranges could overlap
	return true, nil
}

// bounds returns the lower and upper bounds of a range (nil means unbounded).
func (r Range) bounds() (low *Version, high *Version) {
	for _, c := range r.Constraints {
		switch c.Op {
		case ">=", ">":
			v := c.Version
			if low == nil || v.Compare(*low) > 0 {
				low = &v
			}
		case "<=", "<":
			v := c.Version
			if high == nil || v.Compare(*high) < 0 {
				high = &v
			}
		case "==", "===":
			v := c.Version
			low = &v
			high = &v
		case "~=":
			v := c.Version
			low = &v
			upper := Version{Major: v.Major, Minor: v.Minor + 1, Patch: 0}
			high = &upper
		}
	}
	return
}

func (r Range) highInclusive() bool {
	for _, c := range r.Constraints {
		if c.Op == "<=" || c.Op == "==" {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/discover/ -run "TestParseVersion|TestVersionCompare|TestParseRange|TestVersionInRange|TestConstraintOverlaps" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discover/version.go internal/discover/version_test.go
git commit -m "feat(discover): add version range parsing and matching"
```

---

### Task 3: Filesystem walker — `internal/discover/walker.go`

**Files:**
- Create: `internal/discover/walker.go`
- Test: `internal/discover/walker_test.go`
- Reference: `internal/resolve/filters.go` (reuse `IgnoredDirs`, `ManifestFilenames`)

- [ ] **Step 1: Write the failing tests**

```go
// internal/discover/walker_test.go
package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkProjects(t *testing.T) {
	// Create temp dir structure:
	// root/
	//   project-a/pyproject.toml
	//   project-b/package.json
	//   project-c/go.mod
	//   empty-dir/
	//   node_modules/some-pkg/package.json  (should be ignored)
	root := t.TempDir()

	dirs := []string{
		"project-a", "project-b", "project-c", "empty-dir",
		"node_modules/some-pkg",
	}
	for _, d := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(root, d), 0o755))
	}
	files := map[string]string{
		"project-a/pyproject.toml":          "[project]\n",
		"project-b/package.json":            "{}",
		"project-c/go.mod":                  "module example\n",
		"node_modules/some-pkg/package.json": "{}",
	}
	for path, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(content), 0o644))
	}

	projects, err := WalkProjects(root, 10, "")
	require.NoError(t, err)

	// Should find 3 projects, not the node_modules one
	assert.Len(t, projects, 3)

	paths := make([]string, len(projects))
	for i, p := range projects {
		paths[i] = p.Dir
	}
	assert.Contains(t, paths, filepath.Join(root, "project-a"))
	assert.Contains(t, paths, filepath.Join(root, "project-b"))
	assert.Contains(t, paths, filepath.Join(root, "project-c"))
}

func TestWalkProjectsMaxDepth(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(deep, "go.mod"), []byte("module x\n"), 0o644))

	// Depth 2 should NOT find it (4 levels deep)
	projects, err := WalkProjects(root, 2, "")
	require.NoError(t, err)
	assert.Len(t, projects, 0)

	// Depth 5 should find it
	projects, err = WalkProjects(root, 5, "")
	require.NoError(t, err)
	assert.Len(t, projects, 1)
}

func TestWalkProjectsEcosystemFilter(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "py"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "js"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "py", "pyproject.toml"), []byte("[project]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "js", "package.json"), []byte("{}"), 0o644))

	projects, err := WalkProjects(root, 10, "python")
	require.NoError(t, err)
	assert.Len(t, projects, 1)
	assert.Equal(t, filepath.Join(root, "py"), projects[0].Dir)
}

func TestReadProjectList(t *testing.T) {
	root := t.TempDir()
	// Create two project dirs
	for _, name := range []string{"proj1", "proj2"} {
		dir := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644))
	}

	// Write project list file
	listContent := filepath.Join(root, "proj1") + "\n" +
		"# comment\n" +
		"\n" +
		filepath.Join(root, "proj2") + "\n"
	listFile := filepath.Join(root, "projects.txt")
	require.NoError(t, os.WriteFile(listFile, []byte(listContent), 0o644))

	projects, err := ReadProjectList(listFile, "")
	require.NoError(t, err)
	assert.Len(t, projects, 2)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/discover/ -run "TestWalk|TestReadProject" -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/discover/walker.go
package discover

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/resolve"
)

// ProjectInfo holds a discovered project directory and its manifest files.
type ProjectInfo struct {
	Dir            string   // absolute path to project root
	ManifestFiles  []string // basenames of manifest/lockfile files found
}

// WalkProjects walks the filesystem from startPath, finding directories
// that contain manifest files. Skips ignored directories and symlinks.
// Filters by ecosystem if specified (empty string means all ecosystems).
func WalkProjects(startPath string, maxDepth int, ecosystem string) ([]ProjectInfo, error) {
	startPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil, err
	}

	var projects []ProjectInfo
	seen := make(map[string]bool)

	// Use WalkDir instead of Walk so we can detect symlinks via DirEntry.Type()
	err = filepath.WalkDir(startPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Permission denied etc — skip, continue
			return nil
		}

		// Don't follow symlinks (WalkDir exposes the link type, not the target)
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		if d.IsDir() {
			// Check depth
			rel, _ := filepath.Rel(startPath, path)
			depth := len(strings.Split(rel, string(filepath.Separator)))
			if rel == "." {
				depth = 0
			}
			if depth > maxDepth {
				return filepath.SkipDir
			}

			// Skip ignored directories
			if resolve.IsIgnoredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if this is a manifest file
		base := d.Name()
		isManifest := false
		for _, name := range resolve.ManifestFilenames {
			if base == name {
				isManifest = true
				break
			}
		}
		if !isManifest {
			return nil
		}

		dir := filepath.Dir(path)
		if seen[dir] {
			// Already found this project, just add the file
			for i := range projects {
				if projects[i].Dir == dir {
					projects[i].ManifestFiles = append(projects[i].ManifestFiles, base)
					break
				}
			}
			return nil
		}

		// Ecosystem filter
		if ecosystem != "" {
			eco := ecosystemForFile(base)
			if eco != manifest.Ecosystem(ecosystem) {
				return nil
			}
		}

		seen[dir] = true
		projects = append(projects, ProjectInfo{
			Dir:           dir,
			ManifestFiles: []string{base},
		})
		return nil
	})

	return projects, err
}

// ReadProjectList reads a file containing project paths (one per line).
// Lines starting with # are comments. Empty lines are skipped.
func ReadProjectList(listFile string, ecosystem string) ([]ProjectInfo, error) {
	f, err := os.Open(listFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var projects []ProjectInfo
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check if it's a local path with manifest files
		info, err := os.Stat(line)
		if err != nil || !info.IsDir() {
			continue
		}

		var manifests []string
		for _, name := range resolve.ManifestFilenames {
			if _, err := os.Stat(filepath.Join(line, name)); err == nil {
				if ecosystem != "" && ecosystemForFile(name) != manifest.Ecosystem(ecosystem) {
					continue
				}
				manifests = append(manifests, name)
			}
		}
		if len(manifests) > 0 {
			projects = append(projects, ProjectInfo{
				Dir:           line,
				ManifestFiles: manifests,
			})
		}
	}
	return projects, scanner.Err()
}

// ecosystemForFile maps a manifest filename to its ecosystem.
func ecosystemForFile(filename string) manifest.Ecosystem {
	switch filename {
	case "go.mod", "go.sum":
		return manifest.EcosystemGo
	case "requirements.txt", "pyproject.toml", "poetry.lock", "uv.lock":
		return manifest.EcosystemPython
	case "Cargo.toml", "Cargo.lock":
		return manifest.EcosystemRust
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "bun.lock":
		return manifest.EcosystemNPM
	case "composer.json", "composer.lock":
		return manifest.EcosystemPHP
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/discover/ -run "TestWalk|TestReadProject" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discover/walker.go internal/discover/walker_test.go
git commit -m "feat(discover): add filesystem walker and project list reader"
```

---

### Task 4: Phase 1 text matcher — `internal/discover/matcher.go`

**Files:**
- Create: `internal/discover/matcher.go`
- Test: `internal/discover/matcher_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/discover/matcher_test.go
package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchPackageInFiles(t *testing.T) {
	root := t.TempDir()

	// uv.lock with litellm
	uvLock := `[[package]]
name = "litellm"
version = "1.82.8"

[[package]]
name = "requests"
version = "2.31.0"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "uv.lock"), []byte(uvLock), 0o644))

	// pyproject.toml without litellm
	pyproject := `[project]
dependencies = ["requests>=2.0"]
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte(pyproject), 0o644))

	project := ProjectInfo{
		Dir:           root,
		ManifestFiles: []string{"uv.lock", "pyproject.toml"},
	}

	matched := MatchPackageInProject("litellm", project)
	assert.True(t, matched.Bool())
	assert.Contains(t, matched.Files, "uv.lock")
	assert.NotContains(t, matched.Files, "pyproject.toml")
}

func TestMatchPackageNotFound(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\nrequire github.com/gin-gonic/gin v1.8.0\n"), 0o644))

	project := ProjectInfo{
		Dir:           root,
		ManifestFiles: []string{"go.mod"},
	}

	matched := MatchPackageInProject("litellm", project)
	assert.False(t, matched.Bool())
}

func TestMatchPackageCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	content := `[project]
dependencies = ["LiteLLM>=1.80"]
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte(content), 0o644))

	project := ProjectInfo{
		Dir:           root,
		ManifestFiles: []string{"pyproject.toml"},
	}

	matched := MatchPackageInProject("litellm", project)
	assert.True(t, matched.Bool())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/discover/ -run TestMatchPackage -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/discover/matcher.go
package discover

import (
	"os"
	"path/filepath"
	"strings"
)

// MatchResult holds which files in a project matched the package name.
type MatchResult struct {
	Files []string // basenames of matching files
	matched bool
}

// Bool returns whether any files matched.
func (m MatchResult) Bool() bool { return m.matched }

// MatchPackageInProject checks if the target package name appears in any
// manifest/lockfile of the given project. This is a fast text search —
// Phase 1 of the pipeline.
func MatchPackageInProject(pkgName string, project ProjectInfo) MatchResult {
	target := strings.ToLower(pkgName)
	var matchedFiles []string

	for _, filename := range project.ManifestFiles {
		path := filepath.Join(project.Dir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.ToLower(string(data))
		if strings.Contains(content, target) {
			matchedFiles = append(matchedFiles, filename)
		}
	}

	return MatchResult{
		Files:   matchedFiles,
		matched: len(matchedFiles) > 0,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/discover/ -run TestMatchPackage -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discover/matcher.go internal/discover/matcher_test.go
git commit -m "feat(discover): add Phase 1 text matcher for package search"
```

---

### Task 5: Classifier — `internal/discover/classifier.go`

**Files:**
- Create: `internal/discover/classifier.go`
- Test: `internal/discover/classifier_test.go`
- Reference: `internal/manifest/manifest.go:29-37` (Package struct)
- Reference: `internal/manifest/python.go` (parser pattern)

- [ ] **Step 1: Write the failing tests**

```go
// internal/discover/classifier_test.go
package discover

import (
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyConfirmedFromLockfile(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)

	pkg := manifest.Package{
		Name:            "litellm",
		ResolvedVersion: "1.82.8",
		Depth:           1,
	}
	match := Classify(pkg, r, false)
	assert.Equal(t, StatusConfirmed, match.Status)
	assert.Equal(t, "1.82.8", match.Version)
	assert.Equal(t, "direct", match.Depth)
}

func TestClassifySafeFromLockfile(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)

	pkg := manifest.Package{
		Name:            "litellm",
		ResolvedVersion: "1.83.1",
		Depth:           1,
	}
	match := Classify(pkg, r, false)
	assert.Equal(t, StatusSafe, match.Status)
}

func TestClassifyPotentiallyFromConstraint(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)

	pkg := manifest.Package{
		Name:       "litellm",
		Constraint: ">=1.80",
		Depth:      1,
	}
	match := Classify(pkg, r, false)
	assert.Equal(t, StatusPotentially, match.Status)
}

func TestClassifySafeFromConstraint(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)

	pkg := manifest.Package{
		Name:       "litellm",
		Constraint: ">=1.84",
		Depth:      1,
	}
	match := Classify(pkg, r, false)
	assert.Equal(t, StatusSafe, match.Status)
}

func TestClassifyUnresolvable(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)

	// Bare package name, no version, no constraint
	pkg := manifest.Package{
		Name:  "litellm",
		Depth: 1,
	}
	match := Classify(pkg, r, true) // offline mode
	assert.Equal(t, StatusUnresolvable, match.Status)
}

func TestClassifyTransitive(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)

	pkg := manifest.Package{
		Name:            "litellm",
		ResolvedVersion: "1.82.8",
		Depth:           2,
		Parents:         []string{"langchain"},
	}
	match := Classify(pkg, r, false)
	assert.Equal(t, StatusConfirmed, match.Status)
	assert.Equal(t, "transitive", match.Depth)
	assert.Equal(t, []string{"langchain", "litellm"}, match.DependencyPath)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/discover/ -run TestClassify -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/discover/classifier.go
package discover

import (
	"fmt"

	"github.com/depscope/depscope/internal/manifest"
)

// Classify determines the exposure status of a package against the compromised range.
// If offline is true, no registry resolution is available.
func Classify(pkg manifest.Package, compromised Range, offline bool) ProjectMatch {
	match := ProjectMatch{
		Depth: "direct",
	}
	if pkg.Depth > 1 {
		match.Depth = "transitive"
		match.DependencyPath = append(pkg.Parents, pkg.Name)
	}

	// Case 1: Resolved version available (from lockfile)
	if pkg.ResolvedVersion != "" {
		match.Version = pkg.ResolvedVersion
		v, err := ParseVersion(pkg.ResolvedVersion)
		if err != nil {
			match.Status = StatusUnresolvable
			match.Reason = fmt.Sprintf("cannot parse version %q: %s", pkg.ResolvedVersion, err)
			return match
		}
		if compromised.Contains(v) {
			match.Status = StatusConfirmed
			match.Reason = fmt.Sprintf("resolved version %s is in compromised range", pkg.ResolvedVersion)
		} else {
			match.Status = StatusSafe
			match.Reason = fmt.Sprintf("resolved version %s is outside compromised range", pkg.ResolvedVersion)
		}
		return match
	}

	// Case 2: Constraint only (no resolved version)
	if pkg.Constraint != "" {
		match.Constraint = pkg.Constraint
		overlaps, err := ConstraintOverlaps(pkg.Constraint, compromised)
		if err != nil {
			match.Status = StatusUnresolvable
			match.Reason = fmt.Sprintf("cannot parse constraint %q: %s", pkg.Constraint, err)
			return match
		}
		if overlaps {
			match.Status = StatusPotentially
			match.Reason = fmt.Sprintf("constraint %s allows compromised versions", pkg.Constraint)
		} else {
			match.Status = StatusSafe
			match.Reason = fmt.Sprintf("constraint %s excludes compromised range", pkg.Constraint)
		}
		return match
	}

	// Case 3: No version info at all
	if offline {
		match.Status = StatusUnresolvable
		match.Reason = "no version constraint, no lockfile, offline mode"
	} else {
		match.Status = StatusUnresolvable
		match.Reason = "no version constraint, no lockfile"
	}
	return match
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/discover/ -run TestClassify -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discover/classifier.go internal/discover/classifier_test.go
git commit -m "feat(discover): add version classification logic"
```

---

### Task 6: FetchDependencies interface — `internal/registry/deps.go`

**Files:**
- Create: `internal/registry/deps.go`
- Test: `internal/registry/deps_test.go`
- Modify: `internal/registry/pypi.go` (add FetchDependencies for PyPI)
- Modify: `internal/registry/npm.go` (add FetchDependencies for npm)

This task adds the `DependencyFetcher` interface and implements it for PyPI and npm (the two most critical ecosystems for the user's use case). Go, Rust, and PHP can be added in follow-up work.

- [ ] **Step 1: Write the failing tests**

```go
// internal/registry/deps_test.go
package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPyPIFetchDependencies(t *testing.T) {
	// Mock PyPI response with requires_dist
	resp := map[string]any{
		"info": map[string]any{
			"name":    "langchain",
			"version": "0.1.0",
			"requires_dist": []string{
				"litellm (>=1.82.0)",
				"requests (>=2.0)",
				"pydantic (>=1.0) ; extra == \"extended\"",
			},
		},
		"releases": map[string]any{},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewPyPIClient(WithBaseURL(server.URL))
	deps, err := client.FetchDependencies("langchain", "0.1.0")
	require.NoError(t, err)
	assert.Len(t, deps, 2) // pydantic is an extra, should be excluded
	assert.Equal(t, "litellm", deps[0].Name)
	assert.Equal(t, ">=1.82.0", deps[0].Constraint)
	assert.Equal(t, "requests", deps[1].Name)
}

func TestNPMFetchDependencies(t *testing.T) {
	resp := map[string]any{
		"name":    "some-pkg",
		"version": "1.0.0",
		"dependencies": map[string]string{
			"litellm":  "^1.82.0",
			"express":  "^4.18.0",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewNPMClient(WithBaseURL(server.URL))
	deps, err := client.FetchDependencies("some-pkg", "1.0.0")
	require.NoError(t, err)
	assert.Len(t, deps, 2)

	names := make(map[string]bool)
	for _, d := range deps {
		names[d.Name] = true
	}
	assert.True(t, names["litellm"])
	assert.True(t, names["express"])
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/registry/ -run "TestPyPIFetchDep|TestNPMFetchDep" -v`
Expected: FAIL

- [ ] **Step 3: Write the interface and PyPI implementation**

```go
// internal/registry/deps.go
package registry

// Dependency represents a single dependency entry from a registry.
type Dependency struct {
	Name       string
	Constraint string
}

// DependencyFetcher extends Fetcher with the ability to retrieve a package's
// dependency list from the registry. Used by the discover command to resolve
// transitive dependencies when no lockfile is available.
type DependencyFetcher interface {
	Fetcher
	FetchDependencies(name, version string) ([]Dependency, error)
}
```

Add to `internal/registry/pypi.go` (after the existing `Fetch` method):

```go
// FetchDependencies retrieves the dependency list for a PyPI package.
// Uses the requires_dist field from the JSON API response.
func (c *PyPIClient) FetchDependencies(name, version string) ([]Dependency, error) {
	apiURL := fmt.Sprintf("%s/pypi/%s/%s/json", c.baseURL, name, version)
	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("pypi: GET %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fall back to unversioned endpoint
		apiURL = fmt.Sprintf("%s/pypi/%s/json", c.baseURL, name)
		resp, err = c.httpClient.Get(apiURL)
		if err != nil {
			return nil, fmt.Errorf("pypi: GET %s: %w", apiURL, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("pypi: GET %s returned %d", apiURL, resp.StatusCode)
		}
	}

	var raw struct {
		Info struct {
			RequiresDist []string `json:"requires_dist"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("pypi: decode deps for %s: %w", name, err)
	}

	var deps []Dependency
	for _, req := range raw.Info.RequiresDist {
		// Skip extras: entries like "pydantic ; extra == \"extended\""
		if strings.Contains(req, "extra ==") || strings.Contains(req, "extra==") {
			continue
		}
		// Strip environment markers
		if i := strings.Index(req, ";"); i >= 0 {
			req = strings.TrimSpace(req[:i])
		}
		// Parse "litellm (>=1.82.0)" → name="litellm", constraint=">=1.82.0"
		depName, constraint := parsePyPIDep(req)
		if depName == "" {
			continue
		}
		deps = append(deps, Dependency{Name: depName, Constraint: constraint})
	}
	return deps, nil
}

// parsePyPIDep parses a requires_dist entry like "litellm (>=1.82.0)".
func parsePyPIDep(s string) (name, constraint string) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "("); i >= 0 {
		name = strings.TrimSpace(s[:i])
		end := strings.Index(s, ")")
		if end > i {
			constraint = strings.TrimSpace(s[i+1 : end])
		}
		return
	}
	// No parens — check for space-separated constraint
	parts := strings.Fields(s)
	if len(parts) >= 1 {
		name = parts[0]
	}
	if len(parts) >= 2 {
		constraint = strings.Join(parts[1:], "")
	}
	return
}
```

Add to `internal/registry/npm.go` (after the existing `Fetch` method):

```go
// FetchDependencies retrieves the dependency list for an npm package version.
func (c *NPMClient) FetchDependencies(name, version string) ([]Dependency, error) {
	apiURL := fmt.Sprintf("%s/%s/%s", c.baseURL, name, version)
	resp, err := c.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("npm: GET %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm: GET %s returned %d", apiURL, resp.StatusCode)
	}

	var raw struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("npm: decode deps for %s@%s: %w", name, version, err)
	}

	var deps []Dependency
	for depName, constraint := range raw.Dependencies {
		deps = append(deps, Dependency{Name: depName, Constraint: constraint})
	}
	return deps, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/registry/ -run "TestPyPIFetchDep|TestNPMFetchDep" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/registry/deps.go internal/registry/deps_test.go internal/registry/pypi.go internal/registry/npm.go
git commit -m "feat(registry): add FetchDependencies for PyPI and npm"
```

---

### Task 7: Transitive resolver — `internal/discover/resolve.go`

**Files:**
- Create: `internal/discover/resolve.go`
- Test: `internal/discover/resolve_test.go`
- Reference: `internal/registry/deps.go` (DependencyFetcher interface)

- [ ] **Step 1: Write the failing tests**

```go
// internal/discover/resolve_test.go
package discover

import (
	"testing"

	"github.com/depscope/depscope/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDepFetcher implements registry.DependencyFetcher for testing.
type mockDepFetcher struct {
	deps map[string][]registry.Dependency // key: "name@version"
}

func (m *mockDepFetcher) Fetch(name, version string) (*registry.PackageInfo, error) {
	return &registry.PackageInfo{Name: name, Version: version}, nil
}
func (m *mockDepFetcher) Ecosystem() string { return "PyPI" }
func (m *mockDepFetcher) FetchDependencies(name, version string) ([]registry.Dependency, error) {
	key := name + "@" + version
	return m.deps[key], nil
}

func TestResolveTransitiveFindsTarget(t *testing.T) {
	fetcher := &mockDepFetcher{
		deps: map[string][]registry.Dependency{
			"langchain@0.1.0": {
				{Name: "litellm", Constraint: ">=1.82.0"},
				{Name: "requests", Constraint: ">=2.0"},
			},
			"litellm@1.82.8": {},
			"requests@2.31.0": {},
		},
	}

	// Direct deps of the project being checked
	directDeps := []DepEntry{
		{Name: "langchain", Version: "0.1.0"},
	}

	result, err := ResolveTransitive("litellm", directDeps, fetcher, 10)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "litellm", result.Name)
	assert.Equal(t, ">=1.82.0", result.Constraint)
	assert.Equal(t, []string{"langchain", "litellm"}, result.Path)
}

func TestResolveTransitiveNotFound(t *testing.T) {
	fetcher := &mockDepFetcher{
		deps: map[string][]registry.Dependency{
			"requests@2.31.0": {
				{Name: "urllib3", Constraint: ">=1.26"},
			},
			"urllib3@1.26.0": {},
		},
	}

	directDeps := []DepEntry{
		{Name: "requests", Version: "2.31.0"},
	}

	result, err := ResolveTransitive("litellm", directDeps, fetcher, 10)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestResolveTransitiveMaxDepth(t *testing.T) {
	// Chain: a → b → c → litellm, but max depth = 2 → shouldn't find it
	fetcher := &mockDepFetcher{
		deps: map[string][]registry.Dependency{
			"a@1.0.0": {{Name: "b", Constraint: ">=1.0"}},
			"b@1.0.0": {{Name: "c", Constraint: ">=1.0"}},
			"c@1.0.0": {{Name: "litellm", Constraint: ">=1.82.0"}},
		},
	}

	directDeps := []DepEntry{{Name: "a", Version: "1.0.0"}}

	result, err := ResolveTransitive("litellm", directDeps, fetcher, 2)
	require.NoError(t, err)
	assert.Nil(t, result)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/discover/ -run TestResolveTransitive -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/discover/resolve.go
package discover

import (
	"strings"

	"github.com/depscope/depscope/internal/registry"
)

// DepEntry is a direct dependency with name and resolved version.
type DepEntry struct {
	Name    string
	Version string
}

// TransitiveMatch is the result of finding a target package in a dependency tree.
type TransitiveMatch struct {
	Name       string   // the found package name
	Constraint string   // the constraint under which it was required
	Version    string   // the resolved version (if available from registry)
	Path       []string // dependency chain from direct dep to target
}

// ResolveTransitive walks the dependency tree via registry lookups to find
// if targetPkg exists as a transitive dependency. Uses BFS with depth limit.
func ResolveTransitive(
	targetPkg string,
	directDeps []DepEntry,
	fetcher registry.DependencyFetcher,
	maxDepth int,
) (*TransitiveMatch, error) {
	target := strings.ToLower(targetPkg)

	type queueItem struct {
		name    string
		version string
		path    []string
		depth   int
	}

	var queue []queueItem
	visited := make(map[string]bool)

	for _, dep := range directDeps {
		queue = append(queue, queueItem{
			name:    dep.Name,
			version: dep.Version,
			path:    []string{dep.Name},
			depth:   1,
		})
	}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		key := item.name + "@" + item.version
		if visited[key] {
			continue
		}
		visited[key] = true

		if item.depth > maxDepth {
			continue
		}

		deps, err := fetcher.FetchDependencies(item.name, item.version)
		if err != nil {
			continue // graceful degradation
		}

		for _, dep := range deps {
			depName := strings.ToLower(dep.Name)
			newPath := make([]string, len(item.path))
			copy(newPath, item.path)
			newPath = append(newPath, dep.Name)

			if depName == target {
				return &TransitiveMatch{
					Name:       dep.Name,
					Constraint: dep.Constraint,
					Path:       newPath,
				}, nil
			}

			// Extract version from constraint for next lookup
			// For constraints like ">=1.82.0", we need to resolve the actual version.
			// Use the constraint string to fetch — registry handles resolution.
			depVersion := extractVersionFromConstraint(dep.Constraint)
			queue = append(queue, queueItem{
				name:    dep.Name,
				version: depVersion,
				path:    newPath,
				depth:   item.depth + 1,
			})
		}
	}

	return nil, nil
}

// extractVersionFromConstraint tries to extract a concrete version from a constraint.
// For "==1.2.3" returns "1.2.3". For ">=1.0" returns "" (let registry resolve latest).
func extractVersionFromConstraint(constraint string) string {
	constraint = strings.TrimSpace(constraint)
	if strings.HasPrefix(constraint, "==") {
		return strings.TrimSpace(strings.TrimPrefix(constraint, "=="))
	}
	// For non-exact constraints, return empty to let the registry resolve latest
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/discover/ -run TestResolveTransitive -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discover/resolve.go internal/discover/resolve_test.go
git commit -m "feat(discover): add transitive dependency resolver via registry"
```

---

### Task 8: Orchestrator — `internal/discover/discover.go`

**Files:**
- Create: `internal/discover/discover.go`
- Test: `internal/discover/discover_test.go`
- Reference: `internal/manifest/manifest.go:100-115` (ParserFor)

- [ ] **Step 1: Write the failing test**

```go
// internal/discover/discover_test.go
package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverIntegrationOffline(t *testing.T) {
	// Build a temp directory tree with multiple "projects"
	root := t.TempDir()

	// Project 1: has uv.lock with litellm 1.82.8 (CONFIRMED)
	proj1 := filepath.Join(root, "proj1")
	require.NoError(t, os.MkdirAll(proj1, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(proj1, "uv.lock"), []byte(`[[package]]
name = "litellm"
version = "1.82.8"
`), 0o644))

	// Project 2: has pyproject.toml with litellm>=1.80 (POTENTIALLY)
	proj2 := filepath.Join(root, "proj2")
	require.NoError(t, os.MkdirAll(proj2, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(proj2, "pyproject.toml"), []byte(`[project]
dependencies = ["litellm>=1.80"]
`), 0o644))

	// Project 3: has uv.lock with litellm 1.83.1 (SAFE)
	proj3 := filepath.Join(root, "proj3")
	require.NoError(t, os.MkdirAll(proj3, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(proj3, "uv.lock"), []byte(`[[package]]
name = "litellm"
version = "1.83.1"
`), 0o644))

	// Project 4: no litellm at all (should not appear in results)
	proj4 := filepath.Join(root, "proj4")
	require.NoError(t, os.MkdirAll(proj4, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(proj4, "pyproject.toml"), []byte(`[project]
dependencies = ["requests>=2.0"]
`), 0o644))

	cfg := Config{
		Package:   "litellm",
		Range:     ">=1.82.7,<1.83.0",
		StartPath: root,
		MaxDepth:  10,
		Offline:   true,
	}

	result, err := Run(cfg)
	require.NoError(t, err)

	assert.Equal(t, "litellm", result.Package)
	assert.Len(t, result.Matches, 3) // proj4 not included

	summary := result.Summary()
	assert.Equal(t, 1, summary.Confirmed)
	assert.Equal(t, 1, summary.Potentially)
	assert.Equal(t, 1, summary.Safe)
	assert.Equal(t, 0, summary.Unresolvable)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/discover/ -run TestDiscoverIntegration -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/discover/discover.go
package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
)

// Run executes the full discover pipeline.
func Run(cfg Config) (*DiscoverResult, error) {
	// Validate inputs
	compromised, err := ParseRange(cfg.Range)
	if err != nil {
		return nil, fmt.Errorf("invalid range %q: %w", cfg.Range, err)
	}

	// Phase 0: Enumerate projects
	var projects []ProjectInfo
	if cfg.ListFile != "" {
		projects, err = ReadProjectList(cfg.ListFile, cfg.Ecosystem)
	} else {
		startPath := cfg.StartPath
		if startPath == "" {
			startPath = "."
		}
		projects, err = WalkProjects(startPath, cfg.MaxDepth, cfg.Ecosystem)
	}
	if err != nil {
		return nil, fmt.Errorf("enumerating projects: %w", err)
	}

	// Build dependency fetchers for transitive resolution (non-offline mode).
	// Only PyPI and npm support FetchDependencies currently; Go, Rust,
	// and PHP will silently skip transitive resolution until implemented.
	var depFetchers map[string]registry.DependencyFetcher
	if !cfg.Offline {
		depFetchers = map[string]registry.DependencyFetcher{
			"PyPI": registry.NewPyPIClient(),
			"npm":  registry.NewNPMClient(),
		}
	}

	result := &DiscoverResult{
		Package: cfg.Package,
		Range:   cfg.Range,
	}

	// Phase 1: Fast text search
	type matchedProject struct {
		project ProjectInfo
		match   MatchResult
	}
	var matched []matchedProject
	for _, proj := range projects {
		m := MatchPackageInProject(cfg.Package, proj)
		if m.Bool() {
			matched = append(matched, matchedProject{project: proj, match: m})
		}
	}

	// Phase 2: Precise classification
	for _, mp := range matched {
		matches := classifyProject(cfg.Package, mp.project, mp.match, compromised, cfg.Offline, depFetchers)
		result.Matches = append(result.Matches, matches...)
	}

	return result, nil
}

// classifyProject parses the matched manifest/lockfile files and classifies
// the package. Uses ParseFiles with the specific matched file content rather
// than Parse(dir), because Parse(dir) may not check all file types
// (e.g., PythonParser.Parse() skips pyproject.toml if uv.lock exists).
func classifyProject(
	pkgName string,
	project ProjectInfo,
	matchResult MatchResult,
	compromised Range,
	offline bool,
	depFetchers map[string]registry.DependencyFetcher,
) []ProjectMatch {
	target := strings.ToLower(pkgName)
	var results []ProjectMatch

	for _, filename := range matchResult.Files {
		eco := ecosystemForFile(filename)
		if eco == "" {
			continue
		}
		parser := manifest.ParserFor(eco)

		// Read the specific matched file and parse it via ParseFiles.
		// This ensures pyproject.toml is parsed even without a lockfile.
		data, err := os.ReadFile(filepath.Join(project.Dir, filename))
		if err != nil {
			continue
		}
		fileMap := map[string][]byte{filename: data}
		pkgs, err := parser.ParseFiles(fileMap)
		if err != nil {
			continue
		}

		foundTarget := false
		for _, pkg := range pkgs {
			if strings.ToLower(pkg.Name) != target {
				continue
			}
			foundTarget = true

			match := Classify(pkg, compromised, offline)
			match.Project = project.Dir
			match.Source = filename
			results = append(results, match)
		}

		// If target not found as direct dep and we're not offline,
		// try transitive resolution via registry for this project.
		if !foundTarget && !offline && depFetchers != nil {
			ecoStr := eco.String()
			if fetcher, ok := depFetchers[ecoStr]; ok {
				directDeps := make([]DepEntry, 0, len(pkgs))
				for _, pkg := range pkgs {
					directDeps = append(directDeps, DepEntry{
						Name:    pkg.Name,
						Version: pkg.ResolvedVersion,
					})
				}
				tmatch, err := ResolveTransitive(pkgName, directDeps, fetcher, 10)
				if err == nil && tmatch != nil {
					match := ProjectMatch{
						Project:        project.Dir,
						Source:         filename,
						Constraint:     tmatch.Constraint,
						Depth:          "transitive",
						DependencyPath: tmatch.Path,
					}
					// If we got a resolved version from the constraint, classify it
					if v, verr := ParseVersion(tmatch.Version); verr == nil {
						match.Version = tmatch.Version
						if compromised.Contains(v) {
							match.Status = StatusConfirmed
							match.Reason = fmt.Sprintf("transitive dep resolved to %s (in compromised range)", tmatch.Version)
						} else {
							match.Status = StatusSafe
							match.Reason = fmt.Sprintf("transitive dep resolved to %s (outside compromised range)", tmatch.Version)
						}
					} else {
						// Have constraint but no resolved version — check overlap
						overlaps, _ := ConstraintOverlaps(tmatch.Constraint, compromised)
						if overlaps {
							match.Status = StatusPotentially
							match.Reason = fmt.Sprintf("transitive dep constraint %s allows compromised versions", tmatch.Constraint)
						} else {
							match.Status = StatusSafe
							match.Reason = fmt.Sprintf("transitive dep constraint %s excludes compromised range", tmatch.Constraint)
						}
					}
					results = append(results, match)
				}
			}
		}
	}

	// Deduplicate: if same project has both lockfile (CONFIRMED/SAFE) and
	// manifest (POTENTIALLY), prefer the lockfile result.
	return deduplicateMatches(results)
}

// deduplicateMatches keeps the highest-confidence match per project.
// Priority: CONFIRMED > POTENTIALLY > UNRESOLVABLE > SAFE
func deduplicateMatches(matches []ProjectMatch) []ProjectMatch {
	if len(matches) <= 1 {
		return matches
	}

	// Find the best match (most actionable)
	best := matches[0]
	for _, m := range matches[1:] {
		if statusPriority(m.Status) > statusPriority(best.Status) {
			best = m
		}
	}
	return []ProjectMatch{best}
}

func statusPriority(s Status) int {
	switch s {
	case StatusConfirmed:
		return 3
	case StatusPotentially:
		return 2
	case StatusUnresolvable:
		return 1
	case StatusSafe:
		return 0
	default:
		return -1
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/discover/ -run TestDiscoverIntegration -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discover/discover.go internal/discover/discover_test.go
git commit -m "feat(discover): add orchestrator with two-phase pipeline"
```

---

### Task 9: Text and JSON output — `internal/report/discover.go`

**Files:**
- Create: `internal/report/discover.go`
- Test: `internal/report/discover_test.go`
- Reference: `internal/report/text.go` (existing text output pattern)
- Reference: `internal/report/json.go` (existing JSON output pattern)

- [ ] **Step 1: Write the failing tests**

```go
// internal/report/discover_test.go
package report

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/depscope/depscope/internal/discover"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteDiscoverText(t *testing.T) {
	result := &discover.DiscoverResult{
		Package: "litellm",
		Range:   ">=1.82.7,<1.83.0",
		Matches: []discover.ProjectMatch{
			{Project: "/repos/api", Status: discover.StatusConfirmed, Source: "uv.lock", Version: "1.82.8", Depth: "direct"},
			{Project: "/repos/ml", Status: discover.StatusPotentially, Source: "pyproject.toml", Constraint: ">=1.80"},
			{Project: "/repos/chat", Status: discover.StatusSafe, Source: "uv.lock", Version: "1.83.1", Depth: "direct"},
		},
	}

	var buf bytes.Buffer
	err := WriteDiscoverText(&buf, result)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "CONFIRMED")
	assert.Contains(t, output, "/repos/api")
	assert.Contains(t, output, "1.82.8")
	assert.Contains(t, output, "POTENTIALLY")
	assert.Contains(t, output, "SAFE")
}

func TestWriteDiscoverJSON(t *testing.T) {
	result := &discover.DiscoverResult{
		Package: "litellm",
		Range:   ">=1.82.7,<1.83.0",
		Matches: []discover.ProjectMatch{
			{Project: "/repos/api", Status: discover.StatusConfirmed, Source: "uv.lock", Version: "1.82.8", Depth: "direct"},
		},
	}

	var buf bytes.Buffer
	err := WriteDiscoverJSON(&buf, result)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "litellm", parsed["package"])
	assert.Equal(t, ">=1.82.7,<1.83.0", parsed["range"])

	results := parsed["results"].([]any)
	assert.Len(t, results, 1)

	first := results[0].(map[string]any)
	assert.Equal(t, "confirmed", first["status"])
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/report/ -run TestWriteDiscover -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

```go
// internal/report/discover.go
package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/depscope/depscope/internal/discover"
)

// WriteDiscoverText writes a human-readable discover report grouped by status.
func WriteDiscoverText(w io.Writer, result *discover.DiscoverResult) error {
	fmt.Fprintf(w, "Package: %s | Range: %s\n\n", result.Package, result.Range)

	buckets := map[discover.Status][]discover.ProjectMatch{
		discover.StatusConfirmed:    {},
		discover.StatusPotentially:  {},
		discover.StatusUnresolvable: {},
		discover.StatusSafe:         {},
	}
	for _, m := range result.Matches {
		buckets[m.Status] = append(buckets[m.Status], m)
	}

	labels := []struct {
		status discover.Status
		icon   string
		label  string
	}{
		{discover.StatusConfirmed, "\U0001F534", "CONFIRMED AFFECTED"},
		{discover.StatusPotentially, "\U0001F7E1", "POTENTIALLY AFFECTED"},
		{discover.StatusUnresolvable, "\U0001F535", "UNRESOLVABLE"},
		{discover.StatusSafe, "\U0001F7E2", "SAFE"},
	}

	for _, l := range labels {
		matches := buckets[l.status]
		if len(matches) == 0 {
			continue
		}
		fmt.Fprintf(w, "%s %s (%d projects)\n", l.icon, l.label, len(matches))
		for _, m := range matches {
			fmt.Fprintf(w, "  %s\n", m.Project)
			fmt.Fprintf(w, "    Source: %s\n", m.Source)
			if m.Version != "" {
				fmt.Fprintf(w, "    Installed: %s %s\n", result.Package, m.Version)
			}
			if m.Constraint != "" {
				fmt.Fprintf(w, "    Constraint: %s %s\n", result.Package, m.Constraint)
			}
			if m.Depth != "" {
				depthStr := m.Depth
				if len(m.DependencyPath) > 1 {
					chain := ""
					for i, p := range m.DependencyPath {
						if i > 0 {
							chain += " \u2192 "
						}
						chain += p
					}
					depthStr += " (via " + chain + ")"
				}
				fmt.Fprintf(w, "    Depth: %s\n", depthStr)
			}
			if m.Reason != "" {
				fmt.Fprintf(w, "    Reason: %s\n", m.Reason)
			}
			fmt.Fprintln(w)
		}
	}

	s := result.Summary()
	fmt.Fprintf(w, "Summary: %d confirmed, %d potentially, %d unresolvable, %d safe (%d total)\n",
		s.Confirmed, s.Potentially, s.Unresolvable, s.Safe, s.Total)

	return nil
}

// WriteDiscoverJSON writes a JSON-encoded discover report.
func WriteDiscoverJSON(w io.Writer, result *discover.DiscoverResult) error {
	type jsonMatch struct {
		Status         string   `json:"status"`
		Project        string   `json:"project"`
		Source         string   `json:"source"`
		Version        string   `json:"version,omitempty"`
		Constraint     string   `json:"constraint,omitempty"`
		Depth          string   `json:"depth,omitempty"`
		DependencyPath []string `json:"dependency_path,omitempty"`
		Reason         string   `json:"reason,omitempty"`
	}

	matches := make([]jsonMatch, len(result.Matches))
	for i, m := range result.Matches {
		matches[i] = jsonMatch{
			Status:         m.Status.String(),
			Project:        m.Project,
			Source:         m.Source,
			Version:        m.Version,
			Constraint:     m.Constraint,
			Depth:          m.Depth,
			DependencyPath: m.DependencyPath,
			Reason:         m.Reason,
		}
	}

	s := result.Summary()
	out := struct {
		Package string      `json:"package"`
		Range   string      `json:"range"`
		Results []jsonMatch `json:"results"`
		Summary struct {
			Confirmed    int `json:"confirmed"`
			Potentially  int `json:"potentially"`
			Unresolvable int `json:"unresolvable"`
			Safe         int `json:"safe"`
			Total        int `json:"total"`
		} `json:"summary"`
	}{
		Package: result.Package,
		Range:   result.Range,
		Results: matches,
	}
	out.Summary.Confirmed = s.Confirmed
	out.Summary.Potentially = s.Potentially
	out.Summary.Unresolvable = s.Unresolvable
	out.Summary.Safe = s.Safe
	out.Summary.Total = s.Total

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/report/ -run TestWriteDiscover -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/report/discover.go internal/report/discover_test.go
git commit -m "feat(report): add text and JSON formatters for discover results"
```

---

### Task 10: CLI command — `cmd/depscope/discover_cmd.go`

**Files:**
- Create: `cmd/depscope/discover_cmd.go`
- Test: `cmd/depscope/discover_test.go`
- Reference: `cmd/depscope/main.go:11-14` (rootCmd)
- Reference: `cmd/depscope/scan.go:17-26` (init pattern for adding commands)

- [ ] **Step 1: Write the failing test**

```go
// cmd/depscope/discover_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverCmdRequiresRange(t *testing.T) {
	// discover without --range should fail
	rootCmd.SetArgs([]string{"discover", "litellm", "."})
	err := rootCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "range")
}

func TestDiscoverCmdInvalidRange(t *testing.T) {
	rootCmd.SetArgs([]string{"discover", "litellm", "--range", "invalid", "."})
	err := rootCmd.Execute()
	assert.Error(t, err)
}

func TestDiscoverCmdOffline(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "uv.lock"), []byte(`[[package]]
name = "litellm"
version = "1.82.8"
`), 0o644))

	rootCmd.SetArgs([]string{"discover", "litellm", "--range", ">=1.82.7,<1.83.0", "--offline", root})
	err := rootCmd.Execute()
	// Should succeed (exit 0) — packages found
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/depscope/ -run TestDiscoverCmd -v`
Expected: FAIL — command not registered

- [ ] **Step 3: Write minimal implementation**

```go
// cmd/depscope/discover_cmd.go
package main

import (
	"fmt"
	"os"

	"github.com/depscope/depscope/internal/discover"
	"github.com/depscope/depscope/internal/report"
	"github.com/spf13/cobra"
)

func init() {
	discoverCmd.Flags().String("range", "", "compromised version range (required)")
	discoverCmd.Flags().String("list", "", "path to file containing project paths (one per line)")
	discoverCmd.Flags().Bool("resolve", false, "check current installable version via registry")
	discoverCmd.Flags().Bool("offline", false, "no network calls")
	discoverCmd.Flags().String("output", "text", "output format: text, json")
	discoverCmd.Flags().String("ecosystem", "", "filter to ecosystem: python, npm, rust, go, php")
	discoverCmd.Flags().Int("max-depth", 10, "max directory depth for filesystem walk")
	rootCmd.AddCommand(discoverCmd)
}

var discoverCmd = &cobra.Command{
	Use:   "discover <package> [path]",
	Short: "Find projects affected by a compromised package",
	Long: `Search across multiple projects to find all occurrences of a package
and classify exposure against a compromised version range.

Projects are discovered via filesystem walk (default) or a project list file.
Each project is classified as: confirmed, potentially, unresolvable, or safe.

Examples:
  depscope discover litellm --range ">=1.82.7,<1.83.0" /home/me/repos
  depscope discover litellm --range ">=1.82.7,<1.83.0" --list projects.txt
  depscope discover litellm --range ">=1.82.7,<1.83.0" --offline /home/me/repos
  depscope discover litellm --range ">=1.82.7,<1.83.0" --output json /home/me/repos`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runDiscover,
}

func runDiscover(cmd *cobra.Command, args []string) error {
	pkgName := args[0]
	startPath := "."
	if len(args) >= 2 {
		startPath = args[1]
	}

	rangeStr, _ := cmd.Flags().GetString("range")
	if rangeStr == "" {
		return fmt.Errorf("--range is required")
	}

	listFile, _ := cmd.Flags().GetString("list")
	resolve, _ := cmd.Flags().GetBool("resolve")
	offline, _ := cmd.Flags().GetBool("offline")
	outputFmt, _ := cmd.Flags().GetString("output")
	ecosystem, _ := cmd.Flags().GetString("ecosystem")
	maxDepth, _ := cmd.Flags().GetInt("max-depth")

	if offline && resolve {
		return fmt.Errorf("--offline and --resolve are mutually exclusive")
	}

	cfg := discover.Config{
		Package:   pkgName,
		Range:     rangeStr,
		StartPath: startPath,
		ListFile:  listFile,
		Ecosystem: ecosystem,
		MaxDepth:  maxDepth,
		Resolve:   resolve,
		Offline:   offline,
		Output:    outputFmt,
	}

	result, err := discover.Run(cfg)
	if err != nil {
		return err
	}

	switch outputFmt {
	case "json":
		if err := report.WriteDiscoverJSON(os.Stdout, result); err != nil {
			return fmt.Errorf("write json: %w", err)
		}
	default:
		if err := report.WriteDiscoverText(os.Stdout, result); err != nil {
			return fmt.Errorf("write text: %w", err)
		}
	}

	// Exit code 1 if any confirmed or potentially affected projects found
	summary := result.Summary()
	if summary.Confirmed > 0 || summary.Potentially > 0 {
		return exitError{1}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/depscope/ -run TestDiscoverCmd -v`
Expected: PASS

- [ ] **Step 5: Run the full test suite**

Run: `go test ./... -count=1`
Expected: All existing tests still pass, new tests pass

- [ ] **Step 6: Commit**

```bash
git add cmd/depscope/discover_cmd.go cmd/depscope/discover_test.go
git commit -m "feat(discover): add CLI command with all flags and output formats"
```

---

### Task 11: Build and manual smoke test

**Files:**
- No new files

- [ ] **Step 1: Build the binary**

Run: `go build -o depscope ./cmd/depscope/`
Expected: Compiles without errors

- [ ] **Step 2: Verify help text**

Run: `./depscope discover --help`
Expected: Shows usage, flags, and examples

- [ ] **Step 3: Smoke test against the depscope project itself**

Run: `./depscope discover cobra --range ">=1.10.0,<1.10.3" --offline .`
Expected: Finds go.mod with cobra v1.10.2, classifies as CONFIRMED (since 1.10.2 is in range >=1.10.0,<1.10.3)

- [ ] **Step 4: Smoke test JSON output**

Run: `./depscope discover cobra --range ">=1.10.0,<1.10.3" --offline --output json .`
Expected: Valid JSON output with confirmed result

- [ ] **Step 5: Smoke test no match**

Run: `./depscope discover nonexistent-pkg --range ">=1.0.0" --offline .`
Expected: No results, exit code 0

- [ ] **Step 6: Run full test suite one more time**

Run: `go test ./... -race -count=1`
Expected: All tests pass with race detector enabled

- [ ] **Step 7: Commit (if any fixes were needed)**

```bash
git commit -am "fix(discover): fixes from smoke testing"
```
