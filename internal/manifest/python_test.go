package manifest_test

import (
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
