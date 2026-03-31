package resolvers

import (
	"bufio"
	"context"
	"regexp"
	"strings"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
)

// ScriptResolver detects curl|sh and wget|sh patterns in files.
type ScriptResolver struct{}

// NewScriptResolver returns a new ScriptResolver.
func NewScriptResolver() *ScriptResolver { return &ScriptResolver{} }

var (
	// curlPipeSh matches `curl ... | sh` or `curl ... | bash`.
	curlPipeSh = regexp.MustCompile(`curl\s+.*\|\s*(sh|bash)`)
	// wgetPipeSh matches `wget ... | sh` or `wget ... | bash`.
	wgetPipeSh = regexp.MustCompile(`wget\s+.*\|\s*(sh|bash)`)
	// curlOutput matches `curl ... -o` or `curl ... -O` (download to file).
	curlOutput = regexp.MustCompile(`curl\s+.*-[oO]`)
	// urlPattern extracts URLs from a line.
	urlPattern = regexp.MustCompile(`https?://[^\s"'` + "`" + `\)]+`)
)

// Detect scans all files for curl|sh, wget|sh, and curl -o patterns.
func (r *ScriptResolver) Detect(_ context.Context, contents crawler.FileTree) ([]crawler.DepRef, error) {
	var refs []crawler.DepRef
	seen := make(map[string]bool)

	for _, data := range contents {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			if !isScriptLine(line) {
				continue
			}
			urls := urlPattern.FindAllString(line, -1)
			for _, u := range urls {
				if seen[u] {
					continue
				}
				seen[u] = true
				refs = append(refs, crawler.DepRef{
					Source:  crawler.DepSourceScript,
					Name:    u,
					Ref:     u,
					Pinning: graph.PinningUnpinned,
				})
			}
		}
	}
	return refs, nil
}

// isScriptLine checks if a line matches any of the known script download patterns.
func isScriptLine(line string) bool {
	return curlPipeSh.MatchString(line) ||
		wgetPipeSh.MatchString(line) ||
		curlOutput.MatchString(line)
}

// Resolve constructs a leaf ResolvedDep with ProjectID = the URL.
func (r *ScriptResolver) Resolve(_ context.Context, ref crawler.DepRef) (*crawler.ResolvedDep, error) {
	return &crawler.ResolvedDep{
		ProjectID:  ref.Name,
		VersionKey: ref.Ref,
		Contents:   nil,
	}, nil
}
