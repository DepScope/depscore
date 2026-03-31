package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/server"
	"github.com/depscope/depscope/internal/server/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv, err := server.NewServer(server.Options{
		Store: store.NewMemoryStore(),
		Mode:  server.ModeLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler())
}

func TestLandingPage(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html Content-Type, got %q", ct)
	}
}

func TestLandingPageContainsDepscope(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	rawBody, _ := io.ReadAll(resp.Body)
	body := string(rawBody)

	if !strings.Contains(body, "depscope") {
		t.Error("landing page body does not contain 'depscope'")
	}
	if !strings.Contains(body, "<form") {
		t.Error("landing page body does not contain a form element")
	}
}

func TestSubmitScanRedirects(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// Client that does NOT follow redirects so we can inspect the 303.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	form := url.Values{
		"url":     {"https://github.com/psf/requests"},
		"profile": {"enterprise"},
	}

	resp, err := client.PostForm(ts.URL+"/scan", form)
	if err != nil {
		t.Fatalf("POST /scan: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/scan/") {
		t.Errorf("expected redirect to /scan/<id>, got %q", loc)
	}

	// ID portion should be 16 hex chars
	id := strings.TrimPrefix(loc, "/scan/")
	if len(id) != 16 {
		t.Errorf("expected 16-char ID, got %q (len=%d)", id, len(id))
	}
}

func TestSubmitScanInvalidURL(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	form := url.Values{"url": {"not-a-valid-url"}}
	resp, err := client.PostForm(ts.URL+"/scan", form)
	if err != nil {
		t.Fatalf("POST /scan: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid URL, got %d", resp.StatusCode)
	}
}

