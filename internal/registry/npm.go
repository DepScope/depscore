package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const npmDefaultBaseURL = "https://registry.npmjs.org"

// NPMClient fetches package metadata from the npm registry JSON API.
type NPMClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewNPMClient constructs a new npm registry client.
func NewNPMClient(opts ...Option) *NPMClient {
	o := &clientOptions{baseURL: npmDefaultBaseURL}
	for _, opt := range opts {
		opt(o)
	}
	return &NPMClient{
		baseURL:    o.baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Ecosystem implements Fetcher.
func (c *NPMClient) Ecosystem() string { return "npm" }

// Fetch retrieves package info for the given name/version pair.
// When version is empty, fetches the full package doc and uses the latest version.
func (c *NPMClient) Fetch(name, version string) (*PackageInfo, error) {
	var apiURL string
	if version != "" {
		apiURL = fmt.Sprintf("%s/%s/%s", c.baseURL, name, version)
	} else {
		apiURL = fmt.Sprintf("%s/%s", c.baseURL, name)
	}
	resp, err := c.httpClient.Get(apiURL) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("npm: GET %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm: GET %s returned %d", apiURL, resp.StatusCode)
	}

	var raw npmResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("npm: decode %s: %w", name, err)
	}

	// If no version specified, use dist-tags.latest or the version from the response
	if version == "" && raw.DistTags.Latest != "" {
		version = raw.DistTags.Latest
	}

	return raw.toPackageInfo(version), nil
}

// ---- raw JSON shapes -------------------------------------------------------

type npmResponse struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Maintainers []npmMaintainer   `json:"maintainers"`
	Repository  npmRepository     `json:"repository"`
	Time        map[string]string `json:"time"`
	DistTags    npmDistTags       `json:"dist-tags"`
}

type npmDistTags struct {
	Latest string `json:"latest"`
}

type npmMaintainer struct {
	Name string `json:"name"`
}

type npmRepository struct {
	URL string `json:"url"`
}

func (r npmResponse) toPackageInfo(requestedVersion string) *PackageInfo {
	info := &PackageInfo{
		Name:            r.Name,
		Version:         requestedVersion,
		Ecosystem:       "npm",
		MaintainerCount: len(r.Maintainers),
	}

	// SourceRepoURL: strip git+ prefix and .git suffix.
	repoURL := r.Repository.URL
	repoURL = strings.TrimPrefix(repoURL, "git+")
	repoURL = strings.TrimSuffix(repoURL, ".git")
	info.SourceRepoURL = repoURL

	// LastReleaseAt: try version-specific time, then "modified"
	var latest time.Time
	if ts, ok := r.Time[requestedVersion]; ok && ts != "" {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			latest = t
		}
	}
	if latest.IsZero() {
		if ts, ok := r.Time["modified"]; ok && ts != "" {
			if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				latest = t
			}
		}
	}
	if !latest.IsZero() {
		info.LastReleaseAt = latest
	}

	// ReleaseCount: count version entries in time map (exclude "created"/"modified")
	for k := range r.Time {
		if k != "created" && k != "modified" {
			info.ReleaseCount++
		}
	}

	return info
}
