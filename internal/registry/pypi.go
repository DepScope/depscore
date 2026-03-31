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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pypi: GET %s returned %d", url, resp.StatusCode)
	}

	var raw pypiResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("pypi: decode %s: %w", name, err)
	}

	return raw.toPackageInfo(version), nil
}

// FetchDependencies retrieves the dependency list for a PyPI package.
// Uses the requires_dist field from the JSON API response.
func (c *PyPIClient) FetchDependencies(name, version string) ([]Dependency, error) {
	apiURL := fmt.Sprintf("%s/pypi/%s/%s/json", c.baseURL, name, version)
	resp, err := c.httpClient.Get(apiURL) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("pypi: GET %s: %w", apiURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Fall back to unversioned endpoint
		apiURL = fmt.Sprintf("%s/pypi/%s/json", c.baseURL, name)
		resp, err = c.httpClient.Get(apiURL) //nolint:noctx
		if err != nil {
			return nil, fmt.Errorf("pypi: GET %s: %w", apiURL, err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("pypi: GET %s returned %d", apiURL, resp.StatusCode)
		}
	}

	var raw struct {
		Info struct {
			RequiresDist []string `json:"requires_dist"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("pypi: decode deps for %s: %w", name, err)
	}

	var deps []Dependency
	for _, req := range raw.Info.RequiresDist {
		// Skip extras: entries like "pydantic ; extra == \"extended\""
		if strings.Contains(req, "extra ==") || strings.Contains(req, "extra==") {
			continue
		}
		// Strip environment markers
		if i := strings.Index(req, ";"); i >= 0 {
			req = strings.TrimSpace(req[:i])
		}
		// Parse "litellm (>=1.82.0)" → name="litellm", constraint=">=1.82.0"
		depName, constraint := parsePyPIDep(req)
		if depName == "" {
			continue
		}
		deps = append(deps, Dependency{Name: depName, Constraint: constraint})
	}
	return deps, nil
}

// parsePyPIDep parses a requires_dist entry like "litellm (>=1.82.0)".
func parsePyPIDep(s string) (name, constraint string) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "("); i >= 0 {
		name = strings.TrimSpace(s[:i])
		end := strings.Index(s, ")")
		if end > i {
			constraint = strings.TrimSpace(s[i+1 : end])
		}
		return
	}
	// No parens — check for space-separated constraint
	parts := strings.Fields(s)
	if len(parts) >= 1 {
		name = parts[0]
	}
	if len(parts) >= 2 {
		constraint = strings.Join(parts[1:], "")
	}
	return
}

// extractSourceURL finds a source/repository URL from project_urls using
// case-insensitive matching against common key names. Falls back to home_page
// if it looks like a GitHub/GitLab URL.
func extractSourceURL(projectURLs map[string]string, homePage string) string {
	// Priority order of keys to check (case-insensitive)
	priorities := []string{
		"source", "source code", "repository", "code",
		"github", "gitlab", "homepage", "home",
	}
	for _, key := range priorities {
		for k, v := range projectURLs {
			if strings.EqualFold(k, key) && v != "" {
				if isRepoURL(v) {
					return v
				}
			}
		}
	}
	// Second pass: any project URL that looks like a repo
	for _, v := range projectURLs {
		if isRepoURL(v) {
			return v
		}
	}
	// Fall back to home_page if it's a repo URL
	if homePage != "" && isRepoURL(homePage) {
		return homePage
	}
	return ""
}

func isRepoURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.Contains(lower, "github.com") ||
		strings.Contains(lower, "gitlab.com") ||
		strings.Contains(lower, "bitbucket.org") ||
		strings.Contains(lower, "codeberg.org") ||
		strings.Contains(lower, "sr.ht")
}

// ---- raw JSON shapes -------------------------------------------------------

type pypiResponse struct {
	Info     pypiInfo                        `json:"info"`
	Releases map[string][]pypiReleaseFile    `json:"releases"`
}

type pypiInfo struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
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
	version := requestedVersion
	if version == "" {
		version = r.Info.Version // latest version from PyPI
	}
	info := &PackageInfo{
		Name:      r.Info.Name,
		Version:   version,
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

	// SourceRepoURL: search project_urls with various common key names,
	// fall back to home_page. PyPI has no standard key — projects use
	// "Source", "Source Code", "source", "Homepage", "Repository", etc.
	info.SourceRepoURL = extractSourceURL(r.Info.ProjectURLs, r.Info.HomePage)

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
