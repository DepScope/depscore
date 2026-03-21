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

func TestFactorMaintainerCount(t *testing.T) {
	solo := &registry.PackageInfo{MaintainerCount: 1}
	score, issues := core.FactorMaintainerCount(solo)
	assert.Less(t, score, 40)
	assert.NotEmpty(t, issues)

	team := &registry.PackageInfo{MaintainerCount: 5}
	score2, _ := core.FactorMaintainerCount(team)
	assert.GreaterOrEqual(t, score2, 80)
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

func TestFactorVersionPinningLooseEmitsIssue(t *testing.T) {
	_, issues := core.FactorVersionPinning(manifest.ConstraintMajor)
	assert.NotEmpty(t, issues)
	assert.Equal(t, core.SeverityHigh, issues[0].Severity)
}

func TestFactorRepoHealthArchivedRepo(t *testing.T) {
	archived := &vcs.RepoInfo{IsArchived: true}
	score, issues := core.FactorRepoHealth(archived)
	assert.Equal(t, 0, score)
	assert.NotEmpty(t, issues)
}
