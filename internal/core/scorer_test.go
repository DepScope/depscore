package core_test

import (
	"testing"
	"time"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/stretchr/testify/assert"
)

func healthyInfo() *registry.PackageInfo {
	return &registry.PackageInfo{
		Name:             "well-maintained",
		MonthlyDownloads: 5_000_000,
		MaintainerCount:  5,
		HasOrgBacking:    true,
		LastReleaseAt:    time.Now().Add(-30 * 24 * time.Hour),
	}
}

func abandonedInfo() *registry.PackageInfo {
	return &registry.PackageInfo{
		Name:             "abandoned",
		MonthlyDownloads: 500,
		MaintainerCount:  1,
		HasOrgBacking:    false,
		LastReleaseAt:    time.Now().Add(-4 * 365 * 24 * time.Hour),
	}
}

func TestScorerHealthyPackage(t *testing.T) {
	pkg := manifest.Package{
		Name:            "well-maintained",
		ResolvedVersion: "1.2.3",
		ConstraintType:  manifest.ConstraintExact,
		Ecosystem:       manifest.EcosystemNPM,
	}
	fr := &registry.FetchResult{Info: healthyInfo()}
	weights := config.Enterprise().Weights

	result := core.Score(pkg, fr, nil, weights)

	// A well-maintained package should score above the enterprise threshold of 70.
	assert.Greater(t, result.OwnScore, 70, "healthy package should score above enterprise threshold")
	assert.Equal(t, 100, result.TransitiveRiskScore, "TransitiveRiskScore defaults to 100 before propagation")
}

func TestScorerAbandonedPackage(t *testing.T) {
	pkg := manifest.Package{
		Name:            "abandoned",
		ResolvedVersion: "0.1.0",
		ConstraintType:  manifest.ConstraintMajor,
		Ecosystem:       manifest.EcosystemNPM,
	}
	fr := &registry.FetchResult{Info: abandonedInfo()}
	weights := config.Enterprise().Weights

	result := core.Score(pkg, fr, nil, weights)

	// Solo maintainer, 4yr old release, major constraint should score poorly.
	assert.Less(t, result.OwnScore, 50, "abandoned package should score below 50")
}

func TestScorerGoPackageSkipsDownloadVelocity(t *testing.T) {
	pkg := manifest.Package{
		Name:            "github.com/some/pkg",
		ResolvedVersion: "v1.0.0",
		ConstraintType:  manifest.ConstraintExact,
		Ecosystem:       manifest.EcosystemGo,
	}
	goInfo := &registry.PackageInfo{
		Name:             "github.com/some/pkg",
		MonthlyDownloads: 0, // Go packages have no download data
		MaintainerCount:  3,
		HasOrgBacking:    true,
		LastReleaseAt:    time.Now().Add(-60 * 24 * time.Hour),
	}
	fr := &registry.FetchResult{Info: goInfo}
	weights := config.Enterprise().Weights

	// Should not panic; download velocity weight should be redistributed.
	assert.NotPanics(t, func() {
		result := core.Score(pkg, fr, nil, weights)
		assert.Greater(t, result.OwnScore, 0)
	})
}

func TestScorerNilFetchResult(t *testing.T) {
	pkg := manifest.Package{
		Name:           "missing",
		ConstraintType: manifest.ConstraintMinor,
	}
	result := core.Score(pkg, nil, nil, config.Enterprise().Weights)
	assert.Equal(t, 0, result.OwnScore)
	assert.NotEmpty(t, result.Issues)
}

func TestScorerFetchError(t *testing.T) {
	pkg := manifest.Package{
		Name:           "errored",
		ConstraintType: manifest.ConstraintMinor,
	}
	fr := &registry.FetchResult{Err: assert.AnError}
	result := core.Score(pkg, fr, nil, config.Enterprise().Weights)
	assert.Equal(t, 0, result.OwnScore)
	assert.NotEmpty(t, result.Issues)
}
