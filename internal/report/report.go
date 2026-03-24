// Package report provides formatters for depscope scan results.
package report

import (
	"github.com/depscope/depscope/internal/core"
)

// SampleScanResult returns a predictable fixture ScanResult for use in tests.
func SampleScanResult() core.ScanResult {
	return core.ScanResult{
		Profile:        "enterprise",
		PassThreshold:  70,
		DirectDeps:     2,
		TransitiveDeps: 1,
		Packages: []core.PackageResult{
			{
				Name:                "express",
				Version:             "4.18.2",
				Ecosystem:           "npm",
				ConstraintType:      "exact",
				Depth:               1,
				OwnScore:            85,
				TransitiveRiskScore: 80,
				OwnRisk:             core.RiskLow,
				TransitiveRisk:      core.RiskLow,
				DependsOnCount:      1,
				DependedOnCount:     0,
				Issues:              nil,
			},
			{
				Name:                "lodash",
				Version:             "4.17.21",
				Ecosystem:           "npm",
				ConstraintType:      "minor",
				Depth:               1,
				OwnScore:            78,
				TransitiveRiskScore: 78,
				OwnRisk:             core.RiskLow,
				TransitiveRisk:      core.RiskLow,
				DependsOnCount:      0,
				DependedOnCount:     1,
				Issues:              nil,
			},
			{
				Name:                "abandoned-pkg",
				Version:             "0.1.0",
				Ecosystem:           "npm",
				ConstraintType:      "major",
				Depth:               2,
				OwnScore:            30,
				TransitiveRiskScore: 30,
				OwnRisk:             core.RiskCritical,
				TransitiveRisk:      core.RiskCritical,
				DependsOnCount:      0,
				DependedOnCount:     1,
				Issues: []core.Issue{
					{
						Package:  "abandoned-pkg",
						Severity: core.SeverityHigh,
						Message:  "last release was over 3 years ago",
					},
					{
						Package:  "abandoned-pkg",
						Severity: core.SeverityMedium,
						Message:  "single maintainer; bus-factor risk",
					},
				},
			},
		},
		AllIssues: []core.Issue{
			{
				Package:  "abandoned-pkg",
				Severity: core.SeverityHigh,
				Message:  "last release was over 3 years ago",
			},
			{
				Package:  "abandoned-pkg",
				Severity: core.SeverityMedium,
				Message:  "single maintainer; bus-factor risk",
			},
		},
	}
}

