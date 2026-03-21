package resolve_test

import (
	"testing"

	"github.com/depscope/depscope/internal/resolve"
	"github.com/stretchr/testify/assert"
)

func TestIsIgnoredDir(t *testing.T) {
	tests := []struct {
		path    string
		ignored bool
	}{
		{"node_modules/foo/package.json", true},
		{"vendor/github.com/foo/go.mod", true},
		{"target/debug/Cargo.toml", true},
		{".git/config", true},
		{"__pycache__/foo.pyc", true},
		{"dist/bundle.js", true},
		{"build/output.jar", true},
		{"src/go.mod", false},
		{"go.mod", false},
		{"services/api/node_modules/foo/package.json", true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.ignored, resolve.IsIgnoredDir(tt.path), tt.path)
	}
}

func TestMatchesManifest(t *testing.T) {
	tests := []struct {
		path    string
		matches bool
	}{
		{"go.mod", true},
		{"go.sum", true},
		{"requirements.txt", true},
		{"pyproject.toml", true},
		{"poetry.lock", true},
		{"uv.lock", true},
		{"Cargo.toml", true},
		{"Cargo.lock", true},
		{"package.json", true},
		{"package-lock.json", true},
		{"pnpm-lock.yaml", true},
		{"bun.lock", true},
		{"services/api/go.mod", true},
		{"README.md", false},
		{"main.go", false},
		{"node_modules/foo/package.json", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.matches, resolve.MatchesManifest(tt.path), tt.path)
	}
}
