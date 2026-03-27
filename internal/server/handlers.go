package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
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
	ScanID string
	Result interface{} // *core.ScanResult or nil
	Error  string
}

// graphPageData is the template data for the graph page.
type graphPageData struct {
	URL    string
	ScanID string
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

	rawURL := strings.TrimSpace(r.FormValue("url"))
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
		s.renderTemplate(w, r, "results.html", resultsData{URL: job.URL, ScanID: id, Result: job.Result})
	case "failed":
		s.renderTemplate(w, r, "results.html", resultsData{URL: job.URL, ScanID: id, Error: job.Error})
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

	crawlResult, err := scanner.CrawlURL(ctx, rawURL, scanner.CrawlOptions{
		Profile:     profile,
		CacheDBPath: s.cacheDBPath,
		TrustedOrgs: s.trustedOrgs,
	})
	if err != nil {
		log.Printf("scan %s failed: %v", id, err)
		_ = s.store.SaveError(id, err.Error())
		return
	}

	// Build ScanResult from crawl graph for backward compat with the results template.
	cfg := config.ProfileByName(profile)
	g := crawlResult.Graph

	var packages []core.PackageResult
	var allIssues []core.Issue
	directCount, transitiveCount := 0, 0
	depsMap := make(map[string][]string)

	for _, n := range g.Nodes {
		eco := ""
		if e, ok := n.Metadata["ecosystem"].(string); ok {
			eco = e
		}
		if eco == "" && n.ProjectID != "" {
			// Extract ecosystem from ProjectID like "go/github.com/foo"
			if idx := strings.Index(n.ProjectID, "/"); idx > 0 {
				eco = n.ProjectID[:idx]
			}
		}

		risk := n.Risk
		if risk == "" || risk == core.RiskUnknown {
			risk = core.RiskLevelFromScore(n.Score)
		}

		pr := core.PackageResult{
			Name:      n.Name,
			Version:   n.Version,
			Ecosystem: eco,
			OwnScore:  n.Score,
			OwnRisk:   risk,
			TransitiveRiskScore: n.Score,
			TransitiveRisk:      risk,
		}

		// Count deps from graph edges
		children := g.Neighbors(n.ID)
		pr.DependsOn = children
		pr.DependsOnCount = len(children)
		depsMap[n.Name] = children

		packages = append(packages, pr)
	}

	// Count direct vs transitive (depth from edges)
	hasIncoming := make(map[string]bool)
	for _, e := range g.Edges {
		if g.Nodes[e.From] != nil {
			hasIncoming[e.To] = true
		}
	}
	for nodeID := range g.Nodes {
		if !hasIncoming[nodeID] {
			directCount++
		} else {
			transitiveCount++
		}
	}

	scanResult := &core.ScanResult{
		Profile:        profile,
		PassThreshold:  cfg.PassThreshold,
		DirectDeps:     directCount,
		TransitiveDeps: transitiveCount,
		Packages:       packages,
		AllIssues:      allIssues,
		DepsMap:        depsMap,
		Graph:          g,
	}
	_ = s.store.SaveResult(id, scanResult)

	if s.graphStore != nil && g != nil {
		nodes := make([]store.GraphNode, 0, len(g.Nodes))
		for _, n := range g.Nodes {
			nodes = append(nodes, store.GraphNode{
				NodeID:     n.ID,
				Type:       n.Type.String(),
				Name:       n.Name,
				Version:    n.Version,
				Ref:        n.Ref,
				Score:      n.Score,
				Risk:       string(n.Risk),
				Pinning:    n.Pinning.String(),
				Metadata:   n.Metadata,
				ProjectID:  n.ProjectID,
				VersionKey: n.VersionKey,
			})
		}
		edges := make([]store.GraphEdge, 0, len(g.Edges))
		for _, e := range g.Edges {
			edges = append(edges, store.GraphEdge{
				From:  e.From,
				To:    e.To,
				Type:  e.Type.String(),
				Depth: e.Depth,
			})
		}
		if err := s.graphStore.SaveGraph(id, nodes, edges); err != nil {
			log.Printf("save graph for scan %s: %v", id, err)
		}
	}
}

