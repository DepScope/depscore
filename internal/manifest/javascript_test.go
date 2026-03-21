package manifest_test

import (
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJavaScriptParser(t *testing.T) {
	p := manifest.NewJavaScriptParser()
	pkgs, err := p.Parse("testdata/javascript")
	require.NoError(t, err)
	assert.Len(t, pkgs, 3)
	m := pkgMap(pkgs)
	assert.Equal(t, manifest.ConstraintMinor, m["express"].ConstraintType, "^4.18.2 = minor")
	assert.Equal(t, manifest.ConstraintExact, m["lodash"].ConstraintType, "4.17.21 = exact")
	assert.Equal(t, manifest.ConstraintMajor, m["axios"].ConstraintType, ">=1.0.0 = major")
	// Resolved versions come from lockfile
	assert.Equal(t, "1.6.5", m["axios"].ResolvedVersion)
}

func TestPnpmLockParser(t *testing.T) {
	p := manifest.NewJavaScriptParser()
	pkgs, err := p.Parse("testdata/javascript-pnpm")
	require.NoError(t, err)
	assert.Len(t, pkgs, 3)
	m := pkgMap(pkgs)
	assert.Equal(t, "4.18.2", m["express"].ResolvedVersion)
	assert.Equal(t, manifest.ConstraintMinor, m["express"].ConstraintType, "^4.18.2 = minor")
	assert.Equal(t, 1, m["express"].Depth, "express is a direct dep")
	assert.Equal(t, "1.3.8", m["accepts"].ResolvedVersion)
	assert.Equal(t, 2, m["accepts"].Depth, "accepts is transitive")
	assert.Equal(t, "4.17.21", m["lodash"].ResolvedVersion)
	assert.Equal(t, manifest.ConstraintExact, m["lodash"].ConstraintType, "4.17.21 = exact")
}

func TestBunLockParser(t *testing.T) {
	p := manifest.NewJavaScriptParser()
	pkgs, err := p.Parse("testdata/javascript-bun")
	require.NoError(t, err)
	assert.Len(t, pkgs, 2)
	m := pkgMap(pkgs)
	assert.Equal(t, "4.18.2", m["express"].ResolvedVersion)
	assert.Equal(t, manifest.ConstraintMinor, m["express"].ConstraintType, "^4.18.2 = minor")
	assert.Equal(t, 1, m["express"].Depth, "express is a direct dep")
	assert.Equal(t, "1.3.8", m["accepts"].ResolvedVersion)
	assert.Equal(t, 2, m["accepts"].Depth, "accepts is transitive")
}
