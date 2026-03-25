// internal/actions/resolver_test.go
package actions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/depscope/depscope/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// actionYMLNode20 is the content of an action.yml for a node20 action.
const actionYMLNode20 = `
name: My Node Action
description: A node20 action
runs:
  using: node20
  main: dist/index.js
`

// actionYMLComposite is the content of an action.yml for a composite action.
const actionYMLComposite = `
name: My Composite Action
description: A composite action
runs:
  using: composite
  steps:
    - uses: actions/checkout@v3
      run: echo hello
`

// actionYMLDocker is the content of an action.yml for a docker action.
const actionYMLDocker = `
name: My Docker Action
description: A docker action
runs:
  using: docker
  image: Dockerfile
`

const testSHA = "abc123def456abc123def456abc123def456abcd"

func newTestServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, handler := range handlers {
		mux.HandleFunc(path, handler)
	}
	return httptest.NewServer(mux)
}

func makeContentResponse(content string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	resp := map[string]string{
		"content":  encoded,
		"encoding": "base64",
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

func makeRefResponse(sha string) string {
	resp := map[string]interface{}{
		"object": map[string]string{
			"sha": sha,
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

// newTestResolver creates a Resolver pointing at the test server with a temp cache dir.
func newTestResolver(t *testing.T, srv *httptest.Server) *Resolver {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "depscope-resolver-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	dc := cache.NewDiskCache(tmpDir)
	return NewResolver("fake-token", WithCache(dc), WithBaseURL(srv.URL))
}

// TestResolveTagToSHA tests that a tag ref is resolved to a SHA via the GitHub API,
// and that action.yml is fetched and parsed to determine the action type (node20).
func TestResolveTagToSHA(t *testing.T) {
	var refHits, contentHits int

	srv := newTestServer(t, map[string]http.HandlerFunc{
		"/repos/actions/checkout/git/ref/tags/v4": func(w http.ResponseWriter, r *http.Request) {
			refHits++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, makeRefResponse(testSHA))
		},
		"/repos/actions/checkout/contents/action.yml": func(w http.ResponseWriter, r *http.Request) {
			contentHits++
			assert.Equal(t, testSHA, r.URL.Query().Get("ref"))
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, makeContentResponse(actionYMLNode20))
		},
	})
	defer srv.Close()

	resolver := newTestResolver(t, srv)
	ref := ParseActionRef("actions/checkout@v4")

	resolved, err := resolver.Resolve(context.Background(), ref)
	require.NoError(t, err)
	require.NotNil(t, resolved)

	assert.Equal(t, testSHA, resolved.SHA)
	assert.Equal(t, ActionNode, resolved.Type)
	assert.Equal(t, PinMajorTag, resolved.Pinning)
	require.NotNil(t, resolved.ActionYAML)
	assert.Equal(t, "node20", resolved.ActionYAML.Runs.Using)
}

// TestResolveSHAPinnedSkipsTagLookup tests that a SHA-pinned ref skips the tag
// resolution step and goes straight to fetching action.yml.
func TestResolveSHAPinnedSkipsTagLookup(t *testing.T) {
	var refHits int

	srv := newTestServer(t, map[string]http.HandlerFunc{
		// This should NOT be called for a SHA-pinned ref
		"/repos/actions/checkout/git/ref/tags/" + testSHA: func(w http.ResponseWriter, r *http.Request) {
			refHits++
			t.Error("tag resolution should be skipped for SHA-pinned refs")
		},
		"/repos/actions/checkout/contents/action.yml": func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, testSHA, r.URL.Query().Get("ref"))
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, makeContentResponse(actionYMLNode20))
		},
	})
	defer srv.Close()

	resolver := newTestResolver(t, srv)
	ref := ParseActionRef("actions/checkout@" + testSHA)

	resolved, err := resolver.Resolve(context.Background(), ref)
	require.NoError(t, err)
	require.NotNil(t, resolved)

	assert.Equal(t, testSHA, resolved.SHA)
	assert.Equal(t, ActionNode, resolved.Type)
	assert.Equal(t, PinSHA, resolved.Pinning)
	assert.Equal(t, 0, refHits)
}

