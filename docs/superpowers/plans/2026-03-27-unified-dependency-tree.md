# Unified Dependency Tree Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the fragmented scanning infrastructure with a unified BFS crawler that builds a complete dependency tree of everything that runs on dev machines or CI, backed by a Project/ProjectVersion SQLite cache.

**Architecture:** A central Crawler does BFS with dedup, dispatching to 7+ type-specific Resolvers (package, action, precommit, terraform, submodule, tool, script). Each resolver implements Detect (find refs in files) and Resolve (fetch contents + metadata). A SQLite cache with projects/project_versions/version_dependencies tables provides cross-scan speed optimization. Post-processing adds CVE lookups and org trust scoring.

**Tech Stack:** Go 1.26, SQLite (modernc.org/sqlite), Cobra CLI, Bubbletea TUI, D3.js web visualization

**Spec:** `docs/superpowers/specs/2026-03-27-unified-dependency-tree-design.md`

---

## File Map

### New files

| File | Responsibility |
|---|---|
| `internal/crawler/types.go` | FileTree, DepRef, ResolvedDep, DepSourceType, CrawlResult, CrawlStats, CrawlError, queueItem |
| `internal/crawler/resolver.go` | Resolver interface, AuthProvider interface |
| `internal/crawler/crawler.go` | Crawler struct, BFS engine, dedup, concurrency |
| `internal/crawler/crawler_test.go` | Crawler unit tests with mock resolvers |
| `internal/cache/db.go` | CacheDB struct wrapping SQLite (projects, project_versions, version_dependencies, cve_cache) |
| `internal/cache/db_test.go` | CacheDB tests |
| `internal/crawler/resolvers/package.go` | PackageResolver wrapping manifest/ + registry/ |
| `internal/crawler/resolvers/package_test.go` | PackageResolver tests |
| `internal/crawler/resolvers/action.go` | ActionResolver wrapping actions/ |
| `internal/crawler/resolvers/action_test.go` | ActionResolver tests |
| `internal/crawler/resolvers/precommit.go` | PrecommitResolver (.pre-commit-config.yaml) |
| `internal/crawler/resolvers/precommit_test.go` | PrecommitResolver tests |
| `internal/crawler/resolvers/terraform.go` | TerraformResolver (*.tf source blocks) |
| `internal/crawler/resolvers/terraform_test.go` | TerraformResolver tests |
| `internal/crawler/resolvers/submodule.go` | SubmoduleResolver (.gitmodules) |
| `internal/crawler/resolvers/submodule_test.go` | SubmoduleResolver tests |
| `internal/crawler/resolvers/tool.go` | ToolResolver (.tool-versions, .mise.toml) |
| `internal/crawler/resolvers/tool_test.go` | ToolResolver tests |
| `internal/crawler/resolvers/script.go` | ScriptResolver (curl\|sh, wget patterns) |
| `internal/crawler/resolvers/script_test.go` | ScriptResolver tests |
| `internal/crawler/resolvers/buildtool.go` | BuildToolResolver (Makefile, Taskfile, justfile) |
| `internal/crawler/resolvers/buildtool_test.go` | BuildToolResolver tests |
| `internal/core/orgscore.go` | Org detection + trust scoring |
| `internal/core/orgscore_test.go` | Org scoring tests |
| `internal/report/tree.go` | ASCII/Unicode tree renderer |
| `internal/report/tree_test.go` | Tree renderer tests |
| `cmd/depscope/tree_cmd.go` | `depscope tree` subcommand |
| `internal/tui/walker.go` | TUI tree walker view |

### Modified files

| File | Changes |
|---|---|
| `internal/graph/types.go` | Add 5 node types, 5 edge types, PinningSemverRange, ProjectID/VersionKey on Node |
| `internal/graph/types_test.go` | Update for new types |
| `internal/graph/builder.go` | Update Node construction to include ProjectID/VersionKey |
| `internal/config/config.go` | Add TrustedOrgs, Auth, ConcurrencyConfig to Config |
| `internal/config/config_test.go` | Tests for new config fields |
| `internal/scanner/scanner.go` | Refactor ScanDir/ScanURL to use Crawler |
| `internal/scanner/scanner_test.go` | Update tests |
| `internal/discover/discover.go` | Add cache-first query path |
| `cmd/depscope/main.go` | Register `tree` subcommand |
| `cmd/depscope/cache_cmd.go` | Add `prune` subcommand |
| `cmd/depscope/scan.go` | Wire new config options |
| `internal/server/store/sqlite.go` | Add graph_nodes ProjectID/VersionKey columns |
| `internal/web/static/graph.js` | Clustering, filtering, depth control |
| `internal/web/templates/graph.html` | Filter panel, sidebar, depth slider |
| `internal/tui/model.go` | Add walker view mode |

---

## Phase 1: Foundation (Graph Types + Cache DB + Config)

### Task 1: Extend graph types

**Files:**
- Modify: `internal/graph/types.go`
- Modify: `internal/graph/types_test.go`

- [ ] **Step 1: Write test for new node types**

Add to `internal/graph/types_test.go`:
```go
func TestNewNodeTypes(t *testing.T) {
	tests := []struct {
		nt   NodeType
		want string
	}{
		{NodePrecommitHook, "precommit_hook"},
		{NodeTerraformModule, "terraform_module"},
		{NodeGitSubmodule, "git_submodule"},
		{NodeDevTool, "dev_tool"},
		{NodeBuildTool, "build_tool"},
	}
	for _, tt := range tests {
		if got := tt.nt.String(); got != tt.want {
			t.Errorf("NodeType(%d).String() = %q, want %q", tt.nt, got, tt.want)
		}
	}
}

func TestNewEdgeTypes(t *testing.T) {
	tests := []struct {
		et   EdgeType
		want string
	}{
		{EdgeUsesHook, "uses_hook"},
		{EdgeUsesModule, "uses_module"},
		{EdgeIncludesSubmodule, "includes_submodule"},
		{EdgeUsesTool, "uses_tool"},
		{EdgeBuiltWith, "built_with"},
	}
	for _, tt := range tests {
		if got := tt.et.String(); got != tt.want {
			t.Errorf("EdgeType(%d).String() = %q, want %q", tt.et, got, tt.want)
		}
	}
}

func TestPinningSemverRange(t *testing.T) {
	if PinningSemverRange.String() != "semver_range" {
		t.Errorf("PinningSemverRange.String() = %q, want %q", PinningSemverRange.String(), "semver_range")
	}
	// Ensure ordering: ExactVersion < SemverRange < MajorTag
	if PinningSemverRange <= PinningExactVersion || PinningSemverRange >= PinningMajorTag {
		t.Error("PinningSemverRange should be between PinningExactVersion and PinningMajorTag")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go test ./internal/graph/ -run "TestNewNodeTypes|TestNewEdgeTypes|TestPinningSemverRange" -v`
