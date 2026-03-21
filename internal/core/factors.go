package core

import (
	"fmt"
	"time"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
	"github.com/depscope/depscope/internal/vuln"
)

// FactorReleaseRecency: <6mo=100, <1yr=80, <2yr=60, <3yr=40, >=3yr=20
func FactorReleaseRecency(info *registry.PackageInfo) (int, []Issue) {
	if info == nil || info.LastReleaseAt.IsZero() {
		return 0, []Issue{{Severity: SeverityHigh, Message: "no release information available"}}
	}
	age := time.Since(info.LastReleaseAt)
	switch {
	case age < 6*30*24*time.Hour:
		return 100, nil
	case age < 365*24*time.Hour:
		return 80, nil
	case age < 2*365*24*time.Hour:
		return 60, nil
	case age < 3*365*24*time.Hour:
		return 40, []Issue{{Severity: SeverityMedium, Message: "last release over 2 years ago"}}
	default:
		return 20, []Issue{{Severity: SeverityHigh, Message: "last release over 3 years ago — possibly abandoned"}}
	}
}

// FactorMaintainerCount: 1=20, 2=50, 3-4=75, 5+=100
func FactorMaintainerCount(info *registry.PackageInfo) (int, []Issue) {
	if info == nil {
		return 0, nil
	}
	switch {
	case info.MaintainerCount <= 0:
		return 0, []Issue{{Severity: SeverityMedium, Message: "no maintainer information"}}
	case info.MaintainerCount == 1:
		return 20, []Issue{{Severity: SeverityHigh, Message: "solo maintainer — bus factor risk"}}
	case info.MaintainerCount == 2:
		return 50, []Issue{{Severity: SeverityMedium, Message: "only 2 maintainers"}}
	case info.MaintainerCount <= 4:
		return 75, nil
	default:
		return 100, nil
	}
}

// FactorDownloadVelocity: returns 0,nil for packages with no data (e.g. Go).
func FactorDownloadVelocity(info *registry.PackageInfo) (int, []Issue) {
	if info == nil || info.MonthlyDownloads == 0 {
		return 0, nil
	}
	switch {
	case info.MonthlyDownloads > 1_000_000:
		return 100, nil
	case info.MonthlyDownloads > 100_000:
		return 80, nil
	case info.MonthlyDownloads > 10_000:
		return 60, nil
	case info.MonthlyDownloads > 1_000:
		return 40, nil
	default:
		return 20, []Issue{{Severity: SeverityLow, Message: "very low download volume"}}
	}
}

// FactorOpenIssueRatio penalizes high open/total ratio.
func FactorOpenIssueRatio(repo *vcs.RepoInfo) (int, []Issue) {
	if repo == nil {
		return 50, nil
	}
	total := repo.OpenIssueCount + repo.ClosedIssueCount
	if total == 0 {
		return 80, nil
	}
	ratio := float64(repo.OpenIssueCount) / float64(total)
	switch {
	case ratio < 0.1:
		return 100, nil
	case ratio < 0.25:
		return 75, nil
	case ratio < 0.5:
		return 50, []Issue{{Severity: SeverityMedium, Message: "high open issue ratio"}}
	default:
		return 20, []Issue{{Severity: SeverityHigh, Message: "very high open issue ratio — low maintenance activity"}}
	}
}

// FactorOrgBacking scores org-backed packages higher.
func FactorOrgBacking(repo *vcs.RepoInfo, info *registry.PackageInfo) (int, []Issue) {
	backed := (repo != nil && repo.HasOrgBacking) || (info != nil && info.HasOrgBacking)
	if backed {
		return 100, nil
	}
	return 30, []Issue{{Severity: SeverityLow, Message: "no org/company backing — individual maintainer"}}
}

// FactorVersionPinning scores the constraint type.
func FactorVersionPinning(ct manifest.ConstraintType) (int, []Issue) {
	switch ct {
	case manifest.ConstraintExact:
		return 100, nil
	case manifest.ConstraintPatch:
		return 75, []Issue{{Severity: SeverityLow, Message: "patch-level version constraint"}}
	case manifest.ConstraintMinor:
		return 50, []Issue{{Severity: SeverityMedium, Message: "minor-level constraint — supply chain risk"}}
	default:
		return 25, []Issue{{Severity: SeverityHigh, Message: "open/major version constraint — supply chain attack surface"}}
	}
}

// FactorRepoHealth scores commit frequency and archived status.
func FactorRepoHealth(repo *vcs.RepoInfo) (int, []Issue) {
	if repo == nil {
		return 35, []Issue{{Severity: SeverityMedium, Message: "no source repository found"}}
	}
	if repo.IsArchived {
		return 0, []Issue{{Severity: SeverityHigh, Message: "source repository is archived — no further development"}}
	}
	age := time.Since(repo.LastCommitAt)
	switch {
	case age < 90*24*time.Hour:
		return 100, nil
	case age < 365*24*time.Hour:
		return 70, nil
	case age < 2*365*24*time.Hour:
		return 40, []Issue{{Severity: SeverityMedium, Message: "no commits in over a year"}}
	default:
		return 10, []Issue{{Severity: SeverityHigh, Message: "no commits in over 2 years"}}
	}
}

// FactorVulnerabilities penalizes packages with known CVEs.
// No CVEs = 100, only LOW = 80, any MEDIUM = 60, any HIGH = 30, any CRITICAL = 10.
func FactorVulnerabilities(vulns []vuln.Finding) (int, []Issue) {
	if len(vulns) == 0 {
		return 100, nil
	}

	maxSev := vuln.SeverityLow
	var issues []Issue
	for _, v := range vulns {
		issues = append(issues, Issue{
			Severity: mapVulnSeverity(v.Severity),
			Message:  fmt.Sprintf("CVE %s: %s (%s)", v.ID, v.Summary, v.Severity),
		})
		switch v.Severity {
		case vuln.SeverityCritical:
			if maxSev != vuln.SeverityCritical {
				maxSev = vuln.SeverityCritical
			}
		case vuln.SeverityHigh:
			if maxSev != vuln.SeverityCritical {
				maxSev = vuln.SeverityHigh
			}
		case vuln.SeverityMedium:
			if maxSev != vuln.SeverityCritical && maxSev != vuln.SeverityHigh {
				maxSev = vuln.SeverityMedium
			}
		}
	}

	switch maxSev {
	case vuln.SeverityCritical:
		return 10, issues
	case vuln.SeverityHigh:
		return 30, issues
	case vuln.SeverityMedium:
		return 60, issues
	default:
		return 80, issues
	}
}

func mapVulnSeverity(s vuln.Severity) IssueSeverity {
	switch s {
	case vuln.SeverityCritical:
		return SeverityCritical
	case vuln.SeverityHigh:
		return SeverityHigh
	case vuln.SeverityMedium:
		return SeverityMedium
	default:
		return SeverityLow
	}
}
