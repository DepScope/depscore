package vuln

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNVDClient_Query_NoAPIKey(t *testing.T) {
	// Without an API key the client should return empty results, not an error.
	client := NewNVDClient()
	findings, err := client.Query("PyPI", "requests", "2.28.0")
	require.NoError(t, err)
	assert.Nil(t, findings)
}

func TestNVDClient_Query_WithAPIKey(t *testing.T) {
	srv := serveGolden(t, "testdata/nvd_response.json")
	defer srv.Close()

	client := NewNVDClient(
		WithNVDBaseURL(srv.URL),
		WithNVDAPIKey("test-api-key"),
	)

	findings, err := client.Query("PyPI", "requests", "2.28.0")
	require.NoError(t, err)

	require.Len(t, findings, 1)
	f := findings[0]
	assert.Equal(t, "CVE-2023-32681", f.ID)
	assert.Contains(t, f.Summary, "Proxy-Authorization")
	assert.Equal(t, SeverityMedium, f.Severity)
	assert.Equal(t, "nvd", f.Source)
}

func TestNVDClient_Query_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewNVDClient(
		WithNVDBaseURL(srv.URL),
		WithNVDAPIKey("bad-key"),
	)
	_, err := client.Query("PyPI", "requests", "2.28.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}
