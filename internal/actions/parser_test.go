// internal/actions/parser_test.go
package actions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorkflow(t *testing.T) {
	yaml := `
name: CI
on: push
permissions:
  contents: read
  id-token: write
jobs:
  build:
    runs-on: ubuntu-latest
    container:
      image: node:20-alpine
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4.2.0
      - uses: some-org/deploy@abc123def456abc123def456abc123def456abcdef
      - uses: docker://alpine:3.19
      - uses: ./local-action
      - run: |
          curl -sSL https://install.example.com/setup.sh | bash
      - run: npm test
  reusable:
    uses: org/shared/.github/workflows/lint.yml@main
`
	wf, err := ParseWorkflow([]byte(yaml), ".github/workflows/ci.yml")
	require.NoError(t, err)

	assert.Equal(t, ".github/workflows/ci.yml", wf.Path)

	// Should find 7 action refs:
	// 5 step uses: checkout, setup-node, SHA-pinned, docker://alpine, ./local-action
	// 1 reusable workflow
	// 1 container image
	assert.Len(t, wf.Actions, 7)

	// Check first ref (checkout@v4)
	assert.Equal(t, "actions", wf.Actions[0].Owner)
	assert.Equal(t, "v4", wf.Actions[0].Ref)

	// Docker ref from step uses: at index 3
	assert.Equal(t, "alpine:3.19", wf.Actions[3].DockerImage)

	// Local ref at index 4
	assert.Equal(t, "./local-action", wf.Actions[4].LocalPath)

	// Reusable workflow at index 5
	assert.True(t, wf.Actions[5].IsReusableWorkflow())

	// Container image should be detected (stored as a docker ref in Actions list)
	hasContainerImage := false
	for _, a := range wf.Actions {
		if a.DockerImage == "node:20-alpine" {
			hasContainerImage = true
		}
	}
	assert.True(t, hasContainerImage)

	// Permissions
	assert.True(t, wf.Permissions.Defined)
	assert.Equal(t, "read", wf.Permissions.Scopes["contents"])
	assert.Equal(t, "write", wf.Permissions.Scopes["id-token"])

	// Run blocks
	assert.GreaterOrEqual(t, len(wf.RunBlocks), 2)
}

func TestParseWorkflowNoPermissions(t *testing.T) {
	yaml := `
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`
	wf, err := ParseWorkflow([]byte(yaml), "ci.yml")
	require.NoError(t, err)
	assert.False(t, wf.Permissions.Defined)
}

func TestParseWorkflowStringPermissions(t *testing.T) {
	yaml := `
name: CI
on: push
permissions: read-all
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`
	wf, err := ParseWorkflow([]byte(yaml), "ci.yml")
	require.NoError(t, err)
	assert.True(t, wf.Permissions.Defined)
	// String permissions set a special key
	assert.Equal(t, "read-all", wf.Permissions.Scopes["_all"])
}

func TestParseWorkflowDir(t *testing.T) {
	// Create a temp dir with .github/workflows/ fixture files
	tmpDir := t.TempDir()
	workflowDir := filepath.Join(tmpDir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0755))

	// Write a .yml fixture
	ciYAML := `
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`
	require.NoError(t, os.WriteFile(filepath.Join(workflowDir, "ci.yml"), []byte(ciYAML), 0644))

	// Write a .yaml fixture
	deployYAML := `
name: Deploy
on: workflow_dispatch
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/setup-node@v4
`
	require.NoError(t, os.WriteFile(filepath.Join(workflowDir, "deploy.yaml"), []byte(deployYAML), 0644))

	wfs, err := ParseWorkflowDir(tmpDir)
	require.NoError(t, err)
	assert.Len(t, wfs, 2)

	// Each workflow should have at least one action
	for _, wf := range wfs {
		assert.NotEmpty(t, wf.Actions)
		assert.NotEmpty(t, wf.Path)
	}
}

func TestParseWorkflowDirNoWorkflows(t *testing.T) {
	tmpDir := t.TempDir()
	wfs, err := ParseWorkflowDir(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, wfs)
}
