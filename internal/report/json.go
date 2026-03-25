package report

import (
	"encoding/json"
	"io"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
)

// jsonScanResult is the serialized form of a ScanResult with a 'passed' field.
type jsonScanResult struct {
	Profile        string              `json:"profile"`
	PassThreshold  int                 `json:"pass_threshold"`
	Passed         bool                `json:"passed"`
	DirectDeps     int                 `json:"direct_deps"`
	TransitiveDeps int                 `json:"transitive_deps"`
	Packages       []jsonPackageResult `json:"packages"`
	AllIssues      []jsonIssue         `json:"all_issues"`
	RiskPaths      []jsonRiskPath      `json:"risk_paths,omitempty"`
	Suspicious     []jsonSuspicious    `json:"suspicious,omitempty"`
	Graph          *jsonGraph          `json:"graph,omitempty"`
}

type jsonGraph struct {
	Nodes []jsonGraphNode `json:"nodes"`
	Edges []jsonGraphEdge `json:"edges"`
}

type jsonGraphNode struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Score   int    `json:"score"`
	Risk    string `json:"risk"`
	Pinning string `json:"pinning,omitempty"`
}

type jsonGraphEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Type  string `json:"type"`
	Depth int    `json:"depth,omitempty"`
}

type jsonRiskPath struct {
	Chain    []string `json:"chain"`
	EndScore int      `json:"end_score"`
	EndRisk  string   `json:"end_risk"`
	Reason   string   `json:"reason"`
}

type jsonSuspicious struct {
	Package  string `json:"package"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type jsonPackageResult struct {
	Name                string            `json:"name"`
	Version             string            `json:"version"`
	Ecosystem           string            `json:"ecosystem"`
	ConstraintType      string            `json:"constraint_type"`
	Depth               int               `json:"depth"`
	OwnScore            int               `json:"own_score"`
	TransitiveRiskScore int               `json:"transitive_risk_score"`
	OwnRisk             string            `json:"own_risk"`
	TransitiveRisk      string            `json:"transitive_risk"`
	DependsOn           []string          `json:"depends_on,omitempty"`
	DependsOnCount      int               `json:"depends_on_count"`
	DependedOnCount     int               `json:"depended_on_count"`
	Issues              []jsonIssue       `json:"issues,omitempty"`
	Vulnerabilities     []jsonVuln        `json:"vulnerabilities,omitempty"`
}

type jsonVuln struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Severity string `json:"severity"`
}

type jsonIssue struct {
	Package  string `json:"package"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// WriteJSON writes a JSON-encoded scan result to w.
func WriteJSON(w io.Writer, result core.ScanResult) error {
	packages := make([]jsonPackageResult, len(result.Packages))
	for i, p := range result.Packages {
		var issues []jsonIssue
		for _, iss := range p.Issues {
			issues = append(issues, jsonIssue{
				Package:  iss.Package,
				Severity: string(iss.Severity),
				Message:  iss.Message,
			})
		}
		var vulns []jsonVuln
		for _, v := range p.Vulnerabilities {
			vulns = append(vulns, jsonVuln{
				ID:       v.ID,
				Summary:  v.Summary,
				Severity: v.Severity,
			})
		}
		packages[i] = jsonPackageResult{
			Name:                p.Name,
			Version:             p.Version,
			Ecosystem:           p.Ecosystem,
			ConstraintType:      p.ConstraintType,
			Depth:               p.Depth,
			OwnScore:            p.OwnScore,
			TransitiveRiskScore: p.TransitiveRiskScore,
			OwnRisk:             string(p.OwnRisk),
			TransitiveRisk:      string(p.TransitiveRisk),
			DependsOn:           p.DependsOn,
			DependsOnCount:      p.DependsOnCount,
			DependedOnCount:     p.DependedOnCount,
			Issues:              issues,
			Vulnerabilities:     vulns,
		}
	}

	allIssues := make([]jsonIssue, len(result.AllIssues))
	for i, iss := range result.AllIssues {
		allIssues[i] = jsonIssue{
			Package:  iss.Package,
			Severity: string(iss.Severity),
			Message:  iss.Message,
		}
	}

	var riskPaths []jsonRiskPath
	for _, rp := range result.RiskPaths {
		riskPaths = append(riskPaths, jsonRiskPath{
			Chain:    rp.Chain,
			EndScore: rp.EndScore,
			EndRisk:  string(rp.EndRisk),
			Reason:   rp.Reason,
		})
	}

	var suspicious []jsonSuspicious
	for _, s := range result.Suspicious {
		suspicious = append(suspicious, jsonSuspicious{
			Package:  s.Package,
			Type:     s.Type,
			Severity: string(s.Severity),
			Message:  s.Message,
		})
	}

	out := jsonScanResult{
		Profile:        result.Profile,
		PassThreshold:  result.PassThreshold,
		Passed:         result.Passed(),
		DirectDeps:     result.DirectDeps,
		TransitiveDeps: result.TransitiveDeps,
		Packages:       packages,
		AllIssues:      allIssues,
		RiskPaths:      riskPaths,
		Suspicious:     suspicious,
	}

	if result.Graph != nil {
		if g, ok := result.Graph.(*graph.Graph); ok {
			jg := &jsonGraph{}
			for _, n := range g.Nodes {
				jg.Nodes = append(jg.Nodes, jsonGraphNode{
					ID:      n.ID,
					Type:    n.Type.String(),
					Name:    n.Name,
					Version: n.Version,
					Score:   n.Score,
					Risk:    string(n.Risk),
					Pinning: n.Pinning.String(),
				})
			}
			for _, e := range g.Edges {
				jg.Edges = append(jg.Edges, jsonGraphEdge{
					From:  e.From,
					To:    e.To,
					Type:  e.Type.String(),
					Depth: e.Depth,
				})
			}
			out.Graph = jg
		}
	}

	return json.NewEncoder(w).Encode(out)
}
