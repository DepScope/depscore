package core

import (
	"fmt"
	"time"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
)

// FactorReleaseRecency scores how recently the package had a release.
// <6mo=100, <1yr=80, <2yr=60, <3yr=40, >=3yr=20
func FactorReleaseRecency(info *registry.PackageInfo) (int, []Issue) {
	if info.LastReleaseAt.IsZero() {
		return 20, []Issue{{
			Package:  info.Name,
			Severity: SeverityMedium,
			Message:  "no release date available; assuming stale",
		}}
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
		return 40, nil
	default:
		return 20, []Issue{{
			Package:  info.Name,
			Severity: SeverityHigh,
			Message:  fmt.Sprintf("last release was %.0f days ago (>3 years)", age.Hours()/24),
		}}
	}
}

// FactorMaintainerCount scores the number of maintainers.
// 1=20, 2=50, 3-4=75, 5+=100
func FactorMaintainerCount(info *registry.PackageInfo) (int, []Issue) {
	switch {
	case info.MaintainerCount >= 5:
		return 100, nil
	case info.MaintainerCount >= 3:
		return 75, nil
	case info.MaintainerCount == 2:
		return 50, nil
	case info.MaintainerCount == 1:
		return 20, []Issue{{
			Package:  info.Name,
			Severity: SeverityMedium,
			Message:  "single maintainer; bus-factor risk",
		}}
	default:
		return 20, []Issue{{
			Package:  info.Name,
			Severity: SeverityMedium,
			Message:  "no maintainer information available",
		}}
	}
}

// FactorDownloadVelocity scores monthly download volume.
// >1M/mo=100, >100K=80, >10K=60, >1K=40, else=20
// Returns 0,nil when MonthlyDownloads==0 (Go packages — no data).
func FactorDownloadVelocity(info *registry.PackageInfo) (int, []Issue) {
	if info.MonthlyDownloads == 0 {
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
		return 20, []Issue{{
			Package:  info.Name,
			Severity: SeverityLow,
			Message:  fmt.Sprintf("low monthly downloads: %d", info.MonthlyDownloads),
		}}
	}
}

// FactorOpenIssueRatio scores the ratio of open to total issues.
// <10%=100, <25%=75, <50%=50, >=50%=20
func FactorOpenIssueRatio(repo *vcs.RepoInfo) (int, []Issue) {
	total := repo.OpenIssueCount + repo.ClosedIssueCount
	if total == 0 {
		// No issue data — return a neutral score without an issue.
		return 75, nil
	}
	ratio := float64(repo.OpenIssueCount) / float64(total)
	switch {
	case ratio < 0.10:
		return 100, nil
	case ratio < 0.25:
		return 75, nil
	case ratio < 0.50:
		return 50, nil
	default:
		return 20, []Issue{{
			Package:  repo.Owner + "/" + repo.Repo,
			Severity: SeverityMedium,
			Message:  fmt.Sprintf("high open issue ratio: %.0f%% (%d open of %d total)", ratio*100, repo.OpenIssueCount, total),
		}}
	}
}

// FactorOrgBacking scores whether the package has organization backing.
// org-backed=100, individual=30
func FactorOrgBacking(repo *vcs.RepoInfo, info *registry.PackageInfo) (int, []Issue) {
	orgBacked := info.HasOrgBacking || (repo != nil && repo.HasOrgBacking)
	if orgBacked {
		return 100, nil
	}
	return 30, []Issue{{
		Package:  info.Name,
		Severity: SeverityLow,
		Message:  "no organization backing; individually maintained",
	}}
}

// FactorVersionPinning scores how precisely the version constraint is pinned.
// exact=100, patch=75, minor=50, major=25
func FactorVersionPinning(ct manifest.ConstraintType) (int, []Issue) {
	switch ct {
	case manifest.ConstraintExact:
		return 100, nil
	case manifest.ConstraintPatch:
		return 75, nil
	case manifest.ConstraintMinor:
		return 50, nil
	default: // ConstraintMajor or unknown
		return 25, []Issue{{
			Severity: SeverityLow,
			Message:  fmt.Sprintf("loose version constraint %q allows major updates", ct),
		}}
	}
}

// FactorRepoHealth scores repository health based on archive status and commit recency.
// archived=0, <90d commit=100, <1yr=70, <2yr=40, else=10
func FactorRepoHealth(repo *vcs.RepoInfo) (int, []Issue) {
	if repo.IsArchived {
		return 0, []Issue{{
			Package:  repo.Owner + "/" + repo.Repo,
			Severity: SeverityHigh,
			Message:  "repository is archived",
		}}
	}
	if repo.LastCommitAt.IsZero() {
		return 10, []Issue{{
			Package:  repo.Owner + "/" + repo.Repo,
			Severity: SeverityMedium,
			Message:  "no commit date available; assuming inactive",
		}}
	}
	age := time.Since(repo.LastCommitAt)
	switch {
	case age < 90*24*time.Hour:
		return 100, nil
	case age < 365*24*time.Hour:
		return 70, nil
	case age < 2*365*24*time.Hour:
		return 40, nil
	default:
		return 10, []Issue{{
			Package:  repo.Owner + "/" + repo.Repo,
			Severity: SeverityHigh,
			Message:  fmt.Sprintf("last commit was %.0f days ago (>2 years)", age.Hours()/24),
		}}
	}
}