Expected: compilation errors — types don't exist yet

- [ ] **Step 3: Implement new types**

In `internal/graph/types.go`:

Add to NodeType const block (after `NodeScriptDownload`, replacing the Future comment):
```go
NodePrecommitHook   // .pre-commit-config.yaml hook
NodeTerraformModule // Terraform/OpenTofu module
NodeGitSubmodule    // .gitmodules entry
NodeDevTool         // .tool-versions, .mise.toml entry
NodeBuildTool       // Makefile/Taskfile that installs things
```

Add cases to `NodeType.String()`:
```go
case NodePrecommitHook:
    return "precommit_hook"
case NodeTerraformModule:
    return "terraform_module"
case NodeGitSubmodule:
    return "git_submodule"
case NodeDevTool:
    return "dev_tool"
case NodeBuildTool:
    return "build_tool"
```

Add to EdgeType const block (after `EdgeDownloads`, replacing the Future comment):
```go
EdgeUsesHook          // repo → precommit hook
EdgeUsesModule        // repo → terraform module
EdgeIncludesSubmodule // repo → git submodule
EdgeUsesTool          // repo → dev tool
EdgeBuiltWith         // repo → build tool dependency
```

Add cases to `EdgeType.String()`:
```go
case EdgeUsesHook:
    return "uses_hook"
case EdgeUsesModule:
    return "uses_module"
case EdgeIncludesSubmodule:
    return "includes_submodule"
case EdgeUsesTool:
    return "uses_tool"
case EdgeBuiltWith:
    return "built_with"
```

Add `PinningSemverRange` to PinningQuality const block between `PinningExactVersion` and `PinningMajorTag`:
```go
PinningSemverRange // semver range constraint (^4.0.0, ~1.2.3)
```

Add case to `PinningQuality.String()`:
```go
case PinningSemverRange:
    return "semver_range"
```

Add `ProjectID` and `VersionKey` fields to `Node` struct:
```go
ProjectID  string // FK → cache projects.id
VersionKey string // FK → cache project_versions.version_key
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go test ./internal/graph/ -v`
Expected: all tests pass (including existing ones)

- [ ] **Step 5: Fix existing Node construction sites**

Update `internal/graph/builder.go` `BuildFromScanResult` — add empty `ProjectID`/`VersionKey` to the Node literal (these get populated by the crawler, but existing code paths must compile):
```go
ProjectID:  "",
VersionKey: "",
```

Also check and update any Node construction in `internal/actions/scan.go` and `internal/server/store/sqlite.go`.

Run: `go build ./...`
Expected: compiles cleanly

- [ ] **Step 6: Commit**

```bash
git add internal/graph/types.go internal/graph/types_test.go internal/graph/builder.go
git commit -m "feat(graph): add new node/edge types, PinningSemverRange, cache fields on Node"
```

---

### Task 2: Create SQLite cache database

**Files:**
- Create: `internal/cache/db.go`
- Create: `internal/cache/db_test.go`

- [ ] **Step 1: Write tests for CacheDB**

Create `internal/cache/db_test.go`:
```go
package cache

import (
	"testing"
	"time"
)

func TestCacheDB_CreateAndGet(t *testing.T) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Insert project
	p := &Project{
		ID: "github.com/actions/checkout", Type: "repo",
		SourceURL: "https://github.com/actions/checkout",
		MaintainerCount: 5, Stars: 1000,
		OrgName: "actions", OrgType: "corporate",
	}
	if err := db.UpsertProject(p); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetProject("github.com/actions/checkout")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Stars != 1000 {
		t.Errorf("GetProject: got %+v, want stars=1000", got)
	}
}

func TestCacheDB_ProjectTTL(t *testing.T) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	p := &Project{
		ID: "github.com/old/repo", Type: "repo",
		LastFetched: time.Now().Add(-25 * time.Hour),
	}
	if err := db.UpsertProject(p); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetProject("github.com/old/repo")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil for expired project")
	}
}

func TestCacheDB_ProjectVersionImmutable(t *testing.T) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	p := &Project{ID: "pypi/requests", Type: "registry"}
	if err := db.UpsertProject(p); err != nil {
		t.Fatal(err)
	}

	pv := &ProjectVersion{
		ProjectID: "pypi/requests", VersionKey: "pypi/requests@2.31.0",
		Semver: "2.31.0",
	}
	if err := db.UpsertProjectVersion(pv); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetProjectVersion("pypi/requests", "pypi/requests@2.31.0")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Semver != "2.31.0" {
		t.Errorf("GetProjectVersion: got %+v", got)
	}
}

func TestCacheDB_VersionDependencies(t *testing.T) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Setup parent and child projects
	db.UpsertProject(&Project{ID: "npm/lodash", Type: "registry"})
	db.UpsertProject(&Project{ID: "npm/lodash.merge", Type: "registry"})
	db.UpsertProjectVersion(&ProjectVersion{ProjectID: "npm/lodash", VersionKey: "npm/lodash@4.17.21"})
	db.UpsertProjectVersion(&ProjectVersion{ProjectID: "npm/lodash.merge", VersionKey: "npm/lodash.merge@4.6.2"})

	err = db.AddVersionDependency("npm/lodash", "npm/lodash@4.17.21", "npm/lodash.merge", "npm/lodash.merge@4.6.2", "depends_on")
	if err != nil {
		t.Fatal(err)
	}

	deps, err := db.GetVersionDependencies("npm/lodash", "npm/lodash@4.17.21")
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].ChildProjectID != "npm/lodash.merge" {
		t.Errorf("GetVersionDependencies: got %+v", deps)
	}
}

func TestCacheDB_FindDependents(t *testing.T) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.UpsertProject(&Project{ID: "npm/myapp", Type: "registry"})
	db.UpsertProject(&Project{ID: "npm/lodash", Type: "registry"})
	db.UpsertProjectVersion(&ProjectVersion{ProjectID: "npm/myapp", VersionKey: "npm/myapp@1.0.0"})
	db.UpsertProjectVersion(&ProjectVersion{ProjectID: "npm/lodash", VersionKey: "npm/lodash@4.17.21"})
	db.AddVersionDependency("npm/myapp", "npm/myapp@1.0.0", "npm/lodash", "npm/lodash@4.17.21", "depends_on")

	parents, err := db.FindDependents("npm/lodash")
	if err != nil {
		t.Fatal(err)
	}
	if len(parents) != 1 || parents[0].ParentProjectID != "npm/myapp" {
		t.Errorf("FindDependents: got %+v", parents)
	}
}

func TestCacheDB_CVECache(t *testing.T) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.SetCVECache("npm", "lodash", "4.17.20", `[{"id":"CVE-2021-23337"}]`); err != nil {
		t.Fatal(err)
	}

	findings, err := db.GetCVECache("npm", "lodash", "4.17.20")
	if err != nil {
		t.Fatal(err)
	}
	if findings == "" {
		t.Error("expected cached CVE findings")
	}

	// Should return empty for expired (we can't easily test 6h TTL in unit test,
	// just verify the happy path works)
}

func TestCacheDB_Prune(t *testing.T) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.UpsertProject(&Project{ID: "npm/old", Type: "registry"})
	pv := &ProjectVersion{
		ProjectID: "npm/old", VersionKey: "npm/old@1.0.0",
		ScannedAt: time.Now().Add(-100 * 24 * time.Hour),
	}
	db.UpsertProjectVersion(pv)

	pruned, err := db.Prune(90 * 24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 1 {
		t.Errorf("Prune: got %d, want 1", pruned)
	}
}

func TestCacheDB_Status(t *testing.T) {
	db, err := NewCacheDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.UpsertProject(&Project{ID: "npm/a", Type: "registry"})
	db.UpsertProjectVersion(&ProjectVersion{ProjectID: "npm/a", VersionKey: "npm/a@1.0.0"})

	status, err := db.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Projects != 1 || status.Versions != 1 {
		t.Errorf("Status: got %+v", status)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go test ./internal/cache/ -run "TestCacheDB" -v`
