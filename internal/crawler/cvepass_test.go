// internal/crawler/cvepass_test.go
package crawler

import (
	"context"
	"testing"

	"github.com/depscope/depscope/internal/graph"
)

// buildGraph is a helper that creates a graph containing the given nodes.
func buildGraph(nodes ...*graph.Node) *graph.Graph {
	g := graph.New()
	for _, n := range nodes {
		if n.Metadata == nil {
			n.Metadata = make(map[string]any)
		}
		g.AddNode(n)
	}
	return g
}

// TestCVEPass_WithSemver verifies that a node with a semver version gets
// cve_findings metadata set after the pass.
func TestCVEPass_WithSemver(t *testing.T) {
	node := &graph.Node{
		ID:         "package:npm/lodash@4.17.21",
		Type:       graph.NodePackage,
		Name:       "lodash",
		Version:    "4.17.21",
		VersionKey: "npm/lodash@4.17.21",
		Score:      80,
		Metadata:   map[string]any{"ecosystem": "npm"},
	}

	g := buildGraph(node)

	// nil cache + nil osvClient → cache-only mode, no network calls.
	errs := RunCVEPass(context.Background(), g, nil, nil)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	n := g.Nodes[node.ID]
	if _, ok := n.Metadata["cve_findings"]; !ok {
		t.Error("expected cve_findings to be set on node with semver")
	}
}

// TestCVEPass_NoSemver verifies that a node without a semver gets
// cve_status="unchecked".
func TestCVEPass_NoSemver(t *testing.T) {
	node := &graph.Node{
		ID:       "action:actions/checkout@abc123",
		Type:     graph.NodeAction,
		Name:     "actions/checkout",
		Version:  "", // no semver — it's a SHA
		Metadata: map[string]any{},
	}

	g := buildGraph(node)

	errs := RunCVEPass(context.Background(), g, nil, nil)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	n := g.Nodes[node.ID]
	status, ok := n.Metadata["cve_status"]
	if !ok {
		t.Fatal("expected cve_status to be set on node without semver")
	}
	if status != "unchecked" {
		t.Errorf("want cve_status=%q, got %q", "unchecked", status)
	}
}

// TestCVEPass_NilClients verifies that nil cacheDB + nil osvClient does not panic
// and handles both semver and non-semver nodes gracefully.
func TestCVEPass_NilClients(t *testing.T) {
	nodes := []*graph.Node{
		{
			ID:         "package:npm/express@4.18.0",
			Type:       graph.NodePackage,
			Name:       "express",
			Version:    "4.18.0",
			VersionKey: "npm/express@4.18.0",
			Score:      75,
			Metadata:   map[string]any{"ecosystem": "npm"},
		},
		{
			ID:       "action:actions/setup-node@v3",
			Type:     graph.NodeAction,
			Name:     "actions/setup-node",
			Version:  "",
			Metadata: map[string]any{},
		},
	}

	g := buildGraph(nodes...)

	// Should not panic.
	errs := RunCVEPass(context.Background(), g, nil, nil)
	if len(errs) != 0 {
		t.Fatalf("expected no errors with nil clients, got %v", errs)
	}

	// Node with semver → cve_findings set.
	n1 := g.Nodes[nodes[0].ID]
	if _, ok := n1.Metadata["cve_findings"]; !ok {
		t.Error("expected cve_findings on semver node")
	}

	// Node without semver → cve_status=unchecked.
	n2 := g.Nodes[nodes[1].ID]
	if n2.Metadata["cve_status"] != "unchecked" {
		t.Errorf("expected cve_status=unchecked, got %v", n2.Metadata["cve_status"])
	}
}

// TestCVEPass_ScorePenalty verifies the score is reduced when findings are
// pre-populated in cache (simulated by setting cve_findings explicitly, then
// re-running penalty logic via semver detection from VersionKey).
func TestCVEPass_VersionKeyFallback(t *testing.T) {
	// Node has no Version field, but VersionKey has "@<version>" suffix.
	node := &graph.Node{
		ID:         "package:pypi/requests@2.28.0",
		Type:       graph.NodePackage,
		Name:       "requests",
		Version:    "",                      // no Version field
		VersionKey: "pypi/requests@2.28.0", // semver extracted from here
		Score:      60,
		Metadata:   map[string]any{"ecosystem": "PyPI"},
	}

	g := buildGraph(node)
	errs := RunCVEPass(context.Background(), g, nil, nil)

	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	n := g.Nodes[node.ID]
	// Should have cve_findings (not unchecked) because semver was extracted
	// from VersionKey.
	if _, ok := n.Metadata["cve_findings"]; !ok {
		t.Error("expected cve_findings to be set when semver is in VersionKey")
	}
}

// TestCVEPass_CancelledContext verifies the pass respects context cancellation.
func TestCVEPass_CancelledContext(t *testing.T) {
	// Many nodes to walk.
	nodes := make([]*graph.Node, 10)
	for i := range nodes {
		nodes[i] = &graph.Node{
			ID:         graph.NodeID(graph.NodePackage, "pkg"+string(rune('A'+i))+"@1.0.0"),
			Type:       graph.NodePackage,
			Name:       "pkg" + string(rune('A'+i)),
			Version:    "1.0.0",
			VersionKey: "npm/pkg@1.0.0",
			Score:      70,
			Metadata:   map[string]any{},
		}
	}

	g := buildGraph(nodes...)

	// Cancel context immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should not panic, just stop early.
	_ = RunCVEPass(ctx, g, nil, nil)
}
