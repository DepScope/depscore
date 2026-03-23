package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequirementsTxt(t *testing.T) {
	p := manifest.NewPythonParser()
	pkgs, err := p.ParseFile("testdata/python/requirements.txt")
	require.NoError(t, err)
	m := pkgMap(pkgs)
	assert.Equal(t, manifest.ConstraintExact, m["requests"].ConstraintType)
	assert.Equal(t, manifest.ConstraintMajor, m["urllib3"].ConstraintType)
	assert.Equal(t, manifest.ConstraintPatch, m["pytest"].ConstraintType)
}

func TestPoetryLock(t *testing.T) {
	p := manifest.NewPythonParser()
	pkgs, err := p.ParseFile("testdata/python/poetry.lock")
	require.NoError(t, err)
	m := pkgMap(pkgs)
	assert.Equal(t, "2.31.0", m["requests"].ResolvedVersion)
	assert.Contains(t, m["urllib3"].Parents, "requests")
}

func TestUVLock(t *testing.T) {
	p := manifest.NewPythonParser()
	pkgs, err := p.ParseFile("testdata/python/uv.lock")
	require.NoError(t, err)
	assert.Len(t, pkgs, 2)
}

func TestPythonParserMergesLockfileAndManifest(t *testing.T) {
	// Create a dir with BOTH poetry.lock AND requirements.txt
	dir := t.TempDir()

	// requirements.txt has wide constraints
	reqContent := "requests>=2.0\nurllib3~=2.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(reqContent), 0o644))

	// poetry.lock has pinned versions
	lockContent := `[[package]]
name = "requests"
version = "2.31.0"

[[package]]
name = "urllib3"
version = "2.0.7"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "poetry.lock"), []byte(lockContent), 0o644))

	p := manifest.NewPythonParser()
	pkgs, err := p.Parse(dir)
	require.NoError(t, err)

	m := pkgMap(pkgs)

	// Should use lockfile's resolved version
	assert.Equal(t, "2.31.0", m["requests"].ResolvedVersion)

	// Should use manifest's wide constraint for version_pinning factor
	assert.Equal(t, ">=2.0", m["requests"].Constraint)
	assert.Equal(t, manifest.ConstraintMajor, m["requests"].ConstraintType)

	assert.Equal(t, "~=2.0", m["urllib3"].Constraint)
	assert.Equal(t, manifest.ConstraintPatch, m["urllib3"].ConstraintType)
}
