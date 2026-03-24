package discover

import (
	"os"
	"path/filepath"
	"strings"
)

// ProjectInfo holds the location of a discovered project and the manifest
// files found within it. This struct will be superseded by the definition in
// walker.go once that file is created.
type ProjectInfo struct {
	Dir           string
	ManifestFiles []string
}

type MatchResult struct {
	Files   []string
	matched bool
}

func (m MatchResult) Bool() bool { return m.matched }

func MatchPackageInProject(pkgName string, project ProjectInfo) MatchResult {
	target := strings.ToLower(pkgName)
	var matchedFiles []string

	for _, filename := range project.ManifestFiles {
		path := filepath.Join(project.Dir, filename)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.ToLower(string(data))
		if strings.Contains(content, target) {
			matchedFiles = append(matchedFiles, filename)
		}
	}

	return MatchResult{
		Files:   matchedFiles,
		matched: len(matchedFiles) > 0,
	}
}
