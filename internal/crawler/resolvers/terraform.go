package resolvers

import (
	"context"
	"regexp"
	"strings"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
)

// TerraformResolver detects and resolves Terraform module dependencies.
type TerraformResolver struct{}

// NewTerraformResolver returns a new TerraformResolver.
func NewTerraformResolver() *TerraformResolver { return &TerraformResolver{} }

var (
	// tfModuleBlock matches `module "name" {` blocks.
	tfModuleBlock = regexp.MustCompile(`(?m)module\s+"([^"]+)"\s*\{`)
	// tfSourceAttr matches `source = "value"` inside a module block.
	tfSourceAttr = regexp.MustCompile(`(?m)\s+source\s*=\s*"([^"]+)"`)
	// tfVersionAttr matches `version = "value"` inside a module block.
	tfVersionAttr = regexp.MustCompile(`(?m)\s+version\s*=\s*"([^"]+)"`)
	// tfGitRef extracts ref from git URLs like `git::https://...?ref=v1.2.3`.
	tfGitRef = regexp.MustCompile(`[?&]ref=([^&"]+)`)
)

// Detect scans *.tf files for module blocks and extracts source/version.
func (r *TerraformResolver) Detect(_ context.Context, contents crawler.FileTree) ([]crawler.DepRef, error) {
	var refs []crawler.DepRef
	for path, data := range contents {
		if !strings.HasSuffix(path, ".tf") {
			continue
		}
		found := parseTerraformModules(string(data))
		refs = append(refs, found...)
	}
	return refs, nil
}

// parseTerraformModules extracts module references from Terraform HCL content.
func parseTerraformModules(content string) []crawler.DepRef {
	var refs []crawler.DepRef

	// Find all module blocks.
	blockStarts := tfModuleBlock.FindAllStringIndex(content, -1)
	for i, loc := range blockStarts {
		// Determine the end of the block: next module block or end of content.
		end := len(content)
		if i+1 < len(blockStarts) {
			end = blockStarts[i+1][0]
		}
		block := content[loc[0]:end]

		// Extract source.
		srcMatch := tfSourceAttr.FindStringSubmatch(block)
		if srcMatch == nil {
			continue
		}
		source := srcMatch[1]

		// Extract version (optional).
		var version string
		verMatch := tfVersionAttr.FindStringSubmatch(block)
		if verMatch != nil {
			version = verMatch[1]
		}

		ref := classifyTerraformRef(source, version)
		refs = append(refs, ref)
	}
	return refs
}

// classifyTerraformRef creates a DepRef from a Terraform module source and version.
func classifyTerraformRef(source, version string) crawler.DepRef {
	ref := crawler.DepRef{
		Source: crawler.DepSourceTerraform,
	}

	switch {
	case strings.HasPrefix(source, "git::") || strings.HasPrefix(source, "git@"):
		// Git-sourced module.
		ref.Name = source
		gitRef := ""
		if m := tfGitRef.FindStringSubmatch(source); m != nil {
			gitRef = m[1]
		}
		ref.Ref = gitRef
		ref.Pinning = classifyPinning(gitRef)
		if gitRef == "" {
			ref.Pinning = graph.PinningBranch
		}

	case strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../"):
		// Local module — skip.
		ref.Name = source
		ref.Pinning = graph.PinningNA

	default:
		// Registry module (e.g., "hashicorp/consul/aws").
		ref.Name = source
		ref.Ref = version
		if version != "" {
			ref.Pinning = graph.PinningExactVersion
		} else {
			ref.Pinning = graph.PinningUnpinned
		}
	}

	return ref
}

// Resolve constructs a ResolvedDep from the DepRef.
func (r *TerraformResolver) Resolve(_ context.Context, ref crawler.DepRef) (*crawler.ResolvedDep, error) {
	projectID := ref.Name
	// Clean git:: prefix for project ID.
	projectID = strings.TrimPrefix(projectID, "git::")
	projectID = strings.TrimPrefix(projectID, "https://")
	projectID = strings.TrimPrefix(projectID, "http://")
	// Strip query parameters.
	if idx := strings.Index(projectID, "?"); idx >= 0 {
		projectID = projectID[:idx]
	}
	projectID = strings.TrimSuffix(projectID, ".git")

	versionKey := ref.Ref
	if versionKey == "" {
		versionKey = "latest"
	}

	return &crawler.ResolvedDep{
		ProjectID:  projectID,
		VersionKey: versionKey,
		Contents:   nil,
	}, nil
}
