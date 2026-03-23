package core_test

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestRiskLevelFromScore(t *testing.T) {
	tests := []struct {
		score    int
		expected core.RiskLevel
	}{
		{100, core.RiskLow},
		{80, core.RiskLow},
		{79, core.RiskMedium},
		{60, core.RiskMedium},
		{59, core.RiskHigh},
		{40, core.RiskHigh},
		{39, core.RiskCritical},
		{0, core.RiskCritical},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, core.RiskLevelFromScore(tt.score), "score %d", tt.score)
	}
}

func TestFinalScore(t *testing.T) {
	r := core.PackageResult{OwnScore: 75, TransitiveRiskScore: 45}
	assert.Equal(t, 45, r.FinalScore())

	r2 := core.PackageResult{OwnScore: 45, TransitiveRiskScore: 75}
	assert.Equal(t, 45, r2.FinalScore())
}

func TestFinalRisk(t *testing.T) {
	r := core.PackageResult{OwnScore: 75, TransitiveRiskScore: 45}
	assert.Equal(t, core.RiskHigh, r.FinalRisk()) // 45 → High
}

func TestScanResultPassedEmptyPackages(t *testing.T) {
	empty := core.ScanResult{PassThreshold: 70, Packages: nil}
	assert.False(t, empty.Passed(), "empty scan should not pass")
}

func TestRiskUnknownConstant(t *testing.T) {
	assert.Equal(t, core.RiskLevel("UNKNOWN"), core.RiskUnknown)
}

func TestScanResultPassed(t *testing.T) {
	passing := core.ScanResult{
		PassThreshold: 70,
		Packages: []core.PackageResult{
			{OwnScore: 80, TransitiveRiskScore: 80},
			{OwnScore: 75, TransitiveRiskScore: 75},
		},
	}
	assert.True(t, passing.Passed())

	failing := core.ScanResult{
		PassThreshold: 70,
		Packages: []core.PackageResult{
			{OwnScore: 80, TransitiveRiskScore: 80},
			{OwnScore: 50, TransitiveRiskScore: 50},
		},
	}
	assert.False(t, failing.Passed())
}
