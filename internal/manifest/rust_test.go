package manifest_test

import (
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRustParser(t *testing.T) {
	p := manifest.NewRustParser()
	pkgs, err := p.Parse("testdata/rust")
	require.NoError(t, err)
	// myapp is the root, serde + tokio + reqwest are deps
	m := pkgMap(pkgs)
	require.Contains(t, m, "serde")
	assert.Equal(t, "1.0.196", m["serde"].ResolvedVersion)
	assert.Equal(t, manifest.ConstraintMinor, m["serde"].ConstraintType, "bare 1.0 = minor in Cargo")
	assert.Equal(t, manifest.ConstraintExact, m["reqwest"].ConstraintType, "=0.11.23 = exact")
	assert.Equal(t, manifest.ConstraintMinor, m["tokio"].ConstraintType, "^1.35 = minor")
	// reqwest depends on tokio → tokio should list reqwest as a parent
	assert.Contains(t, m["tokio"].Parents, "reqwest")
}
