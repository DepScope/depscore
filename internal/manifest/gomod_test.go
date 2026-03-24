package manifest_test

import (
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pkgMap(pkgs []manifest.Package) map[string]manifest.Package {
	m := make(map[string]manifest.Package, len(pkgs))
	for _, p := range pkgs {
		m[p.Name] = p
	}
	return m
}

func TestGoModParser(t *testing.T) {
	p := manifest.NewGoModParser()
	pkgs, err := p.Parse("testdata/go")
	require.NoError(t, err)
	assert.Len(t, pkgs, 5)

	m := pkgMap(pkgs)
	require.Contains(t, m, "github.com/spf13/cobra")
	assert.Equal(t, "v1.8.0", m["github.com/spf13/cobra"].ResolvedVersion)
	assert.Equal(t, manifest.ConstraintExact, m["github.com/spf13/cobra"].ConstraintType)
	assert.Equal(t, 1, m["github.com/spf13/cobra"].Depth)
	assert.Equal(t, 2, m["github.com/inconshreveable/mousetrap"].Depth)
}
