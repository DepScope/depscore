package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type NPMClient struct{ opts clientOptions }

func NewNPMClient(opts ...Option) *NPMClient {
	return &NPMClient{opts: applyOptions(clientOptions{
		baseURL:    "https://registry.npmjs.org",
		httpClient: http.DefaultClient,
	}, opts)}
}

func (c *NPMClient) Ecosystem() string { return "npm" }

func (c *NPMClient) Fetch(name, version string) (*PackageInfo, error) {
	// Fetch the full package document (not version-specific) so we get the
	// top-level "time", "maintainers", and "repository" fields.
	url := fmt.Sprintf("%s/%s", c.opts.baseURL, name)
	resp, err := c.opts.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Name       string `json:"name"`
		Maintainers []struct {
			Name string `json:"name"`
		} `json:"maintainers"`
		Time       map[string]string `json:"time"`
		Repository struct {
			URL string `json:"url"`
		} `json:"repository"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	info := &PackageInfo{
		Name:            data.Name,
		Version:         version,
		Ecosystem:       "npm",
		MaintainerCount: len(data.Maintainers),
		SourceRepoURL:   data.Repository.URL,
	}
	if info.MaintainerCount == 0 {
		info.MaintainerCount = 1
	}

	if ts, ok := data.Time[version]; ok {
		t, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			// Fall back to basic RFC3339 for timestamps without fractional seconds.
			t, err = time.Parse(time.RFC3339, ts)
		}
		if err == nil {
			info.LastReleaseAt = t
		}
	}

	// Try to get download count from separate endpoint
	dlURL := fmt.Sprintf("%s/downloads/point/last-month/%s", c.opts.baseURL, name)
	if dlResp, err := c.opts.httpClient.Get(dlURL); err == nil {
		defer dlResp.Body.Close()
		var dl struct {
			Downloads int64 `json:"downloads"`
		}
		if json.NewDecoder(dlResp.Body).Decode(&dl) == nil {
			info.MonthlyDownloads = dl.Downloads
		}
	}

	return info, nil
}
