package vcs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupGitHubServer creates a test server that serves golden files for repo
// and contributors endpoints.
func setupGitHubServer(t *testing.T) *httptest.Server {
	t.Helper()

	repoData, err := os.ReadFile("testdata/github_repo.json")
	require.NoError(t, err)

	contribData, err := os.ReadFile("testdata/github_contributors.json")
	require.NoError(t, err)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/contributors") {
			_, _ = w.Write(contribData)
		} else {
			_, _ = w.Write(repoData)
		}
	}))
}

func TestGitHubClient_FetchRepo(t *testing.T) {
	srv := setupGitHubServer(t)
	defer srv.Close()

	client := NewGitHubClient(WithBaseURL(srv.URL))
	info, err := client.FetchRepo("psf", "requests")
	require.NoError(t, err)

	assert.Equal(t, "psf", info.Owner)
	assert.Equal(t, "requests", info.Repo)
	assert.Equal(t, 120, info.OpenIssueCount)
	assert.Equal(t, 51000, info.StarCount)
	assert.False(t, info.IsArchived)
	assert.True(t, info.HasOrgBacking)
	assert.Equal(t, 2, info.ContributorCount)

	want := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, want, info.LastCommitAt)
}

func TestGitHubClient_RepoFromURL(t *testing.T) {
	srv := setupGitHubServer(t)
	defer srv.Close()

	client := NewGitHubClient(WithBaseURL(srv.URL))

	tests := []struct {
		name      string
		sourceURL string
	}{
		{"https URL", "https://github.com/psf/requests"},
		{"git+ prefix", "git+https://github.com/psf/requests.git"},
		{"trailing .git", "https://github.com/psf/requests.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := client.RepoFromURL(tt.sourceURL)
			require.NoError(t, err)
			assert.Equal(t, "psf", info.Owner)
			assert.Equal(t, "requests", info.Repo)
		})
	}
}

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		rawURL      string
		wantOwner   string
		wantRepo    string
		wantErrFrag string
	}{
		{"https://github.com/psf/requests", "psf", "requests", ""},
		{"https://github.com/psf/requests.git", "psf", "requests", ""},
		{"git+https://github.com/expressjs/express.git", "expressjs", "express", ""},
		{"not-a-url", "", "", "cannot extract"},
	}

	for _, tt := range tests {
		owner, repo, err := parseGitHubURL(tt.rawURL)
		if tt.wantErrFrag != "" {
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrFrag)
		} else {
			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		}
	}
}

func TestGitHubClient_FetchRepo_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := NewGitHubClient(WithBaseURL(srv.URL))
	_, err := client.FetchRepo("missing", "repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}
