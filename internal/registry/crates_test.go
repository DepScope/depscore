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

func TestCratesFetch(t *testing.T) {
	data, err := os.ReadFile("testdata/crates/serde.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv.Close()

	client := registry.NewCratesClient(registry.WithBaseURL(srv.URL))
	info, err := client.Fetch("serde", "1.0.196")
	require.NoError(t, err)
	assert.Equal(t, "serde", info.Name)
	assert.Greater(t, info.TotalDownloads, int64(0))
}
