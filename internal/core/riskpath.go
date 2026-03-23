package core

// RiskPath represents a chain of packages from root to a risky dependency.
type RiskPath struct {
	Chain    []string // package names from root → risky dep
	EndScore int      // own score of the final package in the chain
	EndRisk  RiskLevel
	Reason   string // why this package is risky
}

// FindRiskPaths traces the dependency graph and returns the worst risk paths.
// A risk path is a chain from a direct dependency down to a package scoring
// below the threshold. Returns at most maxPaths results, sorted worst-first.
func FindRiskPaths(results []PackageResult, deps map[string][]string, threshold int, maxPaths int) []RiskPath {
	scoreByName := make(map[string]int, len(results))
	riskByName := make(map[string]RiskLevel, len(results))
	issuesByName := make(map[string][]Issue, len(results))
	depthByName := make(map[string]int, len(results))

	for _, r := range results {
		scoreByName[r.Name] = r.OwnScore
		riskByName[r.Name] = r.OwnRisk
		issuesByName[r.Name] = r.Issues
		depthByName[r.Name] = r.Depth
	}

	var paths []RiskPath

	// Start from each direct dependency (depth 1)
	for _, r := range results {
		if r.Depth != 1 {
			continue
		}
		visited := make(map[string]bool)
		findPaths(r.Name, []string{r.Name}, deps, scoreByName, riskByName, issuesByName, threshold, visited, &paths)
	}

	// Sort worst-first (lowest score)
	sortRiskPaths(paths)

	if len(paths) > maxPaths {
		paths = paths[:maxPaths]
	}
	return paths
}

func findPaths(
	name string,
	chain []string,
	deps map[string][]string,
	scores map[string]int,
	risks map[string]RiskLevel,
	issues map[string][]Issue,
	threshold int,
	visited map[string]bool,
	out *[]RiskPath,
) {
	if visited[name] {
		return
	}
	visited[name] = true

	score := scores[name]
	if score < threshold {
		// This package is below threshold — record the path
		reason := summarizeTopIssue(issues[name])
		chainCopy := make([]string, len(chain))
		copy(chainCopy, chain)
		*out = append(*out, RiskPath{
			Chain:    chainCopy,
			EndScore: score,
			EndRisk:  risks[name],
			Reason:   reason,
		})
	}

	// Continue down the tree
	for _, child := range deps[name] {
		if _, ok := scores[child]; !ok {
			continue
		}
		findPaths(child, append(chain, child), deps, scores, risks, issues, threshold, visited, out)
	}
}

func summarizeTopIssue(issues []Issue) string {
	if len(issues) == 0 {
		return "low reputation score"
	}
	// Find the highest severity issue
	best := issues[0]
	for _, iss := range issues[1:] {
		if severityRank(iss.Severity) < severityRank(best.Severity) {
			best = iss
		}
	}
	return best.Message
}

func severityRank(s IssueSeverity) int {
	switch s {
	case SeverityHigh:
		return 0
	case SeverityMedium:
		return 1
	case SeverityLow:
		return 2
	case SeverityInfo:
		return 3
	default:
		return 4
	}
}

func sortRiskPaths(paths []RiskPath) {
	// Simple insertion sort (paths are usually <20)
	for i := 1; i < len(paths); i++ {
		for j := i; j > 0 && paths[j].EndScore < paths[j-1].EndScore; j-- {
			paths[j], paths[j-1] = paths[j-1], paths[j]
		}
	}
}
