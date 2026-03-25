package actions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWorkflowYAML = `name: CI
on: push
permissions:
  contents: read
jobs:
  build:
    runs-on: ubuntu-latest
    container:
      image: node:20-alpine
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@abc123def456abc123def456abc123def456abcdef
      - uses: docker://alpine:3.19
      - run: |
          curl -sSL https://install.example.com/setup.sh | bash
      - run: npm test
`

func setupTestDir(t *testing.T, workflows map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	wfDir := filepath.Join(dir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(wfDir, 0o755))
	for name, content := range workflows {
		require.NoError(t, os.WriteFile(filepath.Join(wfDir, name), []byte(content), 0o644))
	}
	return dir
}

func setupMockGitHub(t *testing.T) *httptest.Server {
	t.Helper()

	actionYAML := `name: Checkout
runs:
  using: node20
  main: dist/index.js
`
	encodedActionYAML := base64.StdEncoding.EncodeToString([]byte(actionYAML))

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		// Tag resolution for actions/checkout@v4
		case r.URL.Path == "/repos/actions/checkout/git/ref/tags/v4":
			json.NewEncoder(w).Encode(map[string]any{
				"object": map[string]string{
					"sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				},
			})
		// action.yml for actions/checkout
		case r.URL.Path == "/repos/actions/checkout/contents/action.yml":
			json.NewEncoder(w).Encode(map[string]any{
				"content":  encodedActionYAML,
				"encoding": "base64",
			})
		// action.yml for actions/setup-node (already SHA-pinned, skip tag resolution)
		case r.URL.Path == "/repos/actions/setup-node/contents/action.yml":
			json.NewEncoder(w).Encode(map[string]any{
				"content":  encodedActionYAML,
				"encoding": "base64",
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestScanWorkflows(t *testing.T) {
	dir := setupTestDir(t, map[string]string{
		"ci.yml": testWorkflowYAML,
	})

	server := setupMockGitHub(t)
	defer server.Close()

	resolver := NewResolver("", WithBaseURL(server.URL))
	g := graph.New()

	err := ScanWorkflows(context.Background(), dir, resolver, g)
	require.NoError(t, err)

	// Should have at least one workflow node
	workflowNodes := g.NodesOfType(graph.NodeWorkflow)
	assert.GreaterOrEqual(t, len(workflowNodes), 1, "should have workflow nodes")

	// Should have action nodes for checkout and setup-node
	actionNodes := g.NodesOfType(graph.NodeAction)
	assert.GreaterOrEqual(t, len(actionNodes), 2, "should have action nodes for checkout and setup-node")

	// Should have docker_image nodes (alpine:3.19 from docker://, node:20-alpine from container)
	dockerNodes := g.NodesOfType(graph.NodeDockerImage)
	assert.GreaterOrEqual(t, len(dockerNodes), 2, "should have docker_image nodes")

	// Should have script_download node from curl|bash
	scriptNodes := g.NodesOfType(graph.NodeScriptDownload)
	assert.GreaterOrEqual(t, len(scriptNodes), 1, "should have script_download nodes")

	// Check that script download node has score 0
	for _, n := range scriptNodes {
		assert.Equal(t, 0, n.Score, "script downloads should score 0")
	}

	// Verify edges exist from workflow to actions
	wfNode := workflowNodes[0]
	neighbors := g.Neighbors(wfNode.ID)
	assert.GreaterOrEqual(t, len(neighbors), 1, "workflow should have outgoing edges")
}

func TestScanWorkflowsEmptyDir(t *testing.T) {
	dir := t.TempDir()
	wfDir := filepath.Join(dir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(wfDir, 0o755))

	resolver := NewResolver("")
	g := graph.New()

	err := ScanWorkflows(context.Background(), dir, resolver, g)
	require.NoError(t, err)

	// No workflow files = no nodes
	assert.Empty(t, g.Nodes)
}

func TestScanWorkflowsNoWorkflowDir(t *testing.T) {
	dir := t.TempDir()
	// Don't create .github/workflows/

	resolver := NewResolver("")
	g := graph.New()

	// Should not error, just produce empty results
	err := ScanWorkflows(context.Background(), dir, resolver, g)
	require.NoError(t, err)
	assert.Empty(t, g.Nodes)
}

func TestScanWorkflowsDockerOnly(t *testing.T) {
	yaml := `name: Docker
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: docker://python:3.12-slim
`
	dir := setupTestDir(t, map[string]string{
		"docker.yml": yaml,
	})

	resolver := NewResolver("")
	g := graph.New()

	err := ScanWorkflows(context.Background(), dir, resolver, g)
	require.NoError(t, err)

	// Should have docker_image node
	dockerNodes := g.NodesOfType(graph.NodeDockerImage)
	assert.GreaterOrEqual(t, len(dockerNodes), 1)

	// Verify the docker image was scored
	for _, n := range dockerNodes {
		assert.Greater(t, n.Score, 0, "docker images should have a non-zero score")
	}
}

func TestScanWorkflowsLocalActions(t *testing.T) {
	yaml := `name: Local
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: ./local-action
`
	dir := setupTestDir(t, map[string]string{
		"local.yml": yaml,
	})

	resolver := NewResolver("")
	g := graph.New()

	err := ScanWorkflows(context.Background(), dir, resolver, g)
	require.NoError(t, err)

	// Local actions are skipped (can't resolve remotely)
	actionNodes := g.NodesOfType(graph.NodeAction)
	assert.Empty(t, actionNodes, "local actions should be skipped")

	// But workflow node should still exist
	workflowNodes := g.NodesOfType(graph.NodeWorkflow)
	assert.Len(t, workflowNodes, 1)
}

func TestScanWorkflowsPermissions(t *testing.T) {
	// Workflow with broad permissions (write)
	yamlBroad := `name: Broad
on: push
permissions:
  contents: write
  id-token: write
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`
	// Workflow with narrow permissions (read-only)
	yamlNarrow := `name: Narrow
on: push
permissions:
  contents: read
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`
	dirBroad := setupTestDir(t, map[string]string{"broad.yml": yamlBroad})
	dirNarrow := setupTestDir(t, map[string]string{"narrow.yml": yamlNarrow})

	server := setupMockGitHub(t)
	defer server.Close()

	resolverBroad := NewResolver("", WithBaseURL(server.URL))
	resolverNarrow := NewResolver("", WithBaseURL(server.URL))

	gBroad := graph.New()
	gNarrow := graph.New()

	require.NoError(t, ScanWorkflows(context.Background(), dirBroad, resolverBroad, gBroad))
	require.NoError(t, ScanWorkflows(context.Background(), dirNarrow, resolverNarrow, gNarrow))

	// Both should have the checkout action node
	broadActions := gBroad.NodesOfType(graph.NodeAction)
	narrowActions := gNarrow.NodesOfType(graph.NodeAction)
	require.GreaterOrEqual(t, len(broadActions), 1)
	require.GreaterOrEqual(t, len(narrowActions), 1)

	// Action with broad perms should score lower than with narrow perms
	// (the permissions factor gives 30 for broad vs 90 for narrow)
	assert.Less(t, broadActions[0].Score, narrowActions[0].Score,
		"action in broad-permissions workflow should score lower")
}

func TestPinQualityToGraphPinning(t *testing.T) {
	assert.Equal(t, graph.PinningSHA, pinQualityToGraphPinning(PinSHA))
	assert.Equal(t, graph.PinningExactVersion, pinQualityToGraphPinning(PinExactVersion))
	assert.Equal(t, graph.PinningMajorTag, pinQualityToGraphPinning(PinMajorTag))
	assert.Equal(t, graph.PinningBranch, pinQualityToGraphPinning(PinBranch))
	assert.Equal(t, graph.PinningUnpinned, pinQualityToGraphPinning(PinUnpinned))
}

func TestDockerPinningQuality(t *testing.T) {
	assert.Equal(t, graph.PinningDigest, dockerPinningQuality(BaseImage{Image: "alpine", Digest: "sha256:abc"}))
	assert.Equal(t, graph.PinningExactVersion, dockerPinningQuality(BaseImage{Image: "node", Tag: "20-alpine"}))
	assert.Equal(t, graph.PinningUnpinned, dockerPinningQuality(BaseImage{Image: "alpine", Tag: "latest"}))
	assert.Equal(t, graph.PinningUnpinned, dockerPinningQuality(BaseImage{Image: "alpine"}))
}

func TestScanWorkflowsMultipleFiles(t *testing.T) {
	ci := `name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`
	deploy := `name: Deploy
on: push
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: docker://alpine:3.19
`
	dir := setupTestDir(t, map[string]string{
		"ci.yml":     ci,
		"deploy.yml": deploy,
	})

	server := setupMockGitHub(t)
	defer server.Close()

	resolver := NewResolver("", WithBaseURL(server.URL))
	g := graph.New()

	err := ScanWorkflows(context.Background(), dir, resolver, g)
	require.NoError(t, err)

	// Should have 2 workflow nodes
	workflowNodes := g.NodesOfType(graph.NodeWorkflow)
	assert.Len(t, workflowNodes, 2)

	// Should have at least 1 action node and 1 docker node
	actionNodes := g.NodesOfType(graph.NodeAction)
	assert.GreaterOrEqual(t, len(actionNodes), 1)

	dockerNodes := g.NodesOfType(graph.NodeDockerImage)
	assert.GreaterOrEqual(t, len(dockerNodes), 1)
}
