// internal/cache/index.go
package cache

import (
	"database/sql"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// IndexManifest represents a discovered manifest file in the local filesystem.
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

// ManifestPackage represents a package declared in a manifest file.
type ManifestPackage struct {
	ManifestID int64
	ProjectID  string
	VersionKey string
	Constraint string
	DepScope   string // "direct", "dev", "transitive", "installed"
}

// IndexRun represents a single indexing run over a project root.
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

// IndexStatus holds aggregated statistics about the index.
type IndexStatus struct {
	ManifestCount   int
	PackageCount    int
	EcosystemCounts map[string]int
	TopPackages     []PackageFrequency
	LastRun         *IndexRun
}

// PackageFrequency tracks how many manifests reference a given project.
type PackageFrequency struct {
	ProjectID string
	Name      string
	Count     int
}

// ---------------------------------------------------------------------------
// Schema Migration
// ---------------------------------------------------------------------------

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

// UpsertIndexManifest inserts or updates a manifest by abs_path. On insert or
// update the ID is populated on the provided struct.
func (c *CacheDB) UpsertIndexManifest(m *IndexManifest) error {
	if m.LastIndexed.IsZero() {
		m.LastIndexed = time.Now()
	}
	res, err := c.db.Exec(
		`INSERT INTO index_manifests (abs_path, rel_path, root_path, ecosystem, mtime, checksum, last_indexed)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(abs_path) DO UPDATE SET
		   rel_path     = excluded.rel_path,
		   root_path    = excluded.root_path,
		   ecosystem    = excluded.ecosystem,
		   mtime        = excluded.mtime,
		   checksum     = excluded.checksum,
		   last_indexed = excluded.last_indexed`,
		m.AbsPath, m.RelPath, m.RootPath, m.Ecosystem, m.Mtime, m.Checksum, m.LastIndexed,
	)
	if err != nil {
		return err
	}

	// LastInsertId returns the rowid for INSERT; on UPDATE it may not reflect
	// the existing row, so we always re-query.
	id, err := res.LastInsertId()
	if err != nil || id == 0 {
		return c.db.QueryRow(
			`SELECT id FROM index_manifests WHERE abs_path = ?`, m.AbsPath,
		).Scan(&m.ID)
	}
	m.ID = id
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

// ListIndexManifests returns all manifests under the given root path.
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

// DeleteIndexManifest deletes a manifest by abs_path. Linked manifest_packages
// rows are removed via ON DELETE CASCADE.
func (c *CacheDB) DeleteIndexManifest(absPath string) error {
	_, err := c.db.Exec(`DELETE FROM index_manifests WHERE abs_path = ?`, absPath)
	return err
}

// ---------------------------------------------------------------------------
// Manifest Packages
// ---------------------------------------------------------------------------

// AddManifestPackage inserts a package reference. Duplicates (same manifest_id,
// project_id, version_key) are silently ignored via INSERT OR IGNORE.
func (c *CacheDB) AddManifestPackage(mp *ManifestPackage) error {
	_, err := c.db.Exec(
		`INSERT OR IGNORE INTO manifest_packages
		   (manifest_id, project_id, version_key, constraint_, dep_scope)
		 VALUES (?, ?, ?, ?, ?)`,
		mp.ManifestID, mp.ProjectID, mp.VersionKey, mp.Constraint, mp.DepScope,
	)
	return err
}

// GetManifestPackages returns all packages declared in the given manifest.
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

// FindPackageManifests returns all manifest_packages rows for a given project_id
// (reverse lookup: "which manifests reference this package?").
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

// ClearManifestPackages deletes all package rows for a given manifest.
func (c *CacheDB) ClearManifestPackages(manifestID int64) error {
	_, err := c.db.Exec(`DELETE FROM manifest_packages WHERE manifest_id = ?`, manifestID)
	return err
}

// PruneDeletedManifests removes manifests under rootPath whose abs_path is
// not in currentPaths. Returns the number of manifests removed.
func (c *CacheDB) PruneDeletedManifests(rootPath string, currentPaths map[string]bool) (int, error) {
	manifests, err := c.ListIndexManifests(rootPath)
	if err != nil {
		return 0, err
	}

	pruned := 0
	for _, m := range manifests {
		if !currentPaths[m.AbsPath] {
			if err := c.DeleteIndexManifest(m.AbsPath); err != nil {
				return pruned, fmt.Errorf("delete stale manifest %s: %w", m.AbsPath, err)
			}
			pruned++
		}
	}
	return pruned, nil
}

// ---------------------------------------------------------------------------
// Index Runs
// ---------------------------------------------------------------------------

// StartIndexRun creates a new index run record with started_at = now.
func (c *CacheDB) StartIndexRun(rootPath, scope string) (*IndexRun, error) {
	now := time.Now()
	res, err := c.db.Exec(
		`INSERT INTO index_runs (root_path, scope, started_at) VALUES (?, ?, ?)`,
		rootPath, scope, now,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &IndexRun{
		ID:        id,
		RootPath:  rootPath,
		Scope:     scope,
		StartedAt: now,
	}, nil
}

// FinishIndexRun updates a run with final stats and sets finished_at = now.
func (c *CacheDB) FinishIndexRun(run *IndexRun) error {
	run.FinishedAt = time.Now()
	_, err := c.db.Exec(
		`UPDATE index_runs SET
		   finished_at       = ?,
		   manifests_found   = ?,
		   manifests_updated = ?,
		   manifests_skipped = ?,
		   packages_total    = ?,
		   packages_new      = ?,
		   errors            = ?
		 WHERE id = ?`,
		run.FinishedAt,
		run.ManifestsFound, run.ManifestsUpdated, run.ManifestsSkipped,
		run.PackagesTotal, run.PackagesNew, run.Errors,
		run.ID,
	)
	return err
}

// GetLastIndexRun returns the most recent completed run for a root path.
// Returns nil if no completed run exists.
func (c *CacheDB) GetLastIndexRun(rootPath string) (*IndexRun, error) {
	r := &IndexRun{}
	var finishedAt sql.NullTime
	err := c.db.QueryRow(
		`SELECT id, root_path, scope, started_at, finished_at,
		        manifests_found, manifests_updated, manifests_skipped,
		        packages_total, packages_new, errors
		 FROM index_runs
		 WHERE root_path = ? AND finished_at IS NOT NULL
		 ORDER BY finished_at DESC LIMIT 1`, rootPath,
	).Scan(&r.ID, &r.RootPath, &r.Scope, &r.StartedAt, &finishedAt,
		&r.ManifestsFound, &r.ManifestsUpdated, &r.ManifestsSkipped,
		&r.PackagesTotal, &r.PackagesNew, &r.Errors)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		r.FinishedAt = finishedAt.Time
	}
	return r, nil
}

// GetIndexRuns returns the most recent runs for a root path, up to limit.
func (c *CacheDB) GetIndexRuns(rootPath string, limit int) ([]IndexRun, error) {
	rows, err := c.db.Query(
		`SELECT id, root_path, scope, started_at, finished_at,
		        manifests_found, manifests_updated, manifests_skipped,
		        packages_total, packages_new, errors
		 FROM index_runs
		 WHERE root_path = ?
		 ORDER BY started_at DESC LIMIT ?`, rootPath, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var runs []IndexRun
	for rows.Next() {
		var r IndexRun
		var finishedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.RootPath, &r.Scope, &r.StartedAt, &finishedAt,
			&r.ManifestsFound, &r.ManifestsUpdated, &r.ManifestsSkipped,
			&r.PackagesTotal, &r.PackagesNew, &r.Errors); err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			r.FinishedAt = finishedAt.Time
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// ---------------------------------------------------------------------------
// Index Stats
// ---------------------------------------------------------------------------

// IndexStats returns aggregated statistics about the index for a given root path.
func (c *CacheDB) IndexStats(rootPath string) (*IndexStatus, error) {
	s := &IndexStatus{
		EcosystemCounts: make(map[string]int),
	}

	// Manifest count
	if err := c.db.QueryRow(
		`SELECT COUNT(*) FROM index_manifests WHERE root_path = ?`, rootPath,
	).Scan(&s.ManifestCount); err != nil {
		return nil, err
	}

	// Unique package count
	if err := c.db.QueryRow(
		`SELECT COUNT(DISTINCT mp.project_id)
		 FROM manifest_packages mp
		 JOIN index_manifests im ON mp.manifest_id = im.id
		 WHERE im.root_path = ?`, rootPath,
	).Scan(&s.PackageCount); err != nil {
		return nil, err
	}

	// Ecosystem breakdown
	rows, err := c.db.Query(
		`SELECT ecosystem, COUNT(*) FROM index_manifests WHERE root_path = ? GROUP BY ecosystem`, rootPath,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var eco string
		var count int
		if err := rows.Scan(&eco, &count); err != nil {
			return nil, err
		}
		s.EcosystemCounts[eco] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Top 10 packages by frequency
	topRows, err := c.db.Query(
		`SELECT mp.project_id, mp.project_id, COUNT(*) as cnt
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
	defer func() { _ = topRows.Close() }()

	for topRows.Next() {
		var pf PackageFrequency
		if err := topRows.Scan(&pf.ProjectID, &pf.Name, &pf.Count); err != nil {
			return nil, err
		}
		s.TopPackages = append(s.TopPackages, pf)
	}
	if err := topRows.Err(); err != nil {
		return nil, err
	}

	// Last run
	lastRun, err := c.GetLastIndexRun(rootPath)
	if err != nil {
		return nil, err
	}
	s.LastRun = lastRun

	return s, nil
}
