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

func TestGoProxyFetch(t *testing.T) {
	data, err := os.ReadFile("testdata/goproxy/cobra.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv.Close()

	client := registry.NewGoProxyClient(registry.WithBaseURL(srv.URL))
	info, err := client.Fetch("github.com/spf13/cobra", "v1.8.0")
	require.NoError(t, err)
	assert.Equal(t, "github.com/spf13/cobra", info.Name)
	assert.NotZero(t, info.LastReleaseAt)
}
