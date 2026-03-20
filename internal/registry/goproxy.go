package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type GoProxyClient struct{ opts clientOptions }

func NewGoProxyClient(opts ...Option) *GoProxyClient {
	return &GoProxyClient{opts: applyOptions(clientOptions{
		baseURL:    "https://proxy.golang.org",
		httpClient: http.DefaultClient,
	}, opts)}
}

func (c *GoProxyClient) Ecosystem() string { return "go" }

func (c *GoProxyClient) Fetch(name, version string) (*PackageInfo, error) {
	// URL-encode the module path (lowercase for Go proxy)
	escaped := url.PathEscape(strings.ToLower(name))
	apiURL := fmt.Sprintf("%s/%s/@v/%s.info", c.opts.baseURL, escaped, version)
	resp, err := c.opts.httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Version string `json:"Version"`
		Time    string `json:"Time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	info := &PackageInfo{
		Name:      name,
		Version:   version,
		Ecosystem: "go",
		// MonthlyDownloads not available from Go proxy
	}
	t, err := time.Parse(time.RFC3339, data.Time)
	if err == nil {
		info.LastReleaseAt = t
	}

	return info, nil
}
