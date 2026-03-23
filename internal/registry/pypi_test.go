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

func TestPyPIFetch(t *testing.T) {
	data, err := os.ReadFile("testdata/pypi/requests.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv.Close()

	client := registry.NewPyPIClient(registry.WithBaseURL(srv.URL))
	info, err := client.Fetch("requests", "2.31.0")
	require.NoError(t, err)
	assert.Equal(t, "requests", info.Name)
	assert.NotZero(t, info.LastReleaseAt)
	assert.Greater(t, info.MaintainerCount, 0)
}
