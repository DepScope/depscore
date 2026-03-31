package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertAndGetIndexManifest(t *testing.T) {
	db := newTestDB(t)

	m := &IndexManifest{
		AbsPath:     "/home/user/project/go.mod",
		RelPath:     "go.mod",
		RootPath:    "/home/user/project",
		Ecosystem:   "go",
		Mtime:       1700000000,
		Checksum:    "sha256:abc123",
		LastIndexed: time.Now().Truncate(time.Second),
	}
	require.NoError(t, db.UpsertIndexManifest(m))
	assert.NotZero(t, m.ID, "ID should be populated after insert")

	got, err := db.GetIndexManifest("/home/user/project/go.mod")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, m.ID, got.ID)
	assert.Equal(t, m.AbsPath, got.AbsPath)
	assert.Equal(t, m.RelPath, got.RelPath)
	assert.Equal(t, m.RootPath, got.RootPath)
	assert.Equal(t, m.Ecosystem, got.Ecosystem)
	assert.Equal(t, m.Mtime, got.Mtime)
	assert.Equal(t, m.Checksum, got.Checksum)

	// Update mtime via upsert
	oldID := m.ID
	m.Mtime = 1700001000
	require.NoError(t, db.UpsertIndexManifest(m))
	assert.Equal(t, oldID, m.ID, "ID should remain the same after update")

	got2, err := db.GetIndexManifest("/home/user/project/go.mod")
	require.NoError(t, err)
	require.NotNil(t, got2)
	assert.Equal(t, int64(1700001000), got2.Mtime)
}

func TestGetIndexManifestNotFound(t *testing.T) {
	db := newTestDB(t)

	got, err := db.GetIndexManifest("/does/not/exist")
	require.NoError(t, err)
	assert.Nil(t, got, "should return nil for nonexistent manifest")
}

func TestListIndexManifests(t *testing.T) {
	db := newTestDB(t)

	manifests := []IndexManifest{
		{AbsPath: "/proj/a/go.mod", RelPath: "a/go.mod", RootPath: "/proj", Ecosystem: "go", Mtime: 1, LastIndexed: time.Now()},
		{AbsPath: "/proj/b/package.json", RelPath: "b/package.json", RootPath: "/proj", Ecosystem: "npm", Mtime: 2, LastIndexed: time.Now()},
		{AbsPath: "/other/go.mod", RelPath: "go.mod", RootPath: "/other", Ecosystem: "go", Mtime: 3, LastIndexed: time.Now()},
	}
	for i := range manifests {
		require.NoError(t, db.UpsertIndexManifest(&manifests[i]))
	}

	got, err := db.ListIndexManifests("/proj")
	require.NoError(t, err)
	assert.Len(t, got, 2, "should return only manifests under /proj")

	got2, err := db.ListIndexManifests("/other")
	require.NoError(t, err)
	assert.Len(t, got2, 1)
}

func TestDeleteIndexManifest(t *testing.T) {
	db := newTestDB(t)

	m := &IndexManifest{
		AbsPath: "/proj/go.mod", RelPath: "go.mod", RootPath: "/proj",
		Ecosystem: "go", Mtime: 1, LastIndexed: time.Now(),
	}
	require.NoError(t, db.UpsertIndexManifest(m))

	// Add a package linked to this manifest
	require.NoError(t, db.AddManifestPackage(&ManifestPackage{
		ManifestID: m.ID, ProjectID: "go/example", VersionKey: "v1.0.0",
		Constraint: "^1.0.0", DepScope: "direct",
	}))

	require.NoError(t, db.DeleteIndexManifest("/proj/go.mod"))

	got, err := db.GetIndexManifest("/proj/go.mod")
	require.NoError(t, err)
	assert.Nil(t, got, "manifest should be deleted")

	// Packages should be cascade-deleted
	pkgs, err := db.GetManifestPackages(m.ID)
	require.NoError(t, err)
	assert.Len(t, pkgs, 0, "packages should be cascade-deleted")
}

func TestManifestPackageCRUD(t *testing.T) {
	db := newTestDB(t)

	m := &IndexManifest{
		AbsPath: "/proj/go.mod", RelPath: "go.mod", RootPath: "/proj",
		Ecosystem: "go", Mtime: 1, LastIndexed: time.Now(),
	}
	require.NoError(t, db.UpsertIndexManifest(m))

	mp := &ManifestPackage{
		ManifestID: m.ID,
		ProjectID:  "go/example",
		VersionKey: "v1.2.3",
		Constraint: ">=1.2.0",
		DepScope:   "direct",
	}
	require.NoError(t, db.AddManifestPackage(mp))

	// Duplicate insert should be ignored (INSERT OR IGNORE)
	require.NoError(t, db.AddManifestPackage(mp))

	pkgs, err := db.GetManifestPackages(m.ID)
	require.NoError(t, err)
	assert.Len(t, pkgs, 1, "duplicate should be ignored")
	assert.Equal(t, "go/example", pkgs[0].ProjectID)
	assert.Equal(t, "v1.2.3", pkgs[0].VersionKey)
	assert.Equal(t, ">=1.2.0", pkgs[0].Constraint)
	assert.Equal(t, "direct", pkgs[0].DepScope)

	// Add second package
	mp2 := &ManifestPackage{
		ManifestID: m.ID, ProjectID: "go/other", VersionKey: "v2.0.0",
		Constraint: "^2.0.0", DepScope: "transitive",
	}
	require.NoError(t, db.AddManifestPackage(mp2))

	pkgs2, err := db.GetManifestPackages(m.ID)
	require.NoError(t, err)
	assert.Len(t, pkgs2, 2)

	// Reverse lookup
	found, err := db.FindPackageManifests("go/example")
	require.NoError(t, err)
	assert.Len(t, found, 1)
	assert.Equal(t, m.ID, found[0].ManifestID)

	// Clear
	require.NoError(t, db.ClearManifestPackages(m.ID))
	pkgs3, err := db.GetManifestPackages(m.ID)
	require.NoError(t, err)
	assert.Len(t, pkgs3, 0, "all packages should be cleared")
}

