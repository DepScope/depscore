# Supply Chain Indexer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `depscope index <path>` that walks any directory tree, discovers all package manifests across every ecosystem, parses dependencies, deduplicates globally in SQLite, and supports incremental re-indexing.

**Architecture:** A filesystem walker discovers manifests (including inside node_modules, hidden dirs). Each manifest is parsed with ecosystem-specific parsers. Packages are globally deduped in SQLite — one row per unique package@version, linked to manifests via a junction table. Incremental mode uses mtime comparison to skip unchanged files. Three scope levels: local (disk only), deps (+ transitive resolution), supply-chain (+ network APIs).

**Tech Stack:** Go, SQLite (modernc.org/sqlite), existing manifest parsers, cobra CLI, crypto/sha256 for checksums.

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/cache/index.go` | `IndexManifest`, `ManifestPackage`, `IndexRun`, `IndexStatus` types + schema migration + all CRUD methods |
| `internal/cache/index_test.go` | Unit tests for index DB operations: CRUD, dedup, cascade, prune, stats |
| `internal/scanner/indexer.go` | Core indexer: walk filesystem, discover manifests, incremental mtime check, parse via ecosystem parsers, dedup + store, node_modules handling, progress output |
| `internal/scanner/indexer_test.go` | Unit + integration tests for indexer with multi-ecosystem fixtures |
| `cmd/depscope/index_cmd.go` | CLI: `depscope index <path>` with `--force`, `--scope`, `--db`; `depscope index status` subcommand |

---

### Task 1: Index SQLite Schema & Types

**Files:**
- Create: `internal/cache/index.go`
- Create: `internal/cache/index_test.go`
- Modify: `internal/cache/db.go` (add migration call)

- [ ] **Step 1: Write failing tests for IndexManifest CRUD**

```go
// internal/cache/index_test.go
package cache

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertAndGetIndexManifest(t *testing.T) {
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	m := &IndexManifest{
		AbsPath:     "/Users/jj/src/app/package.json",
		RelPath:     "src/app/package.json",
		RootPath:    "/Users/jj",
		Ecosystem:   "npm",
		Mtime:       1711900000,
		Checksum:    "abc123",
		LastIndexed: time.Now(),
	}
	err := db.UpsertIndexManifest(m)
	require.NoError(t, err)
	assert.NotZero(t, m.ID) // ID populated after insert

	got, err := db.GetIndexManifest(m.AbsPath)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, m.AbsPath, got.AbsPath)
	assert.Equal(t, "npm", got.Ecosystem)
	assert.Equal(t, int64(1711900000), got.Mtime)

	// Update mtime.
	m.Mtime = 1711900999
	require.NoError(t, db.UpsertIndexManifest(m))
	got, err = db.GetIndexManifest(m.AbsPath)
	require.NoError(t, err)
	assert.Equal(t, int64(1711900999), got.Mtime)
}

