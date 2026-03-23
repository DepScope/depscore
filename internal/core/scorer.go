package core

import (
	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
)

// clamp constrains val to [lo, hi].
func clamp(val, lo, hi int) int {
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}

// weightedScore holds a factor score and its configured weight.
type weightedScore struct {
	name  string
	score int
	w     int
}

// redistributeUnavailable takes a slice of factor contributions and a set of
// unavailable factor names, then scales the remaining weights proportionally so
// they sum to 100.
func redistributeUnavailable(factors []weightedScore, unavailable map[string]bool) []weightedScore {
	if len(unavailable) == 0 {
		return factors
	}

	// Sum weights of available factors.
	availableSum := 0
	for _, f := range factors {
		if !unavailable[f.name] {
			availableSum += f.w
		}
	}
	if availableSum == 0 {
		return factors
	}

	// Redistribute: scale each available factor weight proportionally to fill 100.
	result := make([]weightedScore, len(factors))
	copy(result, factors)
	for i, f := range result {
		if unavailable[f.name] {
			result[i].w = 0
		} else {
			result[i].w = int(float64(f.w) / float64(availableSum) * 100)
		}
	}

	// Fix rounding: adjust the first available factor so total == 100.
	total := 0
	for _, f := range result {
		total += f.w
	}
	if diff := 100 - total; diff != 0 {
		for i := range result {
			if !unavailable[result[i].name] {
				result[i].w += diff
				break
			}
		}
	}
	return result
}

// Score computes the OwnScore for a single package and returns a PackageResult.
// TransitiveRiskScore defaults to 100 (no children known yet; propagator sets it).
func Score(pkg manifest.Package, fetchResult *registry.FetchResult, repoInfo *vcs.RepoInfo, weights config.Weights) PackageResult {
	result := PackageResult{
		Name:                pkg.Name,
		Version:             pkg.ResolvedVersion,
		Ecosystem:           string(pkg.Ecosystem),
		ConstraintType:      string(pkg.ConstraintType),
		Depth:               pkg.Depth,
		TransitiveRiskScore: 100,
	}

	if fetchResult == nil || fetchResult.Err != nil || fetchResult.Info == nil {
		// Cannot score without registry data; use minimum score.
		errMsg := "registry data unavailable"
		if fetchResult != nil && fetchResult.Err != nil {
			errMsg = fetchResult.Err.Error()
		}
		result.Issues = []Issue{{
			Package:  pkg.Name,
			Severity: SeverityHigh,
			Message:  errMsg,
		}}
		result.OwnScore = 0
		result.OwnRisk = RiskCritical
		result.TransitiveRisk = RiskCritical
		return result
	}

	info := fetchResult.Info

	// Run all factor scorers.
	pinScore, pinIssues := FactorVersionPinning(pkg.ConstraintType)
	recScore, recIssues := FactorReleaseRecency(info)
	maintScore, maintIssues := FactorMaintainerCount(info)
	dlScore, dlIssues := FactorDownloadVelocity(info)

	// VCS factors: use real repo data when available, neutral with explanation otherwise.
	var issueScore, orgScore, healthScore int
	var issueIssues, orgIssues, healthIssues []Issue

	if repoInfo != nil {
		issueScore, issueIssues = FactorOpenIssueRatio(repoInfo)
		orgScore, orgIssues = FactorOrgBacking(repoInfo, info)
		healthScore, healthIssues = FactorRepoHealth(repoInfo)
	} else {
		issueScore = 50
		orgScore = 50
		healthScore = 50
		healthIssues = []Issue{{Severity: SeverityInfo, Message: "repository health not checked (no VCS data)"}}
		issueIssues = []Issue{{Severity: SeverityInfo, Message: "open issue ratio not checked (no VCS data)"}}
		orgIssues = []Issue{{Severity: SeverityInfo, Message: "organization backing not checked (no VCS data)"}}
	}

	// Build weighted factor list.
	factors := []weightedScore{
		{name: "release_recency", score: recScore, w: weights["release_recency"]},
		{name: "maintainer_count", score: maintScore, w: weights["maintainer_count"]},
		{name: "download_velocity", score: dlScore, w: weights["download_velocity"]},
		{name: "open_issue_ratio", score: issueScore, w: weights["open_issue_ratio"]},
		{name: "org_backing", score: orgScore, w: weights["org_backing"]},
		{name: "version_pinning", score: pinScore, w: weights["version_pinning"]},
		{name: "repo_health", score: healthScore, w: weights["repo_health"]},
	}

	// Mark unavailable factors.
	unavailable := make(map[string]bool)
	if dlScore == 0 {
		unavailable["download_velocity"] = true
	}

	factors = redistributeUnavailable(factors, unavailable)

	// Compute weighted sum.
	weighted := 0
	for _, f := range factors {
		weighted += f.score * f.w
	}
	ownScore := clamp(weighted/100, 0, 100)

	// Collect all issues and stamp with the package name.
	var allIssues []Issue
	for _, batch := range [][]Issue{recIssues, maintIssues, dlIssues, issueIssues, orgIssues, pinIssues, healthIssues} {
		for _, iss := range batch {
			iss.Package = pkg.Name
			allIssues = append(allIssues, iss)
		}
	}

	result.OwnScore = ownScore
	result.OwnRisk = RiskLevelFromScore(ownScore)
	result.TransitiveRisk = RiskLevelFromScore(ownScore) // will be updated by Propagate
	result.Issues = allIssues
	return result
}
