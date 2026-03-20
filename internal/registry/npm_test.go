package registry_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/depscope/depscope/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNPMFetch(t *testing.T) {
	pkgData, err := os.ReadFile("testdata/npm/express.json")
	require.NoError(t, err)
	dlData, err := os.ReadFile("testdata/npm/express_downloads.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/downloads/point/last-month/express" {
			w.Write(dlData)
		} else {
			w.Write(pkgData)
		}
	}))
	defer srv.Close()

	client := registry.NewNPMClient(registry.WithBaseURL(srv.URL))
	info, err := client.Fetch("express", "4.18.2")
	require.NoError(t, err)
	assert.Equal(t, "express", info.Name)
	assert.NotZero(t, info.LastReleaseAt)
}
