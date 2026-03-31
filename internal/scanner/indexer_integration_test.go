package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/depscope/depscope/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunIndexIntegration(t *testing.T) {
	root := t.TempDir()

	// npm project with node_modules.
	mkIdxFile(t, root, "webapp/package.json", `{
		"name": "webapp",
		"dependencies": {"axios": "^1.14.0", "lodash": "^4.17.21"}
	}`)
	mkIdxFile(t, root, "webapp/node_modules/axios/package.json", `{
		"name": "axios", "version": "1.14.1"
	}`)
	mkIdxFile(t, root, "webapp/node_modules/lodash/package.json", `{
		"name": "lodash", "version": "4.17.21"
	}`)

	// Second npm project sharing axios (test dedup).
	mkIdxFile(t, root, "api/package.json", `{
		"name": "api",
		"dependencies": {"axios": "^1.14.0"}
	}`)
	mkIdxFile(t, root, "api/node_modules/axios/package.json", `{
		"name": "axios", "version": "1.14.1"
	}`)

	// Go project.
	mkIdxFile(t, root, "service/go.mod", "module example.com/svc\n\ngo 1.21\n\nrequire (\n\tgolang.org/x/sync v0.5.0\n)\n")

	// Hidden dir with Python.
	mkIdxFile(t, root, ".scripts/tool/requirements.txt", "requests==2.31.0\nnumpy==1.24.0\n")

	// Scoped npm package.
	mkIdxFile(t, root, "webapp/node_modules/@scope/ui/package.json", `{
		"name": "@scope/ui", "version": "3.0.0"
	}`)

	dbPath := filepath.Join(t.TempDir(), "integration.db")
	var buf strings.Builder

	result, err := RunIndex(context.Background(), root, IndexOptions{
		Scope: "local", DBPath: dbPath,
	}, &buf)
	require.NoError(t, err)

	output := buf.String()

	// Multiple ecosystems detected.
	assert.Contains(t, output, "[npm")
	assert.Contains(t, output, "[go")
	assert.Contains(t, output, "[python")

	// Verify dedup: axios appears in multiple manifests but should be 1 unique project.
	db, err := cache.NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manifests, err := db.FindPackageManifests("npm/axios")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(manifests), 2)

	stats, err := db.IndexStats(root)
	require.NoError(t, err)
	assert.Greater(t, stats.ManifestCount, 4)
	assert.Greater(t, stats.PackageCount, 3)

	// Top packages includes axios.
	found := false
	for _, p := range stats.TopPackages {
		if p.ProjectID == "npm/axios" {
			found = true
			assert.GreaterOrEqual(t, p.Count, 2)
		}
	}
	assert.True(t, found, "axios should be in top packages")

	// Index run recorded.
	lastRun, err := db.GetLastIndexRun(root)
	require.NoError(t, err)
	require.NotNil(t, lastRun)
	assert.Equal(t, result.ManifestsFound, lastRun.ManifestsFound)

	// Scoped package indexed.
	scopedManifests, err := db.FindPackageManifests("npm/@scope/ui")
	require.NoError(t, err)
	assert.Len(t, scopedManifests, 1)

	// No error lines should appear in output (the summary "0 errors" is fine).
	for _, line := range strings.Split(output, "\n") {
		assert.NotContains(t, strings.ToLower(line), "error:", "unexpected error in output")
	}
}

func mkIdxFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	abs := filepath.Join(root, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0644))
}