Expected: compilation error — CacheDB types don't exist

- [ ] **Step 3: Implement CacheDB**

Create `internal/cache/db.go`:
```go
package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Project struct {
	ID              string
	Type            string
	SourceURL       string
	MaintainerCount int
	MaintainerNames []string
	Stars           int
	OrgName         string
	OrgType         string
	LastFetched     time.Time
}

type ProjectVersion struct {
	ProjectID  string
	VersionKey string
	Semver     string
	DepTypes   []string
	ScannedAt  time.Time
}

type VersionDep struct {
	ParentProjectID  string
	ParentVersionKey string
	ChildProjectID   string
	ChildVersionKey  string
	EdgeType         string
}

type CacheStatus struct {
	Projects     int
	Versions     int
	Dependencies int
	CVEEntries   int
}

type CacheDB struct {
	db  *sql.DB
	ttl time.Duration // project TTL, default 24h
}

func NewCacheDB(dsn string) (*CacheDB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	c := &CacheDB{db: db, ttl: 24 * time.Hour}
	if err := c.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return c, nil
}

func (c *CacheDB) Close() error { return c.db.Close() }

func (c *CacheDB) migrate() error {
	_, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS projects (
			id               TEXT PRIMARY KEY,
			type             TEXT NOT NULL,
			source_url       TEXT,
			maintainer_count INTEGER DEFAULT 0,
			maintainer_names TEXT DEFAULT '[]',
			stars            INTEGER DEFAULT 0,
			org_name         TEXT,
			org_type         TEXT,
			last_fetched     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS project_versions (
			project_id  TEXT NOT NULL REFERENCES projects(id),
			version_key TEXT NOT NULL,
			semver      TEXT,
			dep_types   TEXT DEFAULT '[]',
			scanned_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (project_id, version_key)
		);
		CREATE TABLE IF NOT EXISTS version_dependencies (
			parent_project_id  TEXT NOT NULL,
			parent_version_key TEXT NOT NULL,
			child_project_id   TEXT NOT NULL,
			child_version_key  TEXT NOT NULL,
			edge_type          TEXT NOT NULL,
			FOREIGN KEY (parent_project_id, parent_version_key) REFERENCES project_versions(project_id, version_key),
			FOREIGN KEY (child_project_id, child_version_key) REFERENCES project_versions(project_id, version_key)
		);
		CREATE INDEX IF NOT EXISTS idx_vd_parent ON version_dependencies(parent_project_id, parent_version_key);
		CREATE INDEX IF NOT EXISTS idx_vd_child ON version_dependencies(child_project_id, child_version_key);
		CREATE TABLE IF NOT EXISTS cve_cache (
			ecosystem  TEXT NOT NULL,
			name       TEXT NOT NULL,
			version    TEXT NOT NULL,
			findings   TEXT NOT NULL,
			fetched_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (ecosystem, name, version)
		);
	`)
	return err
}

func (c *CacheDB) UpsertProject(p *Project) error {
	if p.LastFetched.IsZero() {
		p.LastFetched = time.Now()
	}
	names, _ := json.Marshal(p.MaintainerNames)
	_, err := c.db.Exec(`
		INSERT INTO projects (id, type, source_url, maintainer_count, maintainer_names, stars, org_name, org_type, last_fetched)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type=excluded.type, source_url=excluded.source_url,
			maintainer_count=excluded.maintainer_count, maintainer_names=excluded.maintainer_names,
			stars=excluded.stars, org_name=excluded.org_name, org_type=excluded.org_type,
			last_fetched=excluded.last_fetched`,
		p.ID, p.Type, p.SourceURL, p.MaintainerCount, string(names), p.Stars, p.OrgName, p.OrgType, p.LastFetched)
	return err
}

