package core

type RiskLevel string

const (
	RiskLow      RiskLevel = "LOW"
	RiskMedium   RiskLevel = "MEDIUM"
	RiskHigh     RiskLevel = "HIGH"
	RiskCritical RiskLevel = "CRITICAL"
	RiskUnknown  RiskLevel = "UNKNOWN"
)

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
	SeverityHigh   IssueSeverity = "HIGH"
	SeverityMedium IssueSeverity = "MEDIUM"
	SeverityLow    IssueSeverity = "LOW"
	SeverityInfo   IssueSeverity = "INFO"
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
	ConstraintType      string
	Depth               int
	OwnScore            int
	TransitiveRiskScore int
	OwnRisk             RiskLevel
	TransitiveRisk      RiskLevel
	Issues              []Issue
	Vulnerabilities     []Vulnerability
	DependsOn           []string
	DependsOnCount      int
	DependedOnCount     int
}

type Vulnerability struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Severity string `json:"severity"`
}

func (r PackageResult) FinalScore() int {
	if r.TransitiveRiskScore < r.OwnScore {
		return r.TransitiveRiskScore
	}
	return r.OwnScore
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
	DepsMap         map[string][]string // package name → direct dependency names
}

func (s ScanResult) Passed() bool {
	for _, p := range s.Packages {
		if p.FinalScore() < s.PassThreshold {
			return false
		}
	}
	return true
}
