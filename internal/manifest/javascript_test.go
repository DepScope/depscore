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
