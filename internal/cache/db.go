// internal/cache/db.go
package cache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// TTL constants for CacheDB entries.
const (
	projectTTL    = 24 * time.Hour
	tagRefTTL     = 1 * time.Hour
	branchRefTTL  = 15 * time.Minute
	cveCacheTTL   = 6 * time.Hour
)

// Project represents a cached project identity (git remote / registry).
type Project struct {
	ID          string
	Ecosystem   string
	Name        string
	LastFetched time.Time
}

// ProjectVersion represents an immutable version (commit SHA or package version).
type ProjectVersion struct {
	ProjectID    string
	VersionKey   string
	Metadata     string
	LastAccessed time.Time
}

// VersionDependency represents a dependency edge between two project versions.
type VersionDependency struct {
	ParentProjectID        string
	ParentVersionKey       string
	ChildProjectID         string
	ChildVersionConstraint string
	DepScope               string
}

// CacheStatus holds row counts for all cache tables.
type CacheStatus struct {
	Projects       int
	Versions       int
	Dependencies   int
	CVEEntries     int
	RefResolutions int
}

// CompromisedFinding records a discovered compromised package in a manifest.
type CompromisedFinding struct {
	ScanID       string
	ManifestPath string
	PackageName  string
	Version      string
	Constraint   string
	Relation     string // "direct" or "indirect"
	ParentChain  string // for indirect: "parentA -> parentB -> pkg"
}

// CacheDB is a SQLite-backed structured cache for dependency resolution results.
type CacheDB struct {
	db *sql.DB
}

// DefaultDBPath returns the default file path for the cache database.
func DefaultDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "depscope", "depscope-cache.db")
}

// NewCacheDB opens (or creates) a SQLite cache database at dsn, enables WAL mode,
// and runs schema migrations.
func NewCacheDB(dsn string) (*CacheDB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode for concurrent reads.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Enable foreign keys.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	c := &CacheDB{db: db}
	if err := c.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return c, nil
}

// Close closes the underlying database connection.
func (c *CacheDB) Close() error {
	return c.db.Close()
}

// DB returns the underlying *sql.DB for advanced queries.
func (c *CacheDB) DB() *sql.DB {
	return c.db
}

