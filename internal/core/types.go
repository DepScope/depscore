package core

type RiskLevel string

const (
	RiskLow      RiskLevel = "LOW"
	RiskMedium   RiskLevel = "MEDIUM"
	RiskHigh     RiskLevel = "HIGH"
	RiskCritical RiskLevel = "CRITICAL"
	RiskUnknown  RiskLevel = "UNKNOWN"
)

// RiskLevelFromScore maps a score in [0, 100] to a RiskLevel.
// Callers are responsible for clamping scores to [0, 100] before calling.
// Scores outside this range are handled by the default case (≤39 → Critical, ≥80 → Low).
// RiskUnknown is used by callers to represent a package whose score could not be computed.
func RiskLevelFromScore(score int) RiskLevel {
	switch {
	case score >= 80:
		return RiskLow
	case score >= 60:
		return RiskMedium
	case score >= 40:
		return RiskHigh
	default:
		return RiskCritical
	}
}

type IssueSeverity string

const (
	SeverityCritical IssueSeverity = "CRITICAL"
	SeverityHigh     IssueSeverity = "HIGH"
	SeverityMedium   IssueSeverity = "MEDIUM"
	SeverityLow      IssueSeverity = "LOW"
	SeverityInfo     IssueSeverity = "INFO"
)

type Issue struct {
	Package  string
	Severity IssueSeverity
	Message  string
}

type PackageResult struct {
	Name                string
	Version             string
	Ecosystem           string
	Constraint          string // the manifest constraint string (e.g., ">=1.26", "^2.0", "==2.31.0")
	ConstraintType      string
	Depth               int
	OwnScore            int // reputation score (7 weighted factors, no CVE)
	VulnScore           int // vulnerability score (100=clean, 10=critical CVE)
	TransitiveRiskScore int
	OwnRisk             RiskLevel
	VulnRisk            RiskLevel
	TransitiveRisk      RiskLevel
	Issues              []Issue
	VulnCount           int
	DependsOnCount      int
	DependedOnCount     int
}

func (r PackageResult) FinalScore() int {
	score := r.OwnScore
	if r.VulnScore < score {
		score = r.VulnScore
	}
	if r.TransitiveRiskScore < score {
		score = r.TransitiveRiskScore
	}
	return score
}

func (r PackageResult) FinalRisk() RiskLevel {
	return RiskLevelFromScore(r.FinalScore())
}

type ScanResult struct {
	Profile         string
	PassThreshold   int
	DirectDeps      int
	TransitiveDeps  int
	MaxDepthReached bool
	Packages        []PackageResult
	AllIssues       []Issue
	Deps            map[string][]string // dependency graph: parent → children
}

func (s ScanResult) Passed() bool {
	if len(s.Packages) == 0 {
		return false
	}
	for _, p := range s.Packages {
		if p.FinalScore() < s.PassThreshold {
			return false
		}
	}
	return true
}
