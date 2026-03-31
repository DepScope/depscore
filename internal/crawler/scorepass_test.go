package crawler

import (
	"sync"
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/vcs"
)

func TestPinningScore(t *testing.T) {
	tests := []struct {
		pinning graph.PinningQuality
		want    int
	}{
		{graph.PinningSHA, 100},
		{graph.PinningDigest, 100},
		{graph.PinningExactVersion, 85},
		{graph.PinningSemverRange, 70},
		{graph.PinningMajorTag, 40},
		{graph.PinningBranch, 20},
		{graph.PinningUnpinned, 0},
		{graph.PinningNA, 50},
		{graph.PinningQuality(99), 0}, // unknown defaults to 0
	}
	for _, tt := range tests {
		got := pinningScore(tt.pinning)
		if got != tt.want {
			t.Errorf("pinningScore(%v) = %d, want %d", tt.pinning, got, tt.want)
		}
	}
}

func TestScorePinningNode(t *testing.T) {
	tests := []struct {
		name      string
		pinning   graph.PinningQuality
		wantScore int
		wantRisk  core.RiskLevel
	}{
		{
			name:      "ExactVersion → score=85, risk=LOW",
			pinning:   graph.PinningExactVersion,
			wantScore: 85,
			wantRisk:  core.RiskLow,
		},
		{
			name:      "Unpinned → score=0, risk=CRITICAL",
			pinning:   graph.PinningUnpinned,
			wantScore: 0,
			wantRisk:  core.RiskCritical,
		},
		{
			name:      "SHA → score=100, risk=LOW",
			pinning:   graph.PinningSHA,
			wantScore: 100,
			wantRisk:  core.RiskLow,
		},
		{
			name:      "MajorTag → score=40, risk=HIGH",
			pinning:   graph.PinningMajorTag,
			wantScore: 40,
			wantRisk:  core.RiskHigh,
		},
		{
			name:      "Branch → score=20, risk=CRITICAL",
			pinning:   graph.PinningBranch,
			wantScore: 20,
			wantRisk:  core.RiskCritical,
		},
		{
			name:      "NA → score=50, risk=HIGH",
			pinning:   graph.PinningNA,
			wantScore: 50,
			wantRisk:  core.RiskHigh,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &graph.Node{
				ID:       "tool:test",
				Type:     graph.NodeDevTool,
				Name:     "test-tool",
				Pinning:  tt.pinning,
				Metadata: make(map[string]any),
			}
			scorePinningNode(node)
			if node.Score != tt.wantScore {
				t.Errorf("scorePinningNode: score = %d, want %d", node.Score, tt.wantScore)
			}
			if node.Risk != tt.wantRisk {
				t.Errorf("scorePinningNode: risk = %s, want %s", node.Risk, tt.wantRisk)
			}
		})
	}
}

func TestScoreGitNode_MutableTag(t *testing.T) {
	var mu sync.Mutex
	repoCache := make(map[string]*vcs.RepoInfo)

	tests := []struct {
		name     string
		pinning  graph.PinningQuality
		wantBase int // expected baseScore when no repo info is available
	}{
		{
			name:     "MajorTag → baseScore = 40+10 = 50",
			pinning:  graph.PinningMajorTag,
			wantBase: 50,
		},
		{
			name:     "SHA → baseScore = 100+10 clamped to 100",
			pinning:  graph.PinningSHA,
			wantBase: 100,
		},
		{
			name:     "Branch → baseScore = 20+10 = 30",
			pinning:  graph.PinningBranch,
			wantBase: 30,
		},
		{
			name:     "Unpinned → baseScore = 0+10 = 10",
			pinning:  graph.PinningUnpinned,
			wantBase: 10,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &graph.Node{
				ID:        "action:test@v1",
				Type:      graph.NodeAction,
				Name:      "test-action",
				Pinning:   tt.pinning,
				ProjectID: "", // no project ID → no VCS lookup, uses baseScore
				Metadata:  make(map[string]any),
			}
			// No VCS client needed since ProjectID is empty.
			scoreGitNode(node, nil, &mu, repoCache)
			if node.Score != tt.wantBase {
				t.Errorf("scoreGitNode: score = %d, want %d", node.Score, tt.wantBase)
			}
		})
	}
}

func TestSplitProjectID(t *testing.T) {
	tests := []struct {
		input         string
		wantEcosystem string
		wantName      string
	}{
		{"Go/github.com/foo/bar", "Go", "github.com/foo/bar"},
		{"npm/lodash", "npm", "lodash"},
		{"PyPI/requests", "PyPI", "requests"},
		{"crates.io/serde", "crates.io", "serde"},
		{"", "", ""},
		{"noSlash", "noSlash", ""},
	}
	for _, tt := range tests {
		eco, name := splitProjectID(tt.input)
		if eco != tt.wantEcosystem {
			t.Errorf("splitProjectID(%q) ecosystem = %q, want %q", tt.input, eco, tt.wantEcosystem)
		}
		if name != tt.wantName {
			t.Errorf("splitProjectID(%q) name = %q, want %q", tt.input, name, tt.wantName)
		}
	}
}

func TestMapEcosystemToRegistry(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"python", "PyPI"},
		{"PyPI", "PyPI"},
		{"pypi", "PyPI"},
		{"go", "Go"},
		{"Go", "Go"},
		{"npm", "npm"},
		{"rust", "crates.io"},
		{"crates.io", "crates.io"},
		{"php", "Packagist"},
		{"Packagist", "Packagist"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := mapEcosystemToRegistry(tt.input)
		if got != tt.want {
			t.Errorf("mapEcosystemToRegistry(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
