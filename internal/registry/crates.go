package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const cratesDefaultBaseURL = "https://crates.io"

// CratesClient fetches package metadata from the crates.io API.
type CratesClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewCratesClient constructs a new crates.io registry client.
func NewCratesClient(opts ...Option) *CratesClient {
	o := &clientOptions{baseURL: cratesDefaultBaseURL}
	for _, opt := range opts {
		opt(o)
	}
	return &CratesClient{
		baseURL:    o.baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Ecosystem implements Fetcher.
func (c *CratesClient) Ecosystem() string { return "crates.io" }

// Fetch retrieves package info for the given crate name. Version is used to
// look up LastReleaseAt from the versions list.
func (c *CratesClient) Fetch(name, version string) (*PackageInfo, error) {
	url := fmt.Sprintf("%s/api/v1/crates/%s", c.baseURL, name)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("crates: build request: %w", err)
	}
	// crates.io requires a User-Agent header.
	req.Header.Set("User-Agent", "depscope/1 (https://github.com/depscope/depscope)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crates: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crates: GET %s returned %d", url, resp.StatusCode)
	}

	var raw cratesResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("crates: decode %s: %w", name, err)
	}

	return raw.toPackageInfo(version), nil
}

// ---- raw JSON shapes -------------------------------------------------------

type cratesResponse struct {
	Crate    cratesMeta      `json:"crate"`
	Versions []cratesVersion `json:"versions"`
}

type cratesMeta struct {
	Name              string `json:"name"`
	Downloads         int64  `json:"downloads"`
	Repository        string `json:"repository"`
	NewestVersion     string `json:"newest_version"`
	MaxStableVersion  string `json:"max_stable_version"`
}

type cratesVersion struct {
	Num        string `json:"num"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	Downloads  int64  `json:"downloads"`
	Yanked     bool   `json:"yanked"`
}

func (r cratesResponse) toPackageInfo(requestedVersion string) *PackageInfo {
	version := requestedVersion
	if version == "" {
		version = r.Crate.MaxStableVersion
		if version == "" {
			version = r.Crate.NewestVersion
		}
	}
	info := &PackageInfo{
		Name:           r.Crate.Name,
		Version:        version,
		Ecosystem:      "crates.io",
		TotalDownloads: r.Crate.Downloads,
		SourceRepoURL:  r.Crate.Repository,
		ReleaseCount:   len(r.Versions),
	}

	// Estimate monthly downloads from total (rough: total / age in months)
	if len(r.Versions) > 0 {
		// Find oldest and newest version to calculate age
		var oldest, newest time.Time
		for _, v := range r.Versions {
			t, err := time.Parse(time.RFC3339, v.CreatedAt)
			if err != nil {
				continue
			}
			if oldest.IsZero() || t.Before(oldest) {
				oldest = t
			}
			if newest.IsZero() || t.After(newest) {
				newest = t
			}
		}
		if !oldest.IsZero() {
			info.FirstReleaseAt = oldest
		}
		if !newest.IsZero() {
			info.LastReleaseAt = newest
		}
		// Estimate monthly from total
		if !oldest.IsZero() {
			months := time.Since(oldest).Hours() / (24 * 30)
			if months > 0 {
				info.MonthlyDownloads = int64(float64(r.Crate.Downloads) / months)
			}
		}
	}

	// Find specific version's release date if requested
	if requestedVersion != "" {
		for _, v := range r.Versions {
			if v.Num == requestedVersion {
				if t, err := time.Parse(time.RFC3339, v.CreatedAt); err == nil {
					info.LastReleaseAt = t
				}
				break
			}
		}
	}

	return info
}
