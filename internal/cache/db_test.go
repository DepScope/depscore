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

// ---------------------------------------------------------------------------
// TTL Boundary Tests (Gap 4 & Gap 8)
// ---------------------------------------------------------------------------

func TestCacheDB_ProjectTTL_NotExpired(t *testing.T) {
	db := newTestDB(t)

	p := &Project{
		ID:          "github.com/recent/project",
		Ecosystem:   "go",
		Name:        "recent/project",
		LastFetched: time.Now().Add(-23 * time.Hour), // 23h ago — within 24h TTL
	}
	require.NoError(t, db.UpsertProject(p))

	got, err := db.GetProject(p.ID)
	require.NoError(t, err)
	require.NotNil(t, got, "project fetched 23h ago should still be valid (24h TTL)")
	assert.Equal(t, p.ID, got.ID)
}

func TestCacheDB_RefResolution_BranchTTL(t *testing.T) {
	db := newTestDB(t)

	// Branch ref resolved 14 min ago (within 15min TTL) — should be valid.
	require.NoError(t, db.SetRefResolution("owner/repo", "main", "branch", "sha-valid"))
	// Backdate to 14 minutes ago.
	_, err := db.db.Exec(
		`UPDATE ref_resolutions SET resolved_at = ? WHERE owner_repo = ? AND ref = ? AND ref_type = ?`,
		time.Now().Add(-14*time.Minute), "owner/repo", "main", "branch",
	)
	require.NoError(t, err)

	sha, err := db.GetRefResolution("owner/repo", "main", "branch")
	require.NoError(t, err)
	assert.Equal(t, "sha-valid", sha, "branch ref resolved 14min ago should still be valid (15min TTL)")

	// Branch ref resolved 16 min ago (beyond 15min TTL) — should be expired.
	require.NoError(t, db.SetRefResolution("owner/repo2", "develop", "branch", "sha-expired"))
	_, err = db.db.Exec(
		`UPDATE ref_resolutions SET resolved_at = ? WHERE owner_repo = ? AND ref = ? AND ref_type = ?`,
		time.Now().Add(-16*time.Minute), "owner/repo2", "develop", "branch",
	)
	require.NoError(t, err)

	sha2, err := db.GetRefResolution("owner/repo2", "develop", "branch")
	require.NoError(t, err)
	assert.Equal(t, "", sha2, "branch ref resolved 16min ago should be expired (15min TTL)")
}

func TestCacheDB_RefResolution_TagVsBranch(t *testing.T) {
	db := newTestDB(t)

	// Tag resolved 30 min ago — should be valid (1h TTL).
	require.NoError(t, db.SetRefResolution("owner/repo", "v4", "tag", "sha-tag"))
	_, err := db.db.Exec(
		`UPDATE ref_resolutions SET resolved_at = ? WHERE owner_repo = ? AND ref = ? AND ref_type = ?`,
		time.Now().Add(-30*time.Minute), "owner/repo", "v4", "tag",
	)
	require.NoError(t, err)

	sha, err := db.GetRefResolution("owner/repo", "v4", "tag")
	require.NoError(t, err)
	assert.Equal(t, "sha-tag", sha, "tag resolved 30min ago should be valid (1h TTL)")

	// Branch resolved 30 min ago — should be expired (15min TTL).
	require.NoError(t, db.SetRefResolution("owner/repo", "main", "branch", "sha-branch"))
	_, err = db.db.Exec(
		`UPDATE ref_resolutions SET resolved_at = ? WHERE owner_repo = ? AND ref = ? AND ref_type = ?`,
		time.Now().Add(-30*time.Minute), "owner/repo", "main", "branch",
	)
	require.NoError(t, err)

	sha2, err := db.GetRefResolution("owner/repo", "main", "branch")
	require.NoError(t, err)
	assert.Equal(t, "", sha2, "branch resolved 30min ago should be expired (15min TTL)")
}

