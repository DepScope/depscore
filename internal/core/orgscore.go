// internal/core/orgscore.go
package core

import "strings"

// knownCorporateOrgs is a seed list of well-known corporate/foundation GitHub
// organizations. The org portion of a "github.com/ORG/..." path is matched
// against this map to identify corporate-level trust.
var knownCorporateOrgs = map[string]bool{
	"google": true, "microsoft": true, "hashicorp": true,
	"aws": true, "facebook": true, "meta": true,
	"apple": true, "vercel": true, "cloudflare": true,
	"github": true, "actions": true, "golang": true,
	"rust-lang": true, "python": true, "nodejs": true,
	"docker": true, "kubernetes": true, "grafana": true,
	"elastic": true, "datadog": true, "apache": true,
	"mozilla": true, "linux": true,
}

// ClassifyOrg classifies a project by organizational trust level.
//
//   - "own"        – projectID has a prefix matching any entry in trustedOrgs
//   - "corporate"  – the org segment of the GitHub path matches knownCorporateOrgs
//   - "individual" – everything else
func ClassifyOrg(projectID string, trustedOrgs []string) string {
	// Check own-org prefixes first (highest trust).
	// Match at path boundary: "github.com/google" matches "github.com/google/repo"
	// but NOT "github.com/google-research/repo".
	for _, org := range trustedOrgs {
		if projectID == org || strings.HasPrefix(projectID, org+"/") {
			return "own"
		}
	}

	// Extract the org segment from a "github.com/ORG/REPO" path.
	org := extractGitHubOrg(projectID)
	if org != "" && knownCorporateOrgs[org] {
		return "corporate"
	}

	return "individual"
}

// extractGitHubOrg returns the ORG portion of a "github.com/ORG/..." path,
// or "" if the path does not match that shape.
func extractGitHubOrg(projectID string) string {
	// Strip leading scheme if present (e.g. "https://github.com/…").
	id := projectID
	if i := strings.Index(id, "://"); i != -1 {
		id = id[i+3:]
	}

	// Expect "github.com/ORG/…"
	const prefix = "github.com/"
	if !strings.HasPrefix(id, prefix) {
		return ""
	}
	rest := id[len(prefix):]
	if slash := strings.Index(rest, "/"); slash != -1 {
		return rest[:slash]
	}
	// Single segment after "github.com/" — treat as org-level path.
	return rest
}

// ApplyOrgTrust adjusts a node's reputation score based on its org classification.
//
//   - "own"        – if score < ownOrgFloor, return ownOrgFloor (default 80)
//   - "corporate"  – boost score by 5 (capped at 100)
//   - otherwise    – return score unchanged
func ApplyOrgTrust(score int, orgType string, ownOrgFloor int) int {
	switch orgType {
	case "own":
		if score < ownOrgFloor {
			return ownOrgFloor
		}
		return score
	case "corporate":
		boosted := score + 5
		if boosted > 100 {
			return 100
		}
		return boosted
	default:
		return score
	}
}
