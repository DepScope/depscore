// internal/discover/walker.go
package discover

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/resolve"
)

// ProjectInfo holds a discovered project directory and its manifest files.
type ProjectInfo struct {
	Dir           string   // absolute path to project root
	ManifestFiles []string // basenames of manifest/lockfile files found
}

// WalkProjects walks the filesystem from startPath, finding directories
// that contain manifest files. Skips ignored directories and symlinks.
// Filters by ecosystem if specified (empty string means all ecosystems).
func WalkProjects(startPath string, maxDepth int, ecosystem string) ([]ProjectInfo, error) {
	startPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil, err
	}

	var projects []ProjectInfo
	seen := make(map[string]bool)

	// Use WalkDir instead of Walk so we can detect symlinks via DirEntry.Type()
	err = filepath.WalkDir(startPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Permission denied etc — skip, continue
			return nil
		}

		// Don't follow symlinks (WalkDir exposes the link type, not the target)
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		if d.IsDir() {
			// Check depth
			rel, _ := filepath.Rel(startPath, path)
			depth := len(strings.Split(rel, string(filepath.Separator)))
			if rel == "." {
				depth = 0
			}
			if depth > maxDepth {
				return filepath.SkipDir
			}

			// Skip ignored directories
			if resolve.IsIgnoredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if this is a manifest file
		base := d.Name()
		isManifest := false
		for _, name := range resolve.ManifestFilenames {
			if base == name {
				isManifest = true
				break
			}
		}
		if !isManifest {
			return nil
		}

		dir := filepath.Dir(path)
		if seen[dir] {
			// Already found this project, just add the file
			for i := range projects {
				if projects[i].Dir == dir {
					projects[i].ManifestFiles = append(projects[i].ManifestFiles, base)
					break
				}
			}
			return nil
		}

		// Ecosystem filter
		if ecosystem != "" {
			eco := ecosystemForFile(base)
			if eco != manifest.Ecosystem(ecosystem) {
				return nil
			}
		}

		seen[dir] = true
		projects = append(projects, ProjectInfo{
			Dir:           dir,
			ManifestFiles: []string{base},
		})
		return nil
	})

	return projects, err
}

// ReadProjectList reads a file containing project paths (one per line).
// Lines starting with # are comments. Empty lines are skipped.
func ReadProjectList(listFile string, ecosystem string) ([]ProjectInfo, error) {
	f, err := os.Open(listFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var projects []ProjectInfo
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check if it's a local path with manifest files
		info, err := os.Stat(line)
		if err != nil || !info.IsDir() {
			continue
		}

		var manifests []string
		for _, name := range resolve.ManifestFilenames {
			if _, err := os.Stat(filepath.Join(line, name)); err == nil {
				if ecosystem != "" && ecosystemForFile(name) != manifest.Ecosystem(ecosystem) {
					continue
				}
				manifests = append(manifests, name)
			}
		}
		if len(manifests) > 0 {
			projects = append(projects, ProjectInfo{
				Dir:           line,
				ManifestFiles: manifests,
			})
		}
	}
	return projects, scanner.Err()
}

// ecosystemForFile maps a manifest filename to its ecosystem.
func ecosystemForFile(filename string) manifest.Ecosystem {
	switch filename {
	case "go.mod", "go.sum":
		return manifest.EcosystemGo
	case "requirements.txt", "pyproject.toml", "poetry.lock", "uv.lock":
		return manifest.EcosystemPython
	case "Cargo.toml", "Cargo.lock":
		return manifest.EcosystemRust
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "bun.lock":
		return manifest.EcosystemNPM
	case "composer.json", "composer.lock":
		return manifest.EcosystemPHP
	default:
		return ""
	}
}
