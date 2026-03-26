package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/depscope/depscope/internal/core"
	_ "modernc.org/sqlite"
)

// SQLiteStore is a SQLite-backed implementation of GraphStore (which embeds ScanStore).
type SQLiteStore struct {
	db *sql.DB
}

// compile-time interface check
var _ GraphStore = (*SQLiteStore)(nil)

const schema = `
CREATE TABLE IF NOT EXISTS scans (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    profile TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    error TEXT DEFAULT '',
    result_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);

CREATE TABLE IF NOT EXISTS graph_nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id TEXT NOT NULL REFERENCES scans(id),
    node_id TEXT NOT NULL,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    version TEXT DEFAULT '',
    ref TEXT DEFAULT '',
    score INTEGER DEFAULT 0,
    risk TEXT DEFAULT '',
    pinning TEXT DEFAULT '',
    metadata TEXT DEFAULT '{}',
    UNIQUE(scan_id, node_id)
);

CREATE TABLE IF NOT EXISTS graph_edges (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id TEXT NOT NULL REFERENCES scans(id),
    from_node TEXT NOT NULL,
    to_node TEXT NOT NULL,
    type TEXT NOT NULL,
    depth INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_nodes_scan ON graph_nodes(scan_id);
CREATE INDEX IF NOT EXISTS idx_edges_scan ON graph_edges(scan_id);
`

// NewSQLiteStore opens or creates a SQLite database at dbPath, creates the
// schema tables if they don't exist, and enables WAL mode.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Enable WAL mode for concurrent reads.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Enable foreign keys.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Create schema.
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Create stores a new ScanJob with status "queued".
func (s *SQLiteStore) Create(id string, req ScanRequest) error {
	_, err := s.db.Exec(
		`INSERT INTO scans (id, url, profile, status, created_at) VALUES (?, ?, ?, 'queued', ?)`,
		id, req.URL, req.Profile, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("create scan %s: %w", id, err)
	}
	return nil
}

// UpdateStatus changes the status of an existing scan job.
func (s *SQLiteStore) UpdateStatus(id, status string) error {
	res, err := s.db.Exec(`UPDATE scans SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return fmt.Errorf("update status %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("job not found: %s", id)
	}
	return nil
}

// SaveResult serializes the result as JSON, stores it, and sets status to "complete".
func (s *SQLiteStore) SaveResult(id string, result *core.ScanResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE scans SET status = 'complete', result_json = ?, completed_at = ? WHERE id = ?`,
		string(data), now, id,
	)
	if err != nil {
		return fmt.Errorf("save result %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("job not found: %s", id)
	}
	return nil
}

// SaveError stores the error message and sets status to "failed".
func (s *SQLiteStore) SaveError(id, errMsg string) error {
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE scans SET status = 'failed', error = ?, completed_at = ? WHERE id = ?`,
		errMsg, now, id,
	)
	if err != nil {
		return fmt.Errorf("save error %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("job not found: %s", id)
	}
	return nil
}

// Get retrieves a scan job by ID, deserializing the result JSON if present.
func (s *SQLiteStore) Get(id string) (*ScanJob, error) {
	row := s.db.QueryRow(
		`SELECT id, url, profile, status, error, result_json, created_at FROM scans WHERE id = ?`,
		id,
	)

	var job ScanJob
	var resultJSON sql.NullString
	var errStr sql.NullString
	var createdAt string

	if err := row.Scan(&job.ID, &job.URL, &job.Profile, &job.Status, &errStr, &resultJSON, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("job not found: %s", id)
		}
		return nil, fmt.Errorf("get scan %s: %w", id, err)
	}

	if errStr.Valid {
		job.Error = errStr.String
	}

	if resultJSON.Valid && resultJSON.String != "" {
		var result core.ScanResult
		if err := json.Unmarshal([]byte(resultJSON.String), &result); err != nil {
			return nil, fmt.Errorf("unmarshal result for %s: %w", id, err)
		}
		job.Result = &result
	}

	if t, err := time.Parse("2006-01-02 15:04:05", createdAt); err == nil {
		job.CreatedAt = t
	} else if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		job.CreatedAt = t
	} else if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err == nil {
		job.CreatedAt = t
	}

	return &job, nil
}

// List returns all stored scan jobs.
func (s *SQLiteStore) List() []*ScanJob {
	rows, err := s.db.Query(
		`SELECT id, url, profile, status, error, result_json, created_at FROM scans ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var jobs []*ScanJob
	for rows.Next() {
		var job ScanJob
		var resultJSON sql.NullString
		var errStr sql.NullString
		var createdAt string

		if err := rows.Scan(&job.ID, &job.URL, &job.Profile, &job.Status, &errStr, &resultJSON, &createdAt); err != nil {
			continue
		}

		if errStr.Valid {
			job.Error = errStr.String
		}

		if resultJSON.Valid && resultJSON.String != "" {
			var result core.ScanResult
			if err := json.Unmarshal([]byte(resultJSON.String), &result); err == nil {
				job.Result = &result
			}
		}

		if t, err := time.Parse("2006-01-02 15:04:05", createdAt); err == nil {
			job.CreatedAt = t
		} else if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			job.CreatedAt = t
		} else if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err == nil {
			job.CreatedAt = t
		}

		jobs = append(jobs, &job)
	}
	return jobs
}

