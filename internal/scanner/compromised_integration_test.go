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

func TestScanCompromisedIntegration(t *testing.T) {
	root := t.TempDir()

	// Project A: direct axios@1.14.1 via ^1.14.0 constraint.
	mkManifest(t, root, "project-a", `{
		"name": "project-a",
		"dependencies": { "axios": "^1.14.0", "lodash": "^4.17.21" }
	}`, `{
		"lockfileVersion": 3,
		"packages": {
			"node_modules/axios": { "version": "1.14.1" },
			"node_modules/lodash": { "version": "4.17.21" }
		}
	}`)

	// Project B (hidden dir): indirect axios@0.30.4.
	mkManifest(t, root, ".tools/projb", `{
		"name": "projb",
		"dependencies": { "my-sdk": "^1.0.0" }
	}`, `{
		"lockfileVersion": 3,
		"packages": {
			"node_modules/my-sdk": { "version": "1.2.3" },
			"node_modules/axios": { "version": "0.30.4" }
		}
	}`)

	// Project C: no axios at all (should produce no findings).
	mkManifest(t, root, "project-c", `{
		"name": "project-c",
		"dependencies": { "express": "^4.18.0" }
	}`, `{
		"lockfileVersion": 3,
		"packages": {
			"node_modules/express": { "version": "4.18.2" }
		}
	}`)

	// Project D: pnpm lockfile with axios@0.30.4.
	mkPnpmManifest(t, root, "project-d", `{
		"name": "project-d",
		"dependencies": { "axios": "~0.30.0" }
	}`, "packages:\n  'axios@0.30.4':\n    resolution: {integrity: sha512-abc}\n")

	// Use a compromised file.
	compFile := filepath.Join(t.TempDir(), "bad.txt")
	require.NoError(t, os.WriteFile(compFile, []byte("axios@1.14.1\naxios@0.30.4\n"), 0644))

	targets, err := ParseCompromisedFile(compFile)
	require.NoError(t, err)

	dbPath := filepath.Join(t.TempDir(), "integration.db")
	var buf strings.Builder
	findings, err := ScanCompromised(context.Background(), root, targets, dbPath, &buf)
	require.NoError(t, err)

	// Expect 3 findings: project-a (direct), .tools/projb (indirect), project-d (direct).
	assert.Len(t, findings, 3)

	output := buf.String()
	assert.Contains(t, output, "DIRECT")
	assert.Contains(t, output, "INDIRECT")
	assert.Contains(t, output, "project-a/package.json")
	assert.Contains(t, output, ".tools/projb/package.json")
	assert.Contains(t, output, "project-d/package.json")
	// project-c should NOT appear
	assert.NotContains(t, output, "project-c")

	// Verify DB has full graph.
	db, err := cache.NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	status, err := db.Status()
	require.NoError(t, err)
	assert.Greater(t, status.Projects, 3)
	assert.Greater(t, status.Dependencies, 3)
}

func mkManifest(t *testing.T, root, subdir, pkgJSON, lockJSON string) {
	t.Helper()
	dir := filepath.Join(root, subdir)
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(lockJSON), 0644))
}

func mkPnpmManifest(t *testing.T, root, subdir, pkgJSON, pnpmLock string) {
	t.Helper()
	dir := filepath.Join(root, subdir)
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte(pnpmLock), 0644))
}
