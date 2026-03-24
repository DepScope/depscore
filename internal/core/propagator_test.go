package core_test

import (
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestEffectiveScore(t *testing.T) {
	assert.Equal(t, 25, core.EffectiveScore(25, 1))  // depth 1: no discount
	assert.Equal(t, 45, core.EffectiveScore(25, 5))  // depth 5: +20
	assert.Equal(t, 70, core.EffectiveScore(25, 10)) // depth 10: +45
	assert.Equal(t, 100, core.EffectiveScore(90, 5)) // clamped to 100
}

func TestEffectiveScoreClamps(t *testing.T) {
	assert.Equal(t, 0, core.EffectiveScore(0, 1))
	assert.Equal(t, 100, core.EffectiveScore(100, 1))
	assert.Equal(t, 100, core.EffectiveScore(100, 100))
}

func TestPropagate(t *testing.T) {
	// root(d1,90) → middle(d2,75) → bad(d3,25)
	// Depth is the absolute depth in the dependency tree.
	results := []core.PackageResult{
		{Name: "root", OwnScore: 90, Depth: 1, TransitiveRiskScore: 100},
		{Name: "middle", OwnScore: 75, Depth: 2, TransitiveRiskScore: 100},
		{Name: "bad", OwnScore: 25, Depth: 3, TransitiveRiskScore: 100},
	}
	deps := map[string][]string{
		"root":   {"middle"},
		"middle": {"bad"},
		// "bad" has no children
	}

	propagated := core.Propagate(results, deps)

	// bad: no descendants, TransitiveRiskScore stays 100
	bad := findResult(propagated, "bad")
	assert.Equal(t, 100, bad.TransitiveRiskScore)

	// middle: only descendant is bad(d3).
	// EffectiveScore(25, 3) = 25 + (3-1)*5 = 35
	middle := findResult(propagated, "middle")
	assert.Equal(t, 35, middle.TransitiveRiskScore)

	// root: descendants are middle(d2) and bad(d3).
	// eff(middle@d2) = 75+(2-1)*5 = 80
	// eff(bad@d3) = 25+(3-1)*5 = 35
	// min = 35
	root := findResult(propagated, "root")
	assert.Equal(t, 35, root.TransitiveRiskScore)
}

func TestPropagateNoDescendants(t *testing.T) {
	results := []core.PackageResult{
		{Name: "leaf", OwnScore: 50, TransitiveRiskScore: 100},
	}
	deps := map[string][]string{}
	propagated := core.Propagate(results, deps)
	assert.Equal(t, 100, propagated[0].TransitiveRiskScore)
}

func TestPropagateCycleGuard(t *testing.T) {
	// Cycles shouldn't infinite loop; just verify no panic.
	results := []core.PackageResult{
		{Name: "a", OwnScore: 80, Depth: 1, TransitiveRiskScore: 100},
		{Name: "b", OwnScore: 60, Depth: 2, TransitiveRiskScore: 100},
	}
	deps := map[string][]string{
		"a": {"b"},
		"b": {"a"}, // cycle
	}
	// Should complete without hanging — we don't require perfect cycle handling,
	// just that it doesn't panic.
	assert.NotPanics(t, func() {
		core.Propagate(results, deps)
	})
}

func findResult(results []core.PackageResult, name string) core.PackageResult {
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	panic("not found: " + name)
}
