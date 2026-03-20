package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type CratesClient struct{ opts clientOptions }

func NewCratesClient(opts ...Option) *CratesClient {
	return &CratesClient{opts: applyOptions(clientOptions{
		baseURL:    "https://crates.io",
		httpClient: http.DefaultClient,
	}, opts)}
}

func (c *CratesClient) Ecosystem() string { return "rust" }

func (c *CratesClient) Fetch(name, version string) (*PackageInfo, error) {
	url := fmt.Sprintf("%s/api/v1/crates/%s", c.opts.baseURL, name)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "depscope/1.0 (https://depscope.com)")
	resp, err := c.opts.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Crate struct {
			Name       string `json:"name"`
			Downloads  int64  `json:"downloads"`
			Repository string `json:"repository"`
		} `json:"crate"`
		Versions []struct {
			Num       string `json:"num"`
			UpdatedAt string `json:"updated_at"`
		} `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	info := &PackageInfo{
		Name:           data.Crate.Name,
		Version:        version,
		Ecosystem:      "rust",
		TotalDownloads: data.Crate.Downloads,
		SourceRepoURL:  data.Crate.Repository,
	}

	// Find release time for this version
	for _, v := range data.Versions {
		if v.Num == version {
			t, err := time.Parse(time.RFC3339, v.UpdatedAt)
			if err == nil {
				info.LastReleaseAt = t
			}
			break
		}
	}
	if info.LastReleaseAt.IsZero() && len(data.Versions) > 0 {
		t, _ := time.Parse(time.RFC3339, data.Versions[0].UpdatedAt)
		info.LastReleaseAt = t
	}

	return info, nil
}
