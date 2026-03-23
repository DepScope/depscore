package core

import (
	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
	"github.com/depscope/depscope/internal/vuln"
)

// FetchResult bundles registry metadata and VCS info for a single package.
type FetchResult struct {
	Info     *registry.PackageInfo
	RepoInfo *vcs.RepoInfo
	Vulns    []vuln.Finding
	Err      error
}

type factorScore struct {
	score     int
	issues    []Issue
	available bool
}

// Score runs all 7 factors on the given package, redistributes weight for
// unavailable factors, and returns a PackageResult with OwnScore populated.
func Score(pkg manifest.Package, fr *FetchResult, weights config.Weights) PackageResult {
	var info *registry.PackageInfo
	var repoInfo *vcs.RepoInfo
	if fr != nil {
		info = fr.Info
		repoInfo = fr.RepoInfo
	}

	factors := map[string]factorScore{}

	s, issues := FactorReleaseRecency(info)
	factors["release_recency"] = factorScore{score: s, issues: issues, available: true}

	s, issues = FactorMaintainerCount(info)
	factors["maintainer_count"] = factorScore{score: s, issues: issues, available: true}

	s, issues = FactorDownloadVelocity(info)
	available := info != nil && info.MonthlyDownloads > 0
	factors["download_velocity"] = factorScore{score: s, issues: issues, available: available}

	s, issues = FactorOpenIssueRatio(repoInfo)
	factors["open_issue_ratio"] = factorScore{score: s, issues: issues, available: true}

	s, issues = FactorOrgBacking(repoInfo, info)
	factors["org_backing"] = factorScore{score: s, issues: issues, available: true}

	s, issues = FactorVersionPinning(pkg.ConstraintType)
	factors["version_pinning"] = factorScore{score: s, issues: issues, available: true}

	s, issues = FactorRepoHealth(repoInfo)
	factors["repo_health"] = factorScore{score: s, issues: issues, available: true}

	activeWeights := redistributeUnavailable(weights, factors)

	total := 0
	for name, fs := range factors {
		if w, ok := activeWeights[name]; ok && fs.available {
			total += fs.score * w
		}
	}
	ownScore := clamp(total/100, 0, 100)

	// Compute vulnerability score as a separate axis (not blended into OwnScore)
	// FinalScore = min(OwnScore, VulnScore, TransitiveRiskScore)
	var vulns []vuln.Finding
	if fr != nil {
		vulns = fr.Vulns
	}
	vulnScore, vulnIssues := FactorVulnerabilities(vulns)

	var allIssues []Issue
	for _, fs := range factors {
		for _, iss := range fs.issues {
			iss.Package = pkg.Name
			allIssues = append(allIssues, iss)
		}
	}
	for _, iss := range vulnIssues {
		iss.Package = pkg.Name
		allIssues = append(allIssues, iss)
	}

	return PackageResult{
		Name:                pkg.Name,
		Version:             pkg.ResolvedVersion,
		Ecosystem:           string(pkg.Ecosystem),
		Constraint:          pkg.Constraint,
		ConstraintType:      string(pkg.ConstraintType),
		Depth:               pkg.Depth,
		OwnScore:            ownScore,
		VulnScore:           vulnScore,
		TransitiveRiskScore: 100,
		OwnRisk:             RiskLevelFromScore(ownScore),
		VulnRisk:            RiskLevelFromScore(vulnScore),
		TransitiveRisk:      RiskLow,
		Issues:              allIssues,
		VulnCount:           len(vulns),
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func redistributeUnavailable(weights config.Weights, factors map[string]factorScore) config.Weights {
	unavailableWeight := 0
	availableBaseWeight := 0
	for name, fs := range factors {
		w := weights[name]
		if !fs.available {
			unavailableWeight += w
		} else {
			availableBaseWeight += w
		}
	}
	if unavailableWeight == 0 {
		return weights
	}
	result := make(config.Weights)
	for name, fs := range factors {
		if !fs.available {
			result[name] = 0
			continue
		}
		w := weights[name]
		extra := int(float64(w) / float64(availableBaseWeight) * float64(unavailableWeight))
		result[name] = w + extra
	}
	// Fix rounding drift: assign remainder to first available factor in canonical order.
	total := 0
	for _, v := range result {
		total += v
	}
	if diff := 100 - total; diff != 0 {
		for _, n := range config.FactorNames {
			if factors[n].available {
				result[n] += diff
				break
			}
		}
	}
	return result
}
