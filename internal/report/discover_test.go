package report

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/depscope/depscope/internal/discover"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteDiscoverText(t *testing.T) {
	result := &discover.DiscoverResult{
		Package: "litellm",
		Range:   ">=1.82.7,<1.83.0",
		Matches: []discover.ProjectMatch{
			{Project: "/repos/api", Status: discover.StatusConfirmed, Source: "uv.lock", Version: "1.82.8", Depth: "direct"},
			{Project: "/repos/ml", Status: discover.StatusPotentially, Source: "pyproject.toml", Constraint: ">=1.80"},
			{Project: "/repos/chat", Status: discover.StatusSafe, Source: "uv.lock", Version: "1.83.1", Depth: "direct"},
		},
	}

	var buf bytes.Buffer
	err := WriteDiscoverText(&buf, result)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "CONFIRMED")
	assert.Contains(t, output, "/repos/api")
	assert.Contains(t, output, "1.82.8")
	assert.Contains(t, output, "POTENTIALLY")
	assert.Contains(t, output, "SAFE")
}

func TestWriteDiscoverJSON(t *testing.T) {
	result := &discover.DiscoverResult{
		Package: "litellm",
		Range:   ">=1.82.7,<1.83.0",
		Matches: []discover.ProjectMatch{
			{Project: "/repos/api", Status: discover.StatusConfirmed, Source: "uv.lock", Version: "1.82.8", Depth: "direct"},
		},
	}

	var buf bytes.Buffer
	err := WriteDiscoverJSON(&buf, result)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, "litellm", parsed["package"])
	assert.Equal(t, ">=1.82.7,<1.83.0", parsed["range"])

	results := parsed["results"].([]any)
	assert.Len(t, results, 1)

	first := results[0].(map[string]any)
	assert.Equal(t, "confirmed", first["status"])
}
