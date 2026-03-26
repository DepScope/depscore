package server

import (
	"encoding/json"
	"net/http"

	"github.com/depscope/depscope/internal/graph"
)

// gapEntry represents a single security gap found in the graph.
type gapEntry struct {
	Type   string `json:"type"`
	NodeID string `json:"node_id"`
	Detail string `json:"detail"`
}

// gapResponse is the JSON body for GET /api/scan/{id}/gaps.
type gapResponse struct {
	Gaps    []gapEntry     `json:"gaps"`
	Summary map[string]int `json:"summary"`
}

// handleGaps handles GET /api/scan/{id}/gaps.
func (s *Server) handleGaps(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

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

	gaps := analyzeGaps(g)

	// Build summary counts by gap type.
	summary := map[string]int{
		"unpinned_action":  0,
		"unpinned_docker":  0,
		"no_lockfile":      0,
		"broad_permissions": 0,
		"script_download":  0,
	}
	for _, gap := range gaps {
		summary[gap.Type]++
	}

	resp := gapResponse{
		Gaps:    gaps,
		Summary: summary,
	}
	if resp.Gaps == nil {
		resp.Gaps = []gapEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// analyzeGaps iterates over graph nodes and identifies security gaps.
func analyzeGaps(g *graph.Graph) []gapEntry {
	var gaps []gapEntry

	for _, n := range g.Nodes {
		switch n.Type {
		case graph.NodeAction:
			// Check pinning quality: anything worse than SHA is a gap.
			if n.Pinning != graph.PinningSHA && n.Pinning != graph.PinningNA {
				detail := pinningGapDetail(n.Pinning)
				gaps = append(gaps, gapEntry{
					Type:   "unpinned_action",
					NodeID: n.ID,
					Detail: detail,
				})
			}

		case graph.NodeDockerImage:
			// Check pinning quality: anything worse than digest is a gap.
			if n.Pinning != graph.PinningDigest && n.Pinning != graph.PinningSHA && n.Pinning != graph.PinningNA {
				detail := pinningGapDetail(n.Pinning)
				gaps = append(gaps, gapEntry{
					Type:   "unpinned_docker",
					NodeID: n.ID,
					Detail: detail,
				})
			}

		case graph.NodePackage:
			// No lockfile: packages with no resolved version (empty version).
			if n.Version == "" {
				gaps = append(gaps, gapEntry{
					Type:   "no_lockfile",
					NodeID: n.ID,
					Detail: "no resolved version, constraint only",
				})
			}

		case graph.NodeWorkflow:
			// Check for broad or missing permissions.
			if n.Metadata != nil {
				if broad, ok := n.Metadata["permissions_broad"]; ok {
					if b, isBool := broad.(bool); isBool && b {
						gaps = append(gaps, gapEntry{
							Type:   "broad_permissions",
							NodeID: n.ID,
							Detail: "broad or missing permissions block",
						})
					}
				}
			}

		case graph.NodeScriptDownload:
			// Script downloads are always a gap.
			detail := "script download detected"
			if n.Metadata != nil {
				if url, ok := n.Metadata["url"].(string); ok && url != "" {
					detail = url
				}
			}
			gaps = append(gaps, gapEntry{
				Type:   "script_download",
				NodeID: n.ID,
				Detail: detail,
			})
		}
	}

	return gaps
}

// pinningGapDetail returns a human-readable description of a pinning gap.
func pinningGapDetail(p graph.PinningQuality) string {
	switch p {
	case graph.PinningExactVersion:
		return "exact version pinned, should be SHA"
	case graph.PinningMajorTag:
		return "major tag pinned, should be SHA"
	case graph.PinningBranch:
		return "branch pinned, should be SHA"
	case graph.PinningUnpinned:
		return "unpinned, should be SHA"
	default:
		return "not optimally pinned"
	}
}
