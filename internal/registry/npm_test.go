package registry

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNPMClient_Fetch(t *testing.T) {
	srv := serveGolden(t, "testdata/npm_express.json")
	defer srv.Close()

	client := NewNPMClient(WithBaseURL(srv.URL))
	info, err := client.Fetch("express", "4.18.2")
	require.NoError(t, err)

	assert.Equal(t, "express", info.Name)
	assert.Equal(t, "4.18.2", info.Version)
	assert.Equal(t, "npm", info.Ecosystem)
	assert.Equal(t, 2, info.MaintainerCount)
	assert.Equal(t, "https://github.com/expressjs/express", info.SourceRepoURL)

	want := time.Date(2023, 10, 12, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, want, info.LastReleaseAt)
}

func TestNPMClient_Fetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewNPMClient(WithBaseURL(srv.URL))
	_, err := client.Fetch("missing", "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestNPMClient_Ecosystem(t *testing.T) {
	c := NewNPMClient()
	assert.Equal(t, "npm", c.Ecosystem())
}
