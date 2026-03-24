package registry

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCratesClient_Fetch(t *testing.T) {
	srv := serveGolden(t, "testdata/crates_serde.json")
	defer srv.Close()

	client := NewCratesClient(WithBaseURL(srv.URL))
	info, err := client.Fetch("serde", "1.0.196")
	require.NoError(t, err)

	assert.Equal(t, "serde", info.Name)
	assert.Equal(t, "1.0.196", info.Version)
	assert.Equal(t, "crates.io", info.Ecosystem)
	assert.Equal(t, int64(150000000), info.TotalDownloads)
	assert.Equal(t, "https://github.com/serde-rs/serde", info.SourceRepoURL)
	assert.Equal(t, 2, info.ReleaseCount)

	want := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, want, info.LastReleaseAt)
}

func TestCratesClient_Fetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewCratesClient(WithBaseURL(srv.URL))
	_, err := client.Fetch("missing", "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestCratesClient_Ecosystem(t *testing.T) {
	c := NewCratesClient()
	assert.Equal(t, "crates.io", c.Ecosystem())
}
