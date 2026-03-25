// internal/graph/types_test.go
package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeTypeString(t *testing.T) {
	assert.Equal(t, "package", NodePackage.String())
	assert.Equal(t, "repo", NodeRepo.String())
	assert.Equal(t, "action", NodeAction.String())
	assert.Equal(t, "workflow", NodeWorkflow.String())
	assert.Equal(t, "docker_image", NodeDockerImage.String())
	assert.Equal(t, "script_download", NodeScriptDownload.String())
}

func TestEdgeTypeString(t *testing.T) {
	assert.Equal(t, "depends_on", EdgeDependsOn.String())
	assert.Equal(t, "hosted_at", EdgeHostedAt.String())
	assert.Equal(t, "uses_action", EdgeUsesAction.String())
	assert.Equal(t, "bundles", EdgeBundles.String())
	assert.Equal(t, "triggers", EdgeTriggers.String())
	assert.Equal(t, "resolves_to", EdgeResolvesTo.String())
	assert.Equal(t, "pulls_image", EdgePullsImage.String())
	assert.Equal(t, "downloads", EdgeDownloads.String())
}

func TestPinningQualityString(t *testing.T) {
	assert.Equal(t, "sha", PinningSHA.String())
	assert.Equal(t, "exact_version", PinningExactVersion.String())
	assert.Equal(t, "major_tag", PinningMajorTag.String())
	assert.Equal(t, "branch", PinningBranch.String())
	assert.Equal(t, "unpinned", PinningUnpinned.String())
	assert.Equal(t, "digest", PinningDigest.String())
	assert.Equal(t, "n/a", PinningNA.String())
}

func TestNodeID(t *testing.T) {
	assert.Equal(t, "package:python/litellm@1.82.8", NodeID(NodePackage, "python/litellm@1.82.8"))
	assert.Equal(t, "action:actions/checkout@v4", NodeID(NodeAction, "actions/checkout@v4"))
	assert.Equal(t, "repo:github.com/pallets/flask", NodeID(NodeRepo, "github.com/pallets/flask"))
}
