package vuln

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveGolden(t *testing.T, path string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
}

func TestOSVClient_Query(t *testing.T) {
	srv := serveGolden(t, "testdata/osv_response.json")
	defer srv.Close()

	client := NewOSVClient(WithOSVBaseURL(srv.URL))
	findings, err := client.Query("PyPI", "requests", "2.28.0")
	require.NoError(t, err)

	require.Len(t, findings, 2)

	first := findings[0]
	assert.Equal(t, "PYSEC-2023-57", first.ID)
	assert.Contains(t, first.Summary, "proxy-authorization")
	assert.Equal(t, "osv.dev", first.Source)
	assert.Contains(t, first.FixedIn, "2.31.0")
	// CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N has 1 ":H" → SeverityMedium
	assert.Equal(t, SeverityMedium, first.Severity)
}

func TestOSVClient_Query_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"vulns":[]}`))
	}))
	defer srv.Close()

	client := NewOSVClient(WithOSVBaseURL(srv.URL))
	findings, err := client.Query("PyPI", "requests", "2.31.0")
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestOSVClient_Query_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewOSVClient(WithOSVBaseURL(srv.URL))
	_, err := client.Query("PyPI", "requests", "2.31.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
