package resolve

import (
	"path/filepath"
	"strings"
)

var IgnoredDirs = []string{
	"node_modules",
	"vendor",
	"target",
	".git",
	"__pycache__",
	"dist",
	"build",
}

// ManifestFilenames lists exact filenames to include in remote file fetches.
var ManifestFilenames = []string{
	// Package manifests + lockfiles
	"go.mod", "go.sum",
	"requirements.txt", "pyproject.toml", "poetry.lock", "uv.lock",
	"Cargo.toml", "Cargo.lock",
	"package.json", "package-lock.json", "pnpm-lock.yaml", "bun.lock",
	"composer.json", "composer.lock",
	"Dockerfile",
	// Pre-commit
	".pre-commit-config.yaml",
	// Git submodules
	".gitmodules",
	// Dev tools
	".tool-versions", ".mise.toml",
	// Build tools
	"Makefile", "Taskfile.yml", "Taskfile.yaml", "justfile",
}

// ManifestPatterns lists file extensions/path patterns to match (beyond exact names).
var ManifestPatterns = []func(string) bool{
	// GitHub Actions: .github/workflows/*.yml
	func(path string) bool {
		p := filepath.ToSlash(path)
		return strings.Contains(p, ".github/workflows/") && strings.HasSuffix(p, ".yml")
	},
	// Terraform: *.tf
	func(path string) bool {
		return strings.HasSuffix(path, ".tf")
	},
}

func IsIgnoredDir(path string) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts {
		for _, ignored := range IgnoredDirs {
			if part == ignored {
				return true
			}
		}
	}
	return false
}

// MatchesManifest returns true if the path is a file that should be fetched
// from remote repos for dependency scanning.
func MatchesManifest(path string) bool {
	if IsIgnoredDir(path) {
		return false
	}
	base := filepath.Base(path)
	for _, name := range ManifestFilenames {
		if base == name {
			return true
		}
	}
	for _, matchFn := range ManifestPatterns {
		if matchFn(path) {
			return true
		}
	}
	return false
}
