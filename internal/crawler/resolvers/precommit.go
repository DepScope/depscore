package resolvers

import (
	"context"
	"strings"

	"github.com/depscope/depscope/internal/crawler"
	"go.yaml.in/yaml/v3"
)

// PrecommitResolver detects and resolves pre-commit hook dependencies.
type PrecommitResolver struct{}

// NewPrecommitResolver returns a new PrecommitResolver.
func NewPrecommitResolver() *PrecommitResolver { return &PrecommitResolver{} }

// precommitConfig represents the top-level .pre-commit-config.yaml structure.
type precommitConfig struct {
	Repos []precommitRepo `yaml:"repos"`
}

// precommitRepo is a single repo entry in a pre-commit config.
type precommitRepo struct {
	Repo string          `yaml:"repo"`
	Rev  string          `yaml:"rev"`
	Hooks []precommitHook `yaml:"hooks"`
}

type precommitHook struct {
	ID string `yaml:"id"`
}

// Detect looks for .pre-commit-config.yaml and extracts repo+rev references.
func (r *PrecommitResolver) Detect(_ context.Context, contents crawler.FileTree) ([]crawler.DepRef, error) {
	data, ok := contents[".pre-commit-config.yaml"]
	if !ok {
		return nil, nil
	}

	var cfg precommitConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, nil // malformed YAML — gracefully return empty
	}

	var refs []crawler.DepRef
	for _, repo := range cfg.Repos {
		if repo.Repo == "" || repo.Repo == "local" || repo.Repo == "meta" {
			continue
		}
		refs = append(refs, crawler.DepRef{
			Source:  crawler.DepSourcePrecommit,
			Name:    repo.Repo,
			Ref:     repo.Rev,
			Pinning: classifyPinning(repo.Rev),
		})
	}
	return refs, nil
}

// Resolve constructs a ResolvedDep from the DepRef.
func (r *PrecommitResolver) Resolve(_ context.Context, ref crawler.DepRef) (*crawler.ResolvedDep, error) {
	projectID := ref.Name
	// Strip common prefixes to produce a cleaner project ID.
	projectID = strings.TrimPrefix(projectID, "https://")
	projectID = strings.TrimPrefix(projectID, "http://")
	projectID = strings.TrimSuffix(projectID, ".git")

	return &crawler.ResolvedDep{
		ProjectID:  projectID,
		VersionKey: ref.Ref,
		Contents:   nil,
	}, nil
}
