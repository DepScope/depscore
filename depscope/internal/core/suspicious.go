package core

import (
	"fmt"
	"time"

	"github.com/depscope/depscope/internal/registry"
)

// SuspiciousIndicator flags a package with an unusual pattern that warrants investigation.
type SuspiciousIndicator struct {
	Package  string
	Type     string // "new_popular", "dormant_spike", "low_age_high_deps", "no_source"
	Severity IssueSeverity
	Message  string
}

// DetectSuspicious analyzes packages for supply chain attack indicators.
// These are heuristic-based flags, not definitive proof of malice.
func DetectSuspicious(results []PackageResult, infos map[string]*registry.PackageInfo) []SuspiciousIndicator {
	var indicators []SuspiciousIndicator

	for _, r := range results {
		info := infos[r.Name]
		if info == nil {
			continue
		}

		// 1. New package with surprisingly high download count
		// A package less than 6 months old with >100K monthly downloads is unusual
		if !info.FirstReleaseAt.IsZero() && !info.LastReleaseAt.IsZero() {
			age := time.Since(info.FirstReleaseAt)
			if age < 6*30*24*time.Hour && info.MonthlyDownloads > 100_000 {
				indicators = append(indicators, SuspiciousIndicator{
					Package:  r.Name,
					Type:     "new_popular",
					Severity: SeverityHigh,
					Message: fmt.Sprintf(
						"new package (%.0f days old) with high downloads (%dk/month) — verify legitimacy",
						age.Hours()/24, info.MonthlyDownloads/1000,
					),
				})
			}
		}

		// 2. Dormant package with recent release spike
		// If a package had no releases for >2 years then suddenly released, flag it
		if info.ReleaseCount > 1 && !info.LastReleaseAt.IsZero() && !info.FirstReleaseAt.IsZero() {
			totalAge := info.LastReleaseAt.Sub(info.FirstReleaseAt)
			lastAge := time.Since(info.LastReleaseAt)
			if totalAge > 2*365*24*time.Hour && lastAge < 90*24*time.Hour {
				// Had releases spanning >2 years but the latest is within 90 days
				// Check if there was a long gap — approximate by checking release frequency
				avgInterval := totalAge / time.Duration(info.ReleaseCount-1)
				if avgInterval > 365*24*time.Hour && lastAge < 90*24*time.Hour {
					indicators = append(indicators, SuspiciousIndicator{
						Package:  r.Name,
						Type:     "dormant_spike",
						Severity: SeverityMedium,
						Message: fmt.Sprintf(
							"long-dormant package with recent activity (avg release interval: %.0f days, last release: %.0f days ago) — check for maintainer changes",
							avgInterval.Hours()/24, lastAge.Hours()/24,
						),
					})
				}
			}
		}

		// 3. Package with no source repository
		// Only flag if the package has significant downloads or is depended on.
		// Many small/new packages legitimately don't have project_urls set yet.
		if info.SourceRepoURL == "" && (info.MonthlyDownloads > 10_000 || r.DependedOnCount > 2) {
			indicators = append(indicators, SuspiciousIndicator{
				Package:  r.Name,
				Type:     "no_source",
				Severity: SeverityMedium,
				Message: fmt.Sprintf(
					"no source repository linked — cannot verify code origin (downloads: %dk/month)",
					info.MonthlyDownloads/1000,
				),
			})
		}

		// 4. Very few releases but high dependency count (depended on by many)
		// A package with only 1-2 releases being depended on by many others is unusual
		if info.ReleaseCount <= 2 && r.DependedOnCount > 5 {
			indicators = append(indicators, SuspiciousIndicator{
				Package:  r.Name,
				Type:     "low_age_high_deps",
				Severity: SeverityLow,
				Message: fmt.Sprintf(
					"only %d releases but depended on by %d packages — verify package authenticity",
					info.ReleaseCount, r.DependedOnCount,
				),
			})
		}

		// 5. Solo maintainer on a very popular package
		// High downloads + single maintainer = attractive target for account takeover
		if info.MaintainerCount == 1 && info.MonthlyDownloads > 1_000_000 {
			indicators = append(indicators, SuspiciousIndicator{
				Package:  r.Name,
				Type:     "high_value_solo",
				Severity: SeverityMedium,
				Message: fmt.Sprintf(
					"single maintainer on high-traffic package (%dk/month) — account takeover risk",
					info.MonthlyDownloads/1000,
				),
			})
		}
	}

	return indicators
}
