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
		// rust
		{"Cargo.toml", "rust", true},
		{"Cargo.lock", "rust", true},
		// python
		{"pyproject.toml", "python", true},
		{"requirements.txt", "python", true},
		{"poetry.lock", "python", true},
		{"uv.lock", "python", true},
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