func TestPruneDeletedManifests(t *testing.T) {
	db := newTestDB(t)

	// Create three manifests: keep two, prune one
	for _, abs := range []string{"/proj/a/go.mod", "/proj/b/go.mod", "/proj/c/go.mod"} {
		m := &IndexManifest{
			AbsPath: abs, RelPath: abs[len("/proj/"):], RootPath: "/proj",
			Ecosystem: "go", Mtime: 1, LastIndexed: time.Now(),
		}
		require.NoError(t, db.UpsertIndexManifest(m))
	}

	currentPaths := map[string]bool{
		"/proj/a/go.mod": true,
		"/proj/b/go.mod": true,
		// /proj/c/go.mod is missing = stale
	}

	pruned, err := db.PruneDeletedManifests("/proj", currentPaths)
	require.NoError(t, err)
	assert.Equal(t, 1, pruned)

	remaining, err := db.ListIndexManifests("/proj")
	require.NoError(t, err)
	assert.Len(t, remaining, 2)

	gone, err := db.GetIndexManifest("/proj/c/go.mod")
	require.NoError(t, err)
	assert.Nil(t, gone, "stale manifest should be pruned")
}

func TestIndexRunLifecycle(t *testing.T) {
	db := newTestDB(t)

	run, err := db.StartIndexRun("/proj", "full")
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.NotZero(t, run.ID)
	assert.Equal(t, "/proj", run.RootPath)
	assert.Equal(t, "full", run.Scope)
	assert.False(t, run.StartedAt.IsZero())

	// Update stats and finish
	run.ManifestsFound = 10
	run.ManifestsUpdated = 3
	run.ManifestsSkipped = 7
	run.PackagesTotal = 50
	run.PackagesNew = 12
	run.Errors = 1
	require.NoError(t, db.FinishIndexRun(run))
	assert.False(t, run.FinishedAt.IsZero())

	// Get last run
	last, err := db.GetLastIndexRun("/proj")
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.Equal(t, run.ID, last.ID)
	assert.Equal(t, 10, last.ManifestsFound)
	assert.Equal(t, 3, last.ManifestsUpdated)
	assert.Equal(t, 50, last.PackagesTotal)
	assert.Equal(t, 1, last.Errors)

	// Get runs list
	runs, err := db.GetIndexRuns("/proj", 10)
	require.NoError(t, err)
	assert.Len(t, runs, 1)

	// No run for different root
	noRun, err := db.GetLastIndexRun("/other")
	require.NoError(t, err)
	assert.Nil(t, noRun)
}

func TestIndexStats(t *testing.T) {
	db := newTestDB(t)

	// Create manifests in two ecosystems
	m1 := &IndexManifest{AbsPath: "/proj/go.mod", RelPath: "go.mod", RootPath: "/proj", Ecosystem: "go", Mtime: 1, LastIndexed: time.Now()}
	m2 := &IndexManifest{AbsPath: "/proj/package.json", RelPath: "package.json", RootPath: "/proj", Ecosystem: "npm", Mtime: 2, LastIndexed: time.Now()}
	m3 := &IndexManifest{AbsPath: "/proj/sub/go.mod", RelPath: "sub/go.mod", RootPath: "/proj", Ecosystem: "go", Mtime: 3, LastIndexed: time.Now()}
	require.NoError(t, db.UpsertIndexManifest(m1))
	require.NoError(t, db.UpsertIndexManifest(m2))
	require.NoError(t, db.UpsertIndexManifest(m3))

	// Add packages: "go/shared" appears in 2 manifests, others appear once
	for _, mp := range []ManifestPackage{
		{ManifestID: m1.ID, ProjectID: "go/shared", VersionKey: "v1.0.0", DepScope: "direct"},
		{ManifestID: m1.ID, ProjectID: "go/onlyhere", VersionKey: "v2.0.0", DepScope: "direct"},
		{ManifestID: m2.ID, ProjectID: "npm/express", VersionKey: "4.18.0", DepScope: "direct"},
		{ManifestID: m3.ID, ProjectID: "go/shared", VersionKey: "v1.0.0", DepScope: "direct"},
	} {
		require.NoError(t, db.AddManifestPackage(&mp))
	}

	// Start and finish a run so LastRun is populated
	run, err := db.StartIndexRun("/proj", "full")
	require.NoError(t, err)
	run.ManifestsFound = 3
	require.NoError(t, db.FinishIndexRun(run))

	stats, err := db.IndexStats("/proj")
	require.NoError(t, err)
	require.NotNil(t, stats)

	assert.Equal(t, 3, stats.ManifestCount)
	assert.Equal(t, 3, stats.PackageCount, "3 unique project_ids")
	assert.Equal(t, 2, stats.EcosystemCounts["go"])
	assert.Equal(t, 1, stats.EcosystemCounts["npm"])

	// Top packages: go/shared should be first (count=2)
	require.NotEmpty(t, stats.TopPackages)
	assert.Equal(t, "go/shared", stats.TopPackages[0].ProjectID)
	assert.Equal(t, 2, stats.TopPackages[0].Count)

	assert.NotNil(t, stats.LastRun)
	assert.Equal(t, run.ID, stats.LastRun.ID)
}
