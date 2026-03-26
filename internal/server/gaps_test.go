package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/server"
	"github.com/depscope/depscope/internal/server/store"
)

// newGapTestGraph creates a scan with a graph containing various gap types.
func newGapTestGraph(t *testing.T, st store.ScanStore) string {
	t.Helper()

	const jobID = "gaps0000test0001"
	if err := st.Create(jobID, store.ScanRequest{URL: "https://github.com/test/gap-repo", Profile: "enterprise"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	g := graph.New()

	// Workflow with broad permissions.
	g.AddNode(&graph.Node{
		ID:   "workflow:release.yml",
		Type: graph.NodeWorkflow,
		Name: "release.yml",
		Metadata: map[string]any{
			"permissions_broad": true,
		},
	})

	// Action with SHA pinning (no gap).
	g.AddNode(&graph.Node{
		ID:      "action:actions/checkout@abc123",
		Type:    graph.NodeAction,
		Name:    "actions/checkout",
		Pinning: graph.PinningSHA,
	})

	// Action with major tag pinning (gap).
	g.AddNode(&graph.Node{
		ID:      "action:org/deploy@v2",
		Type:    graph.NodeAction,
		Name:    "org/deploy",
		Pinning: graph.PinningMajorTag,
	})

	// Action with branch pinning (gap).
	g.AddNode(&graph.Node{
		ID:      "action:org/lint@main",
		Type:    graph.NodeAction,
		Name:    "org/lint",
		Pinning: graph.PinningBranch,
	})

	// Package with resolved version (no gap).
	g.AddNode(&graph.Node{
		ID:      "package:python/requests@2.31.0",
		Type:    graph.NodePackage,
		Name:    "requests",
		Version: "2.31.0",
	})

	// Package with no resolved version (gap).
	g.AddNode(&graph.Node{
		ID:      "package:python/flask@",
		Type:    graph.NodePackage,
		Name:    "flask",
		Version: "",
	})

	// Docker image not digest-pinned (gap).
	g.AddNode(&graph.Node{
		ID:      "docker_image:ubuntu:22.04",
		Type:    graph.NodeDockerImage,
		Name:    "ubuntu",
		Pinning: graph.PinningExactVersion,
	})

	// Docker image with digest (no gap).
	g.AddNode(&graph.Node{
		ID:      "docker_image:alpine@sha256:abc",
		Type:    graph.NodeDockerImage,
		Name:    "alpine",
		Pinning: graph.PinningDigest,
	})

	// Script download (always a gap).
	g.AddNode(&graph.Node{
		ID:   "script:https://install.sh",
		Type: graph.NodeScriptDownload,
		Name: "install.sh",
		Metadata: map[string]any{
			"url": "https://install.sh",
		},
	})

	// Add some edges for graph structure.
	g.AddEdge(&graph.Edge{From: "workflow:release.yml", To: "action:actions/checkout@abc123", Type: graph.EdgeUsesAction})
	g.AddEdge(&graph.Edge{From: "workflow:release.yml", To: "action:org/deploy@v2", Type: graph.EdgeUsesAction})
	g.AddEdge(&graph.Edge{From: "workflow:release.yml", To: "action:org/lint@main", Type: graph.EdgeUsesAction})
	g.AddEdge(&graph.Edge{From: "workflow:release.yml", To: "script:https://install.sh", Type: graph.EdgeDownloads})

	result := &core.ScanResult{
		Profile:    "enterprise",
		DirectDeps: 1,
		Graph:      g,
	}
	if err := st.SaveResult(jobID, result); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}
	return jobID
}

func newGapTestServer(t *testing.T, st store.ScanStore) *httptest.Server {
	t.Helper()
	srv, err := server.NewServer(server.Options{Store: st, Mode: server.ModeLocal})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler())
}