func (c *CacheDB) GetProject(id string) (*Project, error) {
	row := c.db.QueryRow(`SELECT id, type, source_url, maintainer_count, maintainer_names, stars, org_name, org_type, last_fetched FROM projects WHERE id = ?`, id)
	p := &Project{}
	var namesJSON, orgName, orgType, sourceURL sql.NullString
	if err := row.Scan(&p.ID, &p.Type, &sourceURL, &p.MaintainerCount, &namesJSON, &p.Stars, &orgName, &orgType, &p.LastFetched); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if time.Since(p.LastFetched) > c.ttl {
		return nil, nil // expired
	}
	p.SourceURL = sourceURL.String
	p.OrgName = orgName.String
	p.OrgType = orgType.String
	if namesJSON.Valid {
		json.Unmarshal([]byte(namesJSON.String), &p.MaintainerNames)
	}
	return p, nil
}

func (c *CacheDB) UpsertProjectVersion(pv *ProjectVersion) error {
	if pv.ScannedAt.IsZero() {
		pv.ScannedAt = time.Now()
	}
	depTypes, _ := json.Marshal(pv.DepTypes)
	_, err := c.db.Exec(`
		INSERT OR IGNORE INTO project_versions (project_id, version_key, semver, dep_types, scanned_at)
		VALUES (?, ?, ?, ?, ?)`,
		pv.ProjectID, pv.VersionKey, pv.Semver, string(depTypes), pv.ScannedAt)
	return err
}