func TestCacheDB_CVECache_Expired(t *testing.T) {
	db := newTestDB(t)

	jsonData := `[{"id":"CVE-2024-9999","severity":"HIGH"}]`
	require.NoError(t, db.SetCVECache("npm", "lodash", "4.17.20", jsonData))

	// Backdate fetched_at to 7 hours ago (beyond 6h TTL).
	_, err := db.db.Exec(
		`UPDATE cve_cache SET fetched_at = ? WHERE ecosystem = ? AND name = ? AND version = ?`,
		time.Now().Add(-7*time.Hour), "npm", "lodash", "4.17.20",
	)
	require.NoError(t, err)

	got, err := db.GetCVECache("npm", "lodash", "4.17.20")
	require.NoError(t, err)
	assert.Equal(t, "", got, "CVE cache entry from 7h ago should be expired (6h TTL)")
}

func TestCacheDB_RefResolution_TagTTL(t *testing.T) {
	db := newTestDB(t)

	// Tag resolved 59 min ago — should be valid (1h TTL).
	require.NoError(t, db.SetRefResolution("actions/checkout", "v4", "tag", "sha-59min"))
	_, err := db.db.Exec(
		`UPDATE ref_resolutions SET resolved_at = ? WHERE owner_repo = ? AND ref = ? AND ref_type = ?`,
		time.Now().Add(-59*time.Minute), "actions/checkout", "v4", "tag",
	)
	require.NoError(t, err)

	sha, err := db.GetRefResolution("actions/checkout", "v4", "tag")
	require.NoError(t, err)
	assert.Equal(t, "sha-59min", sha, "tag resolved 59min ago should still be valid (1h TTL)")
}

func TestCacheDB_RefResolution_BranchExpired(t *testing.T) {
	db := newTestDB(t)

	// Branch resolved 20 min ago — should be expired (15min TTL).
	require.NoError(t, db.SetRefResolution("actions/checkout", "main", "branch", "sha-20min"))
	_, err := db.db.Exec(
		`UPDATE ref_resolutions SET resolved_at = ? WHERE owner_repo = ? AND ref = ? AND ref_type = ?`,
		time.Now().Add(-20*time.Minute), "actions/checkout", "main", "branch",
	)
	require.NoError(t, err)

	sha, err := db.GetRefResolution("actions/checkout", "main", "branch")
	require.NoError(t, err)
	assert.Equal(t, "", sha, "branch resolved 20min ago should be expired (15min TTL)")
}

// TestCacheDB_PruneWithDependencies verifies that Prune cascades correctly:
// version_dependencies rows for pruned versions are deleted before the
// project_versions rows.
func TestCacheDB_PruneWithDependencies(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.UpsertProject(&Project{ID: "npm/old", Ecosystem: "npm", Name: "old"}))
	require.NoError(t, db.UpsertProject(&Project{ID: "npm/child", Ecosystem: "npm", Name: "child"}))

	oldPV := &ProjectVersion{ProjectID: "npm/old", VersionKey: "npm/old@1.0.0"}
	require.NoError(t, db.UpsertVersion(oldPV))
	require.NoError(t, db.UpsertVersion(&ProjectVersion{ProjectID: "npm/child", VersionKey: "npm/child@1.0.0"}))

	require.NoError(t, db.AddVersionDependency(&VersionDependency{
		ParentProjectID:        "npm/old",
		ParentVersionKey:       "npm/old@1.0.0",
		ChildProjectID:         "npm/child",
		ChildVersionConstraint: "npm/child@1.0.0",
		DepScope:               "depends_on",
	}))

	// Backdate both timestamps on the old version to trigger pruning.
	_, err := db.db.Exec(
		`UPDATE project_versions SET scanned_at = datetime('now', '-100 days'), last_accessed = datetime('now', '-100 days') WHERE version_key = 'npm/old@1.0.0'`,
	)
	// scanned_at column may not exist; fall back to just last_accessed.
	if err != nil {
		_, err = db.db.Exec(
			`UPDATE project_versions SET last_accessed = ? WHERE project_id = ? AND version_key = ?`,
			time.Now().Add(-100*24*time.Hour), "npm/old", "npm/old@1.0.0",
		)
		require.NoError(t, err)
	}

	pruned, err := db.Prune(90 * 24 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, pruned)

	// Dependency edge for the pruned version should also be gone.
	deps, err := db.GetVersionDependencies("npm/old", "npm/old@1.0.0")
	require.NoError(t, err)
	assert.Len(t, deps, 0, "version_dependencies for pruned version should be deleted")

	// The child version itself should still exist.
	child, err := db.GetVersion("npm/child", "npm/child@1.0.0")
	require.NoError(t, err)
	assert.NotNil(t, child)
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
