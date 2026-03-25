package actions

import "github.com/depscope/depscope/internal/graph"

// PinningSummary holds aggregated pinning statistics for GitHub Actions nodes.
type PinningSummary struct {
	SHAPinned       int
	ExactVersion    int
	MajorTag        int
	Branch          int
	Unpinned        int
	FirstParty      int
	ThirdParty      int
	ScriptDownloads int
	Total           int
}

// ComputePinningSummary walks all action and script-download nodes in g and
// returns aggregated pinning statistics.
func ComputePinningSummary(g *graph.Graph) PinningSummary {
	var s PinningSummary
	for _, node := range g.NodesOfType(graph.NodeAction) {
		s.Total++
		switch node.Pinning {
		case graph.PinningSHA:
			s.SHAPinned++
		case graph.PinningExactVersion:
			s.ExactVersion++
		case graph.PinningMajorTag:
			s.MajorTag++
		case graph.PinningBranch:
			s.Branch++
		case graph.PinningUnpinned:
			s.Unpinned++
		}
		// Check first-party from metadata
		if fp, ok := node.Metadata["first_party"].(bool); ok && fp {
			s.FirstParty++
		} else {
			s.ThirdParty++
		}
	}
	s.ScriptDownloads = len(g.NodesOfType(graph.NodeScriptDownload))
	return s
}
