package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeverityRank(t *testing.T) {
	tests := []struct {
		severity IssueSeverity
		expected int
	}{
		{SeverityHigh, 0},
		{SeverityMedium, 1},
		{SeverityLow, 2},
		{SeverityInfo, 3},
		{IssueSeverity("UNKNOWN"), 4},
		{IssueSeverity(""), 4},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, severityRank(tt.severity), "severity %q", tt.severity)
	}
}

func TestSummarizeTopIssue(t *testing.T) {
	t.Run("empty issues returns default message", func(t *testing.T) {
		result := summarizeTopIssue(nil)
		assert.Equal(t, "low reputation score", result)
	})

	t.Run("empty slice returns default message", func(t *testing.T) {
		result := summarizeTopIssue([]Issue{})
		assert.Equal(t, "low reputation score", result)
	})

	t.Run("single HIGH issue returns that message", func(t *testing.T) {
		issues := []Issue{
			{Severity: SeverityHigh, Message: "critical vulnerability found"},
		}
		result := summarizeTopIssue(issues)
		assert.Equal(t, "critical vulnerability found", result)
	})

	t.Run("mix of severities returns highest severity message", func(t *testing.T) {
		issues := []Issue{
			{Severity: SeverityLow, Message: "minor issue"},
			{Severity: SeverityHigh, Message: "major issue"},
			{Severity: SeverityMedium, Message: "moderate issue"},
		}
		result := summarizeTopIssue(issues)
		assert.Equal(t, "major issue", result)
	})

	t.Run("all same severity returns first", func(t *testing.T) {
		issues := []Issue{
			{Severity: SeverityMedium, Message: "issue A"},
			{Severity: SeverityMedium, Message: "issue B"},
		}
		result := summarizeTopIssue(issues)
		assert.Equal(t, "issue A", result)
	})
}

func TestSortRiskPaths(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		var paths []RiskPath
		sortRiskPaths(paths)
		assert.Empty(t, paths)
	})

	t.Run("single element", func(t *testing.T) {
		paths := []RiskPath{{EndScore: 42}}
		sortRiskPaths(paths)
		assert.Equal(t, 42, paths[0].EndScore)
	})

	t.Run("reverse sorted becomes ascending", func(t *testing.T) {
		paths := []RiskPath{
			{EndScore: 80},
			{EndScore: 50},
			{EndScore: 20},
		}
		sortRiskPaths(paths)
		assert.Equal(t, 20, paths[0].EndScore)
		assert.Equal(t, 50, paths[1].EndScore)
		assert.Equal(t, 80, paths[2].EndScore)
	})

	t.Run("already sorted stays sorted", func(t *testing.T) {
		paths := []RiskPath{
			{EndScore: 10},
			{EndScore: 30},
			{EndScore: 70},
		}
		sortRiskPaths(paths)
		assert.Equal(t, 10, paths[0].EndScore)
		assert.Equal(t, 30, paths[1].EndScore)
		assert.Equal(t, 70, paths[2].EndScore)
	})
}

func TestFindRiskPaths(t *testing.T) {
	t.Run("empty results returns empty paths", func(t *testing.T) {
		paths := FindRiskPaths(nil, nil, 60, 10)
		assert.Empty(t, paths)
	})

	t.Run("single direct dep below threshold produces one path", func(t *testing.T) {
		results := []PackageResult{
			{Name: "risky-pkg", Depth: 1, OwnScore: 30, OwnRisk: RiskCritical, Issues: []Issue{
				{Severity: SeverityHigh, Message: "abandoned project"},
			}},
		}
		deps := map[string][]string{}
		paths := FindRiskPaths(results, deps, 60, 10)
		assert.Len(t, paths, 1)
		assert.Equal(t, []string{"risky-pkg"}, paths[0].Chain)
		assert.Equal(t, 30, paths[0].EndScore)
		assert.Equal(t, RiskCritical, paths[0].EndRisk)
		assert.Equal(t, "abandoned project", paths[0].Reason)
	})

	t.Run("chain root->a->b where b is below threshold", func(t *testing.T) {
		results := []PackageResult{
			{Name: "root", Depth: 1, OwnScore: 80, OwnRisk: RiskLow},
			{Name: "a", Depth: 2, OwnScore: 70, OwnRisk: RiskMedium},
			{Name: "b", Depth: 3, OwnScore: 25, OwnRisk: RiskCritical, Issues: []Issue{
				{Severity: SeverityHigh, Message: "no maintainers"},
			}},
		}
		deps := map[string][]string{
			"root": {"a"},
			"a":    {"b"},
		}
		paths := FindRiskPaths(results, deps, 60, 10)
		assert.Len(t, paths, 1)
		assert.Equal(t, []string{"root", "a", "b"}, paths[0].Chain)
		assert.Equal(t, 25, paths[0].EndScore)
		assert.Equal(t, "no maintainers", paths[0].Reason)
	})

	t.Run("maxPaths limits results", func(t *testing.T) {
		results := []PackageResult{
			{Name: "root", Depth: 1, OwnScore: 80, OwnRisk: RiskLow},
			{Name: "bad1", Depth: 2, OwnScore: 20, OwnRisk: RiskCritical},
			{Name: "bad2", Depth: 2, OwnScore: 10, OwnRisk: RiskCritical},
			{Name: "bad3", Depth: 2, OwnScore: 15, OwnRisk: RiskCritical},
		}
		deps := map[string][]string{
			"root": {"bad1", "bad2", "bad3"},
		}
		paths := FindRiskPaths(results, deps, 60, 2)
		assert.Len(t, paths, 2)
		// Should be sorted worst-first (lowest score)
		assert.LessOrEqual(t, paths[0].EndScore, paths[1].EndScore)
	})

	t.Run("direct dep above threshold not included", func(t *testing.T) {
		results := []PackageResult{
			{Name: "safe-pkg", Depth: 1, OwnScore: 90, OwnRisk: RiskLow},
		}
		deps := map[string][]string{}
		paths := FindRiskPaths(results, deps, 60, 10)
		assert.Empty(t, paths)
	})

	t.Run("non-direct dep not used as entry point", func(t *testing.T) {
		// "deep" is below threshold but at Depth=2, not a starting point
		// with no Depth=1 dep linking to it, it should not appear
		results := []PackageResult{
			{Name: "deep", Depth: 2, OwnScore: 20, OwnRisk: RiskCritical},
		}
		deps := map[string][]string{}
		paths := FindRiskPaths(results, deps, 60, 10)
		assert.Empty(t, paths)
	})
}