// packageDetailResponse is the JSON body for GET /api/package/{eco}/{rest...}.
type packageDetailResponse struct {
	Name            string               `json:"name"`
	Version         string               `json:"version"`
	Ecosystem       string               `json:"ecosystem"`
	Score           int                  `json:"score"`
	Risk            core.RiskLevel       `json:"risk"`
	TransitiveRisk  core.RiskLevel       `json:"transitiveRisk"`
	TransitiveScore int                  `json:"transitiveScore"`
	ConstraintType  string               `json:"constraintType"`
	Depth           int                  `json:"depth"`
	Issues          []core.Issue         `json:"issues"`
	Vulnerabilities []core.Vulnerability `json:"vulnerabilities"`
	DependsOn       []string             `json:"dependsOn"`
	DependsOnCount  int                  `json:"dependsOnCount"`
	DependedOnCount int                  `json:"dependedOnCount"`
}

// handlePackageDetail handles GET /api/package/{eco}/{rest...}.
// The URL path after the ecosystem is treated as <name...>/<version>, where
// the last segment is the version and the preceding segments form the package
// name (supporting scoped npm packages like @angular/core).
func (s *Server) handlePackageDetail(w http.ResponseWriter, r *http.Request) {
	eco := r.PathValue("eco")
	rest := r.PathValue("rest")

	// Split name and version: everything before the last "/" is the name,
	// the last segment is the version. If no slash, treat entire rest as name (no version).
	var name, version string
	lastSlash := strings.LastIndex(rest, "/")
	if lastSlash < 0 {
		name = rest
	} else {
		name = rest[:lastSlash]
		version = rest[lastSlash+1:]
	}

	if name == "" {
		http.Error(w, "invalid package path: name must not be empty", http.StatusBadRequest)
		return
	}

	// Search all jobs for a matching package (version may be empty).
	var found *core.PackageResult
outer:
	for _, job := range s.store.List() {
		if job.Result == nil {
			continue
		}
		for i := range job.Result.Packages {
			pkg := &job.Result.Packages[i]
			if strings.EqualFold(pkg.Ecosystem, eco) && pkg.Name == name {
				if version == "" || pkg.Version == version {
					found = pkg
					break outer
				}
			}
		}
	}

	if found == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "package not found"})
		return
	}

	issues := found.Issues
	if issues == nil {
		issues = []core.Issue{}
	}

	deps := found.DependsOn
	if deps == nil {
		deps = []string{}
	}
	vulns := found.Vulnerabilities
	if vulns == nil {
		vulns = []core.Vulnerability{}
	}

	resp := packageDetailResponse{
		Name:            found.Name,
		Version:         found.Version,
		Ecosystem:       found.Ecosystem,
		Score:           found.OwnScore,
		Risk:            found.OwnRisk,
		TransitiveRisk:  found.TransitiveRisk,
		TransitiveScore: found.TransitiveRiskScore,
		ConstraintType:  found.ConstraintType,
		Depth:           found.Depth,
		Issues:          issues,
		Vulnerabilities: vulns,
		DependsOn:       deps,
		DependsOnCount:  found.DependsOnCount,
		DependedOnCount: found.DependedOnCount,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// d3Node is the D3-friendly representation of a graph node.
type d3Node struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Score   int    `json:"score"`
	Risk    string `json:"risk"`
	Pinning string `json:"pinning"`
	Group   string `json:"group"`
}

// d3Link is the D3-friendly representation of a graph edge.
type d3Link struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
	Depth  int    `json:"depth"`
}

// d3GraphResponse is the full D3-friendly graph response.
type d3GraphResponse struct {
	Nodes   []d3Node          `json:"nodes"`
	Links   []d3Link          `json:"links"`
	Summary map[string]any    `json:"summary"`
}

