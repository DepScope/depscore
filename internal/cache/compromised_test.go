package cache

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompromisedFinding(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	f := &CompromisedFinding{
		ScanID:       "scan-001",
		ManifestPath: "apps/web/package.json",
		PackageName:  "axios",
		Version:      "1.14.1",
		Constraint:   "^1.14.0",
		Relation:     "direct",
		ParentChain:  "",
	}
	err = db.AddCompromisedFinding(f)
	require.NoError(t, err)

	findings, err := db.GetCompromisedFindings("scan-001")
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "axios", findings[0].PackageName)
	assert.Equal(t, "1.14.1", findings[0].Version)
	assert.Equal(t, "direct", findings[0].Relation)
}

func TestCompromisedFindingDedup(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	f := &CompromisedFinding{
		ScanID:       "scan-002",
		ManifestPath: "package.json",
		PackageName:  "axios",
		Version:      "1.14.1",
		Constraint:   "^1.14.0",
		Relation:     "direct",
	}
	require.NoError(t, db.AddCompromisedFinding(f))
	require.NoError(t, db.AddCompromisedFinding(f)) // duplicate

	findings, err := db.GetCompromisedFindings("scan-002")
	require.NoError(t, err)
	assert.Len(t, findings, 1) // deduped
}
