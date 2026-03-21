package report

import (
	"encoding/json"
	"io"

	"github.com/depscope/depscope/internal/core"
)

type jsonIssue struct {
	Package  string `json:"package"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type jsonPackage struct {
	Name                string      `json:"name"`
	Version             string      `json:"version"`
	Ecosystem           string      `json:"ecosystem"`
	Constraint          string      `json:"constraint,omitempty"`
	ConstraintType      string      `json:"constraint_type"`
	Depth               int         `json:"depth"`
	OwnScore            int         `json:"own_score"`
	VulnScore           int         `json:"vuln_score"`
	TransitiveRiskScore int         `json:"transitive_risk_score"`
	OwnRisk             string      `json:"own_risk"`
	VulnRisk            string      `json:"vuln_risk"`
	TransitiveRisk      string      `json:"transitive_risk"`
	FinalScore          int         `json:"final_score"`
	VulnCount           int         `json:"vuln_count"`
	Issues              []jsonIssue `json:"issues"`
}

type jsonReport struct {
	Profile        string              `json:"profile"`
	PassThreshold  int                 `json:"pass_threshold"`
	DirectDeps     int                 `json:"direct_deps"`
	TransitiveDeps int                 `json:"transitive_deps"`
	Packages       []jsonPackage       `json:"packages"`
	AllIssues      []jsonIssue         `json:"all_issues"`
	Deps           map[string][]string `json:"deps,omitempty"`
	Passed         bool                `json:"passed"`
}

func toJSONIssue(i core.Issue) jsonIssue {
	return jsonIssue{
		Package:  i.Package,
		Severity: string(i.Severity),
		Message:  i.Message,
	}
}

func WriteJSON(w io.Writer, result core.ScanResult) error {
	packages := make([]jsonPackage, 0, len(result.Packages))
	for _, p := range result.Packages {
		issues := make([]jsonIssue, 0, len(p.Issues))
		for _, iss := range p.Issues {
			issues = append(issues, toJSONIssue(iss))
		}
		packages = append(packages, jsonPackage{
			Name:                p.Name,
			Version:             p.Version,
			Ecosystem:           p.Ecosystem,
			Constraint:          p.Constraint,
			ConstraintType:      p.ConstraintType,
			Depth:               p.Depth,
			OwnScore:            p.OwnScore,
			VulnScore:           p.VulnScore,
			TransitiveRiskScore: p.TransitiveRiskScore,
			OwnRisk:             string(p.OwnRisk),
			VulnRisk:            string(p.VulnRisk),
			TransitiveRisk:      string(p.TransitiveRisk),
			FinalScore:          p.FinalScore(),
			VulnCount:           p.VulnCount,
			Issues:              issues,
		})
	}

	allIssues := make([]jsonIssue, 0, len(result.AllIssues))
	for _, iss := range result.AllIssues {
		allIssues = append(allIssues, toJSONIssue(iss))
	}

	out := jsonReport{
		Profile:        result.Profile,
		PassThreshold:  result.PassThreshold,
		DirectDeps:     result.DirectDeps,
		TransitiveDeps: result.TransitiveDeps,
		Packages:       packages,
		AllIssues:      allIssues,
		Deps:           result.Deps,
		Passed:         result.Passed(),
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