// handleGraphAPI handles GET /api/scan/{id}/graph.
// Returns D3-friendly JSON with nodes, links, and a summary.
func (s *Server) handleGraphAPI(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	job, err := s.store.Get(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "scan not found"})
		return
	}

	var nodes []d3Node
	var links []d3Link

	if s.graphStore != nil {
		// Load from persistent store.
		storeNodes, storeEdges, err := s.graphStore.LoadGraph(id)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to load graph"})
			return
		}
		nodes = make([]d3Node, 0, len(storeNodes))
		for _, n := range storeNodes {
			nodes = append(nodes, storeNodeToD3(n))
		}
		links = make([]d3Link, 0, len(storeEdges))
		for _, e := range storeEdges {
			links = append(links, d3Link{
				Source: e.From,
				Target: e.To,
				Type:   e.Type,
				Depth:  e.Depth,
			})
		}
	} else if job.Result != nil && job.Result.Graph != nil {
		// Fall back to in-memory graph from the scan result.
		g, ok := job.Result.Graph.(*graph.Graph)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "graph type error"})
			return
		}
		nodes = make([]d3Node, 0, len(g.Nodes))
		for _, n := range g.Nodes {
			nodes = append(nodes, graphNodeToD3(n))
		}
		links = make([]d3Link, 0, len(g.Edges))
		for _, e := range g.Edges {
			links = append(links, d3Link{
				Source: e.From,
				Target: e.To,
				Type:   e.Type.String(),
				Depth:  e.Depth,
			})
		}
	}

	if nodes == nil {
		nodes = []d3Node{}
	}
	if links == nil {
		links = []d3Link{}
	}

	// Filter out links that reference non-existent nodes (e.g., phantom "root").
	nodeIDs := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		nodeIDs[n.ID] = true
	}
	validLinks := make([]d3Link, 0, len(links))
	for _, l := range links {
		if nodeIDs[l.Source] && nodeIDs[l.Target] {
			validLinks = append(validLinks, l)
		}
	}
	links = validLinks

	// Build summary counts by node type.
	byType := make(map[string]int)
	for _, n := range nodes {
		byType[n.Type]++
	}
	byTypeAny := make(map[string]any, len(byType))
	for k, v := range byType {
		byTypeAny[k] = v
	}

	resp := d3GraphResponse{
		Nodes: nodes,
		Links: links,
		Summary: map[string]any{
			"total_nodes": len(nodes),
			"total_edges": len(links),
			"by_type":     byTypeAny,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// nodeDetailEdge is an edge in the node detail response.
type nodeDetailEdgeOut struct {
	To   string `json:"to"`
	Type string `json:"type"`
}

type nodeDetailEdgeIn struct {
	From string `json:"from"`
	Type string `json:"type"`
}

// nodeDetailResponse is the JSON body for GET /api/scan/{id}/graph/node/{nodeID...}.
type nodeDetailResponse struct {
	ID            string              `json:"id"`
	Type          string              `json:"type"`
	Name          string              `json:"name"`
	Version       string              `json:"version"`
	Score         int                 `json:"score"`
	Risk          string              `json:"risk"`
	Pinning       string              `json:"pinning"`
	Metadata      map[string]any      `json:"metadata"`
	OutgoingEdges []nodeDetailEdgeOut `json:"outgoing_edges"`
	IncomingEdges []nodeDetailEdgeIn  `json:"incoming_edges"`
}

// handleNodeDetail handles GET /api/scan/{id}/graph/node/{nodeID...}.
// nodeID may contain colons and slashes (e.g., "package:python/litellm@1.82.8").
func (s *Server) handleNodeDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	nodeID := r.PathValue("nodeID")

	if nodeID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "node ID required"})
		return
	}

	job, err := s.store.Get(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "scan not found"})
		return
	}

	var resp *nodeDetailResponse

	if s.graphStore != nil {
		storeNodes, storeEdges, err := s.graphStore.LoadGraph(id)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to load graph"})
			return
		}
		for _, n := range storeNodes {
			if n.NodeID == nodeID {
				outgoing := []nodeDetailEdgeOut{}
				incoming := []nodeDetailEdgeIn{}
				for _, e := range storeEdges {
					if e.From == nodeID {
						outgoing = append(outgoing, nodeDetailEdgeOut{To: e.To, Type: e.Type})
					}
					if e.To == nodeID {
						incoming = append(incoming, nodeDetailEdgeIn{From: e.From, Type: e.Type})
					}
				}
				meta := n.Metadata
				if meta == nil {
					meta = map[string]any{}
				}
				resp = &nodeDetailResponse{
					ID:            n.NodeID,
					Type:          n.Type,
					Name:          n.Name,
					Version:       n.Version,
					Score:         n.Score,
					Risk:          n.Risk,
					Pinning:       n.Pinning,
					Metadata:      meta,
					OutgoingEdges: outgoing,
					IncomingEdges: incoming,
				}
				break
			}
		}
	} else if job.Result != nil && job.Result.Graph != nil {
		g, ok := job.Result.Graph.(*graph.Graph)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "graph type error"})
			return
		}
		n := g.Node(nodeID)
		if n != nil {
			outgoing := []nodeDetailEdgeOut{}
			incoming := []nodeDetailEdgeIn{}
			for _, e := range g.Edges {
				if e.From == nodeID {
					outgoing = append(outgoing, nodeDetailEdgeOut{To: e.To, Type: e.Type.String()})
				}
				if e.To == nodeID {
					incoming = append(incoming, nodeDetailEdgeIn{From: e.From, Type: e.Type.String()})
				}
			}
			meta := n.Metadata
			if meta == nil {
				meta = map[string]any{}
			}
			resp = &nodeDetailResponse{
				ID:            n.ID,
				Type:          n.Type.String(),
				Name:          n.Name,
				Version:       n.Version,
				Score:         n.Score,
				Risk:          string(n.Risk),
				Pinning:       n.Pinning.String(),
				Metadata:      meta,
				OutgoingEdges: outgoing,
				IncomingEdges: incoming,
			}
		}
	}

	if resp == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "node not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleGraphPage renders the graph visualization page.
