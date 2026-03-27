package resolvers

import (
	"context"
	"testing"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrecommitResolver_Detect(t *testing.T) {
	r := NewPrecommitResolver()
	tree := crawler.FileTree{
		".pre-commit-config.yaml": []byte(`
repos:
  - repo: https://github.com/pre-commit/mirrors-mypy
    rev: v1.8.0
    hooks:
      - id: mypy
  - repo: https://github.com/psf/black
    rev: 24.1.1
    hooks:
      - id: black
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 2)

	assert.Equal(t, crawler.DepSourcePrecommit, refs[0].Source)
	assert.Equal(t, "https://github.com/pre-commit/mirrors-mypy", refs[0].Name)
	assert.Equal(t, "v1.8.0", refs[0].Ref)
	assert.Equal(t, graph.PinningExactVersion, refs[0].Pinning)

	assert.Equal(t, "https://github.com/psf/black", refs[1].Name)
	assert.Equal(t, "24.1.1", refs[1].Ref)
	assert.Equal(t, graph.PinningExactVersion, refs[1].Pinning)
}

func TestPrecommitResolver_Detect_SHAPinning(t *testing.T) {
	r := NewPrecommitResolver()
	tree := crawler.FileTree{
		".pre-commit-config.yaml": []byte(`
repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: a5ac7e51b41094c92402da3b24376905380afc29
    hooks:
      - id: trailing-whitespace
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, graph.PinningSHA, refs[0].Pinning)
}

func TestPrecommitResolver_Detect_SkipsLocalRepos(t *testing.T) {
	r := NewPrecommitResolver()
	tree := crawler.FileTree{
		".pre-commit-config.yaml": []byte(`
repos:
  - repo: local
    hooks:
      - id: my-hook
  - repo: meta
    hooks:
      - id: check-hooks-apply
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.5.0
    hooks:
      - id: trailing-whitespace
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "https://github.com/pre-commit/pre-commit-hooks", refs[0].Name)
}

func TestPrecommitResolver_Detect_NoFile(t *testing.T) {
	r := NewPrecommitResolver()
	tree := crawler.FileTree{
		"README.md": []byte("# Hello"),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestPrecommitResolver_Detect_MalformedYAML(t *testing.T) {
	r := NewPrecommitResolver()
	tree := crawler.FileTree{
		".pre-commit-config.yaml": []byte(`{not valid yaml`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestPrecommitResolver_Resolve(t *testing.T) {
	r := NewPrecommitResolver()
	ref := crawler.DepRef{
		Source: crawler.DepSourcePrecommit,
		Name:   "https://github.com/pre-commit/mirrors-mypy",
		Ref:    "v1.8.0",
	}

	dep, err := r.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, "github.com/pre-commit/mirrors-mypy", dep.ProjectID)
	assert.Equal(t, "v1.8.0", dep.VersionKey)
	assert.Nil(t, dep.Contents)
}

func TestPrecommitResolver_ImplementsInterface(t *testing.T) {
	var _ crawler.Resolver = (*PrecommitResolver)(nil)
}
