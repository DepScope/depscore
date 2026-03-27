package resolvers

import (
	"context"
	"testing"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolResolver_Detect_ToolVersions(t *testing.T) {
	r := NewToolResolver()
	tree := crawler.FileTree{
		".tool-versions": []byte(`# Tool versions
golang 1.22.0
nodejs 20.11.0
python 3.12.1
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 3)

	assert.Equal(t, crawler.DepSourceTool, refs[0].Source)
	assert.Equal(t, "golang", refs[0].Name)
	assert.Equal(t, "1.22.0", refs[0].Ref)
	assert.Equal(t, graph.PinningExactVersion, refs[0].Pinning)

	assert.Equal(t, "nodejs", refs[1].Name)
	assert.Equal(t, "20.11.0", refs[1].Ref)

	assert.Equal(t, "python", refs[2].Name)
	assert.Equal(t, "3.12.1", refs[2].Ref)
}

func TestToolResolver_Detect_ToolVersions_EmptyLines(t *testing.T) {
	r := NewToolResolver()
	tree := crawler.FileTree{
		".tool-versions": []byte(`
golang 1.22.0

nodejs 20.11.0
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Len(t, refs, 2)
}

func TestToolResolver_Detect_MiseToml(t *testing.T) {
	r := NewToolResolver()
	tree := crawler.FileTree{
		".mise.toml": []byte(`
[tools]
golang = "1.22.0"
nodejs = "20.11.0"
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 2)

	names := make(map[string]string)
	for _, ref := range refs {
		names[ref.Name] = ref.Ref
	}
	assert.Equal(t, "1.22.0", names["golang"])
	assert.Equal(t, "20.11.0", names["nodejs"])
}

func TestToolResolver_Detect_MiseToml_ArrayVersion(t *testing.T) {
	r := NewToolResolver()
	tree := crawler.FileTree{
		".mise.toml": []byte(`
[tools]
python = ["3.12.1", "3.11.7"]
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, "python", refs[0].Name)
	assert.Equal(t, "3.12.1", refs[0].Ref)
}

func TestToolResolver_Detect_BothFiles(t *testing.T) {
	r := NewToolResolver()
	tree := crawler.FileTree{
		".tool-versions": []byte(`golang 1.22.0`),
		".mise.toml":     []byte("[tools]\nnodejs = \"20.11.0\""),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Len(t, refs, 2)
}

func TestToolResolver_Detect_NoFiles(t *testing.T) {
	r := NewToolResolver()
	tree := crawler.FileTree{
		"README.md": []byte("# Hello"),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestToolResolver_Detect_MalformedToml(t *testing.T) {
	r := NewToolResolver()
	tree := crawler.FileTree{
		".mise.toml": []byte(`{not valid toml`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestToolResolver_Resolve(t *testing.T) {
	r := NewToolResolver()
	ref := crawler.DepRef{
		Source: crawler.DepSourceTool,
		Name:   "golang",
		Ref:    "1.22.0",
	}

	dep, err := r.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, "tool/golang", dep.ProjectID)
	assert.Equal(t, "golang@1.22.0", dep.VersionKey)
	assert.Nil(t, dep.Contents)
}

func TestToolResolver_ImplementsInterface(t *testing.T) {
	var _ crawler.Resolver = (*ToolResolver)(nil)
}
