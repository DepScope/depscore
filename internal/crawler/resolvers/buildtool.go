package resolvers

import (
	"bufio"
	"context"
	"regexp"
	"strings"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
)

// buildToolFiles lists filenames to scan for install patterns.
var buildToolFiles = map[string]bool{
	"Makefile":      true,
	"Taskfile.yml":  true,
	"Taskfile.yaml": true,
	"justfile":      true,
}

// BuildToolResolver detects install patterns inside build tool files.
type BuildToolResolver struct{}

// NewBuildToolResolver returns a new BuildToolResolver.
func NewBuildToolResolver() *BuildToolResolver { return &BuildToolResolver{} }

var (
	// btCurlPipeSh matches curl ... | sh/bash inside build files.
	btCurlPipeSh = regexp.MustCompile(`curl\s+.*\|\s*(sh|bash)`)
	// btWgetPipeSh matches wget ... | sh/bash inside build files.
	btWgetPipeSh = regexp.MustCompile(`wget\s+.*\|\s*(sh|bash)`)
	// btGoInstall matches `go install pkg@version`.
	btGoInstall = regexp.MustCompile(`go\s+install\s+(\S+)@(\S+)`)
	// btPipInstall matches `pip install pkg`.
	btPipInstall = regexp.MustCompile(`pip\s+install\s+(\S+)`)
	// btNpmInstall matches `npm install -g pkg`.
	btNpmInstall = regexp.MustCompile(`npm\s+install\s+-g\s+(\S+)`)
)

// Detect scans build tool files for install patterns.
func (r *BuildToolResolver) Detect(_ context.Context, contents crawler.FileTree) ([]crawler.DepRef, error) {
	var refs []crawler.DepRef
	seen := make(map[string]bool)

	for path, data := range contents {
		if !buildToolFiles[path] {
			continue
		}

		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			found := parseBuildToolLine(line)
			for _, ref := range found {
				key := ref.Name + "@" + ref.Ref
				if seen[key] {
					continue
				}
				seen[key] = true
				refs = append(refs, ref)
			}
		}
	}
	return refs, nil
}

// parseBuildToolLine extracts install references from a single line.
func parseBuildToolLine(line string) []crawler.DepRef {
	var refs []crawler.DepRef

	// curl|sh or wget|sh — extract URLs.
	if btCurlPipeSh.MatchString(line) || btWgetPipeSh.MatchString(line) {
		urls := urlPattern.FindAllString(line, -1)
		for _, u := range urls {
			refs = append(refs, crawler.DepRef{
				Source:  crawler.DepSourceBuildTool,
				Name:    u,
				Ref:     u,
				Pinning: graph.PinningUnpinned,
			})
		}
	}

	// go install pkg@version
	if matches := btGoInstall.FindAllStringSubmatch(line, -1); matches != nil {
		for _, m := range matches {
			refs = append(refs, crawler.DepRef{
				Source:  crawler.DepSourceBuildTool,
				Name:    m[1],
				Ref:     m[2],
				Pinning: classifyPinning(m[2]),
			})
		}
	}

	// pip install pkg
	if matches := btPipInstall.FindAllStringSubmatch(line, -1); matches != nil {
		for _, m := range matches {
			pkg := m[1]
			// Skip flags.
			if strings.HasPrefix(pkg, "-") {
				continue
			}
			refs = append(refs, crawler.DepRef{
				Source:    crawler.DepSourceBuildTool,
				Name:      pkg,
				Ref:       "latest",
				Ecosystem: "python",
				Pinning:   graph.PinningUnpinned,
			})
		}
	}

	// npm install -g pkg
	if matches := btNpmInstall.FindAllStringSubmatch(line, -1); matches != nil {
		for _, m := range matches {
			refs = append(refs, crawler.DepRef{
				Source:    crawler.DepSourceBuildTool,
				Name:      m[1],
				Ref:       "latest",
				Ecosystem: "npm",
				Pinning:   graph.PinningUnpinned,
			})
		}
	}

	return refs
}

// Resolve constructs a leaf ResolvedDep.
func (r *BuildToolResolver) Resolve(_ context.Context, ref crawler.DepRef) (*crawler.ResolvedDep, error) {
	return &crawler.ResolvedDep{
		ProjectID:  ref.Name,
		VersionKey: ref.Name + "@" + ref.Ref,
		Contents:   nil,
	}, nil
}
