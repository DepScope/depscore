package report_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/depscope/depscope/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Text reporter ----------------------------------------------------------

func TestWriteTextContainsPackageNames(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	err := report.WriteText(&buf, result)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "express")
	assert.Contains(t, out, "lodash")
	assert.Contains(t, out, "abandoned-pkg")
}

func TestWriteTextFail(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	err := report.WriteText(&buf, result)
	require.NoError(t, err)

	// The sample result has abandoned-pkg with score 30 < threshold 70, so it fails.
	assert.Contains(t, buf.String(), "Result: FAIL")
}

func TestWriteTextPass(t *testing.T) {
	result := report.SampleScanResult()
	// Raise scores so all packages pass.
	for i := range result.Packages {
		result.Packages[i].OwnScore = 90
		result.Packages[i].TransitiveRiskScore = 90
	}
	var buf bytes.Buffer
	err := report.WriteText(&buf, result)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Result: PASS")
}

func TestWriteTextContainsIssues(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	require.NoError(t, report.WriteText(&buf, result))
	assert.Contains(t, buf.String(), "Issues:")
}

// ---- JSON reporter ----------------------------------------------------------

func TestWriteJSONIsValid(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	err := report.WriteJSON(&buf, result)
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
}

func TestWriteJSONHasExpectedKeys(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	require.NoError(t, report.WriteJSON(&buf, result))

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))

	assert.Contains(t, out, "profile")
	assert.Contains(t, out, "pass_threshold")
	assert.Contains(t, out, "passed")
	assert.Contains(t, out, "packages")
	assert.Contains(t, out, "all_issues")
}

func TestWriteJSONPassedField(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	require.NoError(t, report.WriteJSON(&buf, result))

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))

	// abandoned-pkg scores 30 < threshold 70 → passed=false
	assert.Equal(t, false, out["passed"])
}

// ---- SARIF reporter ---------------------------------------------------------

func TestWriteSARIFHasCorrectVersion(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	err := report.WriteSARIF(&buf, result)
	require.NoError(t, err)

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))

	assert.Equal(t, "2.1.0", out["version"])
}

func TestWriteSARIFHasSchema(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	require.NoError(t, report.WriteSARIF(&buf, result))

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))

	schema, ok := out["$schema"].(string)
	assert.True(t, ok)
	assert.True(t, strings.Contains(schema, "sarif"), "schema URL should reference sarif")
}

func TestWriteSARIFHasHighIssues(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	require.NoError(t, report.WriteSARIF(&buf, result))

	out := buf.String()
	// The fixture has one HIGH issue for abandoned-pkg, so SARIF results should be present.
	assert.Contains(t, out, "DEP-HIGH")
}

func TestWriteSARIFToolName(t *testing.T) {
	result := report.SampleScanResult()
	var buf bytes.Buffer
	require.NoError(t, report.WriteSARIF(&buf, result))

	assert.Contains(t, buf.String(), "depscope")
}
