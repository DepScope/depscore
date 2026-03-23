package vuln

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type osvOption func(*osvOptions)

type osvOptions struct {
	baseURL    string
	httpClient *http.Client
}

func WithOSVBaseURL(url string) osvOption {
	return func(o *osvOptions) { o.baseURL = url }
}

type OSVClient struct{ opts osvOptions }

func NewOSVClient(opts ...osvOption) *OSVClient {
	c := &OSVClient{opts: osvOptions{
		baseURL:    "https://api.osv.dev",
		httpClient: http.DefaultClient,
	}}
	for _, o := range opts {
		o(&c.opts)
	}
	return c
}

func (c *OSVClient) Query(ecosystem, name, version string) ([]Finding, error) {
	body, _ := json.Marshal(map[string]any{
		"version": version,
		"package": map[string]string{
			"name":      name,
			"ecosystem": ecosystem,
		},
	})

	resp, err := c.opts.httpClient.Post(
		c.opts.baseURL+"/v1/query",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Vulns []struct {
			ID      string `json:"id"`
			Summary string `json:"summary"`
			Severity []struct {
				Type  string `json:"type"`
				Score string `json:"score"`
			} `json:"severity"`
			Affected []struct {
				Ranges []struct {
					Events []struct {
						Fixed string `json:"fixed"`
					} `json:"events"`
				} `json:"ranges"`
			} `json:"affected"`
		} `json:"vulns"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("osv decode: %w", err)
	}

	var findings []Finding
	for _, v := range result.Vulns {
		sev := SeverityMedium
		for _, s := range v.Severity {
			switch s.Type {
			case "CVSS_V3":
				sev = cvssToSeverity(s.Score)
			}
		}
		var fixedIn []string
		for _, aff := range v.Affected {
			for _, r := range aff.Ranges {
				for _, e := range r.Events {
					if e.Fixed != "" {
						fixedIn = append(fixedIn, e.Fixed)
					}
				}
			}
		}
		findings = append(findings, Finding{
			ID:       v.ID,
			Summary:  v.Summary,
			Severity: sev,
			FixedIn:  fixedIn,
			Source:   "osv",
		})
	}
	return findings, nil
}

func cvssToSeverity(score string) Severity {
	// CVSS score is a float like "7.5"
	var f float64
	fmt.Sscanf(score, "%f", &f)
	switch {
	case f >= 9.0:
		return SeverityCritical
	case f >= 7.0:
		return SeverityHigh
	case f >= 4.0:
		return SeverityMedium
	default:
		return SeverityLow
	}
}
