package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const pypiDefaultBaseURL = "https://pypi.org"

// PyPIClient fetches package metadata from the PyPI JSON API.
type PyPIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPyPIClient constructs a new PyPI registry client.
func NewPyPIClient(opts ...Option) *PyPIClient {
	o := &clientOptions{baseURL: pypiDefaultBaseURL}
	for _, opt := range opts {
		opt(o)
	}
	return &PyPIClient{
		baseURL:    o.baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Ecosystem implements Fetcher.
func (c *PyPIClient) Ecosystem() string { return "PyPI" }

// Fetch retrieves package info for the given name/version pair.
// version is accepted for API compatibility but PyPI JSON API is name-only.
func (c *PyPIClient) Fetch(name, version string) (*PackageInfo, error) {
	url := fmt.Sprintf("%s/pypi/%s/json", c.baseURL, name)
	resp, err := c.httpClient.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("pypi: GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pypi: GET %s returned %d", url, resp.StatusCode)
	}

	var raw pypiResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("pypi: decode %s: %w", name, err)
	}

	return raw.toPackageInfo(version), nil
}

// ---- raw JSON shapes -------------------------------------------------------

type pypiResponse struct {
	Info     pypiInfo                        `json:"info"`
	Releases map[string][]pypiReleaseFile    `json:"releases"`
}

type pypiInfo struct {
	Name            string            `json:"name"`
	Author          string            `json:"author"`
	AuthorEmail     string            `json:"author_email"`
	Maintainer      string            `json:"maintainer"`
	MaintainerEmail string            `json:"maintainer_email"`
	HomePage        string            `json:"home_page"`
	Classifiers     []string          `json:"classifiers"`
	ProjectURLs     map[string]string `json:"project_urls"`
}

type pypiReleaseFile struct {
	UploadTime string `json:"upload_time"`
}

func (r pypiResponse) toPackageInfo(requestedVersion string) *PackageInfo {
	info := &PackageInfo{
		Name:      r.Info.Name,
		Version:   requestedVersion,
		Ecosystem: "PyPI",
	}

	// MaintainerCount: count unique people from author, author_email, maintainer, maintainer_email.
	people := make(map[string]bool)
	for _, field := range []string{r.Info.Author, r.Info.Maintainer} {
		if field != "" {
			people[strings.ToLower(strings.TrimSpace(field))] = true
		}
	}
	for _, field := range []string{r.Info.AuthorEmail, r.Info.MaintainerEmail} {
		for _, email := range strings.Split(field, ",") {
			email = strings.TrimSpace(email)
			if email != "" {
				people[strings.ToLower(email)] = true
			}
		}
	}
	info.MaintainerCount = len(people)

	// SourceRepoURL: prefer project_urls["Source"], fall back to home_page.
	if src, ok := r.Info.ProjectURLs["Source"]; ok && src != "" {
		info.SourceRepoURL = src
	} else if r.Info.HomePage != "" {
		info.SourceRepoURL = r.Info.HomePage
	}

	// IsDeprecated: check classifiers for "Inactive".
	for _, c := range r.Info.Classifiers {
		if strings.Contains(c, "Inactive") {
			info.IsDeprecated = true
			break
		}
	}

	// Releases: count and find latest upload time.
	info.ReleaseCount = len(r.Releases)
	var latest time.Time
	for _, files := range r.Releases {
		for _, f := range files {
			t, err := time.Parse("2006-01-02T15:04:05", f.UploadTime)
			if err != nil {
				continue
			}
			if t.After(latest) {
				latest = t
			}
		}
	}
	if !latest.IsZero() {
		info.LastReleaseAt = latest
	}

	return info
}