func TestGapAnalysis(t *testing.T) {
	st := store.NewMemoryStore()
	jobID := newGapTestGraph(t, st)
	ts := newGapTestServer(t, st)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/scan/" + jobID + "/gaps")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rawBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, rawBody)
	}

	var result struct {
		Gaps    []map[string]string `json:"gaps"`
		Summary map[string]float64  `json:"summary"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Count gap types from results.
	gapTypes := make(map[string]int)
	for _, gap := range result.Gaps {
		gapTypes[gap["type"]]++
	}

	// Expect 2 unpinned actions (major_tag + branch).
	if got := gapTypes["unpinned_action"]; got != 2 {
		t.Errorf("unpinned_action: got %d, want 2", got)
	}

	// Expect 1 unpinned docker.
	if got := gapTypes["unpinned_docker"]; got != 1 {
		t.Errorf("unpinned_docker: got %d, want 1", got)
	}

	// Expect 1 no_lockfile (flask with no version).
	if got := gapTypes["no_lockfile"]; got != 1 {
		t.Errorf("no_lockfile: got %d, want 1", got)
	}

	// Expect 1 broad_permissions.
	if got := gapTypes["broad_permissions"]; got != 1 {
		t.Errorf("broad_permissions: got %d, want 1", got)
	}

	// Expect 1 script_download.
	if got := gapTypes["script_download"]; got != 1 {
		t.Errorf("script_download: got %d, want 1", got)
	}

	// Verify summary matches.
	if int(result.Summary["unpinned_action"]) != 2 {
		t.Errorf("summary unpinned_action: got %v, want 2", result.Summary["unpinned_action"])
	}
	if int(result.Summary["script_download"]) != 1 {
		t.Errorf("summary script_download: got %v, want 1", result.Summary["script_download"])
	}
}

func TestGapAnalysis_NoGaps(t *testing.T) {
	st := store.NewMemoryStore()

	const jobID = "gaps0000test0002"
	if err := st.Create(jobID, store.ScanRequest{URL: "https://github.com/test/clean-repo", Profile: "enterprise"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	g := graph.New()
	g.AddNode(&graph.Node{
		ID:      "action:actions/checkout@abc123def",
		Type:    graph.NodeAction,
		Name:    "actions/checkout",
		Pinning: graph.PinningSHA,
	})
	g.AddNode(&graph.Node{
		ID:      "package:go/cobra@v1.10.2",
		Type:    graph.NodePackage,
		Name:    "cobra",
		Version: "v1.10.2",
	})

	result := &core.ScanResult{
		Profile:    "enterprise",
		DirectDeps: 1,
		Graph:      g,
	}
	if err := st.SaveResult(jobID, result); err != nil {
		t.Fatalf("SaveResult: %v", err)
	}

	srv, err := server.NewServer(server.Options{Store: st, Mode: server.ModeLocal})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/scan/" + jobID + "/gaps")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result2 struct {
		Gaps []map[string]string `json:"gaps"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result2); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(result2.Gaps) != 0 {
		t.Errorf("expected 0 gaps for clean repo, got %d", len(result2.Gaps))
	}
}

func TestGapAnalysis_ScanNotFound(t *testing.T) {
	st := store.NewMemoryStore()
	srv, err := server.NewServer(server.Options{Store: st, Mode: server.ModeLocal})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/scan/doesnotexist/gaps")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGapAnalysis_GapDetails(t *testing.T) {
	st := store.NewMemoryStore()
	jobID := newGapTestGraph(t, st)
	ts := newGapTestServer(t, st)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/scan/" + jobID + "/gaps")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Gaps []map[string]string `json:"gaps"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Each gap should have type, node_id, and detail fields.
	for _, gap := range result.Gaps {
		if gap["type"] == "" {
			t.Error("gap missing 'type' field")
		}
		if gap["node_id"] == "" {
			t.Error("gap missing 'node_id' field")
		}
		if gap["detail"] == "" {
			t.Error("gap missing 'detail' field")
		}
	}

	// Check specific node IDs.
	nodeIDs := make(map[string]bool)
	for _, gap := range result.Gaps {
		nodeIDs[gap["node_id"]] = true
	}

	if !nodeIDs["action:org/deploy@v2"] {
		t.Error("expected org/deploy action in gaps")
	}
	if !nodeIDs["action:org/lint@main"] {
		t.Error("expected org/lint action in gaps")
	}
	if !nodeIDs["script:https://install.sh"] {
		t.Error("expected install.sh script in gaps")
	}
	if nodeIDs["action:actions/checkout@abc123"] {
		t.Error("SHA-pinned checkout action should not be in gaps")
	}
}
