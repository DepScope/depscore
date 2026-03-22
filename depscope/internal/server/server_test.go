package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	rawBody, _ := io.ReadAll(resp.Body)
	body := string(rawBody)

	if !strings.Contains(body, "Scan Failed") {
		t.Error("results page does not contain 'Scan Failed'")
	}
}

func TestStaticAssets(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/static/style.css")
	if err != nil {
		t.Fatalf("GET /static/style.css: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/css") {
		t.Errorf("expected text/css Content-Type, got %q", ct)
	}
}
