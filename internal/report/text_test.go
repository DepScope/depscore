package report_test

import (
	"bytes"
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/report"
	"github.com/stretchr/testify/assert"
)

func TestTextReportContainsPackageNames(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	report.WriteText(&buf, result)
	out := buf.String()
	assert.Contains(t, out, "requests")
	assert.Contains(t, out, "urllib3")
	assert.Contains(t, out, "LOW")
	assert.Contains(t, out, "HIGH")
}

func TestTextReportShowsFailWhenBelowThreshold(t *testing.T) {
	result := report.SampleScanResult() // urllib3 score 40 < threshold 70
	var buf bytes.Buffer
	report.WriteText(&buf, result)
	assert.Contains(t, buf.String(), "FAIL")
}

func TestTextReportShowsPassWhenAboveThreshold(t *testing.T) {
	result := report.SampleScanResult()
	result.Packages[1].OwnScore = 80
	result.Packages[1].TransitiveRiskScore = 80
	result.Packages[1].OwnRisk = core.RiskLow
	result.Packages[1].TransitiveRisk = core.RiskLow
	var buf bytes.Buffer
	report.WriteText(&buf, result)
	assert.Contains(t, buf.String(), "PASS")
}
