// internal/discover/resolve.go
package discover

import (
	"strings"

	"github.com/depscope/depscope/internal/registry"
)

// DepEntry is a direct dependency with name and resolved version.
type DepEntry struct {
	Name    string
	Version string
}

// TransitiveMatch is the result of finding a target package in a dependency tree.
type TransitiveMatch struct {
	Name       string   // the found package name
	Constraint string   // the constraint under which it was required
	Version    string   // the resolved version (if available from registry)
	Path       []string // dependency chain from direct dep to target
}

// ResolveTransitive walks the dependency tree via registry lookups to find
// if targetPkg exists as a transitive dependency. Uses BFS with depth limit.
func ResolveTransitive(
	targetPkg string,
	directDeps []DepEntry,
	fetcher registry.DependencyFetcher,
	maxDepth int,
) (*TransitiveMatch, error) {
	target := strings.ToLower(targetPkg)

	type queueItem struct {
		name    string
		version string
		path    []string
		depth   int
	}

	var queue []queueItem
	visited := make(map[string]bool)

	for _, dep := range directDeps {
		queue = append(queue, queueItem{
			name:    dep.Name,
			version: dep.Version,
			path:    []string{dep.Name},
			depth:   1,
		})
	}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		key := item.name + "@" + item.version
		if visited[key] {
			continue
		}
		visited[key] = true

		if item.depth > maxDepth {
			continue
		}

		deps, err := fetcher.FetchDependencies(item.name, item.version)
		if err != nil {
			continue // graceful degradation
		}

		for _, dep := range deps {
			depName := strings.ToLower(dep.Name)
			newPath := make([]string, len(item.path))
			copy(newPath, item.path)
			newPath = append(newPath, dep.Name)

			if depName == target {
				return &TransitiveMatch{
					Name:       dep.Name,
					Constraint: dep.Constraint,
					Path:       newPath,
				}, nil
			}

			// Extract version from constraint for next lookup
			// For constraints like ">=1.82.0", we need to resolve the actual version.
			// Use the constraint string to fetch — registry handles resolution.
			depVersion := extractVersionFromConstraint(dep.Constraint)
			queue = append(queue, queueItem{
				name:    dep.Name,
				version: depVersion,
				path:    newPath,
				depth:   item.depth + 1,
			})
		}
	}

	return nil, nil
}

// extractVersionFromConstraint tries to extract a concrete version from a constraint.
// For "==1.2.3" returns "1.2.3". For ">=1.0" returns "" (let registry resolve latest).
func extractVersionFromConstraint(constraint string) string {
	constraint = strings.TrimSpace(constraint)
	if strings.HasPrefix(constraint, "==") {
		return strings.TrimSpace(strings.TrimPrefix(constraint, "=="))
	}
	// For non-exact constraints, return empty to let the registry resolve latest
	return ""
}
