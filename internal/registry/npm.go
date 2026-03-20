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
	url := fmt.Sprintf("%s/%s/%s", c.opts.baseURL, name, version)
	resp, err := c.opts.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Name        string            `json:"name"`
		Maintainers []struct{}        `json:"maintainers"`
		Time        map[string]string `json:"time"`
		Repository  struct {
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
		t, err := time.Parse(time.RFC3339, ts)
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
