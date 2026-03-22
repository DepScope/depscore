package registry

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoProxyClient_Fetch(t *testing.T) {
	srv := serveGolden(t, "testdata/goproxy_gin.json")
	defer srv.Close()

	client := NewGoProxyClient(WithBaseURL(srv.URL))
	info, err := client.Fetch("github.com/gin-gonic/gin", "v1.8.0")
	require.NoError(t, err)

	assert.Equal(t, "github.com/gin-gonic/gin", info.Name)
	assert.Equal(t, "v1.8.0", info.Version)
	assert.Equal(t, "Go", info.Ecosystem)

	want := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, want, info.LastReleaseAt)
}

func TestGoProxyClient_Fetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewGoProxyClient(WithBaseURL(srv.URL))
	_, err := client.Fetch("github.com/missing/pkg", "v1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestGoProxyClient_Ecosystem(t *testing.T) {
	c := NewGoProxyClient()
	assert.Equal(t, "Go", c.Ecosystem())
}
