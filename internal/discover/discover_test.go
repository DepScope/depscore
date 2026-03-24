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
