package resolvers

import (
	"context"
	"testing"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageResolver_Detect_NPM(t *testing.T) {
	r := NewPackageResolver()
	tree := crawler.FileTree{
		"package.json": []byte(`{"name":"test","dependencies":{"lodash":"^4.17.21"}}`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)

	assert.Equal(t, crawler.DepSourcePackage, refs[0].Source)
	assert.Equal(t, "lodash", refs[0].Name)
	assert.Equal(t, "^4.17.21", refs[0].Ref)
	assert.Equal(t, "npm", refs[0].Ecosystem)
	assert.Equal(t, graph.PinningNA, refs[0].Pinning)
}

func TestPackageResolver_Detect_NPM_WithDevDeps(t *testing.T) {
	r := NewPackageResolver()
	tree := crawler.FileTree{
		"package.json": []byte(`{
			"name": "test",
			"dependencies": {"express": "^4.18.0"},
			"devDependencies": {"jest": "^29.0.0"}
		}`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Len(t, refs, 2)

	names := make(map[string]bool)
	for _, ref := range refs {
		names[ref.Name] = true
	}
	assert.True(t, names["express"])
	assert.True(t, names["jest"])
}

func TestPackageResolver_Detect_NoManifest(t *testing.T) {
	r := NewPackageResolver()
	tree := crawler.FileTree{
		"README.md": []byte("# Hello"),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestPackageResolver_Detect_MalformedJSON(t *testing.T) {
	r := NewPackageResolver()
	tree := crawler.FileTree{
		"package.json": []byte(`{not valid json`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestPackageResolver_Detect_EmptyTree(t *testing.T) {
	r := NewPackageResolver()
	refs, err := r.Detect(context.Background(), crawler.FileTree{})
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestPackageResolver_Resolve(t *testing.T) {
	r := NewPackageResolver()
	ref := crawler.DepRef{
		Source:    crawler.DepSourcePackage,
		Name:      "lodash",
		Ref:       "^4.17.21",
		Ecosystem: "npm",
	}

	dep, err := r.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, "npm/lodash", dep.ProjectID)
	assert.Equal(t, "npm/lodash@^4.17.21", dep.VersionKey)
	assert.Nil(t, dep.Contents)
}

func TestPackageResolver_ImplementsInterface(t *testing.T) {
	var _ crawler.Resolver = (*PackageResolver)(nil)
}
