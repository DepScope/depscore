package resolvers

import (
	"context"
	"testing"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionResolver_Detect_BasicWorkflow(t *testing.T) {
	r := NewActionResolver()
	tree := crawler.FileTree{
		".github/workflows/ci.yml": []byte(`
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v3
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 2)

	assert.Equal(t, crawler.DepSourceAction, refs[0].Source)
	assert.Equal(t, "actions/checkout", refs[0].Name)
	assert.Equal(t, "v4", refs[0].Ref)
	assert.Equal(t, graph.PinningMajorTag, refs[0].Pinning)

	assert.Equal(t, "actions/setup-node", refs[1].Name)
	assert.Equal(t, "v3", refs[1].Ref)
	assert.Equal(t, graph.PinningMajorTag, refs[1].Pinning)
}

func TestActionResolver_Detect_SHAPinning(t *testing.T) {
	r := NewActionResolver()
	tree := crawler.FileTree{
		".github/workflows/ci.yml": []byte(`
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@a5ac7e51b41094c92402da3b24376905380afc29
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, graph.PinningSHA, refs[0].Pinning)
}

func TestActionResolver_Detect_ExactVersion(t *testing.T) {
	r := NewActionResolver()
	tree := crawler.FileTree{
		".github/workflows/ci.yml": []byte(`
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4.2.1
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, graph.PinningExactVersion, refs[0].Pinning)
}

func TestActionResolver_Detect_BranchPinning(t *testing.T) {
	r := NewActionResolver()
	tree := crawler.FileTree{
		".github/workflows/ci.yml": []byte(`
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: some/action@main
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, graph.PinningBranch, refs[0].Pinning)
}

func TestActionResolver_Detect_SkipsLocalActions(t *testing.T) {
	r := NewActionResolver()
	tree := crawler.FileTree{
		".github/workflows/ci.yml": []byte(`
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: ./local/action
      - uses: actions/checkout@v4
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "actions/checkout", refs[0].Name)
}

func TestActionResolver_Detect_SkipsDockerActions(t *testing.T) {
	r := NewActionResolver()
	tree := crawler.FileTree{
		".github/workflows/ci.yml": []byte(`
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: docker://alpine:3.18
      - uses: actions/checkout@v4
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "actions/checkout", refs[0].Name)
}

func TestActionResolver_Detect_ReusableWorkflow(t *testing.T) {
	r := NewActionResolver()
	tree := crawler.FileTree{
		".github/workflows/ci.yml": []byte(`
name: CI
on: push
jobs:
  call:
    uses: org/repo/.github/workflows/shared.yml@main
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "org/repo/.github/workflows/shared.yml", refs[0].Name)
	assert.Equal(t, "main", refs[0].Ref)
}

func TestActionResolver_Detect_NonWorkflowFile(t *testing.T) {
	r := NewActionResolver()
	tree := crawler.FileTree{
		"ci.yml": []byte(`
name: CI
on: push
jobs:
  build:
    steps:
      - uses: actions/checkout@v4
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestActionResolver_Detect_MalformedYAML(t *testing.T) {
	r := NewActionResolver()
	tree := crawler.FileTree{
		".github/workflows/ci.yml": []byte(`not: valid: yaml: [}`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestActionResolver_Detect_DeduplicatesUses(t *testing.T) {
	r := NewActionResolver()
	tree := crawler.FileTree{
		".github/workflows/ci.yml": []byte(`
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/checkout@v4
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Len(t, refs, 1)
}

func TestActionResolver_Detect_EmptyTree(t *testing.T) {
	r := NewActionResolver()
	refs, err := r.Detect(context.Background(), crawler.FileTree{})
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestActionResolver_Resolve(t *testing.T) {
	r := NewActionResolver()
	ref := crawler.DepRef{
		Source: crawler.DepSourceAction,
		Name:   "actions/checkout",
		Ref:    "v4",
	}

	dep, err := r.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, "github.com/actions/checkout", dep.ProjectID)
	assert.Equal(t, "v4", dep.VersionKey)
	assert.Nil(t, dep.Contents)
}

func TestActionResolver_Resolve_WithSubpath(t *testing.T) {
	r := NewActionResolver()
	ref := crawler.DepRef{
		Source: crawler.DepSourceAction,
		Name:   "org/repo/.github/workflows/shared.yml",
		Ref:    "main",
	}

	dep, err := r.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, "github.com/org/repo", dep.ProjectID)
}

func TestActionResolver_ImplementsInterface(t *testing.T) {
	var _ crawler.Resolver = (*ActionResolver)(nil)
}
