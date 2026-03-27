package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyCVEPenalty(t *testing.T) {
	t.Run("no vulns leaves score unchanged", func(t *testing.T) {
		r := &PackageResult{OwnScore: 85, OwnRisk: RiskLow}
		ApplyCVEPenalty(r)
		assert.Equal(t, 85, r.OwnScore)
		assert.Equal(t, RiskLow, r.OwnRisk)
	})

	t.Run("single CRITICAL reduces by 15", func(t *testing.T) {
		r := &PackageResult{
			OwnScore:        85,
			OwnRisk:         RiskLow,
			Vulnerabilities: []Vulnerability{{ID: "CVE-1", Severity: "CRITICAL"}},
		}
		ApplyCVEPenalty(r)
		assert.Equal(t, 70, r.OwnScore)
		assert.Equal(t, RiskMedium, r.OwnRisk)
	})

	t.Run("single HIGH reduces by 10", func(t *testing.T) {
		r := &PackageResult{
			OwnScore:        85,
			OwnRisk:         RiskLow,
			Vulnerabilities: []Vulnerability{{ID: "CVE-2", Severity: "HIGH"}},
		}
		ApplyCVEPenalty(r)
		assert.Equal(t, 75, r.OwnScore)
		assert.Equal(t, RiskMedium, r.OwnRisk)
	})

	t.Run("single MEDIUM reduces by 5", func(t *testing.T) {
		r := &PackageResult{
			OwnScore:        85,
			OwnRisk:         RiskLow,
			Vulnerabilities: []Vulnerability{{ID: "CVE-3", Severity: "MEDIUM"}},
		}
		ApplyCVEPenalty(r)
		assert.Equal(t, 80, r.OwnScore)
		assert.Equal(t, RiskLow, r.OwnRisk)
	})

	t.Run("single LOW reduces by 2", func(t *testing.T) {
		r := &PackageResult{
			OwnScore:        85,
			OwnRisk:         RiskLow,
			Vulnerabilities: []Vulnerability{{ID: "CVE-4", Severity: "LOW"}},
		}
		ApplyCVEPenalty(r)
		assert.Equal(t, 83, r.OwnScore)
		assert.Equal(t, RiskLow, r.OwnRisk)
	})

	t.Run("unknown severity reduces by 5", func(t *testing.T) {
		r := &PackageResult{
			OwnScore:        85,
			OwnRisk:         RiskLow,
			Vulnerabilities: []Vulnerability{{ID: "CVE-5", Severity: "WEIRD"}},
		}
		ApplyCVEPenalty(r)
		assert.Equal(t, 80, r.OwnScore)
	})

	t.Run("multiple CVEs combine: 2 CRITICAL + 1 HIGH = -40", func(t *testing.T) {
		r := &PackageResult{
			OwnScore: 85,
			OwnRisk:  RiskLow,
			Vulnerabilities: []Vulnerability{
				{ID: "CVE-A", Severity: "CRITICAL"},
				{ID: "CVE-B", Severity: "CRITICAL"},
				{ID: "CVE-C", Severity: "HIGH"},
			},
		}
		ApplyCVEPenalty(r)
		assert.Equal(t, 45, r.OwnScore) // 85 - 15 - 15 - 10 = 45
		assert.Equal(t, RiskHigh, r.OwnRisk)
	})

	t.Run("score cannot go below 0 (clamped)", func(t *testing.T) {
		r := &PackageResult{
			OwnScore: 10,
			OwnRisk:  RiskCritical,
			Vulnerabilities: []Vulnerability{
				{ID: "CVE-X", Severity: "CRITICAL"},
				{ID: "CVE-Y", Severity: "CRITICAL"},
			},
		}
		ApplyCVEPenalty(r)
		assert.Equal(t, 0, r.OwnScore) // 10 - 30 = -20, clamped to 0
		assert.Equal(t, RiskCritical, r.OwnRisk)
	})

	t.Run("OwnRisk is updated after penalty", func(t *testing.T) {
		r := &PackageResult{
			OwnScore:        90,
			OwnRisk:         RiskLow,
			Vulnerabilities: []Vulnerability{{ID: "CVE-Z", Severity: "CRITICAL"}},
		}
		ApplyCVEPenalty(r)
		assert.Equal(t, 75, r.OwnScore) // 90 - 15 = 75
		assert.Equal(t, RiskMedium, r.OwnRisk)
	})
}
