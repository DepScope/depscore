package resolvers

import (
	"context"
	"testing"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScriptResolver_Detect_CurlPipeSh(t *testing.T) {
	r := NewScriptResolver()
	tree := crawler.FileTree{
		"install.sh": []byte(`#!/bin/bash
curl -sSL https://get.example.com | sh
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)

	assert.Equal(t, crawler.DepSourceScript, refs[0].Source)
	assert.Equal(t, "https://get.example.com", refs[0].Name)
	assert.Equal(t, "https://get.example.com", refs[0].Ref)
	assert.Equal(t, graph.PinningUnpinned, refs[0].Pinning)
}

func TestScriptResolver_Detect_CurlPipeBash(t *testing.T) {
	r := NewScriptResolver()
	tree := crawler.FileTree{
		"install.sh": []byte(`curl -fsSL https://raw.githubusercontent.com/org/repo/install.sh | bash`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "https://raw.githubusercontent.com/org/repo/install.sh", refs[0].Name)
}

func TestScriptResolver_Detect_WgetPipeSh(t *testing.T) {
	r := NewScriptResolver()
	tree := crawler.FileTree{
		"setup.sh": []byte(`wget -qO- https://get.example.com/install | sh`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "https://get.example.com/install", refs[0].Name)
}

func TestScriptResolver_Detect_CurlOutput(t *testing.T) {
	r := NewScriptResolver()
	tree := crawler.FileTree{
		"setup.sh": []byte(`curl -sSL https://get.example.com/binary -o /usr/local/bin/tool`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "https://get.example.com/binary", refs[0].Name)
}

func TestScriptResolver_Detect_MultiplePatterns(t *testing.T) {
	r := NewScriptResolver()
	tree := crawler.FileTree{
		"install.sh": []byte(`#!/bin/bash
curl -sSL https://get.example.com | sh
wget -qO- https://other.example.com/install | bash
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Len(t, refs, 2)
}

func TestScriptResolver_Detect_DeduplicatesURLs(t *testing.T) {
	r := NewScriptResolver()
	tree := crawler.FileTree{
		"install.sh": []byte(`curl -sSL https://get.example.com | sh`),
		"setup.sh":   []byte(`curl -sSL https://get.example.com | bash`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Len(t, refs, 1)
}

func TestScriptResolver_Detect_NoMatches(t *testing.T) {
	r := NewScriptResolver()
	tree := crawler.FileTree{
		"script.sh": []byte(`#!/bin/bash
echo "hello world"
ls -la
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestScriptResolver_Detect_EmptyTree(t *testing.T) {
	r := NewScriptResolver()
	refs, err := r.Detect(context.Background(), crawler.FileTree{})
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestScriptResolver_Resolve(t *testing.T) {
	r := NewScriptResolver()
	ref := crawler.DepRef{
		Source: crawler.DepSourceScript,
		Name:   "https://get.example.com",
		Ref:    "https://get.example.com",
	}

	dep, err := r.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, "https://get.example.com", dep.ProjectID)
	assert.Equal(t, "https://get.example.com", dep.VersionKey)
	assert.Nil(t, dep.Contents)
}

func TestScriptResolver_ImplementsInterface(t *testing.T) {
	var _ crawler.Resolver = (*ScriptResolver)(nil)
}
