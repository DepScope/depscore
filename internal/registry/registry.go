package registry

import (
	"net/http"
	"time"
)

type PackageInfo struct {
	Name             string    `json:"name"`
	Version          string    `json:"version"`
	Ecosystem        string    `json:"ecosystem"`
	TotalDownloads   int64     `json:"total_downloads"`
	MonthlyDownloads int64     `json:"monthly_downloads"`
	DownloadTrend    float64   `json:"download_trend"`
	LastReleaseAt    time.Time `json:"last_release_at"`
	FirstReleaseAt   time.Time `json:"first_release_at"`
	ReleaseCount     int       `json:"release_count"`
	MaintainerCount  int       `json:"maintainer_count"`
	HasOrgBacking    bool      `json:"has_org_backing"`
	SourceRepoURL    string    `json:"source_repo_url"`
	IsDeprecated     bool      `json:"is_deprecated"`
}

type Fetcher interface {
	Fetch(name, version string) (*PackageInfo, error)
	Ecosystem() string
}

type Option func(*clientOptions)

type clientOptions struct {
	baseURL    string
	httpClient *http.Client
}

func WithBaseURL(url string) Option {
	return func(o *clientOptions) { o.baseURL = url }
}

func applyOptions(defaults clientOptions, opts []Option) clientOptions {
	for _, o := range opts {
		o(&defaults)
	}
	return defaults
}
