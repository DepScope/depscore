// internal/discover/discover.go
package discover

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
)

// Run executes the full discover pipeline.
func Run(cfg Config) (*DiscoverResult, error) {
	// Validate inputs
	compromised, err := ParseRange(cfg.Range)
	if err != nil {
		return nil, fmt.Errorf("invalid range %q: %w", cfg.Range, err)
	}

	// Phase 0: Enumerate projects
	var projects []ProjectInfo
	if cfg.ListFile != "" {
		projects, err = ReadProjectList(cfg.ListFile, cfg.Ecosystem)
	} else {
		startPath := cfg.StartPath
		if startPath == "" {
			startPath = "."
		}
		projects, err = WalkProjects(startPath, cfg.MaxDepth, cfg.Ecosystem)
	}
	if err != nil {
		return nil, fmt.Errorf("enumerating projects: %w", err)
	}

	// Build dependency fetchers for transitive resolution (non-offline mode).
	// Only PyPI and npm support FetchDependencies currently; Go, Rust,
	// and PHP will silently skip transitive resolution until implemented.
	var depFetchers map[string]registry.DependencyFetcher
	if !cfg.Offline {
		depFetchers = map[string]registry.DependencyFetcher{
			"PyPI": registry.NewPyPIClient(),
			"npm":  registry.NewNPMClient(),
		}
	}

	result := &DiscoverResult{
		Package: cfg.Package,
		Range:   cfg.Range,
	}

	// Phase 1: Fast text search
	type matchedProject struct {
		project ProjectInfo
		match   MatchResult
	}
	var matched []matchedProject
	for _, proj := range projects {
		m := MatchPackageInProject(cfg.Package, proj)
		if m.Bool() {
			matched = append(matched, matchedProject{project: proj, match: m})
		}
	}

	// Phase 2: Precise classification
	for _, mp := range matched {
		matches := classifyProject(cfg.Package, mp.project, mp.match, compromised, cfg.Offline, depFetchers)
		result.Matches = append(result.Matches, matches...)
	}

	return result, nil
}

// classifyProject parses the matched manifest/lockfile files and classifies
// the package. Uses ParseFiles with the specific matched file content rather
// than Parse(dir), because Parse(dir) may not check all file types
// (e.g., PythonParser.Parse() skips pyproject.toml if uv.lock exists).
func classifyProject(
	pkgName string,
	project ProjectInfo,
	matchResult MatchResult,
	compromised Range,
	offline bool,
	depFetchers map[string]registry.DependencyFetcher,
) []ProjectMatch {
	target := strings.ToLower(pkgName)
	var results []ProjectMatch

	for _, filename := range matchResult.Files {
		eco := ecosystemForFile(filename)
		if eco == "" {
			continue
		}
		parser := manifest.ParserFor(eco)

		// Read the specific matched file and parse it via ParseFiles.
		// This ensures pyproject.toml is parsed even without a lockfile.
		data, err := os.ReadFile(filepath.Join(project.Dir, filename))
		if err != nil {
			continue
		}
		fileMap := map[string][]byte{filename: data}
		pkgs, err := parser.ParseFiles(fileMap)
		if err != nil {
			continue
		}

		foundTarget := false
		for _, pkg := range pkgs {
			if strings.ToLower(pkg.Name) != target {
				continue
			}
			foundTarget = true

			match := Classify(pkg, compromised, offline)
			match.Project = project.Dir
			match.Source = filename
			results = append(results, match)
		}

		// If target not found as direct dep and we're not offline,
		// try transitive resolution via registry for this project.
		if !foundTarget && !offline && depFetchers != nil {
			ecoStr := eco.String()
			if fetcher, ok := depFetchers[ecoStr]; ok {
				directDeps := make([]DepEntry, 0, len(pkgs))
				for _, pkg := range pkgs {
					directDeps = append(directDeps, DepEntry{
						Name:    pkg.Name,
						Version: pkg.ResolvedVersion,
					})
				}
				tmatch, err := ResolveTransitive(pkgName, directDeps, fetcher, 10)
				if err == nil && tmatch != nil {
					match := ProjectMatch{
						Project:        project.Dir,
						Source:         filename,
						Constraint:     tmatch.Constraint,
						Depth:          "transitive",
						DependencyPath: tmatch.Path,
					}
					// If we got a resolved version from the constraint, classify it
					if v, verr := ParseVersion(tmatch.Version); verr == nil {
						match.Version = tmatch.Version
						if compromised.Contains(v) {
							match.Status = StatusConfirmed
							match.Reason = fmt.Sprintf("transitive dep resolved to %s (in compromised range)", tmatch.Version)
						} else {
							match.Status = StatusSafe
							match.Reason = fmt.Sprintf("transitive dep resolved to %s (outside compromised range)", tmatch.Version)
						}
					} else {
						// Have constraint but no resolved version — check overlap
						overlaps, _ := ConstraintOverlaps(tmatch.Constraint, compromised)
						if overlaps {
							match.Status = StatusPotentially
							match.Reason = fmt.Sprintf("transitive dep constraint %s allows compromised versions", tmatch.Constraint)
						} else {
							match.Status = StatusSafe
							match.Reason = fmt.Sprintf("transitive dep constraint %s excludes compromised range", tmatch.Constraint)
						}
					}
					results = append(results, match)
				}
			}
		}
	}

	// Deduplicate: if same project has both lockfile (CONFIRMED/SAFE) and
	// manifest (POTENTIALLY), prefer the lockfile result.
	return deduplicateMatches(results)
}

// deduplicateMatches keeps the highest-confidence match per project.
// Priority: CONFIRMED > POTENTIALLY > UNRESOLVABLE > SAFE
func deduplicateMatches(matches []ProjectMatch) []ProjectMatch {
	if len(matches) <= 1 {
		return matches
	}

	// Find the best match (most actionable)
	best := matches[0]
	for _, m := range matches[1:] {
		if statusPriority(m.Status) > statusPriority(best.Status) {
			best = m
		}
	}
	return []ProjectMatch{best}
}

func statusPriority(s Status) int {
	switch s {
	case StatusConfirmed:
		return 3
	case StatusPotentially:
		return 2
	case StatusUnresolvable:
		return 1
	case StatusSafe:
		return 0
	default:
		return -1
	}
}
