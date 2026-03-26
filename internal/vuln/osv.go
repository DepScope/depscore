package vuln

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const osvDefaultBaseURL = "https://api.osv.dev"

// OSVClient queries the OSV.dev vulnerability database via its REST API.
type OSVClient struct {
	baseURL    string
	httpClient *http.Client
}

// OSVOption is a functional option for OSVClient.
type OSVOption func(*osvOptions)

type osvOptions struct {
	baseURL string
}

// WithOSVBaseURL overrides the OSV API base URL.
func WithOSVBaseURL(url string) OSVOption {
	return func(o *osvOptions) { o.baseURL = url }
}

// NewOSVClient constructs a new OSV.dev vulnerability client.
func NewOSVClient(opts ...OSVOption) *OSVClient {
	o := &osvOptions{baseURL: osvDefaultBaseURL}
	for _, opt := range opts {
		opt(o)
	}
	return &OSVClient{
		baseURL:    o.baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Query implements Source.
func (c *OSVClient) Query(ecosystem, name, version string) ([]Finding, error) {
	reqBody := osvQueryRequest{
		Version: version,
		Package: osvPackage{
			Name:      name,
			Ecosystem: ecosystem,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("osv: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/query", c.baseURL)
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("osv: POST %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("osv: POST %s returned %d", url, resp.StatusCode)
	}

	var raw osvResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("osv: decode response: %w", err)
	}

	return raw.toFindings(), nil
}

// AffectedPackage describes a package and version range affected by a vulnerability.
type AffectedPackage struct {
	Name      string
	Ecosystem string
	Range     string // e.g., ">=1.0.0,<1.5.0"
}

// QueryByID fetches a specific vulnerability by its OSV/CVE ID via GET /v1/vulns/{id}
// and returns the list of affected packages with their version ranges.
func (c *OSVClient) QueryByID(id string) ([]AffectedPackage, error) {
	url := fmt.Sprintf("%s/v1/vulns/%s", c.baseURL, id)
	resp, err := c.httpClient.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("osv: GET %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("osv: GET %s returned %d", url, resp.StatusCode)
	}

	var v osvVuln
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("osv: decode response: %w", err)
	}

	return extractAffectedPackages(&v), nil
}

// extractAffectedPackages extracts affected package info from an osvVuln.
func extractAffectedPackages(v *osvVuln) []AffectedPackage {
	var result []AffectedPackage
	for _, aff := range v.Affected {
		for _, rng := range aff.Ranges {
			var introduced, fixed string
			for _, ev := range rng.Events {
				if ev.Introduced != "" {
					introduced = ev.Introduced
				}
				if ev.Fixed != "" {
					fixed = ev.Fixed
				}
			}
			if introduced != "" || fixed != "" {
				var rangeStr string
				if introduced != "" && introduced != "0" && fixed != "" {
					rangeStr = fmt.Sprintf(">=%s,<%s", introduced, fixed)
				} else if introduced != "" && introduced != "0" {
					rangeStr = fmt.Sprintf(">=%s", introduced)
				} else if fixed != "" {
					rangeStr = fmt.Sprintf("<%s", fixed)
				}
				if rangeStr != "" {
					result = append(result, AffectedPackage{
						Name:      aff.Package.Name,
						Ecosystem: aff.Package.Ecosystem,
						Range:     rangeStr,
					})
				}
			}
		}
	}
	return result
}

// ---- raw JSON shapes -------------------------------------------------------

type osvQueryRequest struct {
	Version string     `json:"version"`
	Package osvPackage `json:"package"`
}

type osvPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type osvResponse struct {
	Vulns []osvVuln `json:"vulns"`
}

type osvVuln struct {
	ID       string        `json:"id"`
	Summary  string        `json:"summary"`
	Severity []osvSeverity `json:"severity"`
	Affected []osvAffected `json:"affected"`
}

type osvSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

type osvAffected struct {
	Package osvPackage  `json:"package"`
	Ranges  []osvRange  `json:"ranges"`
}

type osvRange struct {
	Type   string      `json:"type"`
	Events []osvEvent  `json:"events"`
}

type osvEvent struct {
	Introduced string `json:"introduced,omitempty"`
	Fixed      string `json:"fixed,omitempty"`
}

func (r osvResponse) toFindings() []Finding {
	findings := make([]Finding, 0, len(r.Vulns))
	for _, v := range r.Vulns {
		f := Finding{
			ID:      v.ID,
			Summary: v.Summary,
			Source:  "osv.dev",
		}

		// Derive severity from CVSS score string if present.
		f.Severity = osvDeriveSeverity(v.Severity)

		// Collect "fixed" versions from ranges.
		for _, aff := range v.Affected {
			for _, rng := range aff.Ranges {
				for _, ev := range rng.Events {
					if ev.Fixed != "" {
						f.FixedIn = append(f.FixedIn, ev.Fixed)
					}
				}
			}
		}

		findings = append(findings, f)
	}
	return findings
}

// osvDeriveSeverity maps a CVSS score string to a Severity constant.
// Falls back to SeverityMedium when the score cannot be determined.
func osvDeriveSeverity(severities []osvSeverity) Severity {
	for _, s := range severities {
		if s.Type == "CVSS_V3" || s.Type == "CVSS_V2" {
			score := extractCVSSBaseScore(s.Score)
			switch {
			case score >= 9.0:
				return SeverityCritical
			case score >= 7.0:
				return SeverityHigh
			case score >= 4.0:
				return SeverityMedium
			default:
				return SeverityLow
			}
		}
	}
	return SeverityMedium
}

// extractCVSSBaseScore parses the base score out of a CVSS vector string.
// The score appears as a floating-point number embedded in the AV metric string,
// but the OSV API only gives us the full vector — we derive a rough score from
// the AV component to avoid pulling in a CVSS library.
// For a simpler heuristic we look for the numeric part before the first "/".
func extractCVSSBaseScore(vector string) float64 {
	// The CVSS vector format is e.g.
	// "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"
	// There is no standalone numeric score in this string; the score itself
	// would need to be computed. For our heuristic we count "H" (high-impact)
	// components to approximate criticality.
	upper := strings.ToUpper(vector)
	hCount := strings.Count(upper, ":H")
	switch {
	case hCount >= 3:
		return 9.5
	case hCount == 2:
		return 8.0
	case hCount == 1:
		return 6.5
	default:
		return 3.5
	}
}
