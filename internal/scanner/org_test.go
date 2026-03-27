package scanner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListOrgRepos_TwoRepos verifies that listOrgReposFromBase returns both
// repo names when the API responds with a single page of results.
func TestListOrgRepos_TwoRepos(t *testing.T) {
	page1 := []map[string]string{
		{"name": "repo-alpha"},
		{"name": "repo-beta"},
	}
	page1JSON, _ := json.Marshal(page1)

	// The mock serves page 1 with two repos, then page 2 with an empty list.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/orgs/myorg/repos")
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))

		pageParam := r.URL.Query().Get("page")
		if pageParam == "" || pageParam == "1" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(page1JSON)
		} else {
			// Second page: empty — signals end of pagination
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("[]"))
		}
	}))
	defer srv.Close()

	repos, err := listOrgReposFromBase(context.Background(), "myorg", "test-token", srv.URL)
	require.NoError(t, err)
	assert.Equal(t, []string{"repo-alpha", "repo-beta"}, repos)
}

// TestListOrgRepos_Pagination verifies that pagination stops when an empty
// page is returned, collecting all repos across multiple pages.
func TestListOrgRepos_Pagination(t *testing.T) {
	page1 := []map[string]string{{"name": "repo-1"}, {"name": "repo-2"}}
	page2 := []map[string]string{{"name": "repo-3"}}
	page1JSON, _ := json.Marshal(page1)
	page2JSON, _ := json.Marshal(page2)

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		pageParam := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch pageParam {
		case "", "1":
			_, _ = w.Write(page1JSON)
		case "2":
			_, _ = w.Write(page2JSON)
		default:
			// Page 3+: empty, stop pagination
			_, _ = w.Write([]byte("[]"))
		}
	}))
	defer srv.Close()

	repos, err := listOrgReposFromBase(context.Background(), "myorg", "tok", srv.URL)
	require.NoError(t, err)
	assert.Equal(t, []string{"repo-1", "repo-2", "repo-3"}, repos)
	// Should have made exactly 3 HTTP calls (page 1, page 2, page 3 = empty)
	assert.Equal(t, 3, callCount)
}

// TestListOrgRepos_APIError verifies that a non-200 response from the API
// is propagated as an error with the status code in the message.
func TestListOrgRepos_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := listOrgReposFromBase(context.Background(), "myorg", "bad-token", srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	assert.Contains(t, err.Error(), "myorg")
}

// TestScanOrg_NoToken verifies that ScanOrg fails immediately when
// GITHUB_TOKEN is not set.
func TestScanOrg_NoToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	_, err := ScanOrg(context.Background(), "someorg", Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GITHUB_TOKEN")
}

// TestListOrgRepos_Empty verifies that an empty first page returns an empty
// slice (not nil) and no error.
func TestListOrgRepos_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	repos, err := listOrgReposFromBase(context.Background(), "emptyorg", "tok", srv.URL)
	require.NoError(t, err)
	assert.Empty(t, repos)
}

// TestListOrgRepos_AuthHeader verifies that the Authorization header is set
// when a token is provided.
func TestListOrgRepos_AuthHeader(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	_, err := listOrgReposFromBase(context.Background(), "myorg", "my-secret-token", srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "Bearer my-secret-token", capturedAuth)
}
