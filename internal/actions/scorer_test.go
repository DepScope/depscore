package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScoreAction(t *testing.T) {
	tests := []struct {
		name    string
		input   ScoringInput
		wantMin int
		wantMax int
	}{
		{
			name: "sha-pinned first-party",
			input: ScoringInput{
				Pinning:          PinSHA,
				FirstParty:       true,
				RepoStars:        10000,
				RepoArchived:     false,
				RepoLastCommitDays: 10,
				MaintainerCount:  5,
				HasOrgBacking:    true,
				DaysSinceRelease: 15,
				BundledMinScore:  100,
				PermissionsBroad: false,
			},
			wantMin: 85,
			wantMax: 100,
		},
		{
			name: "major-tag third-party",
			input: ScoringInput{
				Pinning:          PinMajorTag,
				FirstParty:       false,
				RepoStars:        100,
				RepoArchived:     false,
				RepoLastCommitDays: 60,
				MaintainerCount:  2,
				HasOrgBacking:    false,
				DaysSinceRelease: 45,
				BundledMinScore:  70,
				PermissionsBroad: false,
			},
			wantMin: 40,
			wantMax: 70,
		},
		{
			name: "branch-pinned unknown",
			input: ScoringInput{
				Pinning:          PinBranch,
				FirstParty:       false,
				RepoStars:        0,
				RepoArchived:     true,
				RepoLastCommitDays: 400,
				MaintainerCount:  1,
				HasOrgBacking:    false,
				DaysSinceRelease: 400,
				BundledMinScore:  20,
				PermissionsBroad: true,
			},
			wantMin: 0,
			wantMax: 40,
		},
		{
			name: "script download always zero",
			input: ScoringInput{
				IsScriptDownload: true,
				Pinning:          PinSHA,
				FirstParty:       true,
				RepoStars:        99999,
			},
			wantMin: 0,
			wantMax: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScoreAction(tt.input)
			assert.GreaterOrEqual(t, got, tt.wantMin, "score below minimum")
			assert.LessOrEqual(t, got, tt.wantMax, "score above maximum")
		})
	}
}

func TestScoreActionFactors(t *testing.T) {
	// Test individual factor contributions by varying only one factor at a time
	// against a baseline.
	baseline := ScoringInput{
		Pinning:            PinExactVersion,
		FirstParty:         false,
		RepoStars:          500,
		RepoArchived:       false,
		RepoLastCommitDays: 30,
		MaintainerCount:    2,
		HasOrgBacking:      false,
		DaysSinceRelease:   45,
		BundledMinScore:    100,
		PermissionsBroad:   false,
	}

	// SHA pinning should score higher than major tag pinning
	shaPinned := baseline
	shaPinned.Pinning = PinSHA
	majorTagPinned := baseline
	majorTagPinned.Pinning = PinMajorTag

	assert.Greater(t, ScoreAction(shaPinned), ScoreAction(majorTagPinned), "SHA pinning should outscore major tag")

	// First party should score higher than third party
	firstParty := baseline
	firstParty.FirstParty = true
	thirdParty := baseline
	thirdParty.FirstParty = false

	assert.Greater(t, ScoreAction(firstParty), ScoreAction(thirdParty), "first party should outscore third party")

	// Archived repo should score lower than active repo
	active := baseline
	archived := baseline
	archived.RepoArchived = true
	archived.RepoLastCommitDays = 400

	assert.Greater(t, ScoreAction(active), ScoreAction(archived), "active repo should outscore archived repo")

	// Permissions read-only should score higher than broad permissions
	readOnly := baseline
	readOnly.PermissionsBroad = false
	broad := baseline
	broad.PermissionsBroad = true

	assert.Greater(t, ScoreAction(readOnly), ScoreAction(broad), "read-only permissions should outscore broad permissions")
}