func (c *CacheDB) GetProjectVersion(projectID, versionKey string) (*ProjectVersion, error) {
	row := c.db.QueryRow(`SELECT project_id, version_key, semver, dep_types, scanned_at FROM project_versions WHERE project_id = ? AND version_key = ?`, projectID, versionKey)
	pv := &ProjectVersion{}
	var semver sql.NullString
	var depTypesJSON string
	if err := row.Scan(&pv.ProjectID, &pv.VersionKey, &semver, &depTypesJSON, &pv.ScannedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	pv.Semver = semver.String
	json.Unmarshal([]byte(depTypesJSON), &pv.DepTypes)
	return pv, nil
}

func (c *CacheDB) AddVersionDependency(parentPID, parentVK, childPID, childVK, edgeType string) error {
	_, err := c.db.Exec(`INSERT INTO version_dependencies (parent_project_id, parent_version_key, child_project_id, child_version_key, edge_type) VALUES (?, ?, ?, ?, ?)`,
		parentPID, parentVK, childPID, childVK, edgeType)
	return err
}

func (c *CacheDB) GetVersionDependencies(projectID, versionKey string) ([]VersionDep, error) {
	rows, err := c.db.Query(`SELECT parent_project_id, parent_version_key, child_project_id, child_version_key, edge_type FROM version_dependencies WHERE parent_project_id = ? AND parent_version_key = ?`, projectID, versionKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deps []VersionDep
	for rows.Next() {
		var d VersionDep
		if err := rows.Scan(&d.ParentProjectID, &d.ParentVersionKey, &d.ChildProjectID, &d.ChildVersionKey, &d.EdgeType); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, nil
}

func (c *CacheDB) FindDependents(childProjectID string) ([]VersionDep, error) {
	rows, err := c.db.Query(`SELECT DISTINCT parent_project_id, parent_version_key, child_project_id, child_version_key, edge_type FROM version_dependencies WHERE child_project_id = ?`, childProjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var deps []VersionDep
	for rows.Next() {
		var d VersionDep
		if err := rows.Scan(&d.ParentProjectID, &d.ParentVersionKey, &d.ChildProjectID, &d.ChildVersionKey, &d.EdgeType); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, nil
}

func (c *CacheDB) SetCVECache(ecosystem, name, version, findingsJSON string) error {
	_, err := c.db.Exec(`INSERT OR REPLACE INTO cve_cache (ecosystem, name, version, findings, fetched_at) VALUES (?, ?, ?, ?, ?)`,
		ecosystem, name, version, findingsJSON, time.Now())
	return err
}

func (c *CacheDB) GetCVECache(ecosystem, name, version string) (string, error) {
	row := c.db.QueryRow(`SELECT findings, fetched_at FROM cve_cache WHERE ecosystem = ? AND name = ? AND version = ?`, ecosystem, name, version)
	var findings string
	var fetchedAt time.Time
	if err := row.Scan(&findings, &fetchedAt); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	if time.Since(fetchedAt) > 6*time.Hour {
		return "", nil // expired
	}
	return findings, nil
}

func (c *CacheDB) Prune(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	// Delete deps first (FK), then versions, then orphan projects
	result, err := c.db.Exec(`
		DELETE FROM version_dependencies WHERE parent_project_id IN (
			SELECT project_id FROM project_versions WHERE scanned_at < ?
		)`, cutoff)
	if err != nil {
		return 0, err
	}
	result, err = c.db.Exec(`DELETE FROM project_versions WHERE scanned_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	pruned, _ := result.RowsAffected()
	// Clean up expired CVE cache too
	c.db.Exec(`DELETE FROM cve_cache WHERE fetched_at < ?`, time.Now().Add(-6*time.Hour))
	// Vacuum
	c.db.Exec(`VACUUM`)
	return int(pruned), nil
}

func (c *CacheDB) Status() (*CacheStatus, error) {
	s := &CacheStatus{}
	c.db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&s.Projects)
	c.db.QueryRow(`SELECT COUNT(*) FROM project_versions`).Scan(&s.Versions)
	c.db.QueryRow(`SELECT COUNT(*) FROM version_dependencies`).Scan(&s.Dependencies)
	c.db.QueryRow(`SELECT COUNT(*) FROM cve_cache`).Scan(&s.CVEEntries)
	return s, nil
}

func DefaultDBPath() string {
	return fmt.Sprintf("%s/depscope-cache.db", DefaultDir())
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go test ./internal/cache/ -run "TestCacheDB" -v`
Expected: all CacheDB tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/cache/db.go internal/cache/db_test.go
git commit -m "feat(cache): add SQLite CacheDB with projects, versions, dependencies, CVE tables"
```

---

### Task 3: Extend config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write test for new config fields**

Add to `internal/config/config_test.go`:
```go
func TestLoadFile_TrustedOrgs(t *testing.T) {
	// Write temp config with trusted_orgs
	dir := t.TempDir()
	path := filepath.Join(dir, "depscope.yaml")
	os.WriteFile(path, []byte(`
profile: enterprise
trusted_orgs:
  - github.com/my-company
  - gitlab.com/my-team
auth:
  github_token: ${GITHUB_TOKEN}
  gitlab_token: ${GITLAB_TOKEN}
concurrency:
  registry_workers: 15
  git_clone_workers: 5
  github_api_workers: 8
`), 0o644)

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TrustedOrgs) != 2 {
		t.Errorf("TrustedOrgs: got %d, want 2", len(cfg.TrustedOrgs))
	}
	if cfg.ConcurrencyConfig.RegistryWorkers != 15 {
		t.Errorf("RegistryWorkers: got %d, want 15", cfg.ConcurrencyConfig.RegistryWorkers)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go test ./internal/config/ -run "TestLoadFile_TrustedOrgs" -v`
Expected: compilation error — TrustedOrgs field doesn't exist

- [ ] **Step 3: Add new config fields**

In `internal/config/config.go`, add to Config struct:
```go
TrustedOrgs       []string
Auth              Auth
ConcurrencyConfig ConcurrencyConfig
```

Add new types:
```go
type Auth struct {
	GitHubToken    string
	GitLabToken    string
	TerraformToken string
	BitbucketToken string
}

type ConcurrencyConfig struct {
	RegistryWorkers  int // default 10
	GitCloneWorkers  int // default 3
	GitHubAPIWorkers int // default 5
}

func DefaultConcurrency() ConcurrencyConfig {
	return ConcurrencyConfig{RegistryWorkers: 10, GitCloneWorkers: 3, GitHubAPIWorkers: 5}
}
```

In `LoadFile`, after existing config loading, add:
```go
cfg.TrustedOrgs = v.GetStringSlice("trusted_orgs")
cfg.Auth = Auth{
	GitHubToken:    ResolveEnv(v.GetString("auth.github_token")),
	GitLabToken:    ResolveEnv(v.GetString("auth.gitlab_token")),
	TerraformToken: ResolveEnv(v.GetString("auth.terraform_token")),
	BitbucketToken: ResolveEnv(v.GetString("auth.bitbucket_token")),
}
cfg.ConcurrencyConfig = DefaultConcurrency()
if v.IsSet("concurrency.registry_workers") {
	cfg.ConcurrencyConfig.RegistryWorkers = v.GetInt("concurrency.registry_workers")
}
if v.IsSet("concurrency.git_clone_workers") {
	cfg.ConcurrencyConfig.GitCloneWorkers = v.GetInt("concurrency.git_clone_workers")
}
if v.IsSet("concurrency.github_api_workers") {
	cfg.ConcurrencyConfig.GitHubAPIWorkers = v.GetInt("concurrency.github_api_workers")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go test ./internal/config/ -v`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add TrustedOrgs, Auth, ConcurrencyConfig"
```

---

## Phase 2: Crawler Core

### Task 4: Create crawler types and resolver interface

**Files:**
- Create: `internal/crawler/types.go`
- Create: `internal/crawler/resolver.go`

- [ ] **Step 1: Create types.go**

Create `internal/crawler/types.go` with: `FileTree`, `DepSourceType` enum, `DepRef`, `ResolvedDep`, `ProjectMeta`, `CrawlResult`, `CrawlStats`, `CrawlError`, `queueItem`, `CachedChild`.

All types as defined in the spec. `FileTree` is `map[string][]byte`.

- [ ] **Step 2: Create resolver.go**

Create `internal/crawler/resolver.go` with: `Resolver` interface (Detect + Resolve), `AuthProvider` interface.

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go build ./internal/crawler/...`
Expected: compiles

- [ ] **Step 4: Commit**

```bash
git add internal/crawler/
git commit -m "feat(crawler): add types, FileTree, Resolver interface"
```

---

### Task 5: Implement BFS crawler engine

**Files:**
- Create: `internal/crawler/crawler.go`
- Create: `internal/crawler/crawler_test.go`

- [ ] **Step 1: Write test with mock resolvers**

Create `internal/crawler/crawler_test.go`:
- `TestCrawler_SingleLevel` — root with 3 deps, verify graph has 3 nodes + 3 edges
- `TestCrawler_Dedup` — two deps point to same child, verify child scanned once
- `TestCrawler_MaxDepth` — chain of depth 5, maxDepth=3, verify stops at 3
- `TestCrawler_CacheHit` — pre-populate cache, verify no Resolve call for cached dep
- `TestCrawler_ErrorNode` — resolver returns error, verify error node in graph

Use a `mockResolver` that implements `Resolver` and records calls.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go test ./internal/crawler/ -v`
Expected: compilation error — Crawler doesn't exist

- [ ] **Step 3: Implement crawler.go**

Implement the BFS algorithm from the spec:
- `NewCrawler(cache *cache.CacheDB, resolvers map[DepSourceType]Resolver, opts CrawlerOptions) *Crawler`
- `(c *Crawler) Crawl(ctx context.Context, root FileTree) (*CrawlResult, error)`
- Level-by-level BFS with concurrent Resolve calls per level
- `seen` map with mutex protection
- Error nodes for failed resolutions
- Two queue item types (Contents vs CachedDeps)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go test ./internal/crawler/ -v`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add internal/crawler/crawler.go internal/crawler/crawler_test.go
git commit -m "feat(crawler): implement BFS engine with dedup, concurrency, error handling"
```

---

## Phase 3: Resolvers

### Task 6: PackageResolver (wraps existing manifest + registry)

**Files:**
- Create: `internal/crawler/resolvers/package.go`
- Create: `internal/crawler/resolvers/package_test.go`

- [ ] **Step 1: Write test**

Test `Detect` with a FileTree containing `package.json` + `package-lock.json` → returns DepRef list.
Test `Detect` with `go.mod` → returns Go package refs.
Test with empty FileTree → returns empty.

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement**

`Detect`: scan FileTree keys for known manifest filenames, call existing `manifest.ParserFor(eco).ParseFiles(fileMap)`, convert each `manifest.Package` to a `DepRef`.

`Resolve`: call existing `registry.FetchersByEcosystem` to get metadata, construct `ResolvedDep` with `ProjectID = "ecosystem/name"`, `VersionKey = "ecosystem/name@version"`, `Contents = nil` (registry packages don't produce a FileTree — their transitive deps come from registry API).

Special case: for packages with transitive deps from the registry, `Resolve` returns a `ResolvedDep` whose `Contents` is synthesized from the registry's dependency list (a virtual FileTree with just the manifest).

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/crawler/resolvers/
git commit -m "feat(resolvers): add PackageResolver wrapping manifest + registry"
```

---

### Task 7: ActionResolver (wraps existing actions package)

**Files:**
- Create: `internal/crawler/resolvers/action.go`
- Create: `internal/crawler/resolvers/action_test.go`

- [ ] **Step 1: Write test**

Test `Detect` with FileTree containing `.github/workflows/ci.yml` with `uses: actions/checkout@v4` → returns DepRef with `Source=DepSourceAction`, `Name="actions/checkout"`, `Ref="v4"`, `Pinning=PinningMajorTag`.

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement**

`Detect`: scan FileTree for `.github/workflows/*.yml` files, parse YAML, extract `uses:` references. Reuse existing `actions.ParseWorkflow` and `actions.ClassifyPinning`.

`Resolve`: call existing `actions.Resolver.ResolveRef` to get SHA, fetch action.yml contents via GitHub API. Return FileTree of the action's repo contents (for scanning bundled packages).

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/crawler/resolvers/action.go internal/crawler/resolvers/action_test.go
git commit -m "feat(resolvers): add ActionResolver wrapping actions package"
```

---

### Task 8: PrecommitResolver

**Files:**
- Create: `internal/crawler/resolvers/precommit.go`
- Create: `internal/crawler/resolvers/precommit_test.go`

- [ ] **Step 1: Write test**

Test `Detect` with FileTree containing `.pre-commit-config.yaml`:
```yaml
repos:
  - repo: https://github.com/pre-commit/mirrors-mypy
    rev: v1.8.0
    hooks:
      - id: mypy
  - repo: https://github.com/psf/black
    rev: 24.1.1
    hooks:
      - id: black
```
Expected: 2 DepRef entries with correct names, refs, and pinning quality.

Test with `rev` that looks like a SHA (40 hex chars) → `PinningSHA`.
Test with `rev: main` → `PinningBranch`.

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement**

`Detect`: look for `.pre-commit-config.yaml` in FileTree, parse YAML, extract `repos[].repo` + `repos[].rev`. Classify pinning: 40-char hex = SHA, semver pattern = ExactVersion, branch name = Branch.

`Resolve`: clone/fetch the repo at the given rev (use existing `resolve` package for git operations). Return FileTree of the repo contents so the crawler can find its own manifests (e.g., a pre-commit hook repo that has `requirements.txt`).

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/crawler/resolvers/precommit.go internal/crawler/resolvers/precommit_test.go
git commit -m "feat(resolvers): add PrecommitResolver for .pre-commit-config.yaml"
```

---

### Task 9: SubmoduleResolver

**Files:**
- Create: `internal/crawler/resolvers/submodule.go`
- Create: `internal/crawler/resolvers/submodule_test.go`

- [ ] **Step 1: Write test**

Test `Detect` with FileTree containing `.gitmodules`:
```
[submodule "vendor/lib"]
    path = vendor/lib
    url = https://github.com/vendor/lib.git
    branch = main
```
Expected: 1 DepRef with name `github.com/vendor/lib`, pinning based on whether branch is tracking or detached.

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement**

`Detect`: parse `.gitmodules` INI format, extract submodule URLs.
`Resolve`: fetch the submodule repo contents at the pinned SHA (submodules in git are always SHA-pinned in the tree, even if `.gitmodules` says branch). Return FileTree for recursive scanning.

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/crawler/resolvers/submodule.go internal/crawler/resolvers/submodule_test.go
git commit -m "feat(resolvers): add SubmoduleResolver for .gitmodules"
```

---

### Task 10: TerraformResolver

**Files:**
- Create: `internal/crawler/resolvers/terraform.go`
- Create: `internal/crawler/resolvers/terraform_test.go`

- [ ] **Step 1: Write test**

Test `Detect` with FileTree containing `main.tf`:
```hcl
module "consul" {
  source  = "hashicorp/consul/aws"
  version = "0.1.0"
}

module "custom" {
  source = "git::https://github.com/org/module.git?ref=v1.2.3"
}
```
Expected: 2 DepRef entries — one registry module, one git module.

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement**

`Detect`: scan `*.tf` files in FileTree for `module` blocks, extract `source` and `version`/`ref` attributes. Handle Terraform registry format (`namespace/name/provider`) and git URLs.

`Resolve`: for registry modules, query Terraform Registry API. For git modules, fetch repo contents. Return FileTree for recursive scanning (modules can have their own modules).

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/crawler/resolvers/terraform.go internal/crawler/resolvers/terraform_test.go
git commit -m "feat(resolvers): add TerraformResolver for *.tf module blocks"
```

---

### Task 11: ToolResolver

**Files:**
- Create: `internal/crawler/resolvers/tool.go`
- Create: `internal/crawler/resolvers/tool_test.go`

- [ ] **Step 1: Write test**

Test `Detect` with FileTree containing `.tool-versions`:
```
python 3.12.1
nodejs 20.11.0
terraform 1.7.3
```
And `.mise.toml`:
```toml
[tools]
python = "3.12.1"
```
Expected: DepRef entries for each tool with version.

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement**

`Detect`: parse `.tool-versions` (space-separated) and `.mise.toml` (TOML).
`Resolve`: returns `ResolvedDep` with `Contents = nil` (leaf nodes, no recursion). `ProjectID` based on tool name, `VersionKey` includes version.

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/crawler/resolvers/tool.go internal/crawler/resolvers/tool_test.go
git commit -m "feat(resolvers): add ToolResolver for .tool-versions and .mise.toml"
```

---

### Task 12: ScriptResolver + BuildToolResolver

**Files:**
- Create: `internal/crawler/resolvers/script.go`
- Create: `internal/crawler/resolvers/script_test.go`
- Create: `internal/crawler/resolvers/buildtool.go`
- Create: `internal/crawler/resolvers/buildtool_test.go`

- [ ] **Step 1: Write tests for ScriptResolver**

Test `Detect` with FileTree containing workflow YAML with `run: curl -sSL https://get.example.com | sh` and `run: wget -O- https://install.example.com | bash`.
Expected: DepRef entries for each URL.

- [ ] **Step 2: Write tests for BuildToolResolver**

Test `Detect` with FileTree containing `Makefile`:
```makefile
install:
	curl -sSL https://get.example.com | sh
	go install github.com/example/tool@v1.2.3
	pip install awscli
```
Expected: DepRef entries for detected install patterns.

Test with `Taskfile.yml` containing similar patterns.

- [ ] **Step 3: Run tests to verify they fail**

- [ ] **Step 4: Implement ScriptResolver**

`Detect`: regex scan for `curl.*\|.*sh`, `wget.*\|.*bash`, `curl.*-o`, `wget.*-O` patterns in workflow YAML `run:` blocks. Reuse patterns from existing `actions/scriptdetect.go`.
`Resolve`: return leaf node (URL as identity, no recursion).

- [ ] **Step 5: Implement BuildToolResolver**

`Detect`: scan Makefile/Taskfile/justfile contents for install patterns: `curl|sh`, `wget|bash`, `go install X@Y`, `pip install X`, `npm install -g X`. Best-effort heuristic.
`Resolve`: for `go install`/`pip install`/`npm install -g` → create package DepRef that feeds back into PackageResolver. For curl/wget → delegate to ScriptResolver pattern.

- [ ] **Step 6: Run tests to verify they pass**

- [ ] **Step 7: Commit**

```bash
git add internal/crawler/resolvers/script.go internal/crawler/resolvers/script_test.go internal/crawler/resolvers/buildtool.go internal/crawler/resolvers/buildtool_test.go
git commit -m "feat(resolvers): add ScriptResolver and BuildToolResolver (best-effort)"
```

---

## Phase 4: Post-Processing

### Task 13: Org detection and trust scoring

**Files:**
- Create: `internal/core/orgscore.go`
- Create: `internal/core/orgscore_test.go`

- [ ] **Step 1: Write tests**

- `TestClassifyOrg_OwnOrg` — project ID matches trusted org prefix → "own"
- `TestClassifyOrg_Corporate` — org with 10+ repos, 5+ members → "corporate"
- `TestClassifyOrg_Individual` — user account → "individual"
- `TestApplyOrgTrust_Own` — score floor of 80 applied
- `TestApplyOrgTrust_Corporate` — org backing factor boosted

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement**

`ClassifyOrg(projectID string, trustedOrgs []string, repoInfo *vcs.RepoInfo) string` — returns org type.
`ApplyOrgTrust(node *graph.Node, orgType string, scoreCfg config.Config)` — adjusts node score based on org type.

Known corporate orgs seed list: google, microsoft, hashicorp, aws, facebook/meta, apple, vercel, cloudflare, etc.

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/core/orgscore.go internal/core/orgscore_test.go
git commit -m "feat(core): add org classification and trust scoring"
```

---

### Task 14: CVE post-processing pass

**Files:**
- Create: `internal/crawler/cvepass.go`
- Create: `internal/crawler/cvepass_test.go`

- [ ] **Step 1: Write tests**

- `TestCVEPass_RegistryPackage` — node with semver gets CVE lookup
- `TestCVEPass_GitPinnedWithSemver` — node with SHA + semver tag gets CVE lookup
- `TestCVEPass_NoSemver` — node without semver gets `cve_status=unchecked`
- `TestCVEPass_CacheHit` — cached CVE result is reused

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement**

`RunCVEPass(ctx context.Context, g *graph.Graph, cacheDB *cache.CacheDB, osvClient *vuln.OSVClient) []CrawlError`

Walk all nodes. For nodes with semver: check CVE cache → if miss, batch query OSV → store in cache → attach findings to node metadata + apply CVE penalty to score.

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/crawler/cvepass.go internal/crawler/cvepass_test.go
git commit -m "feat(crawler): add CVE post-processing pass with cache"
```

---

## Phase 5: Scanner Integration

### Task 15: Wire crawler into ScanDir/ScanURL

**Files:**
- Modify: `internal/scanner/scanner.go`
- Modify: `internal/scanner/scanner_test.go`
- Modify: `cmd/depscope/scan.go`

- [ ] **Step 1: Write integration test**

Test that `ScanDir` on a directory with `go.mod` + `.github/workflows/ci.yml` + `.pre-commit-config.yaml` produces a graph with package, action, and precommit nodes.

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Refactor ScanDir**

Replace the current ecosystem-detection + separate actions scan with:
1. Build `FileTree` from local directory (walk fs, read only manifest/config files)
2. Create `Crawler` with all resolvers
3. Call `crawler.Crawl(ctx, fileTree)`
4. Run CVE post-processing pass
5. Run org detection pass
6. Convert `CrawlResult.Graph` → `core.ScanResult` for backward compatibility

Keep `ScanURL` working the same way but build FileTree from resolved remote files.

- [ ] **Step 4: Run all existing tests to verify nothing breaks**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go test ./... -v`
Expected: all existing tests still pass (may need test fixture updates for new Node fields)

- [ ] **Step 5: Update scan command flags**

In `cmd/depscope/scan.go`, add flags for `--trusted-orgs`, `--depth` (default 25), `--timeout`.

- [ ] **Step 6: Commit**

```bash
git add internal/scanner/ cmd/depscope/scan.go
git commit -m "feat(scanner): wire crawler into ScanDir/ScanURL, preserve backward compat"
```

---

## Phase 6: Output

### Task 16: ASCII tree command

**Files:**
- Create: `internal/report/tree.go`
- Create: `internal/report/tree_test.go`
- Create: `cmd/depscope/tree_cmd.go`

- [ ] **Step 1: Write test for tree renderer**

Test `RenderTree` with a small graph (5 nodes, 3 levels) → verify Unicode tree output matches expected format with type badges, scores, and mutable ref markers.

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement tree renderer**

`RenderTree(g *graph.Graph, opts TreeOptions) string`

Options: `MaxDepth`, `TypeFilter []graph.NodeType`, `RiskFilter []core.RiskLevel`, `CollapseDepth int`, `JSON bool`.

Walk graph BFS from root, render Unicode box-drawing characters (├── └── │). Each line: `[type] name@version ●score ⚡MUTABLE?`

Summary line at bottom.

- [ ] **Step 4: Implement tree command**

Create `cmd/depscope/tree_cmd.go` with Cobra command. Flags: `--depth`, `--type`, `--risk`, `--collapse`, `--json`. Runs scan, then renders tree.

- [ ] **Step 5: Run tests to verify they pass**

- [ ] **Step 6: Register command in main.go**

Add `rootCmd.AddCommand(treeCmd)` in `cmd/depscope/main.go`.

- [ ] **Step 7: Commit**

```bash
git add internal/report/tree.go internal/report/tree_test.go cmd/depscope/tree_cmd.go cmd/depscope/main.go
git commit -m "feat: add 'depscope tree' command for ASCII dependency tree output"
```

---

### Task 17: TUI tree walker

**Files:**
- Create: `internal/tui/walker.go`
- Modify: `internal/tui/model.go`

- [ ] **Step 1: Implement walker view**

Create `internal/tui/walker.go` — a Bubbletea component that:
- Shows current node's children as a selectable list
- Each item shows: `[type] name@version  score  ⚡?`
- Enter drills into selected child's children
- Backspace goes up to parent
- `i` opens inspect panel (reuse existing `internal/tui/inspect.go`)
- `/` activates search, `f` activates filter
- Color-coded by risk level (reuse existing `internal/tui/styles.go`)

- [ ] **Step 2: Wire into TUI model**

Modify `internal/tui/model.go` to add a walker view mode alongside existing tree/flat/graph views. Add keybinding to switch to walker mode (e.g., `w`).

- [ ] **Step 3: Manual test**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go run ./cmd/depscope explore .`
Verify walker mode works, can drill in/out, shows correct data.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/walker.go internal/tui/model.go
git commit -m "feat(tui): add tree walker view for interactive dependency browsing"
```

---

### Task 18: Web UI enhancements

**Files:**
- Modify: `internal/web/static/graph.js`
- Modify: `internal/web/templates/graph.html`
- Modify: `internal/web/static/graph.css`

- [ ] **Step 1: Add node type clustering**

In `graph.js`, modify the D3 force simulation to group nodes by type. Use `d3.forceCluster()` or manual force to pull same-type nodes together. Color-code by type.

- [ ] **Step 2: Add filter panel**

In `graph.html`, add a sidebar with:
- Checkboxes for each node type (toggle visibility)
- Risk level filter (dropdown or checkboxes)
- Org type filter (own/corporate/individual)
- Text search box
- Depth slider (1-25, default 3)

Wire filters to D3 — hide/show nodes and rerun simulation.

- [ ] **Step 3: Add node detail sidebar**

Click a node → sidebar shows: project metadata (stars, maintainers, org), version info (SHA, semver, pinning), CVE findings, score breakdown, direct children list.

- [ ] **Step 4: Add mutable refs tab**

Dedicated tab listing all mutable refs sorted by risk, with red highlighting for critical.

- [ ] **Step 5: Add semantic zoom**

Implement zoom behavior: zoomed out shows type clusters as labeled circles with count. Zoomed in shows individual nodes. Use D3 zoom transform to switch between views.

- [ ] **Step 6: Test in browser**

Start server: `go run ./cmd/depscope server --port 8080`
Scan a project, navigate to graph view, test all new features.

- [ ] **Step 7: Commit**

```bash
git add internal/web/
git commit -m "feat(web): enhance graph UI with clustering, filtering, sidebar, mutable refs tab"
```

---

## Phase 7: Discover Integration

### Task 19: Rework discover with cache-first query

**Files:**
- Modify: `internal/discover/discover.go`
- Modify: `internal/discover/discover_test.go`

- [ ] **Step 1: Write test for cache-first discover**

Test: pre-populate CacheDB with version_dependencies. Call `Discover("npm/lodash", "<4.17.21")`. Verify it returns affected projects from cache without file walking.

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement cache-first path**

Add `DiscoverFromCache(db *cache.CacheDB, pkg string, versionRange string) ([]DiscoverResult, error)`:
1. `db.FindDependents(pkg)` → get all parent project versions that depend on any version of `pkg`
2. For each, check if the child's semver falls in the vulnerable range
3. Walk backward through `version_dependencies` to reconstruct the full path

Fall back to existing file-walk approach if cache is empty for the queried package.

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git add internal/discover/
git commit -m "feat(discover): add cache-first query path via version_dependencies table"
```

---

## Phase 8: Cache Management

### Task 20: Cache prune command and status enhancements

**Files:**
- Modify: `cmd/depscope/cache_cmd.go`
- Modify: `cmd/depscope/cache_cmd_test.go`

- [ ] **Step 1: Write test for prune command**

Test that `cache prune --older-than 90d` calls `CacheDB.Prune` with correct duration.

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Implement prune subcommand**

Add `pruneCmd` to the cache command group:
```go
var pruneCmd = &cobra.Command{
    Use:   "prune",
    Short: "Remove old cached dependency data",
    RunE: func(cmd *cobra.Command, args []string) error {
        olderThan, _ := cmd.Flags().GetString("older-than")
        // parse duration (e.g., "90d" → 90*24h)
        // open CacheDB
        // call Prune
        // report results
    },
}
```

Update `status` subcommand to use `CacheDB.Status()` when SQLite cache exists (fall back to old DiskCache status for backward compat during transition).

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git add cmd/depscope/cache_cmd.go cmd/depscope/cache_cmd_test.go
git commit -m "feat(cache): add prune command and enhanced status for SQLite cache"
```

---

## Phase 9: Final Integration Test

### Task 21: End-to-end integration test

**Files:**
- Create: `internal/crawler/integration_test.go`

- [ ] **Step 1: Write integration test**

Create a test fixture directory with:
- `go.mod` with 2 dependencies
- `.github/workflows/ci.yml` with 2 actions (one SHA-pinned, one major tag)
- `.pre-commit-config.yaml` with 1 hook
- `.gitmodules` with 1 submodule URL
- `.tool-versions` with 2 tools
- `Makefile` with a `curl | sh` pattern

Run full `ScanDir` on this fixture. Verify:
- Correct number of nodes per type
- Dedup works (shared deps appear once)
- Mutable ref detection flags the major-tag action
- Cache is populated after scan
- Second scan of same fixture uses cache (faster, same result)

- [ ] **Step 2: Run the integration test**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go test ./internal/crawler/ -run "TestIntegration" -v -count=1`
Expected: pass

- [ ] **Step 3: Run full test suite**

Run: `cd /Users/jjverhoeks/src/tries/2026-03-20-supplychain-validation && go test ./... -v`
Expected: all tests pass

- [ ] **Step 4: Commit**

```bash
git add internal/crawler/integration_test.go
git commit -m "test: add end-to-end integration test for unified dependency tree"
```
