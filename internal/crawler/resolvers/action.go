package resolvers

import (
	"context"
	"regexp"
	"strings"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
	"go.yaml.in/yaml/v3"
)

var (
	// shaPattern matches 40-character hexadecimal strings (full SHA).
	shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	// semverExact matches exact semver tags like v1.2.3 or v1.2.3-beta.1.
	semverExact = regexp.MustCompile(`^v?\d+\.\d+\.\d+`)
	// majorTag matches major version tags like v1, v2, v12.
	majorTag = regexp.MustCompile(`^v?\d+$`)
)

// ActionResolver detects and resolves GitHub Actions workflow dependencies.
type ActionResolver struct{}

// NewActionResolver returns a new ActionResolver.
func NewActionResolver() *ActionResolver { return &ActionResolver{} }

// workflowFile is a minimal YAML structure for GitHub Actions workflows.
type workflowFile struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Uses  string         `yaml:"uses"`
	Steps []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Uses string `yaml:"uses"`
}

// Detect scans the FileTree for .github/workflows/*.yml files and extracts
// uses: references from them.
func (r *ActionResolver) Detect(_ context.Context, contents crawler.FileTree) ([]crawler.DepRef, error) {
	var refs []crawler.DepRef
	for path, data := range contents {
		if !isWorkflowFile(path) {
			continue
		}
		found := parseWorkflowUses(data)
		refs = append(refs, found...)
	}
	return refs, nil
}

// isWorkflowFile returns true if the path matches .github/workflows/*.yml or *.yaml.
func isWorkflowFile(path string) bool {
	if !strings.HasPrefix(path, ".github/workflows/") {
		return false
	}
	return strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")
}

// parseWorkflowUses extracts action references from a workflow YAML file.
func parseWorkflowUses(data []byte) []crawler.DepRef {
	var wf workflowFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var refs []crawler.DepRef

	addRef := func(uses string) {
		if uses == "" || seen[uses] {
			return
		}
		// Skip local actions (./path) and Docker references.
		if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "docker://") {
			return
		}
		seen[uses] = true
		ref := parseActionRef(uses)
		refs = append(refs, ref)
	}

	for _, job := range wf.Jobs {
		addRef(job.Uses)
		for _, step := range job.Steps {
			addRef(step.Uses)
		}
	}
	return refs
}

// parseActionRef parses an "owner/repo@ref" or "owner/repo/path@ref" string
// into a DepRef.
func parseActionRef(uses string) crawler.DepRef {
	ref := crawler.DepRef{
		Source: crawler.DepSourceAction,
	}

	atIdx := strings.LastIndex(uses, "@")
	if atIdx < 0 {
		ref.Name = uses
		ref.Ref = ""
		ref.Pinning = graph.PinningUnpinned
		return ref
	}

	nameStr := uses[:atIdx]
	refStr := uses[atIdx+1:]

	ref.Name = nameStr
	ref.Ref = refStr
	ref.Pinning = classifyPinning(refStr)
	return ref
}

// classifyPinning determines the PinningQuality for a version reference.
func classifyPinning(refStr string) graph.PinningQuality {
	switch {
	case shaPattern.MatchString(refStr):
		return graph.PinningSHA
	case semverExact.MatchString(refStr):
		return graph.PinningExactVersion
	case majorTag.MatchString(refStr):
		return graph.PinningMajorTag
	default:
		return graph.PinningBranch
	}
}

// Resolve constructs a ResolvedDep from the DepRef.
func (r *ActionResolver) Resolve(_ context.Context, ref crawler.DepRef) (*crawler.ResolvedDep, error) {
	// Extract owner/repo from the name (strip any sub-path).
	ownerRepo := ref.Name
	parts := strings.SplitN(ownerRepo, "/", 3)
	if len(parts) >= 2 {
		ownerRepo = parts[0] + "/" + parts[1]
	}

	return &crawler.ResolvedDep{
		ProjectID:  "github.com/" + ownerRepo,
		VersionKey: ref.Ref,
		Contents:   nil,
	}, nil
}