// SaveGraph batch-inserts graph nodes and edges for a scan within a transaction.
// Existing graph data for the scan is replaced.
func (s *SQLiteStore) SaveGraph(scanID string, nodes []GraphNode, edges []GraphEdge) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete existing graph data for this scan.
	if _, err := tx.Exec(`DELETE FROM graph_edges WHERE scan_id = ?`, scanID); err != nil {
		return fmt.Errorf("delete old edges: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM graph_nodes WHERE scan_id = ?`, scanID); err != nil {
		return fmt.Errorf("delete old nodes: %w", err)
	}

	// Insert nodes.
	nodeStmt, err := tx.Prepare(
		`INSERT INTO graph_nodes (scan_id, node_id, type, name, version, ref, score, risk, pinning, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare node insert: %w", err)
	}
	defer nodeStmt.Close()

	for _, n := range nodes {
		metaJSON, err := json.Marshal(n.Metadata)
		if err != nil {
			metaJSON = []byte("{}")
		}
		if _, err := nodeStmt.Exec(scanID, n.NodeID, n.Type, n.Name, n.Version, n.Ref, n.Score, n.Risk, n.Pinning, string(metaJSON)); err != nil {
			return fmt.Errorf("insert node %s: %w", n.NodeID, err)
		}
	}

	// Insert edges.
	edgeStmt, err := tx.Prepare(
		`INSERT INTO graph_edges (scan_id, from_node, to_node, type, depth) VALUES (?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("prepare edge insert: %w", err)
	}
	defer edgeStmt.Close()

	for _, e := range edges {
		if _, err := edgeStmt.Exec(scanID, e.From, e.To, e.Type, e.Depth); err != nil {
			return fmt.Errorf("insert edge %s->%s: %w", e.From, e.To, err)
		}
	}

	return tx.Commit()
}

// LoadGraph retrieves the graph nodes and edges for a scan, deserializing
// the metadata JSON on each node.
func (s *SQLiteStore) LoadGraph(scanID string) ([]GraphNode, []GraphEdge, error) {
	// Load nodes.
	nodeRows, err := s.db.Query(
		`SELECT node_id, type, name, version, ref, score, risk, pinning, metadata
		 FROM graph_nodes WHERE scan_id = ? ORDER BY id`,
		scanID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query nodes: %w", err)
	}
	defer nodeRows.Close()

	var nodes []GraphNode
	for nodeRows.Next() {
		var n GraphNode
		var metaJSON string
		if err := nodeRows.Scan(&n.NodeID, &n.Type, &n.Name, &n.Version, &n.Ref, &n.Score, &n.Risk, &n.Pinning, &metaJSON); err != nil {
			return nil, nil, fmt.Errorf("scan node row: %w", err)
		}
		if metaJSON != "" && metaJSON != "{}" {
			if err := json.Unmarshal([]byte(metaJSON), &n.Metadata); err != nil {
				n.Metadata = map[string]any{}
			}
		} else {
			n.Metadata = map[string]any{}
		}
		nodes = append(nodes, n)
	}
	if err := nodeRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("node rows: %w", err)
	}

	// Load edges.
	edgeRows, err := s.db.Query(
		`SELECT from_node, to_node, type, depth
		 FROM graph_edges WHERE scan_id = ? ORDER BY id`,
		scanID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query edges: %w", err)
	}
	defer edgeRows.Close()

	var edges []GraphEdge
	for edgeRows.Next() {
		var e GraphEdge
		if err := edgeRows.Scan(&e.From, &e.To, &e.Type, &e.Depth); err != nil {
			return nil, nil, fmt.Errorf("scan edge row: %w", err)
		}
		edges = append(edges, e)
	}
	if err := edgeRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("edge rows: %w", err)
	}

	return nodes, edges, nil
}
