package core

import (
	"testing"
	"time"

	"github.com/depscope/depscope/internal/registry"
	"github.com/stretchr/testify/assert"
)

func TestDetectSuspicious(t *testing.T) {
	t.Run("empty results yields no indicators", func(t *testing.T) {
		indicators := DetectSuspicious(nil, nil)
		assert.Empty(t, indicators)
	})

	t.Run("missing registry info is skipped gracefully", func(t *testing.T) {
		results := []PackageResult{
			{Name: "unknown-pkg"},
		}
		infos := map[string]*registry.PackageInfo{} // no entry for "unknown-pkg"
		indicators := DetectSuspicious(results, infos)
		assert.Empty(t, indicators)
	})

	t.Run("nil registry info is skipped gracefully", func(t *testing.T) {
		results := []PackageResult{
			{Name: "nil-pkg"},
		}
		infos := map[string]*registry.PackageInfo{
			"nil-pkg": nil,
		}
		indicators := DetectSuspicious(results, infos)
		assert.Empty(t, indicators)
	})

	t.Run("new_popular: package < 6 months old with > 100k downloads", func(t *testing.T) {
		now := time.Now()
		results := []PackageResult{
			{Name: "hot-new-pkg"},
		}
		infos := map[string]*registry.PackageInfo{
			"hot-new-pkg": {
				Name:             "hot-new-pkg",
				FirstReleaseAt:   now.Add(-30 * 24 * time.Hour), // 30 days old
				LastReleaseAt:    now.Add(-2 * 24 * time.Hour),
				MonthlyDownloads: 200_000,
				ReleaseCount:     5,
				MaintainerCount:  2,
				SourceRepoURL:    "https://github.com/foo/hot-new-pkg",
			},
		}
		indicators := DetectSuspicious(results, infos)
		found := false
		for _, ind := range indicators {
			if ind.Type == "new_popular" && ind.Package == "hot-new-pkg" {
				found = true
				assert.Equal(t, SeverityHigh, ind.Severity)
			}
		}
		assert.True(t, found, "expected new_popular indicator for hot-new-pkg")
	})

	t.Run("old package with high downloads is NOT new_popular", func(t *testing.T) {
		now := time.Now()
		results := []PackageResult{
			{Name: "veteran-pkg"},
		}
		infos := map[string]*registry.PackageInfo{
			"veteran-pkg": {
				Name:             "veteran-pkg",
				FirstReleaseAt:   now.Add(-2 * 365 * 24 * time.Hour), // 2 years old
				LastReleaseAt:    now.Add(-10 * 24 * time.Hour),
				MonthlyDownloads: 500_000,
				ReleaseCount:     50,
				MaintainerCount:  5,
				SourceRepoURL:    "https://github.com/foo/veteran-pkg",
			},
		}
		indicators := DetectSuspicious(results, infos)
		for _, ind := range indicators {
			assert.NotEqual(t, "new_popular", ind.Type, "old package should not be flagged as new_popular")
		}
	})

	t.Run("no_source: > 10k downloads but no source repo URL", func(t *testing.T) {
		results := []PackageResult{
			{Name: "opaque-pkg"},
		}
		infos := map[string]*registry.PackageInfo{
			"opaque-pkg": {
				Name:             "opaque-pkg",
				MonthlyDownloads: 50_000,
				SourceRepoURL:    "", // no source repo
				MaintainerCount:  3,
				ReleaseCount:     10,
			},
		}
		indicators := DetectSuspicious(results, infos)
		found := false
		for _, ind := range indicators {
			if ind.Type == "no_source" && ind.Package == "opaque-pkg" {
				found = true
				assert.Equal(t, SeverityMedium, ind.Severity)
			}
		}
		assert.True(t, found, "expected no_source indicator for opaque-pkg")
	})

	t.Run("package with source repo is NOT flagged no_source", func(t *testing.T) {
		results := []PackageResult{
			{Name: "open-pkg"},
		}
		infos := map[string]*registry.PackageInfo{
			"open-pkg": {
				Name:             "open-pkg",
				MonthlyDownloads: 50_000,
				SourceRepoURL:    "https://github.com/foo/open-pkg",
				MaintainerCount:  3,
				ReleaseCount:     10,
			},
		}
		indicators := DetectSuspicious(results, infos)
		for _, ind := range indicators {
			assert.NotEqual(t, "no_source", ind.Type)
		}
	})

	t.Run("high_value_solo: 1 maintainer with > 1M downloads", func(t *testing.T) {
		results := []PackageResult{
			{Name: "solo-mega-pkg"},
		}
		infos := map[string]*registry.PackageInfo{
			"solo-mega-pkg": {
				Name:             "solo-mega-pkg",
				MonthlyDownloads: 2_000_000,
				MaintainerCount:  1,
				SourceRepoURL:    "https://github.com/solo/mega",
				ReleaseCount:     20,
			},
		}
		indicators := DetectSuspicious(results, infos)
		found := false
		for _, ind := range indicators {
			if ind.Type == "high_value_solo" && ind.Package == "solo-mega-pkg" {
				found = true
				assert.Equal(t, SeverityMedium, ind.Severity)
			}
		}
		assert.True(t, found, "expected high_value_solo indicator for solo-mega-pkg")
	})

	t.Run("multiple maintainers with > 1M downloads is NOT high_value_solo", func(t *testing.T) {
		results := []PackageResult{
			{Name: "team-pkg"},
		}
		infos := map[string]*registry.PackageInfo{
			"team-pkg": {
				Name:             "team-pkg",
				MonthlyDownloads: 2_000_000,
				MaintainerCount:  3,
				SourceRepoURL:    "https://github.com/team/pkg",
				ReleaseCount:     20,
			},
		}
		indicators := DetectSuspicious(results, infos)
		for _, ind := range indicators {
			assert.NotEqual(t, "high_value_solo", ind.Type)
		}
	})
}
