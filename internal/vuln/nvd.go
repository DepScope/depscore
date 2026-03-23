package vuln

import "log"

type NVDClient struct{ apiKey string }

func NewNVDClient(apiKey string) *NVDClient { return &NVDClient{apiKey: apiKey} }

func (c *NVDClient) Query(_, _, _ string) ([]Finding, error) {
	if c.apiKey == "" {
		log.Printf("debug: NVD client skipped — no API key configured")
		return nil, nil
	}
	// NVD API implementation would go here
	// Requires: GET https://services.nvd.nist.gov/rest/json/cves/2.0?keywordSearch={name}&apiKey={key}
	// Returning empty for now — full implementation deferred until API key is available
	return nil, nil
}
