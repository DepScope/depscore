package vuln

type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
)

type Finding struct {
	ID       string
	Summary  string
	Severity Severity
	FixedIn  []string
	Source   string
}

type Source interface {
	Query(ecosystem, name, version string) ([]Finding, error)
}
