package resolve_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/depscope/depscope/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitLabResolver(t *testing.T) {
	treeJSON := `[
		{"id":"a1","name":"go.mod","type":"blob","path":"go.mod","mode":"100644"},
		{"id":"a2","name":"go.sum","type":"blob","path":"go.sum","mode":"100644"},
		{"id":"a3","name":"main.go","type":"blob","path":"cmd/main.go","mode":"100644"},
		{"id":"a4","name":"README.md","type":"blob","path":"README.md","mode":"100644"}
	]`

	gomodContent := "module example.com/myproject\n\ngo 1.22\n\nrequire github.com/spf13/cobra v1.8.0\n"
	gosumContent := "github.com/spf13/cobra v1.8.0 h1:abc123\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/repository/tree"):
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(treeJSON))
		case strings.Contains(r.URL.Path, "/repository/files/") && strings.HasSuffix(r.URL.Path, "/raw"):
			w.Header().Set("Content-Type", "text/plain")
			if strings.Contains(r.URL.Path, "go.mod") {
				w.Write([]byte(gomodContent))
			} else if strings.Contains(r.URL.Path, "go.sum") {
				w.Write([]byte(gosumContent))
			}
		default:
			// Default branch lookup
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"default_branch": "main"}`))
		}
	}))
	defer srv.Close()

	resolver := resolve.NewGitLabResolver("fake-token", resolve.WithBaseURL(srv.URL))
	files, cleanup, err := resolver.Resolve(context.Background(), "https://gitlab.com/org/project")
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

	for _, f := range files {
		if f.Path == "go.mod" {
			assert.Contains(t, string(f.Content), "module")
		}
	}
}

func TestGitLabResolverWithRef(t *testing.T) {
	treeJSON := `[{"id":"a1","name":"go.mod","type":"blob","path":"go.mod","mode":"100644"}]`
	gomodContent := "module example.com\n"

	var capturedRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/tree") {
			capturedRef = r.URL.Query().Get("ref")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(treeJSON))
		} else if strings.Contains(r.URL.Path, "/repository/files/") {
			w.Write([]byte(gomodContent))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"default_branch": "main"}`))
		}
	}))
	defer srv.Close()

	resolver := resolve.NewGitLabResolver("fake-token", resolve.WithBaseURL(srv.URL))
	_, cleanup, err := resolver.Resolve(context.Background(), "https://gitlab.com/org/project/-/tree/v1.0")
	defer cleanup()
	require.NoError(t, err)
	assert.Equal(t, "v1.0", capturedRef)
}
