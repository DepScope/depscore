package report

import (
	"github.com/depscope/depscope/internal/core"
)

// SampleScanResult returns a predictable ScanResult for use in formatter tests.
func SampleScanResult() core.ScanResult {
	return core.ScanResult{
		Profile:        "enterprise",
		PassThreshold:  70,
		DirectDeps:     2,
		TransitiveDeps: 5,
		Packages: []core.PackageResult{
			{
				Name:                "requests",
				Version:             "2.31.0",
				Ecosystem:           "python",
				Constraint:          "==2.31.0",
				ConstraintType:      "exact",
				Depth:               1,
				OwnScore:            82,
				TransitiveRiskScore: 82,
				OwnRisk:             core.RiskLow,
				TransitiveRisk:      core.RiskLow,
			},
			{
				Name:                "urllib3",
				Version:             "2.0.7",
				Ecosystem:           "python",
				Constraint:          ">=1.26",
				ConstraintType:      "minor",
				Depth:               1,
				OwnScore:            40,
				TransitiveRiskScore: 40,
				OwnRisk:             core.RiskHigh,
				TransitiveRisk:      core.RiskHigh,
				Issues: []core.Issue{
					{Package: "urllib3", Severity: core.SeverityHigh, Message: "solo maintainer"},
				},
			},
		},
		AllIssues: []core.Issue{
			{Package: "urllib3", Severity: core.SeverityHigh, Message: "solo maintainer"},
		},
	}
}
