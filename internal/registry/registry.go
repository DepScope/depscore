package registry

import (
	"net/http"
	"time"
)

type PackageInfo struct {
	Name             string
	Version          string
	Ecosystem        string
	TotalDownloads   int64
	MonthlyDownloads int64
	DownloadTrend    float64
	LastReleaseAt    time.Time
	FirstReleaseAt   time.Time
	ReleaseCount     int
	MaintainerCount  int
	HasOrgBacking    bool
	SourceRepoURL    string
	IsDeprecated     bool
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
