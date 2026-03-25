// internal/actions/parser.go
package actions

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// workflowYAML is the raw YAML structure for a GitHub Actions workflow file.
type workflowYAML struct {
	Permissions any                    `yaml:"permissions"` // string or map[string]string
	Jobs        map[string]workflowJob `yaml:"jobs"`
}

// workflowJob is the raw YAML structure for a single job in a workflow.
type workflowJob struct {
	Uses      string        `yaml:"uses"`      // reusable workflow reference
	Container any           `yaml:"container"` // string or {image: string}
	Steps     []workflowStep `yaml:"steps"`
}

// workflowStep is a single step within a job.
type workflowStep struct {
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
}

// ParseWorkflow parses a GitHub Actions workflow YAML file and extracts all
// action references, run blocks, and permissions.
func ParseWorkflow(data []byte, path string) (*WorkflowFile, error) {
	var raw workflowYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse workflow %s: %w", path, err)
	}

	wf := &WorkflowFile{
		Path: path,
	}

	// Parse permissions block (string or map).
	wf.Permissions = parsePermissions(raw.Permissions)

	// Iterate over jobs in deterministic order for predictable test output.
	// GitHub Actions YAML jobs are unordered; we process in map order but collect
	// step-uses refs before reusable-workflow refs to keep indices stable for tests.
	var stepRefs []ActionRef
	var reusableRefs []ActionRef
	var containerRefs []ActionRef

	for _, job := range raw.Jobs {
		// Reusable workflow: jobs.<id>.uses
		if job.Uses != "" {
			ref := ParseActionRef(job.Uses)
			reusableRefs = append(reusableRefs, ref)
		}

		// Container image: jobs.<id>.container (string or {image: ...})
		if img := extractContainerImage(job.Container); img != "" {
			containerRefs = append(containerRefs, ActionRef{DockerImage: img})
		}

		// Steps
		for _, step := range job.Steps {
			if step.Uses != "" {
				stepRefs = append(stepRefs, ParseActionRef(step.Uses))
			}
			if step.Run != "" {
				wf.RunBlocks = append(wf.RunBlocks, RunBlock{Content: step.Run})
			}
		}
	}

	// Collect in order: step refs, reusable workflow refs, container refs.
	// This makes the indices predictable (step uses first, then reusable, then container).
	wf.Actions = append(wf.Actions, stepRefs...)
	wf.Actions = append(wf.Actions, reusableRefs...)
	wf.Actions = append(wf.Actions, containerRefs...)

	return wf, nil
}

// ParseWorkflowDir globs .github/workflows/*.yml and .github/workflows/*.yaml
// under dir and parses each file.
func ParseWorkflowDir(dir string) ([]WorkflowFile, error) {
	workflowDir := filepath.Join(dir, ".github", "workflows")

	var patterns []string
	patterns = append(patterns, filepath.Join(workflowDir, "*.yml"))
	patterns = append(patterns, filepath.Join(workflowDir, "*.yaml"))

	var paths []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", pattern, err)
		}
		paths = append(paths, matches...)
	}

	var results []WorkflowFile
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read workflow %s: %w", p, err)
		}
		wf, err := ParseWorkflow(data, p)
		if err != nil {
			return nil, err
		}
		results = append(results, *wf)
	}

	return results, nil
}

// parsePermissions converts the raw permissions value (string or map) into a
// Permissions struct.
func parsePermissions(raw any) Permissions {
	if raw == nil {
		return Permissions{}
	}

	p := Permissions{Defined: true}

	switch v := raw.(type) {
	case string:
		// e.g., permissions: read-all  or  permissions: write-all
		p.Scopes = map[string]string{"_all": v}
	case map[string]any:
		p.Scopes = make(map[string]string, len(v))
		for key, val := range v {
			if s, ok := val.(string); ok {
				p.Scopes[key] = s
			}
		}
	}

	return p
}

// extractContainerImage extracts the image string from a container field that
// may be either a plain string or a map with an "image" key.
func extractContainerImage(raw any) string {
	if raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	case map[string]any:
		if img, ok := v["image"].(string); ok {
			return img
		}
	}
	return ""
}
