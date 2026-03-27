package resolvers

import (
	"context"
	"testing"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubmoduleResolver_Detect(t *testing.T) {
	r := NewSubmoduleResolver()
	tree := crawler.FileTree{
		".gitmodules": []byte(`[submodule "vendor/lib"]
	path = vendor/lib
	url = https://github.com/vendor/lib.git

[submodule "third_party/proto"]
	path = third_party/proto
	url = https://github.com/third-party/proto.git
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 2)

	assert.Equal(t, crawler.DepSourceSubmodule, refs[0].Source)
	assert.Equal(t, "vendor/lib", refs[0].Name)
	assert.Equal(t, "https://github.com/vendor/lib.git", refs[0].Ref)
	assert.Equal(t, graph.PinningBranch, refs[0].Pinning)

	assert.Equal(t, "third_party/proto", refs[1].Name)
	assert.Equal(t, "https://github.com/third-party/proto.git", refs[1].Ref)
}

func TestSubmoduleResolver_Detect_SSHUrl(t *testing.T) {
	r := NewSubmoduleResolver()
	tree := crawler.FileTree{
		".gitmodules": []byte(`[submodule "vendor/lib"]
	path = vendor/lib
	url = git@github.com:vendor/lib.git
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "git@github.com:vendor/lib.git", refs[0].Ref)
}

func TestSubmoduleResolver_Detect_NoFile(t *testing.T) {
	r := NewSubmoduleResolver()
	tree := crawler.FileTree{
		"README.md": []byte("# Hello"),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestSubmoduleResolver_Detect_EmptyFile(t *testing.T) {
	r := NewSubmoduleResolver()
	tree := crawler.FileTree{
		".gitmodules": []byte(""),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestSubmoduleResolver_Resolve_HTTPS(t *testing.T) {
	r := NewSubmoduleResolver()
	ref := crawler.DepRef{
		Source: crawler.DepSourceSubmodule,
		Name:   "vendor/lib",
		Ref:    "https://github.com/vendor/lib.git",
	}

	dep, err := r.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, "github.com/vendor/lib", dep.ProjectID)
	assert.Equal(t, "vendor/lib", dep.VersionKey)
	assert.Nil(t, dep.Contents)
}

func TestSubmoduleResolver_Resolve_SSH(t *testing.T) {
	r := NewSubmoduleResolver()
	ref := crawler.DepRef{
		Source: crawler.DepSourceSubmodule,
		Name:   "vendor/lib",
		Ref:    "git@github.com:vendor/lib.git",
	}

	dep, err := r.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, "github.com/vendor/lib", dep.ProjectID)
}

func TestSubmoduleResolver_ImplementsInterface(t *testing.T) {
	var _ crawler.Resolver = (*SubmoduleResolver)(nil)
}
