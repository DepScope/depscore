// internal/actions/composite_test.go
package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractCompositeActionsNil(t *testing.T) {
	refs := ExtractCompositeActions(nil)
	assert.Nil(t, refs)
}

func TestExtractCompositeActionsNotComposite(t *testing.T) {
	ay := &ActionYAML{
		Runs: ActionYAMLRuns{Using: "node20"},
	}
	refs := ExtractCompositeActions(ay)
	assert.Nil(t, refs)
}

func TestExtractCompositeActionsNoSteps(t *testing.T) {
	ay := &ActionYAML{
		Runs: ActionYAMLRuns{Using: "composite"},
	}
	refs := ExtractCompositeActions(ay)
	assert.Nil(t, refs)
}

func TestExtractCompositeActionsOnlyRunSteps(t *testing.T) {
	ay := &ActionYAML{
		Runs: ActionYAMLRuns{
			Using: "composite",
			Steps: []struct {
				Uses string `yaml:"uses"`
				Run  string `yaml:"run"`
			}{
				{Run: "echo hello"},
				{Run: "npm test"},
			},
		},
	}
	refs := ExtractCompositeActions(ay)
	// No uses: entries — should return nil (not empty slice)
	assert.Nil(t, refs)
}

func TestExtractCompositeActionsWithUses(t *testing.T) {
	ay := &ActionYAML{
		Runs: ActionYAMLRuns{
			Using: "composite",
			Steps: []struct {
				Uses string `yaml:"uses"`
				Run  string `yaml:"run"`
			}{
				{Uses: "actions/checkout@v4"},
				{Run: "echo hello"}, // run-only step, no uses
				{Uses: "actions/setup-node@v4"},
				{Uses: "some-org/deploy@abc123def456abc123def456abc123def456abcdef"},
			},
		},
	}
	refs := ExtractCompositeActions(ay)
	require.Len(t, refs, 3)

	assert.Equal(t, ActionRef{Owner: "actions", Repo: "checkout", Ref: "v4"}, refs[0])
	assert.Equal(t, ActionRef{Owner: "actions", Repo: "setup-node", Ref: "v4"}, refs[1])
	assert.Equal(t, ActionRef{
		Owner: "some-org",
		Repo:  "deploy",
		Ref:   "abc123def456abc123def456abc123def456abcdef",
	}, refs[2])
}

func TestExtractCompositeActionsDockerAndLocal(t *testing.T) {
	ay := &ActionYAML{
		Runs: ActionYAMLRuns{
			Using: "composite",
			Steps: []struct {
				Uses string `yaml:"uses"`
				Run  string `yaml:"run"`
			}{
				{Uses: "docker://alpine:3.19"},
				{Uses: "./local-action"},
			},
		},
	}
	refs := ExtractCompositeActions(ay)
	require.Len(t, refs, 2)

	assert.Equal(t, "alpine:3.19", refs[0].DockerImage)
	assert.Equal(t, "./local-action", refs[1].LocalPath)
}
