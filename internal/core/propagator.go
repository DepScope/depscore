package core

// EffectiveScore discounts a dependency's risk by its depth in the tree.
// Formula: clamp(ownScore + (depth-1)*5, 0, 100)
func EffectiveScore(ownScore, depth int) int {
	discounted := ownScore + (depth-1)*5
	return clamp(discounted, 0, 100)
}

// Propagate computes TransitiveRiskScore for each package as the minimum
// EffectiveScore across all transitive descendants.
func Propagate(results []PackageResult, deps map[string][]string) []PackageResult {
	byName := make(map[string]*PackageResult, len(results))
	for i := range results {
		byName[results[i].Name] = &results[i]
	}

	for i := range results {
		minEffective := 100
		visited := make(map[string]bool)
		var walk func(name string)
		walk = func(name string) {
			for _, depName := range deps[name] {
				if visited[depName] {
					continue
				}
				visited[depName] = true
				if dep, ok := byName[depName]; ok {
					eff := EffectiveScore(dep.OwnScore, dep.Depth)
					if eff < minEffective {
						minEffective = eff
					}
					walk(depName)
				}
			}
		}
		walk(results[i].Name)

		results[i].TransitiveRiskScore = minEffective
		results[i].TransitiveRisk = RiskLevelFromScore(minEffective)
	}
	return results
}
