package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *CacheDB {
	t.Helper()
	db, err := NewCacheDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestCacheDB_CreateAndGet(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		ID:          "github.com/actions/checkout",
		Ecosystem:   "actions",
		Name:        "actions/checkout",
		LastFetched: time.Now(),
	}
	require.NoError(t, db.UpsertProject(p))

	got, err := db.GetProject(p.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, p.Ecosystem, got.Ecosystem)
	assert.Equal(t, p.Name, got.Name)
}

func TestCacheDB_ProjectTTL(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		ID:          "github.com/old/project",
		Ecosystem:   "go",
		Name:        "old/project",
		LastFetched: time.Now().Add(-25 * time.Hour), // expired
	}
	require.NoError(t, db.UpsertProject(p))

	got, err := db.GetProject(p.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "expired project should return nil")
}

func TestCacheDB_ProjectVersionImmutable(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		ID:        "github.com/actions/checkout",
		Ecosystem: "actions",
		Name:      "actions/checkout",
	}
	require.NoError(t, db.UpsertProject(p))

	v := &ProjectVersion{
		ProjectID:  p.ID,
		VersionKey: "abc123sha",
		Metadata:   `{"deps":["lodash"]}`,
	}
	require.NoError(t, db.UpsertVersion(v))

	// Even after a long time, immutable versions never expire
	got, err := db.GetVersion(p.ID, "abc123sha")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, v.VersionKey, got.VersionKey)
	assert.Equal(t, v.Metadata, got.Metadata)
}

