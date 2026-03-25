package actions

// ScoringInput holds all the factors needed to compute an action's risk score.
type ScoringInput struct {
	Pinning            PinQuality
	FirstParty         bool
	RepoStars          int
	RepoArchived       bool
	RepoLastCommitDays int
	MaintainerCount    int
	HasOrgBacking      bool
	DaysSinceRelease   int
	BundledMinScore    int  // min score of bundled packages, 100 if none
	PermissionsBroad   bool // true if no permissions block or write permissions
	IsScriptDownload   bool
}

// DockerScoringInput holds all factors for scoring a Docker image reference.
type DockerScoringInput struct {
	PinningDigest bool
	ExactTag      bool
	IsOfficial    bool
	IsVerified    bool
	DaysSincePush int
	KnownBase     bool
	VulnCount     int
}

// ScoreAction computes a 0-100 risk score for a GitHub Action reference.
// Higher is safer. Script downloads always return 0.
//
// Weighted factors (summing to 100%):
//   - Pinning quality:   25%
//   - First-party:       15%
//   - Repo health:       15%
//   - Maintainer count:  10%
//   - Release recency:   10%
//   - Bundled dep risk:  15%
//   - Permissions scope: 10%
func ScoreAction(input ScoringInput) int {
	if input.IsScriptDownload {
		return 0
	}

	pinScore := scorePinQuality(input.Pinning)
	firstPartyScore := scoreFirstParty(input.FirstParty)
	repoHealthScore := scoreRepoHealth(input.RepoStars, input.RepoArchived, input.RepoLastCommitDays)
	maintainerScore := scoreMaintainer(input.MaintainerCount, input.HasOrgBacking)
	recencyScore := scoreRecency(input.DaysSinceRelease)
	bundledScore := scoreBundled(input.BundledMinScore)
	permissionsScore := scorePermissions(input.PermissionsBroad)

	// Weighted sum: weights are in tenths of a percent (× 1000) to avoid floats.
	// 25 + 15 + 15 + 10 + 10 + 15 + 10 = 100
	total := pinScore*25 +
		firstPartyScore*15 +
		repoHealthScore*15 +
		maintainerScore*10 +
		recencyScore*10 +
		bundledScore*15 +
		permissionsScore*10

	return total / 100
}

// scorePinQuality maps a PinQuality to a factor score (0-100).
func scorePinQuality(p PinQuality) int {
	switch p {
	case PinSHA:
		return 100
	case PinExactVersion:
		return 80
	case PinMajorTag:
		return 50
	case PinBranch:
		return 20
	default: // PinUnpinned
		return 0
	}
}

// scoreFirstParty returns 100 for actions/* or github/*, else 40.
func scoreFirstParty(firstParty bool) int {
	if firstParty {
		return 100
	}
	return 40
}

// scoreRepoHealth returns 0-100 based on stars, recency, and archived status.
func scoreRepoHealth(stars int, archived bool, lastCommitDays int) int {
	if archived {
		return 0
	}

	// Star score: log-ish scale capped at 100
	starScore := 0
	switch {
	case stars >= 10000:
		starScore = 100
	case stars >= 1000:
		starScore = 80
	case stars >= 100:
		starScore = 60
	case stars >= 10:
		starScore = 40
	default:
		starScore = 20
	}

	// Activity score based on last commit
	activityScore := 0
	switch {
	case lastCommitDays == 0:
		// No info provided — neutral
		activityScore = 60
	case lastCommitDays < 30:
		activityScore = 100
	case lastCommitDays < 90:
		activityScore = 80
	case lastCommitDays < 180:
		activityScore = 60
	case lastCommitDays < 365:
		activityScore = 40
	default:
		activityScore = 20
	}

	return (starScore + activityScore) / 2
}

// scoreMaintainer returns 40-100 based on maintainer count and org backing.
func scoreMaintainer(count int, hasOrg bool) int {
	if hasOrg {
		return 100
	}
	switch {
	case count >= 3:
		return 80
	case count == 2:
		return 60
	case count == 1:
		return 40
	default:
		return 40
	}
}

// scoreRecency maps days-since-release to a factor score.
func scoreRecency(days int) int {
	switch {
	case days == 0:
		// No info — neutral
		return 60
	case days < 30:
		return 100
	case days < 90:
		return 80
	case days < 180:
		return 60
	case days < 365:
		return 40
	default:
		return 20
	}
}

// scoreBundled returns the bundled min score directly (100 if no bundled deps).
func scoreBundled(minScore int) int {
	return minScore
}

// scorePermissions maps permission breadth to a factor score.
// broad=true means no permissions block (GitHub defaults to broad) or write perms present.
func scorePermissions(broad bool) int {
	if broad {
		return 30
	}
	return 90
}

// ScoreDockerImage computes a 0-100 risk score for a Docker image reference.
// Higher is safer.
//
// Weighted factors (summing to 100%):
//   - Pinning quality:    30%
//   - Official status:    20%
//   - Image age:          20%
//   - Base image chain:   15%
//   - Vulnerability count: 15%
func ScoreDockerImage(input DockerScoringInput) int {
	pinScore := scoreDockerPinning(input.PinningDigest, input.ExactTag)
	officialScore := scoreDockerOfficial(input.IsOfficial, input.IsVerified)
	ageScore := scoreDockerAge(input.DaysSincePush)
	baseScore := scoreDockerBase(input.KnownBase)
	vulnScore := scoreDockerVulns(input.VulnCount)

	// 30 + 20 + 20 + 15 + 15 = 100
	total := pinScore*30 +
		officialScore*20 +
		ageScore*20 +
		baseScore*15 +
		vulnScore*15

	return total / 100
}

// scoreDockerPinning maps Docker pin quality to a factor score.
func scoreDockerPinning(digest, exactTag bool) int {
	switch {
	case digest:
		return 100
	case exactTag:
		return 70
	default: // latest or unspecified
		return 20
	}
}

// scoreDockerOfficial maps Docker Hub trust levels to a factor score.
func scoreDockerOfficial(official, verified bool) int {
	switch {
	case official:
		return 100
	case verified:
		return 80
	default:
		return 40
	}
}

// scoreDockerAge maps days-since-push to a factor score.
func scoreDockerAge(days int) int {
	switch {
	case days == 0:
		return 60
	case days < 30:
		return 100
	case days < 90:
		return 80
	case days < 180:
		return 60
	case days < 365:
		return 40
	default:
		return 20
	}
}

// scoreDockerBase returns 80 for a known base image chain, 40 for unknown.
func scoreDockerBase(known bool) int {
	if known {
		return 80
	}
	return 40
}

// scoreDockerVulns maps vulnerability count to a factor score.
func scoreDockerVulns(count int) int {
	switch {
	case count == 0:
		return 100
	case count <= 5:
		return 60
	case count <= 20:
		return 30
	default:
		return 0
	}
}
