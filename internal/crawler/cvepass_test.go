// internal/crawler/cvepass_test.go
package crawler

import (
	"context"
	"testing"

	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/vuln"
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

// TestCVEPass_ScorePenalty verifies that CVE findings reduce the node's score
// using the applyFindingsPenalty function's severity-based penalties.
func TestCVEPass_ScorePenalty(t *testing.T) {
	tests := []struct {
		name        string
		startScore  int
		findings    []vuln.Finding
		wantScore   int
	}{
		{
			name:       "single HIGH finding reduces score by 10",
			startScore: 80,
			findings: []vuln.Finding{
				{ID: "CVE-2024-0001", Severity: vuln.SeverityHigh},
			},
			wantScore: 70,
		},
		{
			name:       "single CRITICAL finding reduces score by 15",
			startScore: 80,
			findings: []vuln.Finding{
				{ID: "CVE-2024-0002", Severity: vuln.SeverityCritical},
			},
			wantScore: 65,
		},
		{
			name:       "single MEDIUM finding reduces score by 5",
			startScore: 80,
			findings: []vuln.Finding{
				{ID: "CVE-2024-0003", Severity: vuln.SeverityMedium},
			},
			wantScore: 75,
		},
		{
			name:       "single LOW finding reduces score by 2",
			startScore: 80,
			findings: []vuln.Finding{
				{ID: "CVE-2024-0004", Severity: vuln.SeverityLow},
			},
			wantScore: 78,
		},
		{
			name:       "unknown severity treated as MEDIUM (penalty 5)",
			startScore: 80,
			findings: []vuln.Finding{
				{ID: "CVE-2024-0005", Severity: "UNKNOWN"},
			},
			wantScore: 75,
		},
		{
			name:       "multiple findings accumulate penalties",
			startScore: 80,
			findings: []vuln.Finding{
				{ID: "CVE-2024-0001", Severity: vuln.SeverityHigh},     // -10
				{ID: "CVE-2024-0002", Severity: vuln.SeverityCritical}, // -15
			},
			wantScore: 55,
		},
		{
			name:       "score clamped to zero",
			startScore: 10,
			findings: []vuln.Finding{
				{ID: "CVE-2024-0001", Severity: vuln.SeverityCritical}, // -15
			},
			wantScore: 0,
		},
		{
			name:       "empty findings no change",
			startScore: 80,
			findings:   []vuln.Finding{},
			wantScore:  80,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyFindingsPenalty(tt.startScore, tt.findings)
			if got != tt.wantScore {
				t.Errorf("applyFindingsPenalty(%d, findings) = %d, want %d",
					tt.startScore, got, tt.wantScore)
			}
		})
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

// TestEcosystemForNode_FromProjectID verifies that ecosystemForNode falls back
// to extracting and mapping ecosystem from node.ProjectID when metadata is empty.
func TestEcosystemForNode_FromProjectID(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		metadata  map[string]any
		want      string
	}{
		{
			name:      "Go ecosystem from ProjectID",
			projectID: "Go/github.com/foo/bar",
			metadata:  map[string]any{},
			want:      "Go",
		},
		{
			name:      "npm ecosystem from ProjectID",
			projectID: "npm/lodash",
			metadata:  map[string]any{},
			want:      "npm",
		},
		{
			name:      "PyPI ecosystem from ProjectID",
			projectID: "PyPI/requests",
			metadata:  map[string]any{},
			want:      "PyPI",
		},
		{
			name:      "python maps to PyPI",
			projectID: "python/requests",
			metadata:  map[string]any{},
			want:      "PyPI",
		},
		{
			name:      "metadata takes precedence over ProjectID",
			projectID: "Go/github.com/foo/bar",
			metadata:  map[string]any{"ecosystem": "npm"},
			want:      "npm",
		},
		{
			name:      "empty metadata and empty ProjectID returns empty",
			projectID: "",
			metadata:  map[string]any{},
			want:      "",
		},
		{
			name:      "empty string in metadata falls back to ProjectID",
			projectID: "npm/lodash",
			metadata:  map[string]any{"ecosystem": ""},
			want:      "npm",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &graph.Node{
				ID:        "test:" + tt.projectID,
				ProjectID: tt.projectID,
				Metadata:  tt.metadata,
			}
			got := ecosystemForNode(node)
			if got != tt.want {
				t.Errorf("ecosystemForNode() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMapEcosystemToOSV verifies the mapping from raw identifiers to OSV names.
func TestMapEcosystemToOSV(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"go", "Go"},
		{"Go", "Go"},
		{"npm", "npm"},
		{"pypi", "PyPI"},
		{"python", "PyPI"},
		{"PyPI", "PyPI"},
		{"crates.io", "crates.io"},
		{"rust", "crates.io"},
		{"packagist", "Packagist"},
		{"php", "Packagist"},
		{"Packagist", "Packagist"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := mapEcosystemToOSV(tt.input)
		if got != tt.want {
			t.Errorf("mapEcosystemToOSV(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestCVEPass_EcosystemMapping verifies that the CVE pass uses the correct
// ecosystem from ProjectID when looking up cached CVE data.
func TestCVEPass_EcosystemMapping(t *testing.T) {
	// Node with ProjectID set and a version, but no ecosystem in metadata.
	node := &graph.Node{
		ID:         "package:Go/github.com/foo/bar@1.0.0",
		Type:       graph.NodePackage,
		Name:       "github.com/foo/bar",
		Version:    "1.0.0",
		VersionKey: "Go/github.com/foo/bar@1.0.0",
		ProjectID:  "Go/github.com/foo/bar",
		Score:      80,
		Metadata:   map[string]any{}, // no ecosystem in metadata
	}

	g := buildGraph(node)

	// Run CVE pass with nil cache + nil client (cache-only mode).
	errs := RunCVEPass(context.Background(), g, nil, nil)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}

	// Node has a semver, so it should get cve_findings (empty list).
	n := g.Nodes[node.ID]
	if _, ok := n.Metadata["cve_findings"]; !ok {
		t.Error("expected cve_findings to be set even without ecosystem in metadata")
	}
}