func TestCacheDB_VersionDependencies(t *testing.T) {
	db := newTestDB(t)

	p := &Project{ID: "proj1", Ecosystem: "npm", Name: "proj1"}
	require.NoError(t, db.UpsertProject(p))
	v := &ProjectVersion{ProjectID: "proj1", VersionKey: "v1.0.0"}
	require.NoError(t, db.UpsertVersion(v))

	deps := []VersionDependency{
		{ParentProjectID: "proj1", ParentVersionKey: "v1.0.0", ChildProjectID: "lodash", ChildVersionConstraint: "^4.17.0", DepScope: "runtime"},
		{ParentProjectID: "proj1", ParentVersionKey: "v1.0.0", ChildProjectID: "express", ChildVersionConstraint: "~4.18.0", DepScope: "runtime"},
	}
	for _, d := range deps {
		require.NoError(t, db.AddVersionDependency(&d))
	}

	got, err := db.GetVersionDependencies("proj1", "v1.0.0")
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestCacheDB_FindDependents(t *testing.T) {
	db := newTestDB(t)

	// Create two projects that both depend on "lodash"
	for _, pid := range []string{"proj-a", "proj-b"} {
		p := &Project{ID: pid, Ecosystem: "npm", Name: pid}
		require.NoError(t, db.UpsertProject(p))
		v := &ProjectVersion{ProjectID: pid, VersionKey: "v1.0.0"}
		require.NoError(t, db.UpsertVersion(v))
		d := &VersionDependency{
			ParentProjectID:        pid,
			ParentVersionKey:       "v1.0.0",
			ChildProjectID:         "lodash",
			ChildVersionConstraint: "^4.0.0",
			DepScope:               "runtime",
		}
		require.NoError(t, db.AddVersionDependency(d))
	}

	dependents, err := db.FindDependents("lodash")
	require.NoError(t, err)
	assert.Len(t, dependents, 2)
	ids := []string{dependents[0].ParentProjectID, dependents[1].ParentProjectID}
	assert.Contains(t, ids, "proj-a")
	assert.Contains(t, ids, "proj-b")
}

func TestCacheDB_CVECache(t *testing.T) {
	db := newTestDB(t)

	jsonData := `[{"id":"CVE-2023-1234","severity":"high"}]`
	require.NoError(t, db.SetCVECache("npm", "lodash", "4.17.20", jsonData))

	got, err := db.GetCVECache("npm", "lodash", "4.17.20")
	require.NoError(t, err)
	assert.Equal(t, jsonData, got)

	// Miss for unknown package
	got2, err := db.GetCVECache("npm", "nonexistent", "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, "", got2)
}

func TestCacheDB_RefResolution(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.SetRefResolution("actions/checkout", "v4", "tag", "abc123def456"))

	sha, err := db.GetRefResolution("actions/checkout", "v4", "tag")
	require.NoError(t, err)
	assert.Equal(t, "abc123def456", sha)

	// branch resolution should expire faster (15 min) — but fresh should still work
	require.NoError(t, db.SetRefResolution("actions/checkout", "main", "branch", "deadbeef"))
	sha2, err := db.GetRefResolution("actions/checkout", "main", "branch")
	require.NoError(t, err)
	assert.Equal(t, "deadbeef", sha2)
}

func TestCacheDB_VersionDependencyDedup(t *testing.T) {
	db := newTestDB(t)

	p := &Project{ID: "proj-dedup", Ecosystem: "npm", Name: "proj-dedup"}
	require.NoError(t, db.UpsertProject(p))
	v := &ProjectVersion{ProjectID: "proj-dedup", VersionKey: "v1.0.0"}
	require.NoError(t, db.UpsertVersion(v))

	d := &VersionDependency{
		ParentProjectID:        "proj-dedup",
		ParentVersionKey:       "v1.0.0",
		ChildProjectID:         "lodash",
		ChildVersionConstraint: "^4.0.0",
		DepScope:               "runtime",
	}
	require.NoError(t, db.AddVersionDependency(d))
	require.NoError(t, db.AddVersionDependency(d)) // duplicate — should be ignored

	got, err := db.GetVersionDependencies("proj-dedup", "v1.0.0")
	require.NoError(t, err)
	assert.Len(t, got, 1, "duplicate dependency should be ignored")
}

func TestCacheDB_Prune(t *testing.T) {
	db := newTestDB(t)

	p := &Project{ID: "prunable", Ecosystem: "go", Name: "prunable"}
	require.NoError(t, db.UpsertProject(p))

	// Insert version with an old last_accessed time
	v := &ProjectVersion{ProjectID: "prunable", VersionKey: "old-sha"}
	require.NoError(t, db.UpsertVersion(v))

	// Manually backdate last_accessed
	_, err := db.db.Exec(`UPDATE project_versions SET last_accessed = ? WHERE project_id = ? AND version_key = ?`,
		time.Now().Add(-31*24*time.Hour), "prunable", "old-sha")
	require.NoError(t, err)

	// Insert a fresh version
	v2 := &ProjectVersion{ProjectID: "prunable", VersionKey: "new-sha"}
	require.NoError(t, db.UpsertVersion(v2))

	pruned, err := db.Prune(30 * 24 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, pruned)

	// old-sha should be gone
	got, err := db.GetVersion("prunable", "old-sha")
	require.NoError(t, err)
	assert.Nil(t, got)

	// new-sha should still be there
	got2, err := db.GetVersion("prunable", "new-sha")
	require.NoError(t, err)
	assert.NotNil(t, got2)
}

func TestCacheDB_Status(t *testing.T) {
	db := newTestDB(t)

	p := &Project{ID: "status-proj", Ecosystem: "npm", Name: "status-proj"}
	require.NoError(t, db.UpsertProject(p))

	v := &ProjectVersion{ProjectID: "status-proj", VersionKey: "v1.0.0"}
	require.NoError(t, db.UpsertVersion(v))

	d := &VersionDependency{
		ParentProjectID:        "status-proj",
		ParentVersionKey:       "v1.0.0",
		ChildProjectID:         "lodash",
		ChildVersionConstraint: "^4.0.0",
		DepScope:               "runtime",
	}
	require.NoError(t, db.AddVersionDependency(d))
	require.NoError(t, db.SetCVECache("npm", "lodash", "4.17.20", `[]`))
	require.NoError(t, db.SetRefResolution("actions/checkout", "v4", "tag", "abc123"))

	s, err := db.Status()
	require.NoError(t, err)
	assert.Equal(t, 1, s.Projects)
	assert.Equal(t, 1, s.Versions)
	assert.Equal(t, 1, s.Dependencies)
	assert.Equal(t, 1, s.CVEEntries)
	assert.Equal(t, 1, s.RefResolutions)
}
