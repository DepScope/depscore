package core

// EffectiveScore computes the effective score of a package at a given depth.
// Deeper dependencies are discounted less (score improves with depth > 1) since
// risk is attenuated. Formula: clamp(ownScore + (depth-1)*5, 0, 100).
func EffectiveScore(ownScore, depth int) int {
	return clamp(ownScore+(depth-1)*5, 0, 100)
}

// Propagate walks the dependency graph for each package and sets TransitiveRiskScore
// to the minimum effective score found across all transitive descendants.
// deps maps a package name to its direct dependency names.
// The effective score of each descendant uses that descendant's own Depth field,
// so deeper dependencies are naturally discounted less (score increases with depth).
func Propagate(results []PackageResult, deps map[string][]string) []PackageResult {
	// Build lookups from name to own score and depth.
	scoreByName := make(map[string]int, len(results))
	depthByName := make(map[string]int, len(results))
	for _, r := range results {
		scoreByName[r.Name] = r.OwnScore
		depthByName[r.Name] = r.Depth
	}

	// For each package, walk all descendants and find the minimum effective score.
	for i, r := range results {
		minScore := 100 // sentinel: no descendants means transitive score stays high
		visited := make(map[string]bool)
		walkDescendants(r.Name, deps, scoreByName, depthByName, &minScore, visited)
		results[i].TransitiveRiskScore = minScore
		results[i].TransitiveRisk = RiskLevelFromScore(minScore)
	}
	return results
}

// walkDescendants recursively walks descendants of `name`, updating minScore
// with the minimum effective score encountered across all descendants.
// visited prevents infinite loops in cyclic dependency graphs.
func walkDescendants(name string, deps map[string][]string, scores map[string]int, depths map[string]int, minScore *int, visited map[string]bool) {
	if visited[name] {
		return
	}
	visited[name] = true

	children, ok := deps[name]
	if !ok || len(children) == 0 {
		return
	}
	for _, child := range children {
		childOwn, found := scores[child]
		if !found {
			continue
		}
		childDepth := depths[child]
		if childDepth == 0 {
			childDepth = 1
		}
		eff := EffectiveScore(childOwn, childDepth)
		if eff < *minScore {
			*minScore = eff
		}
		walkDescendants(child, deps, scores, depths, minScore, visited)
	}
}