func (s *Server) handleGraphPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.store.Get(id)
	if err != nil || job == nil {
		http.NotFound(w, r)
		return
	}
	s.renderTemplate(w, r, "graph.html", graphPageData{URL: job.URL, ScanID: id})
}

// storeNodeToD3 converts a store.GraphNode to a D3-friendly node.
func storeNodeToD3(n store.GraphNode) d3Node {
	return d3Node{
		ID:      n.NodeID,
		Type:    n.Type,
		Name:    n.Name,
		Version: n.Version,
		Score:   n.Score,
		Risk:    n.Risk,
		Pinning: n.Pinning,
		Group:   groupFromStoreNode(n),
	}
}

// graphNodeToD3 converts a *graph.Node to a D3-friendly node.
func graphNodeToD3(n *graph.Node) d3Node {
	return d3Node{
		ID:      n.ID,
		Type:    n.Type.String(),
		Name:    n.Name,
		Version: n.Version,
		Score:   n.Score,
		Risk:    string(n.Risk),
		Pinning: n.Pinning.String(),
		Group:   groupFromGraphNode(n),
	}
}

// groupFromStoreNode derives a D3 group label from a store node.
// For packages the group is the ecosystem derived from the node ID prefix.
func groupFromStoreNode(n store.GraphNode) string {
	if n.Type != "package" {
		return n.Type
	}
	// NodeID format: "package:<ecosystem>/<name>@<version>"
	return ecosystemFromID(n.NodeID)
}

// groupFromGraphNode derives a D3 group label from a graph.Node.
func groupFromGraphNode(n *graph.Node) string {
	if n.Type != graph.NodePackage {
		return n.Type.String()
	}
	return ecosystemFromID(n.ID)
}

// ecosystemFromID extracts the ecosystem from a node ID like "package:go/cobra@v1.10.2".
func ecosystemFromID(nodeID string) string {
	// Strip "package:" prefix.
	rest := strings.TrimPrefix(nodeID, "package:")
	// Ecosystem is up to the first "/".
	if slash := strings.Index(rest, "/"); slash >= 0 {
		return rest[:slash]
	}
	return rest
}

// generateID returns a 16-character lowercase hex string from 8 random bytes.
func generateID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
