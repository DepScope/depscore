package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const packagistDefaultBaseURL = "https://repo.packagist.org"

// PackagistClient fetches package metadata from the Packagist registry JSON API.
type PackagistClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPackagistClient constructs a new Packagist registry client.
func NewPackagistClient(opts ...Option) *PackagistClient {
	o := &clientOptions{baseURL: packagistDefaultBaseURL}
	for _, opt := range opts {
		opt(o)
	}
	return &PackagistClient{
		baseURL:    o.baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Ecosystem implements Fetcher.
func (c *PackagistClient) Ecosystem() string { return "Packagist" }

// Fetch retrieves package info for the given name/version pair.
// name must be in "vendor/package" format.
func (c *PackagistClient) Fetch(name, version string) (*PackageInfo, error) {
	// Packagist p2 API URL: /p2/{vendor}/{package}.json
	url := fmt.Sprintf("%s/p2/%s.json", c.baseURL, name)
	resp, err := c.httpClient.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("packagist: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("packagist: GET %s returned %d", url, resp.StatusCode)
	}

	var raw packagistResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("packagist: decode %s: %w", name, err)
	}

	return raw.toPackageInfo(name, version), nil
}

// ---- raw JSON shapes -------------------------------------------------------

type packagistResponse struct {
	Packages map[string][]packagistVersion `json:"packages"`
}

type packagistVersion struct {
	Name    string             `json:"name"`
	Version string             `json:"version"`
	Authors []packagistAuthor  `json:"authors"`
	Source  packagistSource    `json:"source"`
	Time    string             `json:"time"`
}

type packagistAuthor struct {
	Name string `json:"name"`
}

type packagistSource struct {
	URL string `json:"url"`
}

func (r packagistResponse) toPackageInfo(name, requestedVersion string) *PackageInfo {
	versions, ok := r.Packages[name]
	if !ok || len(versions) == 0 {
		return &PackageInfo{
			Name:      name,
			Version:   requestedVersion,
			Ecosystem: "Packagist",
		}
	}

	info := &PackageInfo{
		Name:         name,
		Version:      requestedVersion,
		Ecosystem:    "Packagist",
		ReleaseCount: len(versions),
	}

	// Use the first (latest) entry for maintainer count, source URL, and release time.
	latest := versions[0]
	info.MaintainerCount = len(latest.Authors)

	// SourceRepoURL: strip .git suffix.
	repoURL := latest.Source.URL
	repoURL = strings.TrimSuffix(repoURL, ".git")
	info.SourceRepoURL = repoURL

	// LastReleaseAt: use the time from the first entry (latest version).
	if latest.Time != "" {
		t, err := time.Parse(time.RFC3339, latest.Time)
		if err == nil {
			info.LastReleaseAt = t
		}
	}

	return info
}
