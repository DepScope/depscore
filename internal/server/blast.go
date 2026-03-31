package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/discover"
	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/server/store"
	"github.com/depscope/depscope/internal/vuln"
)

// blastRadiusRequest is the JSON body for POST /api/scan/{id}/blast-radius.
type blastRadiusRequest struct {
	Mode    string `json:"mode"`    // "cve" or "package"
	CVEID   string `json:"cve_id"`  // CVE mode
	Package string `json:"package"` // package mode
	Range   string `json:"range"`   // package mode
}

// blastRadiusResponse is the JSON body for the blast radius and simulate endpoints.
type blastRadiusResponse struct {
	AffectedNodes []string   `json:"affected_nodes"`
	Paths         [][]string `json:"paths"`
	TotalAffected int        `json:"total_affected"`
	BlastDepth    int        `json:"blast_depth"`
}

// simulateRequest is the JSON body for POST /api/scan/{id}/simulate.
type simulateRequest struct {
	Package     string `json:"package"`
	Range       string `json:"range"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// loadGraph resolves the graph for a scan, checking graphStore first and falling
// back to the in-memory graph from the scan result.
func (s *Server) loadGraph(scanID string) (*graph.Graph, error) {
	if s.graphStore != nil {
		storeNodes, storeEdges, err := s.graphStore.LoadGraph(scanID)
		if err == nil && len(storeNodes) > 0 {
			return rebuildGraph(storeNodes, storeEdges), nil
		}
	}

	job, err := s.store.Get(scanID)
	if err != nil {
		return nil, err
	}

	if job.Result != nil && job.Result.Graph != nil {
		if g, ok := job.Result.Graph.(*graph.Graph); ok {
			return g, nil
		}
	}

	return graph.New(), nil
}

// rebuildGraph reconstructs a *graph.Graph from store representations.
func rebuildGraph(nodes []store.GraphNode, edges []store.GraphEdge) *graph.Graph {
	g := graph.New()
	for _, n := range nodes {
		g.AddNode(&graph.Node{
			ID:         n.NodeID,
			Type:       parseNodeType(n.Type),
			Name:       n.Name,
			Version:    n.Version,
			Ref:        n.Ref,
			Score:      n.Score,
			Risk:       core.RiskLevel(n.Risk),
			Pinning:    parsePinning(n.Pinning),
			Metadata:   n.Metadata,
			ProjectID:  n.ProjectID,
			VersionKey: n.VersionKey,
		})
	}
	for _, e := range edges {
		g.AddEdge(&graph.Edge{
			From:  e.From,
			To:    e.To,
			Type:  parseEdgeType(e.Type),
			Depth: e.Depth,
		})
	}
	return g
}

// parseNodeType converts a string node type to graph.NodeType.
func parseNodeType(s string) graph.NodeType {
	switch s {
	case "package":
		return graph.NodePackage
	case "repo":
		return graph.NodeRepo
	case "action":
		return graph.NodeAction
	case "workflow":
		return graph.NodeWorkflow
	case "docker_image":
		return graph.NodeDockerImage
	case "script_download":
		return graph.NodeScriptDownload
	case "precommit_hook":
		return graph.NodePrecommitHook
	case "terraform_module":
		return graph.NodeTerraformModule
	case "git_submodule":
		return graph.NodeGitSubmodule
	case "dev_tool":
		return graph.NodeDevTool
	case "build_tool":
		return graph.NodeBuildTool
	default:
		return graph.NodePackage
	}
}

// parseEdgeType converts a string edge type to graph.EdgeType.
func parseEdgeType(s string) graph.EdgeType {
	switch s {
	case "depends_on":
		return graph.EdgeDependsOn
	case "hosted_at":
		return graph.EdgeHostedAt
	case "uses_action":
		return graph.EdgeUsesAction
	case "bundles":
		return graph.EdgeBundles
	case "triggers":
		return graph.EdgeTriggers
	case "resolves_to":
		return graph.EdgeResolvesTo
	case "pulls_image":
		return graph.EdgePullsImage
	case "downloads":
		return graph.EdgeDownloads
	case "uses_hook":
		return graph.EdgeUsesHook
	case "uses_module":
		return graph.EdgeUsesModule
	case "includes_submodule":
		return graph.EdgeIncludesSubmodule
	case "uses_tool":
		return graph.EdgeUsesTool
	case "built_with":
		return graph.EdgeBuiltWith
	default:
		return graph.EdgeDependsOn
	}
}

// parsePinning converts a string pinning quality to graph.PinningQuality.
func parsePinning(s string) graph.PinningQuality {
	switch s {
	case "sha":
		return graph.PinningSHA
	case "digest":
		return graph.PinningDigest
	case "exact_version":
		return graph.PinningExactVersion
	case "semver_range":
		return graph.PinningSemverRange
	case "major_tag":
		return graph.PinningMajorTag
	case "branch":
		return graph.PinningBranch
	case "unpinned":
		return graph.PinningUnpinned
	case "n/a":
		return graph.PinningNA
	default:
		return graph.PinningNA
	}
}

// findAffectedNodes searches the graph for package nodes matching a given
// package name (case-insensitive) and version range.
func findAffectedNodes(g *graph.Graph, pkgName string, versionRange string) []string {
	rng, err := discover.ParseRange(versionRange)
	if err != nil {
		return nil
	}

	var affected []string
	for _, n := range g.Nodes {
		if n.Type != graph.NodePackage {
			continue
		}
		if !strings.EqualFold(n.Name, pkgName) {
			continue
		}
		ver := n.Version
		if ver == "" {
			continue
		}
		v, err := discover.ParseVersion(ver)
		if err != nil {
			continue
		}
		if rng.Contains(v) {
			affected = append(affected, n.ID)
		}
	}
	return affected
}

// buildReverseAdj builds a reverse adjacency list (to -> [from IDs]).
func buildReverseAdj(g *graph.Graph) map[string][]string {
	rev := make(map[string][]string)
	for _, e := range g.Edges {
		rev[e.To] = append(rev[e.To], e.From)
	}
	return rev
}

// reverseBFS finds all nodes that transitively depend on any of the seed nodes.
// Returns the full set of affected nodes including the seeds.
func reverseBFS(g *graph.Graph, seeds []string) []string {
	rev := buildReverseAdj(g)
	visited := make(map[string]bool)
	queue := make([]string, 0, len(seeds))

	for _, s := range seeds {
		if !visited[s] {
			visited[s] = true
			queue = append(queue, s)
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, parent := range rev[current] {
			if !visited[parent] {
				visited[parent] = true
				queue = append(queue, parent)
			}
		}
	}

	result := make([]string, 0, len(visited))
	for id := range visited {
		result = append(result, id)
	}
	return result
}

// findRootNodes returns node IDs that have no incoming edges (graph entry points).
func findRootNodes(g *graph.Graph) []string {
	hasIncoming := make(map[string]bool)
	for _, e := range g.Edges {
		hasIncoming[e.To] = true
	}
	var roots []string
	for id := range g.Nodes {
		if !hasIncoming[id] {
			roots = append(roots, id)
		}
	}
	return roots
}

// findExposurePaths finds paths from root nodes to affected nodes.
func findExposurePaths(g *graph.Graph, affectedSet map[string]bool) [][]string {
	roots := findRootNodes(g)
	var paths [][]string
	const maxDepth = 10
	const maxPaths = 50

	for _, root := range roots {
		for affected := range affectedSet {
			if root == affected {
				continue
			}
			found := g.FindPaths(root, affected, maxDepth)
			for _, p := range found {
				if len(paths) >= maxPaths {
					return paths
				}
				paths = append(paths, p)
			}
		}
	}
	return paths
}

// computeBlastDepth calculates the maximum path length among all paths.
func computeBlastDepth(paths [][]string) int {
	maxLen := 0
	for _, p := range paths {
		if len(p) > maxLen {
			maxLen = len(p)
		}
	}
	return maxLen
}

// handleBlastRadius handles POST /api/scan/{id}/blast-radius.
func (s *Server) handleBlastRadius(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req blastRadiusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON body"})
		return
	}

	// Validate the scan exists.
	if _, err := s.store.Get(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "scan not found"})
		return
	}

	g, err := s.loadGraph(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to load graph"})
		return
	}

	var pkgName, versionRange string

	switch req.Mode {
	case "cve":
		if req.CVEID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "cve_id is required for CVE mode"})
			return
		}
		// Query OSV for CVE details.
		osvClient := vuln.NewOSVClient()
		affectedPkgs, err := osvClient.QueryByID(req.CVEID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to query OSV: " + err.Error()})
			return
		}
		if len(affectedPkgs) == 0 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(blastRadiusResponse{
				AffectedNodes: []string{},
				Paths:         [][]string{},
			})
			return
		}
		// Find affected nodes for all affected packages.
		allAffected := computeBlastForMultiplePackages(g, affectedPkgs)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(allAffected)
		return

	case "package":
		if req.Package == "" || req.Range == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "package and range are required for package mode"})
			return
		}
		pkgName = req.Package
		versionRange = req.Range

	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "mode must be 'cve' or 'package'"})
		return
	}

	resp := computeBlast(g, pkgName, versionRange)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleSimulate handles POST /api/scan/{id}/simulate.
func (s *Server) handleSimulate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req simulateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON body"})
		return
	}

	if req.Package == "" || req.Range == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "package and range are required"})
		return
	}

	// Validate the scan exists.
	if _, err := s.store.Get(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "scan not found"})
		return
	}

	g, err := s.loadGraph(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to load graph"})
		return
	}

	resp := computeBlast(g, req.Package, req.Range)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// computeBlast is the shared blast radius computation for package mode and simulation.
func computeBlast(g *graph.Graph, pkgName, versionRange string) blastRadiusResponse {
	directlyAffected := findAffectedNodes(g, pkgName, versionRange)

	if len(directlyAffected) == 0 {
		return blastRadiusResponse{
			AffectedNodes: []string{},
			Paths:         [][]string{},
		}
	}

	// Reverse BFS to find all transitively affected nodes.
	allAffected := reverseBFS(g, directlyAffected)

	affectedSet := make(map[string]bool, len(allAffected))
	for _, id := range allAffected {
		affectedSet[id] = true
	}

	paths := findExposurePaths(g, affectedSet)
	if paths == nil {
		paths = [][]string{}
	}

	return blastRadiusResponse{
		AffectedNodes: allAffected,
		Paths:         paths,
		TotalAffected: len(allAffected),
		BlastDepth:    computeBlastDepth(paths),
	}
}

// computeBlastForMultiplePackages computes blast radius for multiple affected packages
// (used in CVE mode where a single CVE may affect multiple packages).
func computeBlastForMultiplePackages(g *graph.Graph, pkgs []vuln.AffectedPackage) blastRadiusResponse {
	var allDirectly []string
	for _, pkg := range pkgs {
		found := findAffectedNodes(g, pkg.Name, pkg.Range)
		allDirectly = append(allDirectly, found...)
	}

	if len(allDirectly) == 0 {
		return blastRadiusResponse{
			AffectedNodes: []string{},
			Paths:         [][]string{},
		}
	}

	allAffected := reverseBFS(g, allDirectly)

	affectedSet := make(map[string]bool, len(allAffected))
	for _, id := range allAffected {
		affectedSet[id] = true
	}

	paths := findExposurePaths(g, affectedSet)
	if paths == nil {
		paths = [][]string{}
	}

	return blastRadiusResponse{
		AffectedNodes: allAffected,
		Paths:         paths,
		TotalAffected: len(allAffected),
		BlastDepth:    computeBlastDepth(paths),
	}
}
