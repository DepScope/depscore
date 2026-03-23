package vuln_test

import (
	"testing"

	"github.com/depscope/depscope/internal/vuln"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNVDNoAPIKey(t *testing.T) {
	client := vuln.NewNVDClient("")
	findings, err := client.Query("PyPI", "requests", "2.28.2")
	require.NoError(t, err, "NVD client without API key should return empty, not error")
	assert.Empty(t, findings)
}
