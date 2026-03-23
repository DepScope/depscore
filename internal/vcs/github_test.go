package vcs_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/depscope/depscope/internal/vcs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubFetchRepo(t *testing.T) {
	repoData, err := os.ReadFile("testdata/repo_requests.json")
	require.NoError(t, err)
	contribData, err := os.ReadFile("testdata/contributors_requests.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "contributors") {
			w.Header().Set("X-Total-Count", "250")
			w.Write(contribData)
		} else {
			w.Write(repoData)
		}
	}))
	defer srv.Close()

	client := vcs.NewGitHubClient("", vcs.WithBaseURL(srv.URL))
	info, err := client.FetchRepo("psf", "requests")
	require.NoError(t, err)
	assert.Greater(t, info.ContributorCount, 10)
	assert.False(t, info.IsArchived)
	assert.True(t, info.HasOrgBacking)
	assert.NotZero(t, info.LastCommitAt)
}
