package resolvers

import (
	"bufio"
	"context"
	"strings"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
)

// SubmoduleResolver detects and resolves git submodule dependencies.
type SubmoduleResolver struct{}

// NewSubmoduleResolver returns a new SubmoduleResolver.
func NewSubmoduleResolver() *SubmoduleResolver { return &SubmoduleResolver{} }

// Detect looks for .gitmodules and extracts submodule URLs.
func (r *SubmoduleResolver) Detect(_ context.Context, contents crawler.FileTree) ([]crawler.DepRef, error) {
	data, ok := contents[".gitmodules"]
	if !ok {
		return nil, nil
	}

	return parseGitmodules(data), nil
}

// parseGitmodules parses an INI-like .gitmodules file and extracts submodule URLs.
func parseGitmodules(data []byte) []crawler.DepRef {
	var refs []crawler.DepRef
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	var currentName string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header: [submodule "name"]
		if strings.HasPrefix(line, "[submodule") {
			// Extract the name between quotes.
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start >= 0 && end > start {
				currentName = line[start+1 : end]
			}
			continue
		}

		// Key = value
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			if key == "url" && value != "" {
				name := currentName
				if name == "" {
					name = value
				}
				refs = append(refs, crawler.DepRef{
					Source:  crawler.DepSourceSubmodule,
					Name:    name,
					Ref:     value,
					Pinning: graph.PinningBranch, // submodules track a branch by default
				})
			}
		}
	}
	return refs
}

// Resolve constructs a ResolvedDep from the DepRef.
func (r *SubmoduleResolver) Resolve(_ context.Context, ref crawler.DepRef) (*crawler.ResolvedDep, error) {
	projectID := ref.Ref
	// Clean up the URL to produce a project ID.
	projectID = strings.TrimPrefix(projectID, "https://")
	projectID = strings.TrimPrefix(projectID, "http://")
	projectID = strings.TrimPrefix(projectID, "git@")
	projectID = strings.TrimSuffix(projectID, ".git")
	// Convert git@github.com:org/repo format.
	projectID = strings.Replace(projectID, ":", "/", 1)

	return &crawler.ResolvedDep{
		ProjectID:  projectID,
		VersionKey: ref.Name,
		Contents:   nil,
	}, nil
}