func TestScanStatusJSON(t *testing.T) {
	st := store.NewMemoryStore()
	const jobID = "abcdef1234567890"
	if err := st.Create(jobID, store.ScanRequest{URL: "https://example.com", Profile: "enterprise"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	srv, err := server.NewServer(server.Options{Store: st, Mode: server.ModeLocal})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/scan/" + jobID)
	if err != nil {
		t.Fatalf("GET /api/scan/%s: %v", jobID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON Content-Type, got %q", ct)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}

	status, ok := payload["status"].(string)
	if !ok {
		t.Fatal("JSON response missing 'status' field")
	}
	if status != "queued" {
		t.Errorf("expected status 'queued', got %q", status)
	}
}

func TestScanPageNotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/scan/nonexistent")
	if err != nil {
		t.Fatalf("GET /scan/nonexistent: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestScanPageQueued(t *testing.T) {
	st := store.NewMemoryStore()
	const jobID = "aaaa000011112222"
	if err := st.Create(jobID, store.ScanRequest{URL: "https://github.com/test/repo", Profile: "enterprise"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	srv, err := server.NewServer(server.Options{Store: st, Mode: server.ModeLocal})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/scan/" + jobID)
	if err != nil {
		t.Fatalf("GET /scan/%s: %v", jobID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	rawBody, _ := io.ReadAll(resp.Body)
	body := string(rawBody)

	if !strings.Contains(body, "Scanning") {
		t.Error("scanning page does not contain 'Scanning'")
	}
	if !strings.Contains(body, jobID) {
		t.Error("scanning page does not contain the job ID")
	}
}

func TestScanPageFailed(t *testing.T) {
	st := store.NewMemoryStore()
	const jobID = "bbbb000011112222"
	if err := st.Create(jobID, store.ScanRequest{URL: "https://github.com/test/repo", Profile: "enterprise"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := st.SaveError(jobID, "something went wrong"); err != nil {
		t.Fatalf("SaveError: %v", err)
	}

	srv, err := server.NewServer(server.Options{Store: st, Mode: server.ModeLocal})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/scan/" + jobID)
	if err != nil {
		t.Fatalf("GET /scan/%s: %v", jobID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	rawBody, _ := io.ReadAll(resp.Body)
	body := string(rawBody)

	if !strings.Contains(body, "Scan Failed") {
		t.Error("results page does not contain 'Scan Failed'")
	}
}

func TestPackageDetail(t *testing.T) {
	st := store.NewMemoryStore()
	const jobID = "cccc000011112222"
	if err := st.Create(jobID, store.ScanRequest{URL: "https://github.com/test/repo", Profile: "enterprise"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	result := &core.ScanResult{
		Profile:       "enterprise",
		PassThreshold: 70,
		DirectDeps:    1,
		Packages: []core.PackageResult{
			{
				Name:                "requests",
				Version:             "2.31.0",
				Ecosystem:           "python",
				OwnScore:            82,
				OwnRisk:             core.RiskLow,
				TransitiveRisk:      core.RiskLow,
				TransitiveRiskScore: 82,
				ConstraintType:      "exact",
				Depth:               1,
				DependsOnCount:      3,
				DependedOnCount:     0,
			},
		},
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

	t.Run("found", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/package/python/requests/2.31.0")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected JSON Content-Type, got %q", ct)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode JSON: %v", err)
		}

		if got := payload["name"]; got != "requests" {
			t.Errorf("name: got %v, want %q", got, "requests")
		}
		if got := payload["version"]; got != "2.31.0" {
			t.Errorf("version: got %v, want %q", got, "2.31.0")
		}
		if got := payload["ecosystem"]; got != "python" {
			t.Errorf("ecosystem: got %v, want %q", got, "python")
		}
		if got, _ := payload["score"].(float64); int(got) != 82 {
			t.Errorf("score: got %v, want 82", got)
		}
		if got := payload["risk"]; got != "LOW" {
			t.Errorf("risk: got %v, want %q", got, "LOW")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/package/python/unknown/1.0.0")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("scoped_npm_package", func(t *testing.T) {
		// Scoped npm packages have a leading @ in the name, e.g. @angular/core
		const npmJobID = "dddd000011112222"
		if err := st.Create(npmJobID, store.ScanRequest{URL: "https://github.com/test/npm-repo", Profile: "enterprise"}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		npmResult := &core.ScanResult{
			Profile:    "enterprise",
			DirectDeps: 1,
			Packages: []core.PackageResult{
				{
					Name:      "@angular/core",
					Version:   "17.0.0",
					Ecosystem: "npm",
					OwnScore:  75,
					OwnRisk:   core.RiskMedium,
				},
			},
		}
		if err := st.SaveResult(npmJobID, npmResult); err != nil {
			t.Fatalf("SaveResult: %v", err)
		}

		resp, err := http.Get(ts.URL + "/api/package/npm/@angular/core/17.0.0")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode JSON: %v", err)
		}
		if got := payload["name"]; got != "@angular/core" {
			t.Errorf("name: got %v, want %q", got, "@angular/core")
		}
	})
}

func TestStaticAssets(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/static/style.css")
	if err != nil {
		t.Fatalf("GET /static/style.css: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/css") {
		t.Errorf("expected text/css Content-Type, got %q", ct)
	}
}

// newScanWithGraph creates a scan job in the store whose result contains a
// small in-memory graph and returns the job ID.
func newScanWithGraph(t *testing.T, st store.ScanStore) string {
	t.Helper()

	const jobID = "graph0000test0001"
	if err := st.Create(jobID, store.ScanRequest{URL: "https://github.com/test/graph-repo", Profile: "enterprise"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	g := graph.New()
	g.AddNode(&graph.Node{
		ID:      "package:go/cobra@v1.10.2",
		Type:    graph.NodePackage,
		Name:    "cobra",
		Version: "v1.10.2",
		Score:   64,
		Risk:    core.RiskMedium,
		Pinning: graph.PinningNA,
	})
	g.AddNode(&graph.Node{
		ID:      "package:go/yaml@v3.0.1",
		Type:    graph.NodePackage,
		Name:    "yaml",
		Version: "v3.0.1",
		Score:   80,
		Risk:    core.RiskLow,
		Pinning: graph.PinningNA,
	})
	g.AddEdge(&graph.Edge{
		From:  "package:go/cobra@v1.10.2",
		To:    "package:go/yaml@v3.0.1",
		Type:  graph.EdgeDependsOn,
		Depth: 0,
	})

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

func TestGraphAPI(t *testing.T) {
	st := store.NewMemoryStore()
	jobID := newScanWithGraph(t, st)

	srv, err := server.NewServer(server.Options{Store: st, Mode: server.ModeLocal})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	t.Run("returns_d3_json", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/scan/" + jobID + "/graph")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected JSON Content-Type, got %q", ct)
		}

		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode JSON: %v", err)
		}

		// Must have nodes, links, summary.
		if _, ok := payload["nodes"]; !ok {
			t.Error("response missing 'nodes' field")
		}
		if _, ok := payload["links"]; !ok {
			t.Error("response missing 'links' field")
		}
		summary, ok := payload["summary"].(map[string]any)
		if !ok {
			t.Fatal("response missing or invalid 'summary' field")
		}

		totalNodes, _ := summary["total_nodes"].(float64)
		if int(totalNodes) != 2 {
			t.Errorf("summary.total_nodes: got %v, want 2", totalNodes)
		}
		totalEdges, _ := summary["total_edges"].(float64)
		if int(totalEdges) != 1 {
			t.Errorf("summary.total_edges: got %v, want 1", totalEdges)
		}
	})

	t.Run("nodes_have_d3_fields", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/scan/" + jobID + "/graph")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		type graphResp struct {
			Nodes []map[string]any `json:"nodes"`
			Links []map[string]any `json:"links"`
		}
		var gr graphResp
		if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
			t.Fatalf("decode: %v", err)
		}

		if len(gr.Nodes) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(gr.Nodes))
		}

		// Each node must have id, type, name, version, score, risk, pinning, group.
		for _, n := range gr.Nodes {
			for _, field := range []string{"id", "type", "name", "version", "score", "risk", "pinning", "group"} {
				if _, ok := n[field]; !ok {
					t.Errorf("node %v missing field %q", n["id"], field)
				}
			}
		}

		if len(gr.Links) != 1 {
			t.Fatalf("expected 1 link, got %d", len(gr.Links))
		}

		link := gr.Links[0]
		// D3 links use source/target, not from/to.
		if _, ok := link["source"]; !ok {
			t.Error("link missing 'source' field")
		}
		if _, ok := link["target"]; !ok {
			t.Error("link missing 'target' field")
		}
		if link["source"] != "package:go/cobra@v1.10.2" {
			t.Errorf("link source: got %v, want package:go/cobra@v1.10.2", link["source"])
		}
		if link["target"] != "package:go/yaml@v3.0.1" {
			t.Errorf("link target: got %v, want package:go/yaml@v3.0.1", link["target"])
		}
	})

	t.Run("not_found", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/scan/doesnotexist/graph")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})
}

func TestNodeDetailAPI(t *testing.T) {
	st := store.NewMemoryStore()
	jobID := newScanWithGraph(t, st)

	srv, err := server.NewServer(server.Options{Store: st, Mode: server.ModeLocal})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	t.Run("found_node_with_slash_in_id", func(t *testing.T) {
		// nodeID "package:go/cobra@v1.10.2" contains both ":" and "/"
		resp, err := http.Get(ts.URL + "/api/scan/" + jobID + "/graph/node/package:go/cobra@v1.10.2")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
		}

		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected JSON Content-Type, got %q", ct)
		}

		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode JSON: %v", err)
		}

		if got := payload["id"]; got != "package:go/cobra@v1.10.2" {
			t.Errorf("id: got %v, want package:go/cobra@v1.10.2", got)
		}
		if got := payload["name"]; got != "cobra" {
			t.Errorf("name: got %v, want cobra", got)
		}
		if got := payload["version"]; got != "v1.10.2" {
			t.Errorf("version: got %v, want v1.10.2", got)
		}
		if got, _ := payload["score"].(float64); int(got) != 64 {
			t.Errorf("score: got %v, want 64", got)
		}
		if got := payload["risk"]; got != "MEDIUM" {
			t.Errorf("risk: got %v, want MEDIUM", got)
		}
		if _, ok := payload["outgoing_edges"]; !ok {
			t.Error("response missing 'outgoing_edges'")
		}
		if _, ok := payload["incoming_edges"]; !ok {
			t.Error("response missing 'incoming_edges'")
		}

		// cobra depends on yaml, so outgoing_edges should have one entry.
		outgoing, _ := payload["outgoing_edges"].([]any)
		if len(outgoing) != 1 {
			t.Errorf("expected 1 outgoing edge, got %d", len(outgoing))
		} else {
			edge, _ := outgoing[0].(map[string]any)
			if edge["to"] != "package:go/yaml@v3.0.1" {
				t.Errorf("outgoing edge to: got %v, want package:go/yaml@v3.0.1", edge["to"])
			}
		}

		// cobra has no incoming edges.
		incoming, _ := payload["incoming_edges"].([]any)
		if len(incoming) != 0 {
			t.Errorf("expected 0 incoming edges for cobra, got %d", len(incoming))
		}
	})

	t.Run("scan_not_found", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/scan/doesnotexist/graph/node/package:go/cobra@v1.10.2")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("node_not_found", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/scan/" + jobID + "/graph/node/package:go/nonexistent@v0.0.0")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})
}