func (c *CacheDB) migrate() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS projects (
		id          TEXT PRIMARY KEY,
		ecosystem   TEXT NOT NULL,
		name        TEXT NOT NULL,
		last_fetched DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS project_versions (
		project_id   TEXT NOT NULL,
		version_key  TEXT NOT NULL,
		metadata     TEXT NOT NULL DEFAULT '',
		last_accessed DATETIME NOT NULL,
		PRIMARY KEY (project_id, version_key),
		FOREIGN KEY (project_id) REFERENCES projects(id)
	);

	CREATE TABLE IF NOT EXISTS version_dependencies (
		parent_project_id        TEXT NOT NULL,
		parent_version_key       TEXT NOT NULL,
		child_project_id         TEXT NOT NULL,
		child_version_constraint TEXT NOT NULL DEFAULT '',
		dep_scope                TEXT NOT NULL DEFAULT '',
		UNIQUE(parent_project_id, parent_version_key, child_project_id, child_version_constraint, dep_scope),
		FOREIGN KEY (parent_project_id, parent_version_key) REFERENCES project_versions(project_id, version_key)
	);

	CREATE TABLE IF NOT EXISTS ref_resolutions (
		owner_repo TEXT NOT NULL,
		ref        TEXT NOT NULL,
		ref_type   TEXT NOT NULL,
		sha        TEXT NOT NULL,
		resolved_at DATETIME NOT NULL,
		PRIMARY KEY (owner_repo, ref, ref_type)
	);

	CREATE TABLE IF NOT EXISTS cve_cache (
		ecosystem  TEXT NOT NULL,
		name       TEXT NOT NULL,
		version    TEXT NOT NULL,
		findings   TEXT NOT NULL,
		fetched_at DATETIME NOT NULL,
		PRIMARY KEY (ecosystem, name, version)
	);

	CREATE TABLE IF NOT EXISTS compromised_findings (
		scan_id       TEXT NOT NULL,
		manifest_path TEXT NOT NULL,
		package_name  TEXT NOT NULL,
		version       TEXT NOT NULL,
		constraint_   TEXT NOT NULL DEFAULT '',
		relation      TEXT NOT NULL,
		parent_chain  TEXT NOT NULL DEFAULT '',
		found_at      DATETIME NOT NULL,
		UNIQUE(scan_id, manifest_path, package_name, version)
	);
	`
	_, err := c.db.Exec(schema)
	if err != nil {
		return err
	}
	return c.migrateIndex()
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------

// UpsertProject inserts or replaces a project row. If LastFetched is zero,
// it defaults to time.Now().
func (c *CacheDB) UpsertProject(p *Project) error {
	if p.LastFetched.IsZero() {
		p.LastFetched = time.Now()
	}
	_, err := c.db.Exec(
		`INSERT INTO projects (id, ecosystem, name, last_fetched)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   ecosystem = excluded.ecosystem,
		   name = excluded.name,
		   last_fetched = excluded.last_fetched`,
		p.ID, p.Ecosystem, p.Name, p.LastFetched,
	)
	return err
}

// GetProject retrieves a project by ID. Returns nil if not found or if the
// project's TTL (24h) has expired.
func (c *CacheDB) GetProject(id string) (*Project, error) {
	p := &Project{}
	err := c.db.QueryRow(
		`SELECT id, ecosystem, name, last_fetched FROM projects WHERE id = ?`, id,
	).Scan(&p.ID, &p.Ecosystem, &p.Name, &p.LastFetched)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if time.Since(p.LastFetched) > projectTTL {
		return nil, nil
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Project Versions
// ---------------------------------------------------------------------------

// UpsertVersion inserts or updates a project version. LastAccessed is set to now.
func (c *CacheDB) UpsertVersion(v *ProjectVersion) error {
	now := time.Now()
	_, err := c.db.Exec(
		`INSERT INTO project_versions (project_id, version_key, metadata, last_accessed)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(project_id, version_key) DO UPDATE SET
		   metadata = excluded.metadata,
		   last_accessed = excluded.last_accessed`,
		v.ProjectID, v.VersionKey, v.Metadata, now,
	)
	return err
}

// GetVersion retrieves a project version. Immutable versions never expire.
// Updates last_accessed on cache hit.
func (c *CacheDB) GetVersion(projectID, versionKey string) (*ProjectVersion, error) {
	v := &ProjectVersion{}
	err := c.db.QueryRow(
		`SELECT project_id, version_key, metadata, last_accessed
		 FROM project_versions WHERE project_id = ? AND version_key = ?`,
		projectID, versionKey,
	).Scan(&v.ProjectID, &v.VersionKey, &v.Metadata, &v.LastAccessed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Touch on read.
	_ = c.TouchVersion(projectID, versionKey)
	return v, nil
}

// TouchVersion updates the last_accessed timestamp for a version.
func (c *CacheDB) TouchVersion(projectID, versionKey string) error {
	_, err := c.db.Exec(
		`UPDATE project_versions SET last_accessed = ? WHERE project_id = ? AND version_key = ?`,
		time.Now(), projectID, versionKey,
	)
	return err
}

// ---------------------------------------------------------------------------
// Version Dependencies
// ---------------------------------------------------------------------------

// AddVersionDependency inserts a dependency edge. Duplicates are silently ignored
// via INSERT OR IGNORE (UNIQUE constraint on the table).
func (c *CacheDB) AddVersionDependency(d *VersionDependency) error {
	_, err := c.db.Exec(
		`INSERT OR IGNORE INTO version_dependencies
		   (parent_project_id, parent_version_key, child_project_id, child_version_constraint, dep_scope)
		 VALUES (?, ?, ?, ?, ?)`,
		d.ParentProjectID, d.ParentVersionKey, d.ChildProjectID, d.ChildVersionConstraint, d.DepScope,
	)
	return err
}

// GetVersionDependencies returns all dependency edges for a given parent version.
func (c *CacheDB) GetVersionDependencies(projectID, versionKey string) ([]VersionDependency, error) {
	rows, err := c.db.Query(
		`SELECT parent_project_id, parent_version_key, child_project_id, child_version_constraint, dep_scope
		 FROM version_dependencies
		 WHERE parent_project_id = ? AND parent_version_key = ?`,
		projectID, versionKey,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var deps []VersionDependency
	for rows.Next() {
		var d VersionDependency
		if err := rows.Scan(&d.ParentProjectID, &d.ParentVersionKey, &d.ChildProjectID, &d.ChildVersionConstraint, &d.DepScope); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

// FindDependents returns all dependency edges where child_project_id matches
// the given project ID (reverse lookup).
func (c *CacheDB) FindDependents(childProjectID string) ([]VersionDependency, error) {
	rows, err := c.db.Query(
		`SELECT parent_project_id, parent_version_key, child_project_id, child_version_constraint, dep_scope
		 FROM version_dependencies
		 WHERE child_project_id = ?`,
		childProjectID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var deps []VersionDependency
	for rows.Next() {
		var d VersionDependency
		if err := rows.Scan(&d.ParentProjectID, &d.ParentVersionKey, &d.ChildProjectID, &d.ChildVersionConstraint, &d.DepScope); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

// ---------------------------------------------------------------------------
// Ref Resolutions
// ---------------------------------------------------------------------------

// SetRefResolution stores a ref → SHA resolution with the current timestamp.
func (c *CacheDB) SetRefResolution(ownerRepo, ref, refType, sha string) error {
	_, err := c.db.Exec(
		`INSERT INTO ref_resolutions (owner_repo, ref, ref_type, sha, resolved_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(owner_repo, ref, ref_type) DO UPDATE SET
		   sha = excluded.sha,
		   resolved_at = excluded.resolved_at`,
		ownerRepo, ref, refType, sha, time.Now(),
	)
	return err
}

// GetRefResolution retrieves a ref resolution. Returns "" if not found or
// if the TTL has expired (1h for tags, 15min for branches).
func (c *CacheDB) GetRefResolution(ownerRepo, ref, refType string) (string, error) {
	var sha string
	var resolvedAt time.Time
	err := c.db.QueryRow(
		`SELECT sha, resolved_at FROM ref_resolutions
		 WHERE owner_repo = ? AND ref = ? AND ref_type = ?`,
		ownerRepo, ref, refType,
	).Scan(&sha, &resolvedAt)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	ttl := tagRefTTL
	if refType == "branch" {
		ttl = branchRefTTL
	}
	if time.Since(resolvedAt) > ttl {
		return "", nil
	}
	return sha, nil
}

// ---------------------------------------------------------------------------
// CVE Cache
// ---------------------------------------------------------------------------

// SetCVECache stores CVE findings JSON for an ecosystem/name/version triple.
func (c *CacheDB) SetCVECache(ecosystem, name, version, findings string) error {
	_, err := c.db.Exec(
		`INSERT INTO cve_cache (ecosystem, name, version, findings, fetched_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(ecosystem, name, version) DO UPDATE SET
		   findings = excluded.findings,
		   fetched_at = excluded.fetched_at`,
		ecosystem, name, version, findings, time.Now(),
	)
	return err
}

// GetCVECache retrieves cached CVE findings. Returns "" if not found or if
// the TTL (6h) has expired.
func (c *CacheDB) GetCVECache(ecosystem, name, version string) (string, error) {
	var findings string
	var fetchedAt time.Time
	err := c.db.QueryRow(
		`SELECT findings, fetched_at FROM cve_cache
		 WHERE ecosystem = ? AND name = ? AND version = ?`,
		ecosystem, name, version,
	).Scan(&findings, &fetchedAt)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if time.Since(fetchedAt) > cveCacheTTL {
		return "", nil
	}
	return findings, nil
}

// ---------------------------------------------------------------------------
// Maintenance
// ---------------------------------------------------------------------------

// Prune removes project versions whose last_accessed is older than the given
// duration. Associated version_dependencies rows are deleted first (cascade).
// Returns the number of versions pruned.
func (c *CacheDB) Prune(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)

	tx, err := c.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// Delete dependency edges for versions that will be pruned.
	_, err = tx.Exec(
		`DELETE FROM version_dependencies
		 WHERE (parent_project_id, parent_version_key) IN (
		   SELECT project_id, version_key FROM project_versions WHERE last_accessed < ?
		 )`, cutoff,
	)
	if err != nil {
		return 0, err
	}

	res, err := tx.Exec(
		`DELETE FROM project_versions WHERE last_accessed < ?`, cutoff,
	)
	if err != nil {
		return 0, err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	// VACUUM outside transaction.
	_, _ = c.db.Exec("VACUUM")

	return int(n), nil
}

// Status returns row counts for all cache tables.
func (c *CacheDB) Status() (*CacheStatus, error) {
	s := &CacheStatus{}
	for _, q := range []struct {
		query string
		dest  *int
	}{
		{"SELECT COUNT(*) FROM projects", &s.Projects},
		{"SELECT COUNT(*) FROM project_versions", &s.Versions},
		{"SELECT COUNT(*) FROM version_dependencies", &s.Dependencies},
		{"SELECT COUNT(*) FROM cve_cache", &s.CVEEntries},
		{"SELECT COUNT(*) FROM ref_resolutions", &s.RefResolutions},
	} {
		if err := c.db.QueryRow(q.query).Scan(q.dest); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Compromised Findings
// ---------------------------------------------------------------------------

// AddCompromisedFinding inserts a compromised finding. Duplicates (same scan,
// manifest, package, version) are silently ignored via INSERT OR IGNORE.
func (c *CacheDB) AddCompromisedFinding(f *CompromisedFinding) error {
	_, err := c.db.Exec(
		`INSERT OR IGNORE INTO compromised_findings
		   (scan_id, manifest_path, package_name, version, constraint_, relation, parent_chain, found_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ScanID, f.ManifestPath, f.PackageName, f.Version, f.Constraint, f.Relation, f.ParentChain, time.Now(),
	)
	return err
}

// GetCompromisedFindings returns all compromised findings for a given scan ID.
func (c *CacheDB) GetCompromisedFindings(scanID string) ([]CompromisedFinding, error) {
	rows, err := c.db.Query(
		`SELECT scan_id, manifest_path, package_name, version, constraint_, relation, parent_chain
		 FROM compromised_findings WHERE scan_id = ?`, scanID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var findings []CompromisedFinding
	for rows.Next() {
		var f CompromisedFinding
		if err := rows.Scan(&f.ScanID, &f.ManifestPath, &f.PackageName, &f.Version, &f.Constraint, &f.Relation, &f.ParentChain); err != nil {
			return nil, err
		}
		findings = append(findings, f)
	}
	return findings, rows.Err()
}