func TestScoreDockerImage(t *testing.T) {
	tests := []struct {
		name    string
		input   DockerScoringInput
		wantMin int
		wantMax int
	}{
		{
			name: "official with digest",
			input: DockerScoringInput{
				PinningDigest: true,
				ExactTag:      false,
				IsOfficial:    true,
				IsVerified:    false,
				DaysSincePush: 10,
				KnownBase:     true,
				VulnCount:     0,
			},
			wantMin: 80,
			wantMax: 100,
		},
		{
			name: "latest tag unofficial",
			input: DockerScoringInput{
				PinningDigest: false,
				ExactTag:      false,
				IsOfficial:    false,
				IsVerified:    false,
				DaysSincePush: 200,
				KnownBase:     false,
				VulnCount:     10,
			},
			wantMin: 0,
			wantMax: 40,
		},
		{
			name: "verified publisher with exact tag",
			input: DockerScoringInput{
				PinningDigest: false,
				ExactTag:      true,
				IsOfficial:    false,
				IsVerified:    true,
				DaysSincePush: 20,
				KnownBase:     true,
				VulnCount:     2,
			},
			wantMin: 50,
			wantMax: 85,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScoreDockerImage(tt.input)
			assert.GreaterOrEqual(t, got, tt.wantMin, "score below minimum")
			assert.LessOrEqual(t, got, tt.wantMax, "score above maximum")
		})
	}
}

func TestScoreDockerImageFactors(t *testing.T) {
	// Digest pinning should score higher than exact tag
	digestPinned := DockerScoringInput{PinningDigest: true, IsOfficial: true, DaysSincePush: 5, KnownBase: true, VulnCount: 0}
	exactTagPinned := DockerScoringInput{PinningDigest: false, ExactTag: true, IsOfficial: true, DaysSincePush: 5, KnownBase: true, VulnCount: 0}
	latestTag := DockerScoringInput{PinningDigest: false, ExactTag: false, IsOfficial: true, DaysSincePush: 5, KnownBase: true, VulnCount: 0}

	assert.Greater(t, ScoreDockerImage(digestPinned), ScoreDockerImage(exactTagPinned), "digest should outscore exact tag")
	assert.Greater(t, ScoreDockerImage(exactTagPinned), ScoreDockerImage(latestTag), "exact tag should outscore latest")

	// More vulns should lower score
	noVulns := DockerScoringInput{PinningDigest: true, IsOfficial: true, DaysSincePush: 5, KnownBase: true, VulnCount: 0}
	someVulns := DockerScoringInput{PinningDigest: true, IsOfficial: true, DaysSincePush: 5, KnownBase: true, VulnCount: 5}
	manyVulns := DockerScoringInput{PinningDigest: true, IsOfficial: true, DaysSincePush: 5, KnownBase: true, VulnCount: 25}

	assert.Greater(t, ScoreDockerImage(noVulns), ScoreDockerImage(someVulns), "no vulns should outscore some vulns")
	assert.Greater(t, ScoreDockerImage(someVulns), ScoreDockerImage(manyVulns), "some vulns should outscore many vulns")
}

func TestScoreActionUnpinned(t *testing.T) {
	// Unpinned action should score below branch-pinned
	unpinned := ScoringInput{Pinning: PinUnpinned, FirstParty: false, BundledMinScore: 100}
	branchPinned := ScoringInput{Pinning: PinBranch, FirstParty: false, BundledMinScore: 100}

	assert.Less(t, ScoreAction(unpinned), ScoreAction(branchPinned), "unpinned should score less than branch-pinned")
}

func TestScoreActionBundledRisk(t *testing.T) {
	// Low bundled score should drag down overall score
	noBundled := ScoringInput{
		Pinning: PinSHA, FirstParty: true, RepoStars: 5000,
		MaintainerCount: 3, DaysSinceRelease: 10,
		BundledMinScore: 100,
	}
	highRiskBundled := ScoringInput{
		Pinning: PinSHA, FirstParty: true, RepoStars: 5000,
		MaintainerCount: 3, DaysSinceRelease: 10,
		BundledMinScore: 0,
	}

	assert.Greater(t, ScoreAction(noBundled), ScoreAction(highRiskBundled), "no bundled risk should outscore high bundled risk")
}
