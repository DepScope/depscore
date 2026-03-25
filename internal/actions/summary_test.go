package actions

import (
	"testing"

	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
)

func TestComputePinningSummary_Empty(t *testing.T) {
	g := graph.New()
	s := ComputePinningSummary(g)
	assert.Equal(t, PinningSummary{}, s)
}

func TestComputePinningSummary_NoActions(t *testing.T) {
	g := graph.New()
	g.AddNode(&graph.Node{
		ID:   "package:python/flask@3.0.0",
		Type: graph.NodePackage,
		Name: "flask",
	})
	s := ComputePinningSummary(g)
	assert.Equal(t, 0, s.Total)
	assert.Equal(t, 0, s.ScriptDownloads)
}

func TestComputePinningSummary_PinningCounts(t *testing.T) {
	g := graph.New()

	addAction := func(id string, pinning graph.PinningQuality, firstParty bool) {
		meta := map[string]any{"first_party": firstParty}
		g.AddNode(&graph.Node{
			ID:       id,
			Type:     graph.NodeAction,
			Pinning:  pinning,
			Metadata: meta,
		})
	}

	// SHA-pinned actions (2)
	addAction("action:actions/checkout@abc123", graph.PinningSHA, true)
	addAction("action:actions/setup-node@def456", graph.PinningSHA, true)

	// Exact version (1)
	addAction("action:some-org/deploy@v1.2.3", graph.PinningExactVersion, false)

	// Major tag (1)
	addAction("action:some-org/release@v2", graph.PinningMajorTag, false)

	// Branch (1)
	addAction("action:some-org/nightly@main", graph.PinningBranch, false)

	// Unpinned (1)
	addAction("action:some-org/noref@", graph.PinningUnpinned, false)

	// Script downloads (2)
	g.AddNode(&graph.Node{ID: "script_download:1", Type: graph.NodeScriptDownload})
	g.AddNode(&graph.Node{ID: "script_download:2", Type: graph.NodeScriptDownload})

	s := ComputePinningSummary(g)

	assert.Equal(t, 6, s.Total)
	assert.Equal(t, 2, s.SHAPinned)
	assert.Equal(t, 1, s.ExactVersion)
	assert.Equal(t, 1, s.MajorTag)
	assert.Equal(t, 1, s.Branch)
	assert.Equal(t, 1, s.Unpinned)
	assert.Equal(t, 2, s.FirstParty)
	assert.Equal(t, 4, s.ThirdParty)
	assert.Equal(t, 2, s.ScriptDownloads)
}

func TestComputePinningSummary_NoFirstPartyMetadata(t *testing.T) {
	// Nodes without first_party metadata should count as third-party.
	g := graph.New()
	g.AddNode(&graph.Node{
		ID:      "action:some-org/tool@v1",
		Type:    graph.NodeAction,
		Pinning: graph.PinningMajorTag,
		// no Metadata
	})
	s := ComputePinningSummary(g)
	assert.Equal(t, 1, s.Total)
	assert.Equal(t, 0, s.FirstParty)
	assert.Equal(t, 1, s.ThirdParty)
}
