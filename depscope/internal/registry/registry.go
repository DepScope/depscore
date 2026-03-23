package registry

import "time"

// PackageInfo holds registry metadata fetched for a dependency.
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

// Fetcher is the common interface for all registry clients.
type Fetcher interface {
	Fetch(name, version string) (*PackageInfo, error)
	Ecosystem() string
}

// Option is a functional option for configuring a registry client.
type Option func(*clientOptions)

type clientOptions struct {
	baseURL string
}

// WithBaseURL overrides the default base URL of a registry client.
func WithBaseURL(url string) Option {
	return func(o *clientOptions) { o.baseURL = url }
}
