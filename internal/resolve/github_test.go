package resolve_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/depscope/depscope/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubResolver(t *testing.T) {
	// Synthetic tree with go.mod, go.sum, main.go, README.md, and node_modules/foo/package.json
	treeResp := map[string]interface{}{
		"sha":       "abc123",
		"truncated": false,
		"tree": []map[string]string{
			{"path": "go.mod", "type": "blob"},
			{"path": "go.sum", "type": "blob"},
			{"path": "cmd/main.go", "type": "blob"},
			{"path": "README.md", "type": "blob"},
			{"path": "node_modules/foo/package.json", "type": "blob"},
		},
	}
	treeData, _ := json.Marshal(treeResp)

	// go.mod content: "module example.com\n\ngo 1.22\n" in base64 with newlines (as GitHub returns)
	gomodB64 := "bW9kdWxlIGV4YW1wbGUuY29tCgpnbyAxLjIyCg=="
	gomodContents := map[string]interface{}{
		"name": "go.mod", "encoding": "base64",
		"content": gomodB64[:20] + "\n" + gomodB64[20:], // simulate GitHub's newline-split base64
	}
	gomodData, _ := json.Marshal(gomodContents)

	gosumContents := map[string]interface{}{
		"name": "go.sum", "encoding": "base64",
		"content": "Z2l0aHViLmNvbS9mb28vYmFyIHYxLjAuMAo=", // "github.com/foo/bar v1.0.0\n"
	}
	gosumData, _ := json.Marshal(gosumContents)

	repoResp := `{"default_branch": "main"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/git/trees/"):
			_, _ = w.Write(treeData)
		case strings.HasSuffix(r.URL.Path, "/go.mod"):
			_, _ = w.Write(gomodData)
		case strings.HasSuffix(r.URL.Path, "/go.sum"):
			_, _ = w.Write(gosumData)
		case strings.HasSuffix(r.URL.Path, "/spf13/cobra"):
			_, _ = w.Write([]byte(repoResp))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	resolver := resolve.NewGitHubResolver("fake-token", resolve.WithBaseURL(srv.URL))
	files, cleanup, err := resolver.Resolve(context.Background(), "https://github.com/spf13/cobra")
	defer cleanup()
	require.NoError(t, err)

	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Path
	}
	assert.Contains(t, names, "go.mod")
	assert.Contains(t, names, "go.sum")
	assert.NotContains(t, names, "cmd/main.go")
	assert.NotContains(t, names, "README.md")
	assert.NotContains(t, names, "node_modules/foo/package.json")

	// Verify content decoding with newlines
	for _, f := range files {
		if f.Path == "go.mod" {
			assert.Contains(t, string(f.Content), "module example.com")
		}
	}
}

func TestGitHubResolverTruncatedTree(t *testing.T) {
	treeResp := map[string]interface{}{
		"sha":       "abc123",
		"truncated": true,
		"tree": []map[string]string{
			{"path": "go.mod", "type": "blob"},
		},
	}
	data, _ := json.Marshal(treeResp)

	gomodContent := map[string]interface{}{
		"name": "go.mod", "encoding": "base64",
		"content": "bW9kdWxlIGV4YW1wbGUuY29t",
	}
	gomodData, _ := json.Marshal(gomodContent)

	repoResp := `{"default_branch": "main"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/git/trees/"):
			_, _ = w.Write(data)
		case strings.Contains(r.URL.Path, "/contents/"):
			_, _ = w.Write(gomodData)
		default:
			_, _ = w.Write([]byte(repoResp))
		}
	}))
	defer srv.Close()

	resolver := resolve.NewGitHubResolver("fake-token", resolve.WithBaseURL(srv.URL))
	files, cleanup, err := resolver.Resolve(context.Background(), "https://github.com/owner/repo")
	defer cleanup()
	require.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Equal(t, "go.mod", files[0].Path)
}

func TestGitHubResolverWithRef(t *testing.T) {
	treeResp := map[string]interface{}{
		"sha": "abc", "truncated": false,
		"tree": []map[string]string{{"path": "go.mod", "type": "blob"}},
	}
	treeData, _ := json.Marshal(treeResp)

	gomodContent := map[string]interface{}{
		"name": "go.mod", "encoding": "base64",
		"content": "bW9kdWxlIGV4YW1wbGUuY29t",
	}
	gomodData, _ := json.Marshal(gomodContent)

	var capturedTreeRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/git/trees/") {
			parts := strings.Split(r.URL.Path, "/git/trees/")
			if len(parts) == 2 {
				capturedTreeRef = strings.Split(parts[1], "?")[0]
			}
			_, _ = w.Write(treeData)
		} else {
			_, _ = w.Write(gomodData)
		}
	}))
	defer srv.Close()

	resolver := resolve.NewGitHubResolver("fake-token", resolve.WithBaseURL(srv.URL))
	_, cleanup, err := resolver.Resolve(context.Background(), "https://github.com/owner/repo/tree/v2.0")
	defer cleanup()
	require.NoError(t, err)
	assert.Equal(t, "v2.0", capturedTreeRef)
}
