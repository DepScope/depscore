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

var ManifestFilenames = []string{
	"go.mod", "go.sum",
	"requirements.txt", "pyproject.toml", "poetry.lock", "uv.lock",
	"Cargo.toml", "Cargo.lock",
	"package.json", "package-lock.json", "pnpm-lock.yaml", "bun.lock",
	"composer.json", "composer.lock",
	"Dockerfile",
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
	return false
}
