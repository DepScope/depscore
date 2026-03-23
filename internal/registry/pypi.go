package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type PyPIClient struct {
	opts clientOptions
}

func NewPyPIClient(opts ...Option) *PyPIClient {
	c := &PyPIClient{
		opts: applyOptions(clientOptions{
			baseURL:    "https://pypi.org",
			httpClient: http.DefaultClient,
		}, opts),
	}
	if c.opts.httpClient == nil {
		c.opts.httpClient = http.DefaultClient
	}
	return c
}

func (c *PyPIClient) Ecosystem() string { return "python" }

func (c *PyPIClient) Fetch(name, version string) (*PackageInfo, error) {
	url := fmt.Sprintf("%s/pypi/%s/json", c.opts.baseURL, name)
	resp, err := c.opts.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Info struct {
			Name            string            `json:"name"`
			Maintainer      string            `json:"maintainer"`
			MaintainerEmail string            `json:"maintainer_email"`
			Author          string            `json:"author"`
			AuthorEmail     string            `json:"author_email"`
			HomePage        string            `json:"home_page"`
			ProjectURLs     map[string]string `json:"project_urls"`
			Classifiers     []string          `json:"classifiers"`
		} `json:"info"`
		Releases map[string][]struct {
			UploadTime string `json:"upload_time"`
		} `json:"releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	info := &PackageInfo{
		Name:      data.Info.Name,
		Version:   version,
		Ecosystem: "python",
	}

	// Count maintainers from all available fields
	maintainers := make(map[string]bool)
	for _, field := range []string{data.Info.Maintainer, data.Info.MaintainerEmail, data.Info.Author, data.Info.AuthorEmail} {
		for _, m := range strings.Split(field, ",") {
			if s := strings.TrimSpace(m); s != "" {
				maintainers[s] = true
			}
		}
	}
	info.MaintainerCount = len(maintainers)
	if info.MaintainerCount == 0 {
		info.MaintainerCount = 1 // assume at least 1
	}

	// Source repo URL — PyPI uses various key names
	for _, key := range []string{"Source", "Source Code", "Repository", "Homepage", "Home"} {
		if src, ok := data.Info.ProjectURLs[key]; ok && src != "" {
			info.SourceRepoURL = src
			break
		}
	}
	if info.SourceRepoURL == "" {
		info.SourceRepoURL = data.Info.HomePage
	}

	// Last release time
	if releases, ok := data.Releases[version]; ok && len(releases) > 0 {
		t, err := time.Parse("2006-01-02T15:04:05", releases[0].UploadTime)
		if err == nil {
			info.LastReleaseAt = t
		}
	}
	// Fallback: use any release
	if info.LastReleaseAt.IsZero() {
		for _, releases := range data.Releases {
			if len(releases) > 0 {
				t, err := time.Parse("2006-01-02T15:04:05", releases[0].UploadTime)
				if err == nil && t.After(info.LastReleaseAt) {
					info.LastReleaseAt = t
				}
			}
		}
	}

	// Deprecated check
	for _, c := range data.Info.Classifiers {
		if strings.Contains(c, "Development Status :: 7 - Inactive") {
			info.IsDeprecated = true
			break
		}
	}

	return info, nil
}
