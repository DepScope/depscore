package scanner

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/depscope/depscope/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCompromisedList(t *testing.T) {
	targets, err := ParseCompromisedList("axios@1.14.1,axios@0.30.4")
	require.NoError(t, err)
	require.Len(t, targets, 2)
	assert.Equal(t, "axios", targets[0].Name)
	assert.Equal(t, "1.14.1", targets[0].VersionOrRange)
	assert.Equal(t, "axios", targets[1].Name)
	assert.Equal(t, "0.30.4", targets[1].VersionOrRange)
}

func TestParseCompromisedFile(t *testing.T) {
	content := `# Known compromised packages
axios@1.14.1
axios@0.30.4

# Ranges
event-stream@>=3.3.4,<3.3.7
`
	f := filepath.Join(t.TempDir(), "compromised.txt")
	require.NoError(t, os.WriteFile(f, []byte(content), 0644))

	targets, err := ParseCompromisedFile(f)
	require.NoError(t, err)
	require.Len(t, targets, 3)
	assert.Equal(t, "axios", targets[0].Name)
	assert.Equal(t, "1.14.1", targets[0].VersionOrRange)
	assert.Equal(t, "event-stream", targets[2].Name)
	assert.Equal(t, ">=3.3.4,<3.3.7", targets[2].VersionOrRange)
}

func TestScanCompromised(t *testing.T) {
	// Build a fake project directory with package.json + lockfile.
	root := t.TempDir()

	// Direct dep: package.json lists axios ^1.14.0, lockfile resolves to 1.14.1.
	appDir := filepath.Join(root, "apps", "web")
	require.NoError(t, os.MkdirAll(appDir, 0755))

	pkgJSON := `{
		"name": "my-app",
		"dependencies": {
			"axios": "^1.14.0",
			"express": "^4.18.0"
		}
	}`
	lockJSON := `{
		"lockfileVersion": 3,
		"packages": {
			"node_modules/axios": { "version": "1.14.1" },
			"node_modules/express": { "version": "4.18.2" },
			"node_modules/follow-redirects": { "version": "1.15.6" }
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "package.json"), []byte(pkgJSON), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "package-lock.json"), []byte(lockJSON), 0644))

	// Indirect dep: another project has axios 0.30.4 only in lockfile.
	libDir := filepath.Join(root, "libs", "api")
	require.NoError(t, os.MkdirAll(libDir, 0755))

	libPkgJSON := `{
		"name": "api-lib",
		"dependencies": {
			"@mylib/http": "^2.0.0"
		}
	}`
	libLockJSON := `{
		"lockfileVersion": 3,
		"packages": {
			"node_modules/@mylib/http": { "version": "2.1.0" },
			"node_modules/axios": { "version": "0.30.4" }
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(libDir, "package.json"), []byte(libPkgJSON), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(libDir, "package-lock.json"), []byte(libLockJSON), 0644))

	// Hidden dir with another hit.
	hiddenDir := filepath.Join(root, ".config", "tool")
	require.NoError(t, os.MkdirAll(hiddenDir, 0755))
	hiddenPkgJSON := `{
		"name": "tool-config",
		"dependencies": { "axios": "1.14.1" }
	}`
	hiddenLockJSON := `{
		"lockfileVersion": 3,
		"packages": {
			"node_modules/axios": { "version": "1.14.1" }
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(hiddenDir, "package.json"), []byte(hiddenPkgJSON), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(hiddenDir, "package-lock.json"), []byte(hiddenLockJSON), 0644))

	targets := []CompromisedTarget{
		{Name: "axios", VersionOrRange: "1.14.1"},
		{Name: "axios", VersionOrRange: "0.30.4"},
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	findings, err := ScanCompromised(context.Background(), root, targets, dbPath, io.Discard)
	require.NoError(t, err)

	// Expect 3 findings: direct in apps/web, indirect in libs/api, direct in .config/tool.
	require.Len(t, findings, 3)

	// Sort by manifest path for stable assertions.
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].ManifestPath < findings[j].ManifestPath
	})

	// .config/tool — direct
	assert.Equal(t, ".config/tool/package.json", findings[0].ManifestPath)
	assert.Equal(t, "direct", findings[0].Relation)
	assert.Equal(t, "1.14.1", findings[0].Version)

	// apps/web — direct
	assert.Equal(t, "apps/web/package.json", findings[1].ManifestPath)
	assert.Equal(t, "direct", findings[1].Relation)
	assert.Equal(t, "1.14.1", findings[1].Version)

	// libs/api — indirect
	assert.Equal(t, "libs/api/package.json", findings[2].ManifestPath)
	assert.Equal(t, "indirect", findings[2].Relation)
	assert.Equal(t, "0.30.4", findings[2].Version)

	// Verify SQLite was populated.
	db, err := cache.NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	status, err := db.Status()
	require.NoError(t, err)
	assert.Greater(t, status.Projects, 0)
	assert.Greater(t, status.Dependencies, 0)
}
