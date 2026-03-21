package report_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/depscope/depscope/internal/report"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSARIFVersion(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, report.WriteSARIF(&buf, report.SampleScanResult()))
	var out map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	assert.Equal(t, "2.1.0", out["version"])
}
