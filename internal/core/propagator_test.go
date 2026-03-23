package core_test

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestEffectiveScore(t *testing.T) {
	assert.Equal(t, 25, core.EffectiveScore(25, 1))
	assert.Equal(t, 45, core.EffectiveScore(25, 5))
	assert.Equal(t, 70, core.EffectiveScore(25, 10))
	assert.Equal(t, 100, core.EffectiveScore(90, 5))
}

func TestPropagate(t *testing.T) {
	results := []core.PackageResult{
		{Name: "root", OwnScore: 90, Depth: 1, TransitiveRiskScore: 100},
		{Name: "middle", OwnScore: 75, Depth: 2, TransitiveRiskScore: 100},
		{Name: "bad", OwnScore: 25, Depth: 3, TransitiveRiskScore: 100},
	}
	deps := map[string][]string{
		"root":   {"middle"},
		"middle": {"bad"},
	}
	propagated := core.Propagate(results, deps)

	byName := make(map[string]core.PackageResult)
	for _, r := range propagated {
		byName[r.Name] = r
	}

	assert.Equal(t, 100, byName["bad"].TransitiveRiskScore)
	assert.Equal(t, 35, byName["middle"].TransitiveRiskScore)
	assert.Equal(t, core.RiskCritical, byName["middle"].TransitiveRisk)
	assert.Equal(t, 35, byName["root"].TransitiveRiskScore)
}

func TestPropagateNoRisk(t *testing.T) {
	results := []core.PackageResult{
		{Name: "a", OwnScore: 85, Depth: 1, TransitiveRiskScore: 100},
		{Name: "b", OwnScore: 90, Depth: 2, TransitiveRiskScore: 100},
	}
	deps := map[string][]string{"a": {"b"}}
	propagated := core.Propagate(results, deps)
	for _, r := range propagated {
		if r.Name == "a" {
			assert.Equal(t, core.RiskLow, r.TransitiveRisk)
		}
	}
}
