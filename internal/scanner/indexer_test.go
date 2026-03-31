package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/depscope/depscope/internal/cache"
)

// ---------------------------------------------------------------------------
// detectManifestEcosystem
// ---------------------------------------------------------------------------

func TestDetectManifestEcosystem(t *testing.T) {
	tests := []struct {
		relPath string
		eco     string
		ok      bool
	}{
		// npm (only package.json — lockfiles are companions)
		{"package.json", "npm", true},
		{"webapp/package.json", "npm", true},
		// go
		{"go.mod", "go", true},
		{"service/go.mod", "go", true},
		// rust (Cargo.lock is a companion to Cargo.toml)
		{"Cargo.toml", "rust", true},
		{"Cargo.lock", "", false},
		// python (poetry.lock, uv.lock are companions to pyproject.toml)
		{"pyproject.toml", "python", true},
		{"requirements.txt", "python", true},
		{"poetry.lock", "", false},
		{"uv.lock", "", false},
		// php
		{"composer.json", "php", true},
		// NOT indexed (no packages at local scope)
		{"package-lock.json", "", false},     // lockfile without package.json
		{"pnpm-lock.yaml", "", false},        // lockfile without package.json
		{"composer.lock", "", false},          // lockfile without composer.json
		{"go.sum", "", false},                 // companion to go.mod
		{".pre-commit-config.yaml", "", false},
		{".gitmodules", "", false},
		{"Makefile", "", false},
		{"Taskfile.yml", "", false},
		{"justfile", "", false},
		{"infra/main.tf", "", false},          // terraform — no packages at local scope
		{".github/workflows/ci.yml", "", false}, // actions — no packages at local scope
		// not a manifest
		{"README.md", "", false},
		{"src/main.go", "", false},
		{".github/dependabot.yml", "", false},
		{"node_modules/.package-lock.json", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.relPath, func(t *testing.T) {
			eco, ok := detectManifestEcosystem(tt.relPath)
			assert.Equal(t, tt.ok, ok, "expected ok=%v for %s", tt.ok, tt.relPath)
			if tt.ok {
				assert.Equal(t, tt.eco, eco, "ecosystem mismatch for %s", tt.relPath)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RunIndex — end-to-end with fixture directories
// ---------------------------------------------------------------------------

// writeFile is a test helper that creates a file inside dir.
func writeFile(t *testing.T, dir string, relPath string, content []byte) {
	t.Helper()
	abs := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, content, 0o644))
}

// makePackageJSON returns a minimal package.json with the given dependencies.
func makePackageJSON(name string, deps map[string]string) []byte {
	obj := map[string]interface{}{
		"name":         name,
		"version":      "1.0.0",
		"dependencies": deps,
	}
	b, _ := json.Marshal(obj)
	return b
}

// makeNodeModulePackageJSON returns a minimal package.json for an installed package.
func makeNodeModulePackageJSON(name, version string) []byte {
	obj := map[string]interface{}{
		"name":    name,
		"version": version,
	}
	b, _ := json.Marshal(obj)
	return b
}

func buildFixtureTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// npm project
	writeFile(t, root, "webapp/package.json",
		makePackageJSON("my-webapp", map[string]string{"axios": "^1.7.0", "lodash": "^4.17.21"}))

	// node_modules installed package
	writeFile(t, root, "webapp/node_modules/axios/package.json",
		makeNodeModulePackageJSON("axios", "1.7.9"))

	// go project
	writeFile(t, root, "service/go.mod", []byte(`module example.com/service

go 1.22

require (
	golang.org/x/text v0.16.0
	golang.org/x/sync v0.7.0
)
`))

	// hidden dir with python requirements
	writeFile(t, root, ".tools/requirements.txt", []byte(`requests==2.32.3
flask==3.0.3
`))

	// These files exist on disk but should NOT be indexed (no packages at local scope)
	writeFile(t, root, "Makefile", []byte("all:\n\techo hello\n"))
	writeFile(t, root, ".pre-commit-config.yaml", []byte("repos: []\n"))
	writeFile(t, root, "infra/main.tf", []byte(`resource "aws_s3_bucket" "b" {}`))

	// .git dir (should be skipped entirely)
	writeFile(t, root, ".git/config", []byte("[core]\n"))

	return root
}

func TestRunIndex(t *testing.T) {
	root := buildFixtureTree(t)

	dbPath := filepath.Join(t.TempDir(), "test-index.db")
	opts := IndexOptions{
		Scope:  "local",
		DBPath: dbPath,
	}

	var buf bytes.Buffer
	result, err := RunIndex(context.Background(), root, opts, &buf)
	require.NoError(t, err)
	require.NotNil(t, result)

	output := buf.String()

	// Should find: package.json, node_modules/axios/package.json,
	// go.mod, requirements.txt (Makefile, .pre-commit-config.yaml, main.tf are excluded)
	assert.GreaterOrEqual(t, result.ManifestsFound, 4, "should find at least 4 manifests")

	// npm project (2 deps), node_modules axios (1), go project (2), python (2) = 7
	assert.GreaterOrEqual(t, result.PackagesTotal, 3, "should index packages from parsers")

	// Output should contain ecosystem tags
	assert.Contains(t, output, "[npm")
	assert.Contains(t, output, "[go")
	assert.Contains(t, output, "[python")

	// Output should have a Done line
	assert.Contains(t, output, "Done:")

	// Verify DB state
	db, err := cache.NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	stats, err := db.IndexStats(root)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, stats.ManifestCount, 4)
	assert.GreaterOrEqual(t, stats.PackageCount, 3)

	// --- Incremental run: everything should be skipped ---
	var buf2 bytes.Buffer
	result2, err := RunIndex(context.Background(), root, opts, &buf2)
	require.NoError(t, err)
	require.NotNil(t, result2)

	output2 := buf2.String()
	assert.Equal(t, result2.ManifestsUpdated, 0, "second run should update nothing")
	assert.Equal(t, result2.ManifestsSkipped, result.ManifestsFound,
		"second run should skip all manifests")
	assert.Contains(t, output2, "[skip")
}

// ---------------------------------------------------------------------------
// extractNpmName
// ---------------------------------------------------------------------------

func TestExtractNpmName(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"", ""},
		{"node_modules/axios", "axios"},
		{"node_modules/@scope/pkg", "@scope/pkg"},
		{"node_modules/foo/node_modules/bar", "bar"},
		{"node_modules/foo/node_modules/@scope/baz", "@scope/baz"},
		{"something/else", ""},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := extractNpmName(tt.key)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// extractNpmDeps
// ---------------------------------------------------------------------------

func TestExtractNpmDeps(t *testing.T) {
	dir := t.TempDir()

	lockfile := `{
  "lockfileVersion": 3,
  "packages": {
    "": { "dependencies": {"axios": "^1.14.0"} },
    "node_modules/axios": {
      "version": "1.14.1",
      "dependencies": { "follow-redirects": "^1.15.0", "form-data": "^4.0.0" }
    },
    "node_modules/follow-redirects": { "version": "1.15.6" },
    "node_modules/form-data": {
      "version": "4.0.0",
      "dependencies": { "asynckit": "^0.4.0" }
    },
    "node_modules/asynckit": { "version": "0.4.0" }
  }
}`
	writeFile(t, dir, "package-lock.json", []byte(lockfile))

	deps, pkgs := extractNpmDeps(dir)

	// Should have packages for: root, axios, follow-redirects, form-data, asynckit
	assert.Len(t, pkgs, 5, "should create 5 package entries (root + 4 deps)")

	// Expected edges:
	// root → axios (from root "")
	// axios → follow-redirects
	// axios → form-data
	// form-data → asynckit
	assert.Len(t, deps, 4, "should have 4 dependency edges")

	// Build a set of edges for easy lookup.
	type edge struct{ parent, child string }
	edgeSet := map[edge]string{}
	for _, d := range deps {
		edgeSet[edge{d.ParentProjectID, d.ChildProjectID}] = d.ChildConstraint
	}

	assert.Contains(t, edgeSet, edge{"npm/__root__", "npm/axios"})
	assert.Contains(t, edgeSet, edge{"npm/axios", "npm/follow-redirects"})
	assert.Contains(t, edgeSet, edge{"npm/axios", "npm/form-data"})
	assert.Contains(t, edgeSet, edge{"npm/form-data", "npm/asynckit"})

	// Verify constraints are preserved.
	assert.Equal(t, "^1.14.0", edgeSet[edge{"npm/__root__", "npm/axios"}])
	assert.Equal(t, "^0.4.0", edgeSet[edge{"npm/form-data", "npm/asynckit"}])
}

func TestExtractNpmDeps_NoLockfile(t *testing.T) {
	dir := t.TempDir()
	deps, pkgs := extractNpmDeps(dir)
	assert.Nil(t, deps)
	assert.Nil(t, pkgs)
}

// ---------------------------------------------------------------------------
// extractCargoDeps
// ---------------------------------------------------------------------------

func TestExtractCargoDeps(t *testing.T) {
	dir := t.TempDir()

	cargoLock := `[[package]]
name = "my-crate"
version = "0.1.0"
dependencies = [
 "serde",
 "tokio 1.37.0",
]

[[package]]
name = "serde"
version = "1.0.200"
dependencies = [
 "serde_derive",
]

[[package]]
name = "serde_derive"
version = "1.0.200"

[[package]]
name = "tokio"
version = "1.37.0"
`
	writeFile(t, dir, "Cargo.lock", []byte(cargoLock))

	deps, pkgs := extractCargoDeps(dir)

	// 4 packages: my-crate, serde, serde_derive, tokio
	assert.Len(t, pkgs, 4, "should create 4 package entries")

	// Expected edges:
	// my-crate → serde (no version constraint)
	// my-crate → tokio (version 1.37.0)
	// serde → serde_derive (no version constraint)
	assert.Len(t, deps, 3, "should have 3 dependency edges")

	type edge struct{ parent, child string }
	edgeSet := map[edge]string{}
	for _, d := range deps {
		edgeSet[edge{d.ParentProjectID, d.ChildProjectID}] = d.ChildConstraint
	}

	assert.Contains(t, edgeSet, edge{"rust/my-crate", "rust/serde"})
	assert.Contains(t, edgeSet, edge{"rust/my-crate", "rust/tokio"})
	assert.Contains(t, edgeSet, edge{"rust/serde", "rust/serde_derive"})

	// Verify version constraint is captured for tokio.
	assert.Equal(t, "1.37.0", edgeSet[edge{"rust/my-crate", "rust/tokio"}])
	// serde has no version constraint.
	assert.Equal(t, "", edgeSet[edge{"rust/my-crate", "rust/serde"}])
}

func TestExtractCargoDeps_NoLockfile(t *testing.T) {
	dir := t.TempDir()
	deps, pkgs := extractCargoDeps(dir)
	assert.Nil(t, deps)
	assert.Nil(t, pkgs)
}

// ---------------------------------------------------------------------------
// RunIndex with dependency edges (npm lockfile)
// ---------------------------------------------------------------------------

func TestRunIndexWithNpmDeps(t *testing.T) {
	root := t.TempDir()

	// Create package.json
	writeFile(t, root, "package.json",
		makePackageJSON("my-project", map[string]string{"axios": "^1.14.0"}))

	// Create package-lock.json with transitive deps
	lockfile := `{
  "lockfileVersion": 3,
  "packages": {
    "": { "dependencies": {"axios": "^1.14.0"} },
    "node_modules/axios": {
      "version": "1.14.1",
      "dependencies": { "follow-redirects": "^1.15.0", "form-data": "^4.0.0" }
    },
    "node_modules/follow-redirects": { "version": "1.15.6" },
    "node_modules/form-data": {
      "version": "4.0.0",
      "dependencies": { "asynckit": "^0.4.0" }
    },
    "node_modules/asynckit": { "version": "0.4.0" }
  }
}`
	writeFile(t, root, "package-lock.json", []byte(lockfile))

	dbPath := filepath.Join(t.TempDir(), "test-deps.db")
	opts := IndexOptions{
		Scope:  "local",
		DBPath: dbPath,
	}

	var buf bytes.Buffer
	result, err := RunIndex(context.Background(), root, opts, &buf)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have extracted dependency edges.
	assert.Equal(t, 4, result.DepsTotal, "should have 4 dependency edges")

	// Output should mention dep edges.
	output := buf.String()
	assert.Contains(t, output, "dep edges")

	// Verify edges in the database.
	db, err := cache.NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Check root → axios edge.
	rootDeps, err := db.GetVersionDependencies("npm/__root__", "npm/__root__")
	require.NoError(t, err)
	assert.Len(t, rootDeps, 1, "root should have 1 dependency (axios)")
	if len(rootDeps) > 0 {
		assert.Equal(t, "npm/axios", rootDeps[0].ChildProjectID)
		assert.Equal(t, "^1.14.0", rootDeps[0].ChildVersionConstraint)
		assert.Equal(t, "lockfile", rootDeps[0].DepScope)
	}

	// Check axios → follow-redirects, form-data edges.
	axiosDeps, err := db.GetVersionDependencies("npm/axios", "npm/axios@1.14.1")
	require.NoError(t, err)
	assert.Len(t, axiosDeps, 2, "axios should have 2 dependencies")

	childIDs := map[string]bool{}
	for _, d := range axiosDeps {
		childIDs[d.ChildProjectID] = true
	}
	assert.True(t, childIDs["npm/follow-redirects"])
	assert.True(t, childIDs["npm/form-data"])

	// Check form-data → asynckit edge.
	formDataDeps, err := db.GetVersionDependencies("npm/form-data", "npm/form-data@4.0.0")
	require.NoError(t, err)
	assert.Len(t, formDataDeps, 1)
	if len(formDataDeps) > 0 {
		assert.Equal(t, "npm/asynckit", formDataDeps[0].ChildProjectID)
	}
}

// ---------------------------------------------------------------------------
// extractPoetryDeps
// ---------------------------------------------------------------------------

func TestExtractPoetryDeps(t *testing.T) {
	dir := t.TempDir()
	poetryLock := `[[package]]
name = "requests"
version = "2.31.0"

[package.dependencies]
urllib3 = ">=1.21.1,<3"
certifi = ">=2017.4.17"

[[package]]
name = "urllib3"
version = "2.2.1"

[[package]]
name = "certifi"
version = "2024.2.2"
`
	writeFile(t, dir, "poetry.lock", []byte(poetryLock))

	deps, pkgs := extractPoetryDeps(dir)

	assert.Len(t, pkgs, 3)
	assert.Len(t, deps, 2)

	type edge struct{ parent, child string }
	edgeSet := map[edge]string{}
	for _, d := range deps {
		edgeSet[edge{d.ParentProjectID, d.ChildProjectID}] = d.ChildConstraint
	}

	assert.Contains(t, edgeSet, edge{"python/requests", "python/urllib3"})
	assert.Contains(t, edgeSet, edge{"python/requests", "python/certifi"})
	assert.Equal(t, ">=1.21.1,<3", edgeSet[edge{"python/requests", "python/urllib3"}])
}

func TestExtractPoetryDeps_TableDeps(t *testing.T) {
	dir := t.TempDir()
	poetryLock := `[[package]]
name = "flask"
version = "3.0.0"

[package.dependencies]
werkzeug = {version = ">=3.0.0", optional = false}
jinja2 = {version = ">=3.1.2"}

[[package]]
name = "werkzeug"
version = "3.0.1"

[[package]]
name = "jinja2"
version = "3.1.3"
`
	writeFile(t, dir, "poetry.lock", []byte(poetryLock))

	deps, pkgs := extractPoetryDeps(dir)
	assert.Len(t, pkgs, 3)
	assert.Len(t, deps, 2)

	type edge struct{ parent, child string }
	edgeSet := map[edge]string{}
	for _, d := range deps {
		edgeSet[edge{d.ParentProjectID, d.ChildProjectID}] = d.ChildConstraint
	}

	assert.Contains(t, edgeSet, edge{"python/flask", "python/werkzeug"})
	assert.Contains(t, edgeSet, edge{"python/flask", "python/jinja2"})
	assert.Equal(t, ">=3.0.0", edgeSet[edge{"python/flask", "python/werkzeug"}])
	assert.Equal(t, ">=3.1.2", edgeSet[edge{"python/flask", "python/jinja2"}])
}

func TestExtractPoetryDeps_NoLockfile(t *testing.T) {
	dir := t.TempDir()
	deps, pkgs := extractPoetryDeps(dir)
	assert.Nil(t, deps)
	assert.Nil(t, pkgs)
}

func TestRunIndexForce(t *testing.T) {
	root := buildFixtureTree(t)

	dbPath := filepath.Join(t.TempDir(), "test-index-force.db")
	opts := IndexOptions{
		Scope:  "local",
		DBPath: dbPath,
	}

	// First run
	var buf1 bytes.Buffer
	result1, err := RunIndex(context.Background(), root, opts, &buf1)
	require.NoError(t, err)
	require.NotNil(t, result1)
	assert.Greater(t, result1.ManifestsFound, 0)

	// Second run with Force — nothing should be skipped
	opts.Force = true
	var buf2 bytes.Buffer
	result2, err := RunIndex(context.Background(), root, opts, &buf2)
	require.NoError(t, err)
	require.NotNil(t, result2)

	output2 := buf2.String()
	assert.Equal(t, 0, result2.ManifestsSkipped, "force mode should skip nothing")
	assert.NotContains(t, output2, "[skip")
	assert.Equal(t, result1.ManifestsFound, result2.ManifestsFound)
}
