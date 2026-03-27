package resolvers

import (
	"context"
	"testing"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildToolResolver_Detect_GoInstall(t *testing.T) {
	r := NewBuildToolResolver()
	tree := crawler.FileTree{
		"Makefile": []byte(`.PHONY: tools
tools:
	go install golang.org/x/tools/cmd/goimports@v0.18.0
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.56.0
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 2)

	assert.Equal(t, crawler.DepSourceBuildTool, refs[0].Source)
	assert.Equal(t, "golang.org/x/tools/cmd/goimports", refs[0].Name)
	assert.Equal(t, "v0.18.0", refs[0].Ref)
	assert.Equal(t, graph.PinningExactVersion, refs[0].Pinning)

	assert.Equal(t, "github.com/golangci/golangci-lint/cmd/golangci-lint", refs[1].Name)
	assert.Equal(t, "v1.56.0", refs[1].Ref)
}

func TestBuildToolResolver_Detect_PipInstall(t *testing.T) {
	r := NewBuildToolResolver()
	tree := crawler.FileTree{
		"Makefile": []byte(`install:
	pip install black
	pip install mypy
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 2)

	assert.Equal(t, "black", refs[0].Name)
	assert.Equal(t, "latest", refs[0].Ref)
	assert.Equal(t, "python", refs[0].Ecosystem)
	assert.Equal(t, graph.PinningUnpinned, refs[0].Pinning)
}

func TestBuildToolResolver_Detect_NpmInstall(t *testing.T) {
	r := NewBuildToolResolver()
	tree := crawler.FileTree{
		"Makefile": []byte(`install:
	npm install -g typescript
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)

	assert.Equal(t, "typescript", refs[0].Name)
	assert.Equal(t, "latest", refs[0].Ref)
	assert.Equal(t, "npm", refs[0].Ecosystem)
}

func TestBuildToolResolver_Detect_CurlPipeSh(t *testing.T) {
	r := NewBuildToolResolver()
	tree := crawler.FileTree{
		"Makefile": []byte(`setup:
	curl -sSL https://get.example.com/install.sh | sh
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "https://get.example.com/install.sh", refs[0].Name)
}

func TestBuildToolResolver_Detect_Taskfile(t *testing.T) {
	r := NewBuildToolResolver()
	tree := crawler.FileTree{
		"Taskfile.yml": []byte(`version: "3"
tasks:
  tools:
    cmds:
      - go install github.com/swaggo/swag/cmd/swag@v1.16.3
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "github.com/swaggo/swag/cmd/swag", refs[0].Name)
}

func TestBuildToolResolver_Detect_Justfile(t *testing.T) {
	r := NewBuildToolResolver()
	tree := crawler.FileTree{
		"justfile": []byte(`tools:
    go install github.com/air-verse/air@v1.52.0
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "github.com/air-verse/air", refs[0].Name)
}

func TestBuildToolResolver_Detect_MixedPatterns(t *testing.T) {
	r := NewBuildToolResolver()
	tree := crawler.FileTree{
		"Makefile": []byte(`setup:
	go install golang.org/x/tools/cmd/goimports@v0.18.0
	pip install black
	npm install -g typescript
	curl -sSL https://get.example.com | sh
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Len(t, refs, 4)
}

func TestBuildToolResolver_Detect_DeduplicatesReferences(t *testing.T) {
	r := NewBuildToolResolver()
	tree := crawler.FileTree{
		"Makefile": []byte(`setup:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.56.0
lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.56.0
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Len(t, refs, 1)
}

func TestBuildToolResolver_Detect_SkipsNonBuildFiles(t *testing.T) {
	r := NewBuildToolResolver()
	tree := crawler.FileTree{
		"script.sh": []byte(`go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.56.0`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestBuildToolResolver_Detect_EmptyTree(t *testing.T) {
	r := NewBuildToolResolver()
	refs, err := r.Detect(context.Background(), crawler.FileTree{})
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestBuildToolResolver_Detect_SkipsPipFlags(t *testing.T) {
	r := NewBuildToolResolver()
	tree := crawler.FileTree{
		"Makefile": []byte(`install:
	pip install -r requirements.txt
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs) // -r is a flag, should be skipped
}

func TestBuildToolResolver_Resolve(t *testing.T) {
	r := NewBuildToolResolver()
	ref := crawler.DepRef{
		Source: crawler.DepSourceBuildTool,
		Name:   "golang.org/x/tools/cmd/goimports",
		Ref:    "v0.18.0",
	}

	dep, err := r.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, "golang.org/x/tools/cmd/goimports", dep.ProjectID)
	assert.Equal(t, "golang.org/x/tools/cmd/goimports@v0.18.0", dep.VersionKey)
	assert.Nil(t, dep.Contents)
}

func TestBuildToolResolver_ImplementsInterface(t *testing.T) {
	var _ crawler.Resolver = (*BuildToolResolver)(nil)
}
