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

	// Should have serde, tokio, reqwest (not myapp — it's the root package)
	m := pkgMap(pkgs)
	require.Len(t, pkgs, 3)

	assert.Equal(t, "1.0.196", m["serde"].ResolvedVersion)
	assert.Equal(t, "1.35.1", m["tokio"].ResolvedVersion)
	assert.Equal(t, "0.11.23", m["reqwest"].ResolvedVersion)

	// Constraint types from Cargo.toml
	assert.Equal(t, manifest.ConstraintMinor, m["serde"].ConstraintType)   // bare "1.0" = minor
	assert.Equal(t, manifest.ConstraintMinor, m["tokio"].ConstraintType)   // "^1.35" = minor
	assert.Equal(t, manifest.ConstraintExact, m["reqwest"].ConstraintType) // "=0.11.23" = exact

	// Parent relationships from Cargo.lock dependencies
	assert.Contains(t, m["serde"].Parents, "myapp")
	assert.Contains(t, m["tokio"].Parents, "myapp")
	assert.Contains(t, m["tokio"].Parents, "reqwest") // reqwest depends on tokio
}
