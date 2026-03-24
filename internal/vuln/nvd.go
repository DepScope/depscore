package vuln

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const nvdDefaultBaseURL = "https://services.nvd.nist.gov"

// NVDClient queries the NIST National Vulnerability Database REST API v2.
// When no API key is configured it returns empty results rather than failing,
// because NVD heavily rate-limits unauthenticated clients.
type NVDClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NVDOption is a functional option for NVDClient.
type NVDOption func(*nvdOptions)

type nvdOptions struct {
	baseURL string
	apiKey  string
}

// WithNVDBaseURL overrides the NVD API base URL.
func WithNVDBaseURL(u string) NVDOption {
	return func(o *nvdOptions) { o.baseURL = u }
}

// WithNVDAPIKey sets the NVD API key for authenticated requests.
func WithNVDAPIKey(key string) NVDOption {
	return func(o *nvdOptions) { o.apiKey = key }
}

// NewNVDClient constructs a new NVD vulnerability client.
func NewNVDClient(opts ...NVDOption) *NVDClient {
	o := &nvdOptions{baseURL: nvdDefaultBaseURL}
	for _, opt := range opts {
		opt(o)
	}
	return &NVDClient{
		baseURL:    o.baseURL,
		apiKey:     o.apiKey,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

// Query implements Source. Returns empty results when no API key is configured.
func (c *NVDClient) Query(ecosystem, name, version string) ([]Finding, error) {
	if c.apiKey == "" {
		return nil, nil
	}

	endpoint := fmt.Sprintf("%s/rest/json/cves/2.0", c.baseURL)
	params := url.Values{}
	params.Set("keywordSearch", name)
	fullURL := endpoint + "?" + params.Encode()

	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("nvd: build request: %w", err)
	}
	req.Header.Set("apiKey", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nvd: GET %s: %w", fullURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nvd: GET %s returned %d", fullURL, resp.StatusCode)
	}

	var raw nvdResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("nvd: decode response: %w", err)
	}

	return raw.toFindings(), nil
}

// ---- raw JSON shapes -------------------------------------------------------

type nvdResponse struct {
	Vulnerabilities []nvdVulnWrapper `json:"vulnerabilities"`
}

type nvdVulnWrapper struct {
	CVE nvdCVE `json:"cve"`
}

type nvdCVE struct {
	ID           string           `json:"id"`
	Descriptions []nvdDescription `json:"descriptions"`
	Metrics      nvdMetrics       `json:"metrics"`
}

type nvdDescription struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type nvdMetrics struct {
	CVSSV31 []nvdCVSSMetric `json:"cvssMetricV31"`
	CVSSV2  []nvdCVSSMetric `json:"cvssMetricV2"`
}

type nvdCVSSMetric struct {
	CVSSData nvdCVSSData `json:"cvssData"`
}

type nvdCVSSData struct {
	BaseScore    float64 `json:"baseScore"`
	BaseSeverity string  `json:"baseSeverity"`
}

func (r nvdResponse) toFindings() []Finding {
	findings := make([]Finding, 0, len(r.Vulnerabilities))
	for _, v := range r.Vulnerabilities {
		cve := v.CVE

		summary := ""
		for _, d := range cve.Descriptions {
			if d.Lang == "en" {
				summary = d.Value
				break
			}
		}

		severity := SeverityMedium
		var score float64
		if len(cve.Metrics.CVSSV31) > 0 {
			score = cve.Metrics.CVSSV31[0].CVSSData.BaseScore
		} else if len(cve.Metrics.CVSSV2) > 0 {
			score = cve.Metrics.CVSSV2[0].CVSSData.BaseScore
		}
		switch {
		case score >= 9.0:
			severity = SeverityCritical
		case score >= 7.0:
			severity = SeverityHigh
		case score >= 4.0:
			severity = SeverityMedium
		case score > 0:
			severity = SeverityLow
		}

		findings = append(findings, Finding{
			ID:      cve.ID,
			Summary: summary,
			Source:  "nvd",
			Severity: severity,
		})
	}
	return findings
}
