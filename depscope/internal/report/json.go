package report

import (
	"encoding/json"
	"io"

	"github.com/depscope/depscope/internal/core"
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
}

type jsonPackageResult struct {
	Name                string `json:"name"`
	Version             string `json:"version"`
	Ecosystem           string `json:"ecosystem"`
	ConstraintType      string `json:"constraint_type"`
	Depth               int    `json:"depth"`
	OwnScore            int    `json:"own_score"`
	TransitiveRiskScore int    `json:"transitive_risk_score"`
	OwnRisk             string `json:"own_risk"`
	TransitiveRisk      string `json:"transitive_risk"`
	DependsOnCount      int    `json:"depends_on_count"`
	DependedOnCount     int    `json:"depended_on_count"`
	Issues              []jsonIssue `json:"issues,omitempty"`
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
			DependsOnCount:      p.DependsOnCount,
			DependedOnCount:     p.DependedOnCount,
			Issues:              issues,
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

	out := jsonScanResult{
		Profile:        result.Profile,
		PassThreshold:  result.PassThreshold,
		Passed:         result.Passed(),
		DirectDeps:     result.DirectDeps,
		TransitiveDeps: result.TransitiveDeps,
		Packages:       packages,
		AllIssues:      allIssues,
	}

	return json.NewEncoder(w).Encode(out)
}