// TestResolveCompositeAction tests that a composite action is correctly detected.
func TestResolveCompositeAction(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"/repos/myorg/myaction/git/ref/tags/v1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, makeRefResponse(testSHA))
		},
		"/repos/myorg/myaction/contents/action.yml": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, makeContentResponse(actionYMLComposite))
		},
	})
	defer srv.Close()

	resolver := newTestResolver(t, srv)
	ref := ParseActionRef("myorg/myaction@v1")

	resolved, err := resolver.Resolve(context.Background(), ref)
	require.NoError(t, err)
	require.NotNil(t, resolved)

	assert.Equal(t, ActionComposite, resolved.Type)
	assert.Equal(t, "composite", resolved.ActionYAML.Runs.Using)
}

// TestResolveDockerAction tests that a docker action is correctly detected.
func TestResolveDockerAction(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"/repos/myorg/dockeraction/git/ref/tags/v2": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, makeRefResponse(testSHA))
		},
		"/repos/myorg/dockeraction/contents/action.yml": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, makeContentResponse(actionYMLDocker))
		},
	})
	defer srv.Close()

	resolver := newTestResolver(t, srv)
	ref := ParseActionRef("myorg/dockeraction@v2")

	resolved, err := resolver.Resolve(context.Background(), ref)
	require.NoError(t, err)
	require.NotNil(t, resolved)

	assert.Equal(t, ActionDocker, resolved.Type)
}

// TestResolveLocalRefSkipped tests that local (./path) refs are returned early
// without any API calls.
func TestResolveLocalRefSkipped(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		// No handlers; any request would be unexpected
	})
	defer srv.Close()

	resolver := newTestResolver(t, srv)
	ref := ParseActionRef("./local-action")

	resolved, err := resolver.Resolve(context.Background(), ref)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	// SHA is empty and type is unknown for local refs
	assert.Empty(t, resolved.SHA)
	assert.Equal(t, ActionUnknown, resolved.Type)
	assert.Nil(t, resolved.ActionYAML)
}

// TestResolveDockerImageRefSkipped tests that docker:// refs are returned early
// without any API calls.
func TestResolveDockerImageRefSkipped(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		// No handlers expected
	})
	defer srv.Close()

	resolver := newTestResolver(t, srv)
	ref := ParseActionRef("docker://alpine:3.19")

	resolved, err := resolver.Resolve(context.Background(), ref)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Empty(t, resolved.SHA)
	assert.Equal(t, ActionDocker, resolved.Type)
	assert.Nil(t, resolved.ActionYAML)
}

// TestResolveCachesTagToSHA tests that the second Resolve call for the same tag
// does not make another API call (cache hit).
func TestResolveCachesTagToSHA(t *testing.T) {
	var refHits, contentHits int

	srv := newTestServer(t, map[string]http.HandlerFunc{
		"/repos/actions/checkout/git/ref/tags/v4": func(w http.ResponseWriter, r *http.Request) {
			refHits++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, makeRefResponse(testSHA))
		},
		"/repos/actions/checkout/contents/action.yml": func(w http.ResponseWriter, r *http.Request) {
			contentHits++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, makeContentResponse(actionYMLNode20))
		},
	})
	defer srv.Close()

	resolver := newTestResolver(t, srv)
	ref := ParseActionRef("actions/checkout@v4")

	// First call — hits the API
	_, err := resolver.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, 1, refHits)
	assert.Equal(t, 1, contentHits)

	// Second call — should use cache
	_, err = resolver.Resolve(context.Background(), ref)
	require.NoError(t, err)
	assert.Equal(t, 1, refHits, "tag resolution should be cached")
	assert.Equal(t, 1, contentHits, "action.yml fetch should be cached")
}

// TestResolveFallsBackToActionYaml tests that when action.yml is not found,
// the resolver falls back to action.yaml.
func TestResolveFallsBackToActionYaml(t *testing.T) {
	srv := newTestServer(t, map[string]http.HandlerFunc{
		"/repos/actions/checkout/git/ref/tags/v4": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, makeRefResponse(testSHA))
		},
		"/repos/actions/checkout/contents/action.yml": func(w http.ResponseWriter, r *http.Request) {
			// Return 404 to trigger fallback to action.yaml
			http.NotFound(w, r)
		},
		"/repos/actions/checkout/contents/action.yaml": func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, testSHA, r.URL.Query().Get("ref"))
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, makeContentResponse(actionYMLNode20))
		},
	})
	defer srv.Close()

	resolver := newTestResolver(t, srv)
	ref := ParseActionRef("actions/checkout@v4")

	resolved, err := resolver.Resolve(context.Background(), ref)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, ActionNode, resolved.Type)
	require.NotNil(t, resolved.ActionYAML)
}
