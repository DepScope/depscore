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
	// Resolved versions come from package-lock.json
	assert.Equal(t, "4.18.2", m["express"].ResolvedVersion)
	assert.Equal(t, "4.17.21", m["lodash"].ResolvedVersion)
	assert.Equal(t, "1.6.5", m["axios"].ResolvedVersion)

	// Constraint types from package.json
	assert.Equal(t, manifest.ConstraintMinor, m["express"].ConstraintType) // ^4.18.2
	assert.Equal(t, manifest.ConstraintExact, m["lodash"].ConstraintType)  // 4.17.21
	assert.Equal(t, manifest.ConstraintMajor, m["axios"].ConstraintType)   // >=1.0.0
}
