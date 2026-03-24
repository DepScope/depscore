package registry

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

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

func TestPyPIClient_Fetch(t *testing.T) {
	srv := serveGolden(t, "testdata/pypi_requests.json")
	defer srv.Close()

	client := NewPyPIClient(WithBaseURL(srv.URL))
	info, err := client.Fetch("requests", "2.31.0")
	require.NoError(t, err)

	assert.Equal(t, "requests", info.Name)
	assert.Equal(t, "2.31.0", info.Version)
	assert.Equal(t, "PyPI", info.Ecosystem)
	assert.Equal(t, 1, info.MaintainerCount)
	assert.Equal(t, "https://github.com/psf/requests", info.SourceRepoURL)
	assert.False(t, info.IsDeprecated)
	assert.Equal(t, 2, info.ReleaseCount)

	want := time.Date(2023, 5, 22, 15, 12, 34, 0, time.UTC)
	assert.Equal(t, want, info.LastReleaseAt)
}

func TestPyPIClient_Fetch_Deprecated(t *testing.T) {
	srv := serveGolden(t, "testdata/pypi_deprecated.json")
	defer srv.Close()

	client := NewPyPIClient(WithBaseURL(srv.URL))
	info, err := client.Fetch("old-package", "1.0.0")
	require.NoError(t, err)

	assert.True(t, info.IsDeprecated)
	assert.Equal(t, 0, info.MaintainerCount)
	assert.Empty(t, info.SourceRepoURL)
}

func TestPyPIClient_Fetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewPyPIClient(WithBaseURL(srv.URL))
	_, err := client.Fetch("missing", "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestPyPIClient_Ecosystem(t *testing.T) {
	c := NewPyPIClient()
	assert.Equal(t, "PyPI", c.Ecosystem())
}
