package report_test

import (
	"bytes"
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/report"
	"github.com/stretchr/testify/assert"
)

func SampleScanResultWithDeps() core.ScanResult {
	r := report.SampleScanResult()
	r.Deps = map[string][]string{
		"requests": {"urllib3"},
	}
	return r
}

func TestTextReportContainsPackageNames(t *testing.T) {
	result := SampleScanResultWithDeps()
	var buf bytes.Buffer
	report.WriteText(&buf, result)
	out := buf.String()
	assert.Contains(t, out, "requests")
	assert.Contains(t, out, "urllib3")
	assert.Contains(t, out, "Score:")
	// requests has ==2.31.0 constraint which matches resolved → just show version
	assert.Contains(t, out, "requests 2.31.0")
	// urllib3 has >=1.26 constraint which differs from 2.0.7 → show constraint → resolved
	assert.Contains(t, out, "urllib3 >=1.26 \u2192 2.0.7")
	// Should NOT contain the old (minor) format
	assert.NotContains(t, out, "(minor)")
}

func TestTextReportTreeConnectors(t *testing.T) {
	result := SampleScanResultWithDeps()
	var buf bytes.Buffer
	report.WriteText(&buf, result)
	out := buf.String()
	// Tree should use connectors — single root means └── for root and child
	assert.Contains(t, out, "└── ")

	// Add a second root to get ├── connector
	result.Packages = append(result.Packages, core.PackageResult{
		Name:                "flask",
		Version:             "3.0.0",
		Ecosystem:           "python",
		ConstraintType:      "exact",
		Depth:               1,
		OwnScore:            75,
		VulnScore:           100,
		TransitiveRiskScore: 75,
		OwnRisk:             core.RiskMedium,
		TransitiveRisk:      core.RiskMedium,
	})
	result.Deps["flask"] = nil
	buf.Reset()
	report.WriteText(&buf, result)
	out = buf.String()
	assert.Contains(t, out, "├── ")
	assert.Contains(t, out, "└── ")
}

func TestTextReportShowsFailWhenBelowThreshold(t *testing.T) {
	result := SampleScanResultWithDeps()
	var buf bytes.Buffer
	report.WriteText(&buf, result)
	assert.Contains(t, buf.String(), "FAIL")
}

func TestTextReportShowsPassWhenAboveThreshold(t *testing.T) {
	result := SampleScanResultWithDeps()
	result.Packages[1].OwnScore = 80
	result.Packages[1].VulnScore = 100
	result.Packages[1].TransitiveRiskScore = 80
	result.Packages[1].OwnRisk = core.RiskLow
	result.Packages[1].TransitiveRisk = core.RiskLow
	var buf bytes.Buffer
	report.WriteText(&buf, result)
	assert.Contains(t, buf.String(), "PASS")
}

func TestTextReportFlatFallback(t *testing.T) {
	// When no deps map is provided, fall back to flat list
	result := report.SampleScanResult()
	var buf bytes.Buffer
	report.WriteText(&buf, result)
	out := buf.String()
	assert.Contains(t, out, "requests")
	assert.Contains(t, out, "urllib3")
	assert.Contains(t, out, "Score:")
	// Flat list should NOT contain tree connectors
	assert.NotContains(t, out, "├── ")
	assert.NotContains(t, out, "└── ")
}
