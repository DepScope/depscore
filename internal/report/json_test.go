package report_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/depscope/depscope/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONReportValid(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, report.WriteJSON(&buf, report.SampleScanResult()))
	var out map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Contains(t, out, "packages")
	assert.Contains(t, out, "passed")
	assert.Contains(t, out, "profile")
}
