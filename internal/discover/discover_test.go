// internal/discover/discover_test.go
package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/depscope/depscope/internal/cache"
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

func TestDiscoverFromCache(t *testing.T) {
	// Set up a temp CacheDB.
	dbPath := filepath.Join(t.TempDir(), "test-discover-cache.db")
	db, err := cache.NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Pre-populate: project A depends on lodash@4.17.20
	require.NoError(t, db.UpsertProject(&cache.Project{
		ID: "project-a", Ecosystem: "npm", Name: "project-a",
	}))
	require.NoError(t, db.UpsertVersion(&cache.ProjectVersion{
		ProjectID: "project-a", VersionKey: "project-a@1.0.0",
	}))
	require.NoError(t, db.AddVersionDependency(&cache.VersionDependency{
		ParentProjectID:        "project-a",
		ParentVersionKey:       "project-a@1.0.0",
		ChildProjectID:         "npm/lodash",
		ChildVersionConstraint: "npm/lodash@4.17.20",
		DepScope:               "depends_on",
	}))

	// Pre-populate: project B depends on lodash@4.17.21
	require.NoError(t, db.UpsertProject(&cache.Project{
		ID: "project-b", Ecosystem: "npm", Name: "project-b",
	}))
	require.NoError(t, db.UpsertVersion(&cache.ProjectVersion{
		ProjectID: "project-b", VersionKey: "project-b@2.0.0",
	}))
	require.NoError(t, db.AddVersionDependency(&cache.VersionDependency{
		ParentProjectID:        "project-b",
		ParentVersionKey:       "project-b@2.0.0",
		ChildProjectID:         "npm/lodash",
		ChildVersionConstraint: "npm/lodash@4.17.21",
		DepScope:               "depends_on",
	}))

	// Query with range "<4.17.21" — only project A should match.
	results, err := DiscoverFromCache(db, "npm/lodash", "<4.17.21")
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "project-a", results[0].ProjectID)
	assert.Equal(t, "project-a@1.0.0", results[0].VersionKey)
	assert.Equal(t, "4.17.20", results[0].ChildVersion)
	assert.Equal(t, "depends_on", results[0].EdgeType)
}

func TestDiscoverFromCache_InvalidRange(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test-discover-cache-err.db")
	db, err := cache.NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	_, err = DiscoverFromCache(db, "npm/lodash", "")
	assert.Error(t, err)
}

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"npm/lodash@4.17.20", "4.17.20"},
		{"4.17.20", "4.17.20"},
		{"Go/github.com/foo/bar@v1.2.3", "v1.2.3"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, extractVersion(tt.input))
		})
	}
}
