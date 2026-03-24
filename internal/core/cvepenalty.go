package core

// ApplyCVEPenalty reduces a package's OwnScore based on unpatched CVEs.
// The penalty is applied AFTER the reputation score is computed.
//
// Penalty rules:
//   - Each CRITICAL CVE: -15 points
//   - Each HIGH CVE:     -10 points
//   - Each MEDIUM CVE:    -5 points
//   - Each LOW CVE:       -2 points
//   - Minimum score after penalty: 0
//   - No CVEs: no change
//
// This means a package with reputation score 85 (LOW risk) but 2 CRITICAL
// CVEs drops to 55 (HIGH risk). The reputation score still matters —
// a well-maintained package with CVEs scores higher than an abandoned one
// with the same CVEs, because it's more likely to be patched soon.
func ApplyCVEPenalty(result *PackageResult) {
	if len(result.Vulnerabilities) == 0 {
		return
	}

	penalty := 0
	for _, v := range result.Vulnerabilities {
		switch v.Severity {
		case "CRITICAL":
			penalty += 15
		case "HIGH":
			penalty += 10
		case "MEDIUM":
			penalty += 5
		case "LOW":
			penalty += 2
		default:
			penalty += 5 // unknown severity treated as medium
		}
	}

	result.OwnScore = clamp(result.OwnScore-penalty, 0, 100)
	result.OwnRisk = RiskLevelFromScore(result.OwnScore)
}
