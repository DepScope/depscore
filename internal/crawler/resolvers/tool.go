package resolvers

import (
	"bufio"
	"context"
	"strings"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
	toml "github.com/pelletier/go-toml/v2"
)

// ToolResolver detects and resolves dev tool version specifications.
type ToolResolver struct{}

// NewToolResolver returns a new ToolResolver.
func NewToolResolver() *ToolResolver { return &ToolResolver{} }

// Detect looks for .tool-versions and .mise.toml and extracts tool+version pairs.
func (r *ToolResolver) Detect(_ context.Context, contents crawler.FileTree) ([]crawler.DepRef, error) {
	var refs []crawler.DepRef

	if data, ok := contents[".tool-versions"]; ok {
		refs = append(refs, parseToolVersions(data)...)
	}
	if data, ok := contents[".mise.toml"]; ok {
		refs = append(refs, parseMiseToml(data)...)
	}

	return refs, nil
}

// parseToolVersions parses the `.tool-versions` format: one tool per line,
// `tool version` separated by whitespace.
func parseToolVersions(data []byte) []crawler.DepRef {
	var refs []crawler.DepRef
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		tool := fields[0]
		version := fields[1]
		refs = append(refs, crawler.DepRef{
			Source:  crawler.DepSourceTool,
			Name:    tool,
			Ref:     version,
			Pinning: graph.PinningExactVersion,
		})
	}
	return refs
}

// miseConfig is a minimal representation of .mise.toml.
type miseConfig struct {
	Tools map[string]any `toml:"tools"`
}

// parseMiseToml parses the `.mise.toml` file and extracts tools from the [tools] section.
func parseMiseToml(data []byte) []crawler.DepRef {
	var cfg miseConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	var refs []crawler.DepRef
	for tool, versionVal := range cfg.Tools {
		var version string
		switch v := versionVal.(type) {
		case string:
			version = v
		case []any:
			// Array of versions — use the first one.
			if len(v) > 0 {
				if s, ok := v[0].(string); ok {
					version = s
				}
			}
		default:
			continue
		}
		if version == "" {
			continue
		}
		refs = append(refs, crawler.DepRef{
			Source:  crawler.DepSourceTool,
			Name:    tool,
			Ref:     version,
			Pinning: graph.PinningExactVersion,
		})
	}
	return refs
}

// Resolve constructs a leaf ResolvedDep (no contents for recursion).
func (r *ToolResolver) Resolve(_ context.Context, ref crawler.DepRef) (*crawler.ResolvedDep, error) {
	return &crawler.ResolvedDep{
		ProjectID:  "tool/" + ref.Name,
		VersionKey: ref.Name + "@" + ref.Ref,
		Contents:   nil,
	}, nil
}
