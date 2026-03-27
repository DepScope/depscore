package resolvers

import (
	"context"
	"testing"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerraformResolver_Detect_RegistryModule(t *testing.T) {
	r := NewTerraformResolver()
	tree := crawler.FileTree{
		"main.tf": []byte(`
module "consul" {
  source  = "hashicorp/consul/aws"
  version = "0.1.0"
}
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)

	assert.Equal(t, crawler.DepSourceTerraform, refs[0].Source)
	assert.Equal(t, "hashicorp/consul/aws", refs[0].Name)
	assert.Equal(t, "0.1.0", refs[0].Ref)
	assert.Equal(t, graph.PinningExactVersion, refs[0].Pinning)
}

func TestTerraformResolver_Detect_GitModule(t *testing.T) {
	r := NewTerraformResolver()
	tree := crawler.FileTree{
		"main.tf": []byte(`
module "vpc" {
  source = "git::https://github.com/org/terraform-vpc.git?ref=v1.2.3"
}
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)

	assert.Equal(t, "git::https://github.com/org/terraform-vpc.git?ref=v1.2.3", refs[0].Name)
	assert.Equal(t, "v1.2.3", refs[0].Ref)
	assert.Equal(t, graph.PinningExactVersion, refs[0].Pinning)
}

func TestTerraformResolver_Detect_GitModuleSHA(t *testing.T) {
	r := NewTerraformResolver()
	tree := crawler.FileTree{
		"main.tf": []byte(`
module "vpc" {
  source = "git::https://github.com/org/module.git?ref=a5ac7e51b41094c92402da3b24376905380afc29"
}
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, graph.PinningSHA, refs[0].Pinning)
}

func TestTerraformResolver_Detect_MultipleModules(t *testing.T) {
	r := NewTerraformResolver()
	tree := crawler.FileTree{
		"main.tf": []byte(`
module "consul" {
  source  = "hashicorp/consul/aws"
  version = "0.1.0"
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.0.0"
}
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Len(t, refs, 2)
}

func TestTerraformResolver_Detect_LocalModule(t *testing.T) {
	r := NewTerraformResolver()
	tree := crawler.FileTree{
		"main.tf": []byte(`
module "local" {
  source = "./modules/local"
}
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, graph.PinningNA, refs[0].Pinning)
}

func TestTerraformResolver_Detect_RegistryNoVersion(t *testing.T) {
	r := NewTerraformResolver()
	tree := crawler.FileTree{
		"main.tf": []byte(`
module "test" {
  source = "hashicorp/consul/aws"
}
`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	assert.Equal(t, graph.PinningUnpinned, refs[0].Pinning)
}

func TestTerraformResolver_Detect_NonTFFile(t *testing.T) {
	r := NewTerraformResolver()
	tree := crawler.FileTree{
		"main.py": []byte(`module "test" { source = "test" }`),
	}

	refs, err := r.Detect(context.Background(), tree)
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestTerraformResolver_Detect_EmptyTree(t *testing.T) {
	r := NewTerraformResolver()
	refs, err := r.Detect(context.Background(), crawler.FileTree{})
	require.NoError(t, err)
	assert.Empty(t, refs)
}

func TestTerraformResolver_Resolve_Registry(t *testing.T) {
	r := NewTerraformResolver()
	ref := crawler.DepRef{
		Source: crawler.DepSourceTerraform,
		Name:   "hashicorp/consul/aws",
		Ref:    "0.1.0",
	}

	dep, err := r.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, "hashicorp/consul/aws", dep.ProjectID)
	assert.Equal(t, "0.1.0", dep.VersionKey)
	assert.Nil(t, dep.Contents)
}

func TestTerraformResolver_Resolve_Git(t *testing.T) {
	r := NewTerraformResolver()
	ref := crawler.DepRef{
		Source: crawler.DepSourceTerraform,
		Name:   "git::https://github.com/org/terraform-vpc.git?ref=v1.2.3",
		Ref:    "v1.2.3",
	}

	dep, err := r.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, "github.com/org/terraform-vpc", dep.ProjectID)
	assert.Equal(t, "v1.2.3", dep.VersionKey)
}

func TestTerraformResolver_ImplementsInterface(t *testing.T) {
	var _ crawler.Resolver = (*TerraformResolver)(nil)
}
