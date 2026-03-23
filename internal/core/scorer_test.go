package core_test

import (
	"testing"
	"time"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
	"github.com/stretchr/testify/assert"
)

func TestScorerHealthyPackage(t *testing.T) {
	pkg := manifest.Package{
		Name: "requests", ResolvedVersion: "2.31.0",
		ConstraintType: manifest.ConstraintExact, Ecosystem: manifest.EcosystemPython, Depth: 1,
	}
	fr := &core.FetchResult{
		Info: &registry.PackageInfo{
			MaintainerCount: 3, HasOrgBacking: true,
			LastReleaseAt:    time.Now().Add(-3 * 30 * 24 * time.Hour),
			MonthlyDownloads: 5_000_000,
		},
		RepoInfo: &vcs.RepoInfo{
			ContributorCount: 50, OpenIssueCount: 10, ClosedIssueCount: 200,
			IsArchived: false, HasOrgBacking: true,
			LastCommitAt: time.Now().Add(-7 * 24 * time.Hour),
		},
	}
	result := core.Score(pkg, fr, config.Enterprise().Weights)
	assert.Greater(t, result.OwnScore, 75, "healthy package should exceed enterprise threshold")
}

func TestScorerAbandonedPackage(t *testing.T) {
	pkg := manifest.Package{
		Name: "abandoned", ConstraintType: manifest.ConstraintMajor,
		Ecosystem: manifest.EcosystemNPM, Depth: 1,
	}
	fr := &core.FetchResult{
		Info: &registry.PackageInfo{
			MaintainerCount: 1,
			LastReleaseAt:   time.Now().Add(-4 * 365 * 24 * time.Hour),
		},
		RepoInfo: &vcs.RepoInfo{IsArchived: true},
	}
	result := core.Score(pkg, fr, config.Enterprise().Weights)
	assert.Less(t, result.OwnScore, 40, "abandoned package should be Critical")
	assert.NotEmpty(t, result.Issues)
}

func TestScorerGoPackageSkipsDownloadVelocity(t *testing.T) {
	pkg := manifest.Package{
		Name: "golang.org/x/sync", ResolvedVersion: "v0.6.0",
		ConstraintType: manifest.ConstraintExact, Ecosystem: manifest.EcosystemGo, Depth: 1,
	}
	fr := &core.FetchResult{
		Info: &registry.PackageInfo{
			MaintainerCount: 5, LastReleaseAt: time.Now().Add(-30 * 24 * time.Hour),
			MonthlyDownloads: 0,
		},
	}
	result := core.Score(pkg, fr, config.Enterprise().Weights)
	assert.Greater(t, result.OwnScore, 0)
}
