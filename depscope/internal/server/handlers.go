package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/depscope/depscope/internal/scanner"
	"github.com/depscope/depscope/internal/server/store"
)

// landingData is the template data for the landing page.
type landingData struct{}

// scanningData is the template data for the scanning page.
type scanningData struct {
	URL string
	ID  string
}

// resultsData is the template data for the results page.
type resultsData struct {
	URL    string
	Result interface{} // *core.ScanResult or nil
	Error  string
}

// scanStatusResponse is the JSON body for GET /api/scan/{id}.
type scanStatusResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.renderTemplate(w, r, "landing.html", landingData{})
}

func (s *Server) handleSubmitScan(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	rawURL := r.FormValue("url")
	profile := r.FormValue("profile")
	if profile == "" {
		profile = "enterprise"
	}

	if err := ValidateScanURL(rawURL); err != nil {
		http.Error(w, fmt.Sprintf("invalid URL: %s", err), http.StatusBadRequest)
		return
	}

	id, err := generateID()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	req := store.ScanRequest{URL: rawURL, Profile: profile}
	if err := s.store.Create(id, req); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	switch s.mode {
	case ModeLambda:
		s.runScan(context.Background(), id, rawURL, profile)
	default: // ModeLocal
		go s.runScan(context.Background(), id, rawURL, profile)
	}

	http.Redirect(w, r, "/scan/"+id, http.StatusSeeOther)
}

func (s *Server) handleScanPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	job, err := s.store.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	switch job.Status {
	case "queued", "running":
		s.renderTemplate(w, r, "scanning.html", scanningData{URL: job.URL, ID: job.ID})
	case "complete":
		s.renderTemplate(w, r, "results.html", resultsData{URL: job.URL, Result: job.Result})
	case "failed":
		s.renderTemplate(w, r, "results.html", resultsData{URL: job.URL, Error: job.Error})
	default:
		s.renderTemplate(w, r, "scanning.html", scanningData{URL: job.URL, ID: job.ID})
	}
}

func (s *Server) handleScanStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	job, err := s.store.Get(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(scanStatusResponse{Status: "not_found"})
		return
	}

	resp := scanStatusResponse{
		Status: job.Status,
		Error:  job.Error,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// renderTemplate clones the base layout, adds the named page template,
// then executes it. Each page is independent so their "content" blocks
// don't conflict with one another.
func (s *Server) renderTemplate(w http.ResponseWriter, r *http.Request, name string, data interface{}) {
	tmpl, err := s.pageTemplate(name)
	if err != nil {
		log.Printf("load template %s: %v", name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Execute "layout.html" which is the entry point; the page's "content"
	// block has been registered by parsing the page template file.
	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("execute template %s: %v", name, err)
	}
}

// runScan executes the scan pipeline and persists the result.
func (s *Server) runScan(ctx context.Context, id, rawURL, profile string) {
	_ = s.store.UpdateStatus(id, "running")

	result, err := scanner.ScanURL(ctx, rawURL, scanner.Options{Profile: profile})
	if err != nil {
		log.Printf("scan %s failed: %v", id, err)
		_ = s.store.SaveError(id, err.Error())
		return
	}

	_ = s.store.SaveResult(id, result)
}

// generateID returns a 16-character lowercase hex string from 8 random bytes.
func generateID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
