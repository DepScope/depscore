package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const goProxyDefaultBaseURL = "https://proxy.golang.org"

// GoProxyClient fetches module metadata from the Go module proxy.
type GoProxyClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewGoProxyClient constructs a new Go proxy registry client.
func NewGoProxyClient(opts ...Option) *GoProxyClient {
	o := &clientOptions{baseURL: goProxyDefaultBaseURL}
	for _, opt := range opts {
		opt(o)
	}
	return &GoProxyClient{
		baseURL:    o.baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Ecosystem implements Fetcher.
func (c *GoProxyClient) Ecosystem() string { return "Go" }

// Fetch retrieves module info from the Go proxy.
// name is the module path (e.g. "github.com/gin-gonic/gin"), version must be
// a semver string prefixed with "v" (e.g. "v1.8.0").
func (c *GoProxyClient) Fetch(name, version string) (*PackageInfo, error) {
	// If no version specified, fetch @latest
	if version == "" {
		version = "latest"
	}

	var apiURL string
	if version == "latest" {
		apiURL = fmt.Sprintf("%s/%s/@latest", c.baseURL, name)
	} else {
		apiURL = fmt.Sprintf("%s/%s/@v/%s.info", c.baseURL, name, version)
	}
	resp, err := c.httpClient.Get(apiURL) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("goproxy: GET %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("goproxy: GET %s returned %d", apiURL, resp.StatusCode)
	}

	var raw goProxyInfo
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("goproxy: decode %s: %w", name, err)
	}

	return raw.toPackageInfo(name), nil
}

// ---- raw JSON shapes -------------------------------------------------------

type goProxyInfo struct {
	Version string `json:"Version"`
	Time    string `json:"Time"`
}

func (r goProxyInfo) toPackageInfo(name string) *PackageInfo {
	info := &PackageInfo{
		Name:      name,
		Version:   r.Version,
		Ecosystem: "Go",
	}

	if t, err := time.Parse(time.RFC3339, r.Time); err == nil {
		info.LastReleaseAt = t
	}

	// Go module path often IS the repo URL (e.g. github.com/spf13/cobra)
	if strings.HasPrefix(name, "github.com/") {
		parts := strings.SplitN(name, "/", 4) // github.com/owner/repo[/subpackage]
		if len(parts) >= 3 {
			info.SourceRepoURL = "https://" + parts[0] + "/" + parts[1] + "/" + parts[2]
		}
	} else if strings.HasPrefix(name, "gitlab.com/") {
		parts := strings.SplitN(name, "/", 4)
		if len(parts) >= 3 {
			info.SourceRepoURL = "https://" + parts[0] + "/" + parts[1] + "/" + parts[2]
		}
	}

	return info
}
