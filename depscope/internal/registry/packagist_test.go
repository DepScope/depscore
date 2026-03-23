package registry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackagistFetch(t *testing.T) {
	// Synthetic JSON response
	data := `{"packages":{"laravel/framework":[{"name":"laravel/framework","version":"v13.0.1","authors":[{"name":"Taylor Otwell"}],"source":{"url":"https://github.com/laravel/framework.git"},"time":"2024-03-01T10:00:00+00:00"},{"name":"laravel/framework","version":"v13.0.0","authors":[{"name":"Taylor Otwell"}],"source":{"url":"https://github.com/laravel/framework.git"},"time":"2024-02-01T10:00:00+00:00"}]}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(data)) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewPackagistClient(WithBaseURL(srv.URL))
	info, err := client.Fetch("laravel/framework", "13.0.1")
	require.NoError(t, err)
	assert.Equal(t, "laravel/framework", info.Name)
	assert.Equal(t, 1, info.MaintainerCount)
	assert.Equal(t, 2, info.ReleaseCount)
	assert.NotZero(t, info.LastReleaseAt)
	assert.Contains(t, info.SourceRepoURL, "github.com")
	// .git suffix should be stripped
	assert.NotContains(t, info.SourceRepoURL, ".git")
}

func TestPackagistFetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewPackagistClient(WithBaseURL(srv.URL))
	_, err := client.Fetch("missing/package", "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestPackagistEcosystem(t *testing.T) {
	c := NewPackagistClient()
	assert.Equal(t, "Packagist", c.Ecosystem())
}
