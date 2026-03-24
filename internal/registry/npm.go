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
func (c *NPMClient) Fetch(name, version string) (*PackageInfo, error) {
	url := fmt.Sprintf("%s/%s/%s", c.baseURL, name, version)
	resp, err := c.httpClient.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("npm: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm: GET %s returned %d", url, resp.StatusCode)
	}

	var raw npmResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("npm: decode %s: %w", name, err)
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

	// LastReleaseAt: look up publish time for this version.
	if ts, ok := r.Time[requestedVersion]; ok && ts != "" {
		t, err := time.Parse(time.RFC3339Nano, ts)
		if err == nil {
			info.LastReleaseAt = t
		}
	}

	return info
}
