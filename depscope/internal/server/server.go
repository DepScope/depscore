package server

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/server/store"
	"github.com/depscope/depscope/internal/web"
)

// Mode controls how scans are executed.
type Mode string

const (
	ModeLocal  Mode = "local"
	ModeLambda Mode = "lambda"
)

// Options configures the server.
type Options struct {
	Store store.ScanStore
	Mode  Mode
}

// Server is the HTTP handler for the depscope web interface.
type Server struct {
	store store.ScanStore
	mode  Mode
	// base is the layout-only template; each page is cloned from it.
	base *template.Template
	mux  *http.ServeMux
}

// NewServer creates and configures a new Server.
func NewServer(opts Options) (*Server, error) {
	s := &Server{
		store: opts.Store,
		mode:  opts.Mode,
		mux:   http.NewServeMux(),
	}

	funcMap := template.FuncMap{
		"lower":          strings.ToLower,
		"riskColor":      riskColorName,
		"scoreDashOffset": scoreDashOffset,
		"issueCounts":    issueCounts,
	}

	// Parse only layout.html as the base template.
	base, err := template.New("").Funcs(funcMap).ParseFS(web.Assets, "templates/layout.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout template: %w", err)
	}
	s.base = base

	s.mux.HandleFunc("GET /", s.handleLanding)
	s.mux.HandleFunc("POST /scan", s.handleSubmitScan)
	s.mux.HandleFunc("GET /scan/{id}", s.handleScanPage)
	s.mux.HandleFunc("GET /api/scan/{id}", s.handleScanStatus)
	// Serve static files from the embedded FS (strip the /static/ prefix so
	// http.FileServerFS sees paths relative to the root of the FS).
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticSubFS())))

	return s, nil
}

// Handler returns the HTTP handler for the server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// pageTemplate clones the base layout and adds the named page template.
// This ensures each page's "content" block is independent.
func (s *Server) pageTemplate(name string) (*template.Template, error) {
	t, err := s.base.Clone()
	if err != nil {
		return nil, fmt.Errorf("clone base template: %w", err)
	}
	t, err = t.ParseFS(web.Assets, "templates/"+name)
	if err != nil {
		return nil, fmt.Errorf("parse page template %s: %w", name, err)
	}
	return t, nil
}

// staticSubFS returns a sub-FS rooted at "static/" within the embedded assets,
// so the file server exposes files at their bare names (e.g. "style.css").
func staticSubFS() fs.FS {
	sub, err := fs.Sub(web.Assets, "static")
	if err != nil {
		panic(fmt.Sprintf("depscope: create static sub-FS: %v", err))
	}
	return sub
}

// riskColorName maps a score to a CSS-friendly risk level name used in the SVG gauge.
func riskColorName(score int) string {
	return strings.ToLower(string(core.RiskLevelFromScore(score)))
}

// scoreDashOffset computes the SVG stroke-dashoffset for a given score (0–100)
// and total circumference so the gauge arc fills proportionally.
func scoreDashOffset(score, circumference int) int {
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return circumference - (circumference * score / 100)
}

// issueCounts tallies issues by severity and returns a map for the template.
func issueCounts(issues []core.Issue) map[string]int {
	counts := make(map[string]int)
	for _, iss := range issues {
		counts[string(iss.Severity)]++
	}
	return counts
}
