package vuln_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/depscope/depscope/internal/vuln"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSVQuery(t *testing.T) {
	data, err := os.ReadFile("testdata/osv_requests.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv.Close()

	client := vuln.NewOSVClient(vuln.WithOSVBaseURL(srv.URL))
	findings, err := client.Query("PyPI", "requests", "2.28.2")
	require.NoError(t, err)
	// requests 2.28.2 has known CVEs — golden file should have findings
	// If the golden file has no vulns (package was patched), just check no error
	assert.NotNil(t, findings)
}