func TestGetIndexManifestNotFound(t *testing.T) {
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	got, err := db.GetIndexManifest("/does/not/exist")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestListIndexManifests(t *testing.T) {
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	for _, p := range []string{"/root/a/package.json", "/root/b/go.mod", "/other/c/Cargo.toml"} {
		require.NoError(t, db.UpsertIndexManifest(&IndexManifest{
			AbsPath:  p,
			RelPath:  p,
			RootPath: filepath.Dir(filepath.Dir(p)),
			Ecosystem: "test",
			Mtime:    1,
			LastIndexed: time.Now(),
		}))
	}

	list, err := db.ListIndexManifests("/root")
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestDeleteIndexManifest(t *testing.T) {
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	require.NoError(t, db.UpsertIndexManifest(&IndexManifest{
		AbsPath: "/a/package.json", RelPath: "package.json", RootPath: "/a",
		Ecosystem: "npm", Mtime: 1, LastIndexed: time.Now(),
	}))

	require.NoError(t, db.DeleteIndexManifest("/a/package.json"))
	got, err := db.GetIndexManifest("/a/package.json")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func newTestDB(t *testing.T) *CacheDB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := NewCacheDB(dbPath)
	require.NoError(t, err)
	return db
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cache/ -run TestUpsertAndGetIndexManifest -v`
Expected: FAIL — `IndexManifest` undefined.

- [ ] **Step 3: Implement types and IndexManifest CRUD**

```go
// internal/cache/index.go
package cache

import (
	"database/sql"
	"time"
)

// IndexManifest represents a discovered manifest file on disk.
type IndexManifest struct {
	ID          int64
	AbsPath     string
	RelPath     string
	RootPath    string
	Ecosystem   string
	Mtime       int64
	Checksum    string
	LastIndexed time.Time
}

// ManifestPackage links a manifest to a package it contains.
type ManifestPackage struct {
	ManifestID int64
	ProjectID  string
	VersionKey string
	Constraint string
	DepScope   string // "direct", "dev", "transitive"
}

// IndexRun tracks a single indexing run.
type IndexRun struct {
	ID               int64
	RootPath         string
	Scope            string
	StartedAt        time.Time
	FinishedAt       time.Time
	ManifestsFound   int
	ManifestsUpdated int
	ManifestsSkipped int
	PackagesTotal    int
	PackagesNew      int
	Errors           int
}

// IndexStatus holds aggregate statistics for the index.
type IndexStatus struct {
	ManifestCount   int
	PackageCount    int
	EcosystemCounts map[string]int
	TopPackages     []PackageFrequency
	LastRun         *IndexRun
}

// PackageFrequency tracks how many manifests reference a package.
type PackageFrequency struct {
	ProjectID string
	Name      string
	Count     int
}

// migrateIndex adds index-specific tables. Called from CacheDB.migrate().
func (c *CacheDB) migrateIndex() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS index_manifests (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		abs_path     TEXT UNIQUE NOT NULL,
		rel_path     TEXT NOT NULL,
		root_path    TEXT NOT NULL,
		ecosystem    TEXT NOT NULL,
		mtime        INTEGER NOT NULL,
		checksum     TEXT NOT NULL DEFAULT '',
		last_indexed DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_manifests_root ON index_manifests(root_path);
	CREATE INDEX IF NOT EXISTS idx_manifests_eco ON index_manifests(ecosystem);

	CREATE TABLE IF NOT EXISTS manifest_packages (
		manifest_id  INTEGER NOT NULL,
		project_id   TEXT NOT NULL,
		version_key  TEXT NOT NULL,
		constraint_  TEXT NOT NULL DEFAULT '',
		dep_scope    TEXT NOT NULL DEFAULT 'direct',
		UNIQUE(manifest_id, project_id, version_key),
		FOREIGN KEY (manifest_id) REFERENCES index_manifests(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_mp_project ON manifest_packages(project_id);
	CREATE INDEX IF NOT EXISTS idx_mp_version ON manifest_packages(version_key);

	CREATE TABLE IF NOT EXISTS index_runs (
		id                INTEGER PRIMARY KEY AUTOINCREMENT,
		root_path         TEXT NOT NULL,
		scope             TEXT NOT NULL,
		started_at        DATETIME NOT NULL,
		finished_at       DATETIME,
		manifests_found   INTEGER NOT NULL DEFAULT 0,
		manifests_updated INTEGER NOT NULL DEFAULT 0,
		manifests_skipped INTEGER NOT NULL DEFAULT 0,
		packages_total    INTEGER NOT NULL DEFAULT 0,
		packages_new      INTEGER NOT NULL DEFAULT 0,
		errors            INTEGER NOT NULL DEFAULT 0
	);
	`
	_, err := c.db.Exec(schema)
	return err
}

// ---------------------------------------------------------------------------
// Index Manifests
// ---------------------------------------------------------------------------

// UpsertIndexManifest inserts or updates a manifest. On insert, m.ID is populated.
func (c *CacheDB) UpsertIndexManifest(m *IndexManifest) error {
	if m.LastIndexed.IsZero() {
		m.LastIndexed = time.Now()
	}
	res, err := c.db.Exec(
		`INSERT INTO index_manifests (abs_path, rel_path, root_path, ecosystem, mtime, checksum, last_indexed)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(abs_path) DO UPDATE SET
		   rel_path = excluded.rel_path,
		   root_path = excluded.root_path,
		   ecosystem = excluded.ecosystem,
		   mtime = excluded.mtime,
		   checksum = excluded.checksum,
		   last_indexed = excluded.last_indexed`,
		m.AbsPath, m.RelPath, m.RootPath, m.Ecosystem, m.Mtime, m.Checksum, m.LastIndexed,
	)
	if err != nil {
		return err
	}
	if m.ID == 0 {
		id, _ := res.LastInsertId()
		m.ID = id
	}
	return nil
}

// GetIndexManifest retrieves a manifest by absolute path. Returns nil if not found.
func (c *CacheDB) GetIndexManifest(absPath string) (*IndexManifest, error) {
	m := &IndexManifest{}
	err := c.db.QueryRow(
		`SELECT id, abs_path, rel_path, root_path, ecosystem, mtime, checksum, last_indexed
		 FROM index_manifests WHERE abs_path = ?`, absPath,
	).Scan(&m.ID, &m.AbsPath, &m.RelPath, &m.RootPath, &m.Ecosystem, &m.Mtime, &m.Checksum, &m.LastIndexed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ListIndexManifests returns all manifests under a root path.
func (c *CacheDB) ListIndexManifests(rootPath string) ([]IndexManifest, error) {
	rows, err := c.db.Query(
		`SELECT id, abs_path, rel_path, root_path, ecosystem, mtime, checksum, last_indexed
		 FROM index_manifests WHERE root_path = ?`, rootPath,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var manifests []IndexManifest
	for rows.Next() {
		var m IndexManifest
		if err := rows.Scan(&m.ID, &m.AbsPath, &m.RelPath, &m.RootPath, &m.Ecosystem, &m.Mtime, &m.Checksum, &m.LastIndexed); err != nil {
			return nil, err
		}
		manifests = append(manifests, m)
	}
	return manifests, rows.Err()
}

// DeleteIndexManifest removes a manifest and its package links (via CASCADE).
func (c *CacheDB) DeleteIndexManifest(absPath string) error {
	_, err := c.db.Exec(`DELETE FROM index_manifests WHERE abs_path = ?`, absPath)
	return err
}
```

- [ ] **Step 4: Wire migrateIndex into CacheDB.migrate()**

In `internal/cache/db.go`, at the end of the existing `migrate()` function (after `c.db.Exec(schema)` succeeds, before `return err`), add:

```go
	if err != nil {
		return err
	}
	return c.migrateIndex()
```

Replace the current `return err` at the end of `migrate()`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cache/ -run "TestUpsert|TestGetIndex|TestList|TestDelete" -v`
Expected: PASS

- [ ] **Step 6: Write failing tests for ManifestPackage and PruneDeletedManifests**

```go
// Append to internal/cache/index_test.go

func TestManifestPackageCRUD(t *testing.T) {
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	// Create a manifest.
	m := &IndexManifest{
		AbsPath: "/a/package.json", RelPath: "package.json", RootPath: "/a",
		Ecosystem: "npm", Mtime: 1, LastIndexed: time.Now(),
	}
	require.NoError(t, db.UpsertIndexManifest(m))

	// Add packages.
	require.NoError(t, db.AddManifestPackage(&ManifestPackage{
		ManifestID: m.ID, ProjectID: "npm/axios", VersionKey: "npm/axios@1.14.1",
		Constraint: "^1.14.0", DepScope: "direct",
	}))
	require.NoError(t, db.AddManifestPackage(&ManifestPackage{
		ManifestID: m.ID, ProjectID: "npm/lodash", VersionKey: "npm/lodash@4.17.21",
		Constraint: "^4.17.0", DepScope: "direct",
	}))

	// Duplicate is ignored.
	require.NoError(t, db.AddManifestPackage(&ManifestPackage{
		ManifestID: m.ID, ProjectID: "npm/axios", VersionKey: "npm/axios@1.14.1",
		Constraint: "^1.14.0", DepScope: "direct",
	}))

	pkgs, err := db.GetManifestPackages(m.ID)
	require.NoError(t, err)
	assert.Len(t, pkgs, 2)

	// Reverse lookup.
	manifests, err := db.FindPackageManifests("npm/axios")
	require.NoError(t, err)
	assert.Len(t, manifests, 1)
	assert.Equal(t, m.ID, manifests[0].ManifestID)

	// Clear and re-populate.
	require.NoError(t, db.ClearManifestPackages(m.ID))
	pkgs, err = db.GetManifestPackages(m.ID)
	require.NoError(t, err)
	assert.Len(t, pkgs, 0)
}

func TestPruneDeletedManifests(t *testing.T) {
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	for _, p := range []string{"/root/a/package.json", "/root/b/go.mod", "/root/c/Cargo.toml"} {
		require.NoError(t, db.UpsertIndexManifest(&IndexManifest{
			AbsPath: p, RelPath: p, RootPath: "/root",
			Ecosystem: "test", Mtime: 1, LastIndexed: time.Now(),
		}))
	}

	// Only /root/a and /root/c still exist on disk.
	current := map[string]bool{
		"/root/a/package.json": true,
		"/root/c/Cargo.toml":   true,
	}
	pruned, err := db.PruneDeletedManifests("/root", current)
	require.NoError(t, err)
	assert.Equal(t, 1, pruned) // /root/b/go.mod removed

	list, err := db.ListIndexManifests("/root")
	require.NoError(t, err)
	assert.Len(t, list, 2)
}
```

- [ ] **Step 7: Implement ManifestPackage methods and PruneDeletedManifests**

```go
// Append to internal/cache/index.go

// ---------------------------------------------------------------------------
// Manifest Packages
// ---------------------------------------------------------------------------

// AddManifestPackage links a package to a manifest. Duplicates are silently ignored.
func (c *CacheDB) AddManifestPackage(mp *ManifestPackage) error {
	_, err := c.db.Exec(
		`INSERT OR IGNORE INTO manifest_packages (manifest_id, project_id, version_key, constraint_, dep_scope)
		 VALUES (?, ?, ?, ?, ?)`,
		mp.ManifestID, mp.ProjectID, mp.VersionKey, mp.Constraint, mp.DepScope,
	)
	return err
}

// GetManifestPackages returns all packages linked to a manifest.
func (c *CacheDB) GetManifestPackages(manifestID int64) ([]ManifestPackage, error) {
	rows, err := c.db.Query(
		`SELECT manifest_id, project_id, version_key, constraint_, dep_scope
		 FROM manifest_packages WHERE manifest_id = ?`, manifestID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var pkgs []ManifestPackage
	for rows.Next() {
		var mp ManifestPackage
		if err := rows.Scan(&mp.ManifestID, &mp.ProjectID, &mp.VersionKey, &mp.Constraint, &mp.DepScope); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, mp)
	}
	return pkgs, rows.Err()
}

// FindPackageManifests returns all manifest links for a given project ID (reverse lookup).
func (c *CacheDB) FindPackageManifests(projectID string) ([]ManifestPackage, error) {
	rows, err := c.db.Query(
		`SELECT manifest_id, project_id, version_key, constraint_, dep_scope
		 FROM manifest_packages WHERE project_id = ?`, projectID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var pkgs []ManifestPackage
	for rows.Next() {
		var mp ManifestPackage
		if err := rows.Scan(&mp.ManifestID, &mp.ProjectID, &mp.VersionKey, &mp.Constraint, &mp.DepScope); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, mp)
	}
	return pkgs, rows.Err()
}

// ClearManifestPackages removes all package links for a manifest (before re-indexing).
func (c *CacheDB) ClearManifestPackages(manifestID int64) error {
	_, err := c.db.Exec(`DELETE FROM manifest_packages WHERE manifest_id = ?`, manifestID)
	return err
}

// PruneDeletedManifests removes manifests under rootPath that are NOT in currentPaths.
// Returns the number of manifests pruned.
func (c *CacheDB) PruneDeletedManifests(rootPath string, currentPaths map[string]bool) (int, error) {
	manifests, err := c.ListIndexManifests(rootPath)
	if err != nil {
		return 0, err
	}
	pruned := 0
	for _, m := range manifests {
		if !currentPaths[m.AbsPath] {
			if err := c.DeleteIndexManifest(m.AbsPath); err != nil {
				return pruned, err
			}
			pruned++
		}
	}
	return pruned, nil
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/cache/ -run "TestManifest|TestPrune" -v`
Expected: PASS

- [ ] **Step 9: Write failing tests for IndexRun and IndexStats**

```go
// Append to internal/cache/index_test.go

func TestIndexRunLifecycle(t *testing.T) {
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	run, err := db.StartIndexRun("/Users/jj", "local")
	require.NoError(t, err)
	assert.NotZero(t, run.ID)
	assert.Equal(t, "/Users/jj", run.RootPath)
	assert.Equal(t, "local", run.Scope)

	run.ManifestsFound = 100
	run.ManifestsUpdated = 20
	run.ManifestsSkipped = 80
	run.PackagesTotal = 500
	run.PackagesNew = 50
	run.Errors = 2
	require.NoError(t, db.FinishIndexRun(run))

	last, err := db.GetLastIndexRun("/Users/jj")
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.Equal(t, 100, last.ManifestsFound)
	assert.Equal(t, 500, last.PackagesTotal)
	assert.False(t, last.FinishedAt.IsZero())
}

func TestIndexStats(t *testing.T) {
	db := newTestDB(t)
	defer func() { _ = db.Close() }()

	// Create two manifests with packages.
	m1 := &IndexManifest{
		AbsPath: "/r/a/package.json", RelPath: "a/package.json", RootPath: "/r",
		Ecosystem: "npm", Mtime: 1, LastIndexed: time.Now(),
	}
	require.NoError(t, db.UpsertIndexManifest(m1))
	require.NoError(t, db.AddManifestPackage(&ManifestPackage{
		ManifestID: m1.ID, ProjectID: "npm/axios", VersionKey: "npm/axios@1.14.1", DepScope: "direct",
	}))
	require.NoError(t, db.AddManifestPackage(&ManifestPackage{
		ManifestID: m1.ID, ProjectID: "npm/lodash", VersionKey: "npm/lodash@4.17.21", DepScope: "direct",
	}))

	m2 := &IndexManifest{
		AbsPath: "/r/b/go.mod", RelPath: "b/go.mod", RootPath: "/r",
		Ecosystem: "go", Mtime: 1, LastIndexed: time.Now(),
	}
	require.NoError(t, db.UpsertIndexManifest(m2))
	require.NoError(t, db.AddManifestPackage(&ManifestPackage{
		ManifestID: m2.ID, ProjectID: "go/example.com/foo", VersionKey: "go/example.com/foo@v1.0.0", DepScope: "direct",
	}))
	// Also link axios from m2 to test dedup counting.
	require.NoError(t, db.AddManifestPackage(&ManifestPackage{
		ManifestID: m2.ID, ProjectID: "npm/axios", VersionKey: "npm/axios@1.14.1", DepScope: "direct",
	}))

	stats, err := db.IndexStats("/r")
	require.NoError(t, err)
	assert.Equal(t, 2, stats.ManifestCount)
	assert.Equal(t, 3, stats.PackageCount) // 3 unique project_ids
	assert.Equal(t, 2, stats.EcosystemCounts["npm"])
	assert.Equal(t, 1, stats.EcosystemCounts["go"])
	require.NotEmpty(t, stats.TopPackages)
	assert.Equal(t, "npm/axios", stats.TopPackages[0].ProjectID) // appears in 2 manifests
	assert.Equal(t, 2, stats.TopPackages[0].Count)
}
```

- [ ] **Step 10: Implement IndexRun methods and IndexStats**

```go
// Append to internal/cache/index.go

// ---------------------------------------------------------------------------
// Index Runs
// ---------------------------------------------------------------------------

// StartIndexRun creates a new index run record.
func (c *CacheDB) StartIndexRun(rootPath, scope string) (*IndexRun, error) {
	now := time.Now()
	res, err := c.db.Exec(
		`INSERT INTO index_runs (root_path, scope, started_at) VALUES (?, ?, ?)`,
		rootPath, scope, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &IndexRun{ID: id, RootPath: rootPath, Scope: scope, StartedAt: now}, nil
}

// FinishIndexRun updates an index run with final stats and finish time.
func (c *CacheDB) FinishIndexRun(run *IndexRun) error {
	run.FinishedAt = time.Now()
	_, err := c.db.Exec(
		`UPDATE index_runs SET finished_at = ?, manifests_found = ?, manifests_updated = ?,
		 manifests_skipped = ?, packages_total = ?, packages_new = ?, errors = ?
		 WHERE id = ?`,
		run.FinishedAt, run.ManifestsFound, run.ManifestsUpdated,
		run.ManifestsSkipped, run.PackagesTotal, run.PackagesNew, run.Errors,
		run.ID,
	)
	return err
}

// GetLastIndexRun returns the most recent completed index run for a root path.
func (c *CacheDB) GetLastIndexRun(rootPath string) (*IndexRun, error) {
	r := &IndexRun{}
	err := c.db.QueryRow(
		`SELECT id, root_path, scope, started_at, finished_at,
		        manifests_found, manifests_updated, manifests_skipped,
		        packages_total, packages_new, errors
		 FROM index_runs WHERE root_path = ? AND finished_at IS NOT NULL
		 ORDER BY id DESC LIMIT 1`, rootPath,
	).Scan(&r.ID, &r.RootPath, &r.Scope, &r.StartedAt, &r.FinishedAt,
		&r.ManifestsFound, &r.ManifestsUpdated, &r.ManifestsSkipped,
		&r.PackagesTotal, &r.PackagesNew, &r.Errors)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

// GetIndexRuns returns the most recent index runs for a root path.
func (c *CacheDB) GetIndexRuns(rootPath string, limit int) ([]IndexRun, error) {
	rows, err := c.db.Query(
		`SELECT id, root_path, scope, started_at, COALESCE(finished_at, ''),
		        manifests_found, manifests_updated, manifests_skipped,
		        packages_total, packages_new, errors
		 FROM index_runs WHERE root_path = ? ORDER BY id DESC LIMIT ?`,
		rootPath, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var runs []IndexRun
	for rows.Next() {
		var r IndexRun
		var fin string
		if err := rows.Scan(&r.ID, &r.RootPath, &r.Scope, &r.StartedAt, &fin,
			&r.ManifestsFound, &r.ManifestsUpdated, &r.ManifestsSkipped,
			&r.PackagesTotal, &r.PackagesNew, &r.Errors); err != nil {
			return nil, err
		}
		if fin != "" {
			r.FinishedAt, _ = time.Parse(time.RFC3339, fin)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// ---------------------------------------------------------------------------
// Statistics
// ---------------------------------------------------------------------------

// IndexStats returns aggregate statistics for the index under rootPath.
func (c *CacheDB) IndexStats(rootPath string) (*IndexStatus, error) {
	s := &IndexStatus{EcosystemCounts: make(map[string]int)}

	// Manifest count.
	err := c.db.QueryRow(
		`SELECT COUNT(*) FROM index_manifests WHERE root_path = ?`, rootPath,
	).Scan(&s.ManifestCount)
	if err != nil {
		return nil, err
	}

	// Unique package count.
	err = c.db.QueryRow(
		`SELECT COUNT(DISTINCT project_id) FROM manifest_packages mp
		 JOIN index_manifests im ON mp.manifest_id = im.id
		 WHERE im.root_path = ?`, rootPath,
	).Scan(&s.PackageCount)
	if err != nil {
		return nil, err
	}

	// Ecosystem counts.
	rows, err := c.db.Query(
		`SELECT ecosystem, COUNT(*) FROM index_manifests WHERE root_path = ? GROUP BY ecosystem`, rootPath,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var eco string
		var count int
		if err := rows.Scan(&eco, &count); err != nil {
			_ = rows.Close()
			return nil, err
		}
		s.EcosystemCounts[eco] = count
	}
	_ = rows.Close()

	// Top packages (by number of manifests referencing them).
	topRows, err := c.db.Query(
		`SELECT mp.project_id, COUNT(DISTINCT mp.manifest_id) as cnt
		 FROM manifest_packages mp
		 JOIN index_manifests im ON mp.manifest_id = im.id
		 WHERE im.root_path = ?
		 GROUP BY mp.project_id
		 ORDER BY cnt DESC
		 LIMIT 10`, rootPath,
	)
	if err != nil {
		return nil, err
	}
	for topRows.Next() {
		var pf PackageFrequency
		if err := topRows.Scan(&pf.ProjectID, &pf.Count); err != nil {
			_ = topRows.Close()
			return nil, err
		}
		// Extract name from project_id ("npm/axios" → "axios").
		if i := len(pf.ProjectID); i > 0 {
			if idx := indexOf(pf.ProjectID, '/'); idx >= 0 {
				pf.Name = pf.ProjectID[idx+1:]
			} else {
				pf.Name = pf.ProjectID
			}
		}
		s.TopPackages = append(s.TopPackages, pf)
	}
	_ = topRows.Close()

	// Last run.
	s.LastRun, _ = c.GetLastIndexRun(rootPath)

	return s, nil
}

func indexOf(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 11: Run all cache tests**

Run: `go test ./internal/cache/ -v`
Expected: all pass including new index tests.

- [ ] **Step 12: Commit**

```bash
git add internal/cache/index.go internal/cache/index_test.go internal/cache/db.go
git commit -m "feat(cache): add index tables for manifests, packages, runs, and stats"
```

---

### Task 2: Core Indexer — Walk, Discover, Parse, Store

**Files:**
- Create: `internal/scanner/indexer.go`
- Create: `internal/scanner/indexer_test.go`

- [ ] **Step 1: Write failing test for ecosystem detection from filename**

```go
// internal/scanner/indexer_test.go
package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectManifestEcosystem(t *testing.T) {
	tests := []struct {
		path string
		eco  string
		ok   bool
	}{
		{"package.json", "npm", true},
		{"package-lock.json", "npm", true},
		{"pnpm-lock.yaml", "npm", true},
		{"go.mod", "go", true},
		{"go.sum", "go", true},
		{"Cargo.toml", "rust", true},
		{"Cargo.lock", "rust", true},
		{"pyproject.toml", "python", true},
		{"requirements.txt", "python", true},
		{"poetry.lock", "python", true},
		{"uv.lock", "python", true},
		{"composer.json", "php", true},
		{"composer.lock", "php", true},
		{".pre-commit-config.yaml", "config", true},
		{".gitmodules", "config", true},
		{".tool-versions", "config", true},
		{".mise.toml", "config", true},
		{"Makefile", "build", true},
		{"Taskfile.yml", "build", true},
		{"justfile", "build", true},
		{"random.txt", "", false},
		{"README.md", "", false},
	}
	for _, tt := range tests {
		eco, ok := detectManifestEcosystem(tt.path)
		assert.Equal(t, tt.ok, ok, tt.path)
		if ok {
			assert.Equal(t, tt.eco, eco, tt.path)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/scanner/ -run TestDetectManifestEcosystem -v`
Expected: FAIL — `detectManifestEcosystem` undefined.

- [ ] **Step 3: Implement indexer scaffold with detectManifestEcosystem**

```go
// internal/scanner/indexer.go
package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/manifest"
)

// IndexOptions configures an index run.
type IndexOptions struct {
	Force   bool   // ignore mtime cache
	Scope   string // "local", "deps", "supply-chain"
	DBPath  string // SQLite path
}

// IndexResult summarizes an index run.
type IndexResult struct {
	ManifestsFound   int
	ManifestsUpdated int
	ManifestsSkipped int
	PackagesTotal    int
	PackagesNew      int
	Errors           int
}

// manifestEcosystems maps filenames to ecosystem strings.
var manifestEcosystems = map[string]string{
	"package.json":            "npm",
	"package-lock.json":       "npm",
	"pnpm-lock.yaml":          "npm",
	"bun.lock":                "npm",
	"go.mod":                  "go",
	"go.sum":                  "go",
	"Cargo.toml":              "rust",
	"Cargo.lock":              "rust",
	"pyproject.toml":          "python",
	"requirements.txt":        "python",
	"poetry.lock":             "python",
	"uv.lock":                 "python",
	"composer.json":           "php",
	"composer.lock":           "php",
	".pre-commit-config.yaml": "config",
	".gitmodules":             "config",
	".tool-versions":          "config",
	".mise.toml":              "config",
	"Makefile":                "build",
	"Taskfile.yml":            "build",
	"Taskfile.yaml":           "build",
	"justfile":                "build",
}

// detectManifestEcosystem returns the ecosystem for a manifest filename.
// Also handles .github/workflows/*.yml and *.tf via suffix matching.
func detectManifestEcosystem(path string) (string, bool) {
	base := filepath.Base(path)
	if eco, ok := manifestEcosystems[base]; ok {
		return eco, true
	}
	if strings.HasSuffix(base, ".tf") {
		return "terraform", true
	}
	if strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml") {
		dir := filepath.Dir(path)
		if strings.HasSuffix(filepath.ToSlash(dir), ".github/workflows") {
			return "actions", true
		}
	}
	return "", false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/scanner/ -run TestDetectManifestEcosystem -v`
Expected: PASS

- [ ] **Step 5: Write failing test for RunIndex (main function)**

```go
// Append to internal/scanner/indexer_test.go

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/depscope/depscope/internal/cache"
	"github.com/stretchr/testify/require"
)

func TestRunIndex(t *testing.T) {
	root := t.TempDir()

	// Create npm project.
	npmDir := filepath.Join(root, "webapp")
	require.NoError(t, os.MkdirAll(npmDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(npmDir, "package.json"), []byte(`{
		"name": "webapp",
		"dependencies": {"axios": "^1.14.0", "express": "^4.18.0"}
	}`), 0644))

	// Create go project.
	goDir := filepath.Join(root, "api")
	require.NoError(t, os.MkdirAll(goDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(goDir, "go.mod"), []byte(`module example.com/api

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
	golang.org/x/sync v0.5.0
)
`), 0644))

	// Create hidden dir with python.
	pyDir := filepath.Join(root, ".tools", "ml")
	require.NoError(t, os.MkdirAll(pyDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pyDir, "requirements.txt"), []byte(
		"numpy==1.24.0\npandas>=2.0.0\n",
	), 0644))

	// Create node_modules package (installed).
	nmDir := filepath.Join(root, "webapp", "node_modules", "axios")
	require.NoError(t, os.MkdirAll(nmDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(nmDir, "package.json"), []byte(`{
		"name": "axios",
		"version": "1.14.1"
	}`), 0644))

	dbPath := filepath.Join(t.TempDir(), "index.db")
	var buf strings.Builder

	result, err := RunIndex(context.Background(), root, IndexOptions{
		Scope:  "local",
		DBPath: dbPath,
	}, &buf)
	require.NoError(t, err)

	assert.Greater(t, result.ManifestsFound, 3)
	assert.Greater(t, result.PackagesTotal, 3)
	assert.Equal(t, 0, result.ManifestsSkipped)

	output := buf.String()
	assert.Contains(t, output, "[npm]")
	assert.Contains(t, output, "[go]")
	assert.Contains(t, output, "[python]")
	assert.Contains(t, output, "Done:")

	// Verify DB.
	db, err := cache.NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	stats, err := db.IndexStats(root)
	require.NoError(t, err)
	assert.Greater(t, stats.ManifestCount, 3)
	assert.Greater(t, stats.PackageCount, 3)

	// Run again — should skip all (incremental).
	var buf2 strings.Builder
	result2, err := RunIndex(context.Background(), root, IndexOptions{
		Scope:  "local",
		DBPath: dbPath,
	}, &buf2)
	require.NoError(t, err)
	assert.Equal(t, result.ManifestsFound, result2.ManifestsFound)
	assert.Equal(t, result2.ManifestsFound, result2.ManifestsSkipped) // all skipped
	assert.Equal(t, 0, result2.ManifestsUpdated)
	assert.Contains(t, buf2.String(), "[skip]")
}

func TestRunIndexForce(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(root, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"), []byte(`{
		"name": "test", "dependencies": {"axios": "^1.0.0"}
	}`), 0644))

	dbPath := filepath.Join(t.TempDir(), "index.db")

	// First run.
	_, err := RunIndex(context.Background(), root, IndexOptions{Scope: "local", DBPath: dbPath}, io.Discard)
	require.NoError(t, err)

	// Second run with force — should NOT skip.
	result, err := RunIndex(context.Background(), root, IndexOptions{
		Scope:  "local",
		DBPath: dbPath,
		Force:  true,
	}, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ManifestsSkipped)
	assert.Greater(t, result.ManifestsUpdated, 0)
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./internal/scanner/ -run TestRunIndex -v`
Expected: FAIL — `RunIndex` undefined.

- [ ] **Step 7: Implement RunIndex**

```go
// Append to internal/scanner/indexer.go

// RunIndex walks root, discovers manifests, parses packages, and stores in SQLite.
func RunIndex(ctx context.Context, root string, opts IndexOptions, w io.Writer) (*IndexResult, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}

	scope := opts.Scope
	if scope == "" {
		scope = "local"
	}

	db, err := cache.NewCacheDB(opts.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	run, err := db.StartIndexRun(absRoot, scope)
	if err != nil {
		return nil, fmt.Errorf("start index run: %w", err)
	}

	result := &IndexResult{}
	seenPaths := make(map[string]bool)

	// Walk the filesystem.
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			result.Errors++
			return nil // skip inaccessible
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(absRoot, path)
		relPath = filepath.ToSlash(relPath)

		eco, isManifest := detectManifestEcosystem(relPath)
		if !isManifest {
			return nil
		}

		result.ManifestsFound++
		seenPaths[path] = true

		// Incremental: check mtime.
		info, err := d.Info()
		if err != nil {
			result.Errors++
			return nil
		}
		mtime := info.ModTime().Unix()

		if !opts.Force {
			existing, err := db.GetIndexManifest(path)
			if err == nil && existing != nil && existing.Mtime == mtime {
				result.ManifestsSkipped++
				_, _ = fmt.Fprintf(w, "  [skip]    %-50s (unchanged)\n", relPath)
				return nil
			}
		}

		// Parse the manifest.
		data, err := os.ReadFile(path)
		if err != nil {
			result.Errors++
			return nil
		}

		checksum := sha256sum(data)
		packages := parseManifestPackages(path, relPath, eco, data)

		// Store manifest.
		m := &cache.IndexManifest{
			AbsPath:     path,
			RelPath:     relPath,
			RootPath:    absRoot,
			Ecosystem:   eco,
			Mtime:       mtime,
			Checksum:    checksum,
			LastIndexed: time.Now(),
		}

		// Check if manifest already exists to get its ID.
		existing, _ := db.GetIndexManifest(path)
		if existing != nil {
			m.ID = existing.ID
		}

		if err := db.UpsertIndexManifest(m); err != nil {
			result.Errors++
			return nil
		}

		// Re-fetch to get ID if it was a new insert.
		if m.ID == 0 {
			fetched, _ := db.GetIndexManifest(path)
			if fetched != nil {
				m.ID = fetched.ID
			}
		}

		// Clear old package links and add new ones.
		_ = db.ClearManifestPackages(m.ID)

		newCount := 0
		for _, pkg := range packages {
			projectID := pkg.ProjectID
			versionKey := pkg.VersionKey

			// Upsert into global projects/versions tables.
			existingProj, _ := db.GetProject(projectID)
			if existingProj == nil {
				_ = db.UpsertProject(&cache.Project{
					ID:        projectID,
					Ecosystem: eco,
					Name:      pkg.Name,
				})
				newCount++
			}
			_ = db.UpsertVersion(&cache.ProjectVersion{
				ProjectID:  projectID,
				VersionKey: versionKey,
			})

			// Link manifest → package.
			_ = db.AddManifestPackage(&cache.ManifestPackage{
				ManifestID: m.ID,
				ProjectID:  projectID,
				VersionKey: versionKey,
				Constraint: pkg.Constraint,
				DepScope:   pkg.DepScope,
			})
		}

		result.ManifestsUpdated++
		result.PackagesTotal += len(packages)
		result.PackagesNew += newCount

		_, _ = fmt.Fprintf(w, "  [%-8s] %-50s %d packages (%d new)\n", eco, relPath, len(packages), newCount)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}

	// Prune deleted manifests.
	pruned, _ := db.PruneDeletedManifests(absRoot, seenPaths)
	if pruned > 0 {
		_, _ = fmt.Fprintf(w, "  Pruned %d deleted manifest(s)\n", pruned)
	}

	// Finish the run.
	run.ManifestsFound = result.ManifestsFound
	run.ManifestsUpdated = result.ManifestsUpdated
	run.ManifestsSkipped = result.ManifestsSkipped
	run.PackagesTotal = result.PackagesTotal
	run.PackagesNew = result.PackagesNew
	run.Errors = result.Errors
	_ = db.FinishIndexRun(run)

	_, _ = fmt.Fprintf(w, "\nDone: %d manifests scanned, %d packages indexed (%d new), %d errors\n",
		result.ManifestsFound, result.PackagesTotal, result.PackagesNew, result.Errors)
	_, _ = fmt.Fprintf(w, "Index: %s\n", opts.DBPath)

	return result, nil
}

// indexedPackage is an intermediate type for parsed packages before DB storage.
type indexedPackage struct {
	ProjectID  string
	VersionKey string
	Name       string
	Constraint string
	DepScope   string
}

// parseManifestPackages extracts packages from a manifest file.
func parseManifestPackages(absPath, relPath, eco string, data []byte) []indexedPackage {
	dir := filepath.Dir(absPath)

	// Special case: node_modules/*/package.json — extract name+version directly.
	if eco == "npm" && isInNodeModules(relPath) {
		return parseNodeModulesPackage(data)
	}

	// Use ecosystem parsers for project-level manifests.
	switch eco {
	case "npm":
		return parseNPMManifest(dir, data)
	case "go":
		return parseGoManifest(data)
	case "python":
		return parsePythonManifest(filepath.Base(absPath), data)
	case "rust":
		return parseRustManifest(filepath.Base(absPath), data)
	case "php":
		return parsePHPManifest(dir, data)
	default:
		return nil // config, build, terraform, actions — no package extraction for local scope
	}
}

func isInNodeModules(relPath string) bool {
	return strings.Contains(relPath, "node_modules/")
}

// parseNodeModulesPackage extracts name + version from a node_modules package.json.
func parseNodeModulesPackage(data []byte) []indexedPackage {
	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || pkg.Name == "" || pkg.Version == "" {
		return nil
	}
	projectID := "npm/" + pkg.Name
	return []indexedPackage{{
		ProjectID:  projectID,
		VersionKey: projectID + "@" + pkg.Version,
		Name:       pkg.Name,
		Constraint: pkg.Version,
		DepScope:   "installed",
	}}
}

// parseNPMManifest parses a project-level package.json (not in node_modules).
func parseNPMManifest(dir string, data []byte) []indexedPackage {
	files := map[string][]byte{"package.json": data}

	// Try to load lockfile for resolved versions.
	for _, lockName := range []string{"package-lock.json", "pnpm-lock.yaml"} {
		lockData, err := os.ReadFile(filepath.Join(dir, lockName))
		if err == nil {
			files[lockName] = lockData
			break
		}
	}

	parser := manifest.NewJavaScriptParser()
	pkgs, err := parser.ParseFiles(files)
	if err != nil {
		return nil
	}

	var result []indexedPackage
	for _, p := range pkgs {
		version := p.ResolvedVersion
		if version == "" {
			version = p.Constraint
		}
		projectID := "npm/" + p.Name
		result = append(result, indexedPackage{
			ProjectID:  projectID,
			VersionKey: projectID + "@" + version,
			Name:       p.Name,
			Constraint: p.Constraint,
			DepScope:   "direct",
		})
	}
	return result
}

// parseGoManifest parses go.mod content.
func parseGoManifest(data []byte) []indexedPackage {
	files := map[string][]byte{"go.mod": data}
	parser := manifest.NewGoModParser()
	pkgs, err := parser.ParseFiles(files)
	if err != nil {
		return nil
	}

	var result []indexedPackage
	for _, p := range pkgs {
		version := p.ResolvedVersion
		if version == "" {
			version = p.Constraint
		}
		projectID := "go/" + p.Name
		result = append(result, indexedPackage{
			ProjectID:  projectID,
			VersionKey: projectID + "@" + version,
			Name:       p.Name,
			Constraint: p.Constraint,
			DepScope:   "direct",
		})
	}
	return result
}

// parsePythonManifest parses requirements.txt, pyproject.toml, or poetry.lock.
func parsePythonManifest(filename string, data []byte) []indexedPackage {
	files := map[string][]byte{filename: data}
	parser := manifest.NewPythonParser()
	pkgs, err := parser.ParseFiles(files)
	if err != nil {
		return nil
	}

	var result []indexedPackage
	for _, p := range pkgs {
		version := p.ResolvedVersion
		if version == "" {
			version = p.Constraint
		}
		projectID := "python/" + p.Name
		result = append(result, indexedPackage{
			ProjectID:  projectID,
			VersionKey: projectID + "@" + version,
			Name:       p.Name,
			Constraint: p.Constraint,
			DepScope:   "direct",
		})
	}
	return result
}

// parseRustManifest parses Cargo.toml or Cargo.lock.
func parseRustManifest(filename string, data []byte) []indexedPackage {
	files := map[string][]byte{filename: data}
	parser := manifest.NewRustParser()
	pkgs, err := parser.ParseFiles(files)
	if err != nil {
		return nil
	}

	var result []indexedPackage
	for _, p := range pkgs {
		version := p.ResolvedVersion
		if version == "" {
			version = p.Constraint
		}
		projectID := "rust/" + p.Name
		result = append(result, indexedPackage{
			ProjectID:  projectID,
			VersionKey: projectID + "@" + version,
			Name:       p.Name,
			Constraint: p.Constraint,
			DepScope:   "direct",
		})
	}
	return result
}

// parsePHPManifest parses composer.json.
func parsePHPManifest(dir string, data []byte) []indexedPackage {
	files := map[string][]byte{"composer.json": data}

	lockData, err := os.ReadFile(filepath.Join(dir, "composer.lock"))
	if err == nil {
		files["composer.lock"] = lockData
	}

	parser := manifest.NewPHPParser()
	pkgs, err := parser.ParseFiles(files)
	if err != nil {
		return nil
	}

	var result []indexedPackage
	for _, p := range pkgs {
		version := p.ResolvedVersion
		if version == "" {
			version = p.Constraint
		}
		projectID := "php/" + p.Name
		result = append(result, indexedPackage{
			ProjectID:  projectID,
			VersionKey: projectID + "@" + version,
			Name:       p.Name,
			Constraint: p.Constraint,
			DepScope:   "direct",
		})
	}
	return result
}

func sha256sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/scanner/ -run TestRunIndex -v`
Expected: PASS

- [ ] **Step 9: Run full test suite**

Run: `go test ./...`
Expected: all pass.

- [ ] **Step 10: Commit**

```bash
git add internal/scanner/indexer.go internal/scanner/indexer_test.go
git commit -m "feat(scanner): add supply chain indexer with incremental mtime-based reindexing"
```

---

### Task 3: CLI Command — `depscope index`

**Files:**
- Create: `cmd/depscope/index_cmd.go`

- [ ] **Step 1: Implement the CLI command**

```go
// cmd/depscope/index_cmd.go
package main

import (
	"fmt"
	"os"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/scanner"
	"github.com/spf13/cobra"
)

func init() {
	indexCmd.Flags().Bool("force", false, "ignore mtime cache, re-parse all manifests")
	indexCmd.Flags().String("scope", "local", "indexing depth: local, deps, supply-chain")
	indexCmd.Flags().String("db", cache.DefaultDBPath(), "path to SQLite database")
	indexCmd.AddCommand(indexStatusCmd)
	rootCmd.AddCommand(indexCmd)
}

var indexCmd = &cobra.Command{
	Use:   "index [path]",
	Short: "Index all dependencies in a directory tree",
	Long: `Walk a directory tree (including hidden directories, node_modules, vendor)
and catalog every package manifest and dependency found. Builds a searchable
SQLite index for fast compromised-package queries and statistics.

Supports incremental indexing — only re-parses manifests that changed since
the last run. Use --force to re-scan everything.

Scope levels:
  local         Parse manifests on disk only (fastest, default)
  deps          Also resolve transitive dependencies from lockfiles
  supply-chain  Also follow GitHub/registry APIs for full supply chain

Examples:
  depscope index ~                              # index home directory
  depscope index /src --scope deps              # with transitive resolution
  depscope index . --force                      # full rescan
  depscope index status                         # show index statistics`,
	Args:    cobra.MaximumNArgs(1),
	RunE:    runIndex,
	SilenceUsage: true,
}

var indexStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show index statistics",
	RunE:  runIndexStatus,
	SilenceUsage: true,
}

func runIndex(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}

	force, _ := cmd.Flags().GetBool("force")
	scope, _ := cmd.Flags().GetString("scope")
	dbPath, _ := cmd.Flags().GetString("db")

	// Ensure DB directory exists.
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Indexing %s (scope: %s, force: %v)...\n\n", absRoot, scope, force)

	result, err := scanner.RunIndex(cmd.Context(), absRoot, scanner.IndexOptions{
		Force:  force,
		Scope:  scope,
		DBPath: dbPath,
	}, os.Stdout)
	if err != nil {
		return err
	}

	if result.Errors > 0 {
		fmt.Fprintf(os.Stderr, "\n%d error(s) during indexing (manifests skipped)\n", result.Errors)
	}
	return nil
}

func runIndexStatus(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")
	if dbPath == "" {
		dbPath = cache.DefaultDBPath()
	}

	db, err := cache.NewCacheDB(dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Get all unique root paths.
	rows, err := db.DB().Query(`SELECT DISTINCT root_path FROM index_manifests`)
	if err != nil {
		fmt.Println("No index data found. Run 'depscope index <path>' first.")
		return nil
	}
	defer func() { _ = rows.Close() }()

	var roots []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return err
		}
		roots = append(roots, r)
	}

	if len(roots) == 0 {
		fmt.Println("No index data found. Run 'depscope index <path>' first.")
		return nil
	}

	fmt.Printf("Index: %s\n\n", dbPath)

	for _, root := range roots {
		stats, err := db.IndexStats(root)
		if err != nil {
			return err
		}

		if stats.LastRun != nil {
			fmt.Printf("Last run: %s (scope: %s)\n", stats.LastRun.FinishedAt.Format("2006-01-02 15:04:05"), stats.LastRun.Scope)
		}
		fmt.Printf("Root: %s\n\n", root)
		fmt.Printf("Manifests:  %d\n", stats.ManifestCount)
		fmt.Printf("Packages:   %d unique\n", stats.PackageCount)

		for eco, count := range stats.EcosystemCounts {
			fmt.Printf("  %-10s %d\n", eco+":", count)
		}

		if len(stats.TopPackages) > 0 {
			fmt.Printf("\nTop packages:")
			for i, p := range stats.TopPackages {
				if i > 4 {
					break
				}
				if i > 0 {
					fmt.Print(",")
				}
				fmt.Printf(" %s (%d)", p.Name, p.Count)
			}
			fmt.Println()
		}
		fmt.Println()
	}
	return nil
}
```

Note: add `"path/filepath"` to imports. Also need to expose `db.DB()` — add a method to `CacheDB`:

In `internal/cache/db.go`, add after the `Close()` method:

```go
// DB returns the underlying *sql.DB for direct queries.
func (c *CacheDB) DB() *sql.DB {
	return c.db
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/depscope/`
Expected: compiles.

- [ ] **Step 3: Smoke test**

Run: `go run ./cmd/depscope/ index --help`
Expected: shows usage with --force, --scope, --db flags and status subcommand.

- [ ] **Step 4: Commit**

```bash
git add cmd/depscope/index_cmd.go internal/cache/db.go
git commit -m "feat(cli): add depscope index command with status subcommand"
```

---

### Task 4: Integration Test — Multi-Ecosystem with Dedup

**Files:**
- Create: `internal/scanner/indexer_integration_test.go`

- [ ] **Step 1: Write integration test**

```go
// internal/scanner/indexer_integration_test.go
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
	mkFile(t, root, "webapp/package.json", `{
		"name": "webapp",
		"dependencies": {"axios": "^1.14.0", "lodash": "^4.17.21"}
	}`)
	mkFile(t, root, "webapp/node_modules/axios/package.json", `{
		"name": "axios", "version": "1.14.1"
	}`)
	mkFile(t, root, "webapp/node_modules/lodash/package.json", `{
		"name": "lodash", "version": "4.17.21"
	}`)

	// Second npm project sharing axios.
	mkFile(t, root, "api/package.json", `{
		"name": "api",
		"dependencies": {"axios": "^1.14.0"}
	}`)
	mkFile(t, root, "api/node_modules/axios/package.json", `{
		"name": "axios", "version": "1.14.1"
	}`)

	// Go project.
	mkFile(t, root, "service/go.mod", `module example.com/svc

go 1.21

require (
	golang.org/x/sync v0.5.0
)
`)

	// Hidden dir with Python.
	mkFile(t, root, ".scripts/tool/requirements.txt", "requests==2.31.0\nnumpy==1.24.0\n")

	// Scoped npm package in node_modules.
	mkFile(t, root, "webapp/node_modules/@scope/ui/package.json", `{
		"name": "@scope/ui", "version": "3.0.0"
	}`)

	dbPath := filepath.Join(t.TempDir(), "integration.db")
	var buf strings.Builder

	result, err := RunIndex(context.Background(), root, IndexOptions{
		Scope: "local", DBPath: dbPath,
	}, &buf)
	require.NoError(t, err)

	output := buf.String()

	// Check multiple ecosystems detected.
	assert.Contains(t, output, "[npm")
	assert.Contains(t, output, "[go")
	assert.Contains(t, output, "[python")

	// Verify dedup: axios appears in 3 manifests but should be 1 unique package.
	db, err := cache.NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	manifests, err := db.FindPackageManifests("npm/axios")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(manifests), 2) // at least 2 manifest links

	stats, err := db.IndexStats(root)
	require.NoError(t, err)
	assert.Greater(t, stats.ManifestCount, 4)
	assert.Greater(t, stats.PackageCount, 4)

	// Verify top packages includes axios.
	found := false
	for _, p := range stats.TopPackages {
		if p.ProjectID == "npm/axios" {
			found = true
			assert.GreaterOrEqual(t, p.Count, 2) // linked from multiple manifests
		}
	}
	assert.True(t, found, "axios should be in top packages")

	// Verify index run was recorded.
	lastRun, err := db.GetLastIndexRun(root)
	require.NoError(t, err)
	require.NotNil(t, lastRun)
	assert.Equal(t, result.ManifestsFound, lastRun.ManifestsFound)

	// Verify scoped package was indexed.
	scopedManifests, err := db.FindPackageManifests("npm/@scope/ui")
	require.NoError(t, err)
	assert.Len(t, scopedManifests, 1)
}

func mkFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	abs := filepath.Join(root, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0644))
}
```

- [ ] **Step 2: Run the integration test**

Run: `go test ./internal/scanner/ -run TestRunIndexIntegration -v`
Expected: PASS

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`
Expected: all pass, no regressions.

- [ ] **Step 4: Commit**

```bash
git add internal/scanner/indexer_integration_test.go
git commit -m "test: add multi-ecosystem integration test for supply chain indexer"
```

---

### Task 5: Final Verification

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`
Expected: all pass.

- [ ] **Step 2: Manual smoke test**

```bash
# Create a temp directory with a small npm project.
mkdir -p /tmp/index-test/project-a
echo '{"name":"test","dependencies":{"axios":"^1.14.0"}}' > /tmp/index-test/project-a/package.json

go run ./cmd/depscope/ index /tmp/index-test
go run ./cmd/depscope/ index status
go run ./cmd/depscope/ index /tmp/index-test  # should show [skip]
go run ./cmd/depscope/ index /tmp/index-test --force  # should re-index
```

- [ ] **Step 3: Fix any issues found**

- [ ] **Step 4: Final commit if needed**
