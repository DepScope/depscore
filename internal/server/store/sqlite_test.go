package store_test

import (
	"path/filepath"
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *store.SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSQLiteStoreCreateAndGet(t *testing.T) {
	db := newTestDB(t)
	err := db.Create("scan-1", store.ScanRequest{URL: "https://github.com/org/repo", Profile: "enterprise"})
	require.NoError(t, err)

	job, err := db.Get("scan-1")
	require.NoError(t, err)
	assert.Equal(t, "scan-1", job.ID)
	assert.Equal(t, "https://github.com/org/repo", job.URL)
	assert.Equal(t, "enterprise", job.Profile)
	assert.Equal(t, "queued", job.Status)
	assert.False(t, job.CreatedAt.IsZero())
}

func TestSQLiteStoreUpdateStatus(t *testing.T) {
	db := newTestDB(t)
	_ = db.Create("scan-1", store.ScanRequest{URL: "https://github.com/org/repo", Profile: "hobby"})

	err := db.UpdateStatus("scan-1", "running")
	require.NoError(t, err)

	job, err := db.Get("scan-1")
	require.NoError(t, err)
	assert.Equal(t, "running", job.Status)
}

func TestSQLiteStoreSaveResult(t *testing.T) {
	db := newTestDB(t)
	_ = db.Create("scan-1", store.ScanRequest{URL: "https://github.com/org/repo", Profile: "enterprise"})
	_ = db.UpdateStatus("scan-1", "running")

	result := &core.ScanResult{
		Profile:       "enterprise",
		PassThreshold: 70,
		DirectDeps:    2,
		Packages: []core.PackageResult{
			{
				Name:      "cobra",
				Version:   "v1.10.2",
				Ecosystem: "go",
				OwnScore:  85,
				OwnRisk:   core.RiskLow,
			},
		},
	}
	err := db.SaveResult("scan-1", result)
	require.NoError(t, err)

	job, err := db.Get("scan-1")
	require.NoError(t, err)
	assert.Equal(t, "complete", job.Status)
	require.NotNil(t, job.Result)
	assert.Equal(t, 2, job.Result.DirectDeps)
	assert.Equal(t, "enterprise", job.Result.Profile)
	require.Len(t, job.Result.Packages, 1)
	assert.Equal(t, "cobra", job.Result.Packages[0].Name)
}

func TestSQLiteStoreSaveError(t *testing.T) {
	db := newTestDB(t)
	_ = db.Create("scan-1", store.ScanRequest{URL: "https://github.com/org/repo", Profile: "enterprise"})
	_ = db.UpdateStatus("scan-1", "running")

	err := db.SaveError("scan-1", "something went wrong")
	require.NoError(t, err)

	job, err := db.Get("scan-1")
	require.NoError(t, err)
	assert.Equal(t, "failed", job.Status)
	assert.Equal(t, "something went wrong", job.Error)
}

func TestSQLiteStoreList(t *testing.T) {
	db := newTestDB(t)
	_ = db.Create("scan-1", store.ScanRequest{URL: "https://github.com/org/repo1", Profile: "enterprise"})
	_ = db.Create("scan-2", store.ScanRequest{URL: "https://github.com/org/repo2", Profile: "hobby"})
	_ = db.Create("scan-3", store.ScanRequest{URL: "https://github.com/org/repo3", Profile: "enterprise"})

	jobs := db.List()
	assert.Len(t, jobs, 3)

	// Verify all IDs are present.
	ids := map[string]bool{}
	for _, j := range jobs {
		ids[j.ID] = true
	}
	assert.True(t, ids["scan-1"])
	assert.True(t, ids["scan-2"])
	assert.True(t, ids["scan-3"])
}

func TestSQLiteStoreGraphRoundTrip(t *testing.T) {
	db := newTestDB(t)
	_ = db.Create("scan-1", store.ScanRequest{URL: "https://github.com/org/repo", Profile: "enterprise"})

	nodes := []store.GraphNode{
		{NodeID: "package:go/cobra@v1.10.2", Type: "package", Name: "cobra", Version: "v1.10.2", Score: 64, Risk: "MEDIUM"},
		{NodeID: "package:go/yaml@v3.0.1", Type: "package", Name: "yaml", Version: "v3.0.1", Score: 82, Risk: "LOW"},
	}
	edges := []store.GraphEdge{
		{From: "package:go/cobra@v1.10.2", To: "package:go/yaml@v3.0.1", Type: "depends_on", Depth: 1},
	}

	err := db.SaveGraph("scan-1", nodes, edges)
	require.NoError(t, err)

	loadedNodes, loadedEdges, err := db.LoadGraph("scan-1")
	require.NoError(t, err)
	assert.Len(t, loadedNodes, 2)
	assert.Len(t, loadedEdges, 1)

	// Verify node data round-trips.
	nodeByID := map[string]store.GraphNode{}
	for _, n := range loadedNodes {
		nodeByID[n.NodeID] = n
	}
	cobra := nodeByID["package:go/cobra@v1.10.2"]
	assert.Equal(t, "cobra", cobra.Name)
	assert.Equal(t, "v1.10.2", cobra.Version)
	assert.Equal(t, "package", cobra.Type)
	assert.Equal(t, 64, cobra.Score)
	assert.Equal(t, "MEDIUM", cobra.Risk)

	// Verify edge data round-trips.
	assert.Equal(t, "package:go/cobra@v1.10.2", loadedEdges[0].From)
	assert.Equal(t, "package:go/yaml@v3.0.1", loadedEdges[0].To)
	assert.Equal(t, "depends_on", loadedEdges[0].Type)
	assert.Equal(t, 1, loadedEdges[0].Depth)
}

func TestSQLiteStoreGraphMetadata(t *testing.T) {
	db := newTestDB(t)
	_ = db.Create("scan-1", store.ScanRequest{URL: "https://github.com/org/repo", Profile: "enterprise"})

	meta := map[string]any{
		"cves":       []any{"CVE-2024-1234", "CVE-2024-5678"},
		"maintainer": "someone",
		"downloads":  float64(42000),
	}
	nodes := []store.GraphNode{
		{
			NodeID:   "package:go/cobra@v1.10.2",
			Type:     "package",
			Name:     "cobra",
			Version:  "v1.10.2",
			Score:    64,
			Risk:     "MEDIUM",
			Metadata: meta,
		},
	}

	err := db.SaveGraph("scan-1", nodes, nil)
	require.NoError(t, err)

	loadedNodes, _, err := db.LoadGraph("scan-1")
	require.NoError(t, err)
	require.Len(t, loadedNodes, 1)

	loadedMeta := loadedNodes[0].Metadata
	require.NotNil(t, loadedMeta)
	assert.Equal(t, "someone", loadedMeta["maintainer"])
	assert.Equal(t, float64(42000), loadedMeta["downloads"])

	cves, ok := loadedMeta["cves"].([]any)
	require.True(t, ok, "cves should be []any")
	assert.Len(t, cves, 2)
	assert.Equal(t, "CVE-2024-1234", cves[0])
}

func TestSQLiteStoreGetNotFound(t *testing.T) {
	db := newTestDB(t)
	_, err := db.Get("does-not-exist")
	assert.Error(t, err)
}

func TestSQLiteStoreGraphEmptyScan(t *testing.T) {
	db := newTestDB(t)
	_ = db.Create("scan-1", store.ScanRequest{URL: "https://github.com/org/repo", Profile: "enterprise"})

	nodes, edges, err := db.LoadGraph("scan-1")
	require.NoError(t, err)
	assert.Empty(t, nodes)
	assert.Empty(t, edges)
}
