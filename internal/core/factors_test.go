package core_test

import (
	"testing"
	"time"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
	"github.com/stretchr/testify/assert"
)

func TestFactorReleaseRecency(t *testing.T) {
	fresh := &registry.PackageInfo{LastReleaseAt: time.Now().Add(-3 * 30 * 24 * time.Hour)}
	score, issues := core.FactorReleaseRecency(fresh)
	assert.Greater(t, score, 70)
	assert.Empty(t, issues)

	old := &registry.PackageInfo{LastReleaseAt: time.Now().Add(-3 * 365 * 24 * time.Hour)}
	score2, issues2 := core.FactorReleaseRecency(old)
	assert.Less(t, score2, 40)
	assert.NotEmpty(t, issues2)
}

func TestFactorReleaseRecencyBoundaries(t *testing.T) {
	cases := []struct {
		age      time.Duration
		expected int
	}{
		{3 * 30 * 24 * time.Hour, 100},   // <6mo
		{9 * 30 * 24 * time.Hour, 80},    // <1yr
		{18 * 30 * 24 * time.Hour, 60},   // <2yr
		{30 * 30 * 24 * time.Hour, 40},   // <3yr
		{4 * 365 * 24 * time.Hour, 20},   // >=3yr
	}
	for _, c := range cases {
		info := &registry.PackageInfo{LastReleaseAt: time.Now().Add(-c.age)}
		score, _ := core.FactorReleaseRecency(info)
		assert.Equal(t, c.expected, score)
	}
}

func TestFactorMaintainerCount(t *testing.T) {
	solo := &registry.PackageInfo{MaintainerCount: 1}
	score, issues := core.FactorMaintainerCount(solo)
	assert.Less(t, score, 40)
	assert.NotEmpty(t, issues)

	team := &registry.PackageInfo{MaintainerCount: 5}
	score2, _ := core.FactorMaintainerCount(team)
	assert.GreaterOrEqual(t, score2, 80)
}

func TestFactorMaintainerCountBoundaries(t *testing.T) {
	cases := []struct {
		count    int
		expected int
	}{
		{0, 20},
		{1, 30},  // solo, no recent release
		{2, 60},
		{3, 80},
		{4, 80},
		{5, 100},
		{10, 100},
	}
	for _, c := range cases {
		info := &registry.PackageInfo{MaintainerCount: c.count}
		score, _ := core.FactorMaintainerCount(info)
		assert.Equal(t, c.expected, score, "maintainers=%d", c.count)
	}

	// Solo maintainer with recent release gets a better score
	activeInfo := &registry.PackageInfo{
		MaintainerCount: 1,
		LastReleaseAt:   time.Now().Add(-30 * 24 * time.Hour),
	}
	activeScore, activeIssues := core.FactorMaintainerCount(activeInfo)
	assert.Equal(t, 45, activeScore, "active solo maintainer")
	assert.Equal(t, core.SeverityLow, activeIssues[0].Severity)
}

func TestFactorDownloadVelocity(t *testing.T) {
	goPackage := &registry.PackageInfo{MonthlyDownloads: 0}
	score, issues := core.FactorDownloadVelocity(goPackage)
	assert.Equal(t, 0, score)
	assert.Nil(t, issues)

	popular := &registry.PackageInfo{MonthlyDownloads: 2_000_000}
	score2, _ := core.FactorDownloadVelocity(popular)
	assert.Equal(t, 100, score2)

	medium := &registry.PackageInfo{MonthlyDownloads: 500_000}
	score3, _ := core.FactorDownloadVelocity(medium)
	assert.Equal(t, 80, score3)
}

func TestFactorOpenIssueRatio(t *testing.T) {
	healthy := &vcs.RepoInfo{OpenIssueCount: 5, ClosedIssueCount: 95}
	score, issues := core.FactorOpenIssueRatio(healthy)
	assert.Equal(t, 100, score)
	assert.Empty(t, issues)

	unhealthy := &vcs.RepoInfo{OpenIssueCount: 60, ClosedIssueCount: 40}
	score2, issues2 := core.FactorOpenIssueRatio(unhealthy)
	assert.Equal(t, 20, score2)
	assert.NotEmpty(t, issues2)

	noData := &vcs.RepoInfo{}
	score3, issues3 := core.FactorOpenIssueRatio(noData)
	assert.Equal(t, 75, score3)
	assert.Empty(t, issues3)
}

func TestFactorOrgBacking(t *testing.T) {
	orgRepo := &vcs.RepoInfo{HasOrgBacking: true}
	orgInfo := &registry.PackageInfo{Name: "pkg"}
	score, issues := core.FactorOrgBacking(orgRepo, orgInfo)
	assert.Equal(t, 100, score)
	assert.Empty(t, issues)

	individual := &vcs.RepoInfo{HasOrgBacking: false}
	indivInfo := &registry.PackageInfo{Name: "pkg", HasOrgBacking: false}
	score2, issues2 := core.FactorOrgBacking(individual, indivInfo)
	assert.Equal(t, 30, score2)
	assert.NotEmpty(t, issues2)
}

func TestFactorVersionPinning(t *testing.T) {
	cases := []struct {
		ct       manifest.ConstraintType
		expected int
	}{
		{manifest.ConstraintExact, 100},
		{manifest.ConstraintPatch, 75},
		{manifest.ConstraintMinor, 50},
		{manifest.ConstraintMajor, 25},
	}
	for _, c := range cases {
		score, _ := core.FactorVersionPinning(c.ct)
		assert.Equal(t, c.expected, score, string(c.ct))
	}
}

func TestFactorRepoHealthArchivedRepo(t *testing.T) {
	archived := &vcs.RepoInfo{IsArchived: true}
	score, issues := core.FactorRepoHealth(archived)
	assert.Equal(t, 0, score)
	assert.NotEmpty(t, issues)
}

func TestFactorRepoHealthRecency(t *testing.T) {
	active := &vcs.RepoInfo{LastCommitAt: time.Now().Add(-30 * 24 * time.Hour)}
	score, _ := core.FactorRepoHealth(active)
	assert.Equal(t, 100, score)

	inactive := &vcs.RepoInfo{LastCommitAt: time.Now().Add(-3 * 365 * 24 * time.Hour)}
	score2, issues2 := core.FactorRepoHealth(inactive)
	assert.Equal(t, 10, score2)
	assert.NotEmpty(t, issues2)
}
