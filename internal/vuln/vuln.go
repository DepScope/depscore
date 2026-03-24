package vuln

// Severity represents the severity level of a vulnerability.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
)

// Finding represents a single vulnerability finding.
type Finding struct {
	ID       string
	Summary  string
	Severity Severity
	FixedIn  []string
	Source   string
}

// Source is the interface for vulnerability data sources.
type Source interface {
	Query(ecosystem, name, version string) ([]Finding, error)
}
