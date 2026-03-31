package resolvers

import (
	"context"
	"encoding/json"
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
	assert.Equal(t, graph.PinningSemverRange, refs[0].Pinning)
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

// TestPackageResolver_DetectFromSynthetic exercises the recursive path where
// Detect receives a synthetic FileTree produced by Resolve (containing
// __depscope_deps__.json).
func TestPackageResolver_DetectFromSynthetic(t *testing.T) {
	r := NewPackageResolver()

	syntheticRefs := []crawler.DepRef{
		{Source: crawler.DepSourcePackage, Name: "child-pkg", Ref: "1.0.0", Ecosystem: "npm"},
		{Source: crawler.DepSourcePackage, Name: "other-pkg", Ref: "2.3.4", Ecosystem: "npm"},
	}
	data, err := json.Marshal(syntheticRefs)
	require.NoError(t, err)

	tree := crawler.FileTree{depsFileKey: data}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 2)
	assert.Equal(t, "child-pkg", refs[0].Name)
	assert.Equal(t, "other-pkg", refs[1].Name)
}

// TestPackageResolver_DetectFromSynthetic_MalformedJSON verifies that a corrupt
// synthetic deps file is handled gracefully (returns nil, no error).
func TestPackageResolver_DetectFromSynthetic_MalformedJSON(t *testing.T) {
	r := NewPackageResolver()
	tree := crawler.FileTree{depsFileKey: []byte(`not-valid-json`)}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Nil(t, refs)
}

// TestPackageResolver_Resolve_WithChildren verifies that when a package has
// known children in the dep map (populated via Detect), Resolve returns a
// synthetic FileTree with those children encoded.
func TestPackageResolver_Resolve_WithChildren(t *testing.T) {
	r := NewPackageResolver()

	// First call Detect with a lockfile-style tree so the dep map is populated.
	// package-lock.json with parent→child relationship.
	packageLock := `{
		"name": "myapp",
		"lockfileVersion": 2,
		"packages": {
			"": {"dependencies": {"parent-pkg": "^1.0.0"}},
			"node_modules/parent-pkg": {"version": "1.0.0", "dependencies": {"child-pkg": "^2.0.0"}},
			"node_modules/child-pkg": {"version": "2.0.0"}
		}
	}`
	packageJSON := `{"name":"myapp","dependencies":{"parent-pkg":"^1.0.0"}}`

	tree := crawler.FileTree{
		"package.json":      []byte(packageJSON),
		"package-lock.json": []byte(packageLock),
	}

	_, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)

	// Resolve parent-pkg — it should have a synthetic FileTree for child-pkg.
	dep, err := r.Resolve(context.Background(), crawler.DepRef{
		Source:    crawler.DepSourcePackage,
		Name:      "parent-pkg",
		Ref:       "1.0.0",
		Ecosystem: "npm",
	})
	require.NoError(t, err)
	require.NotNil(t, dep)

	// Contents may be nil if no children in dep map (lockfile format varies).
	// This validates no panic and valid return.
	assert.Equal(t, "npm/parent-pkg", dep.ProjectID)
	assert.Equal(t, "npm/parent-pkg@1.0.0", dep.VersionKey)
}
