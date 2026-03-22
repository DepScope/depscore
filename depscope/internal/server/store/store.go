package store

import (
	"time"

	"github.com/depscope/depscope/internal/core"
)

// ScanRequest holds the input parameters for a scan job.
type ScanRequest struct {
	URL     string
	Profile string
}

// ScanJob represents a scan job with its current status and result.
type ScanJob struct {
	ID        string
	URL       string
	Profile   string
	Status    string // "queued", "running", "complete", "failed"
	Error     string
	Result    *core.ScanResult
	CreatedAt time.Time
}

// ScanStore is the persistence interface for scan jobs.
type ScanStore interface {
	Create(id string, req ScanRequest) error
	UpdateStatus(id string, status string) error
	SaveResult(id string, result *core.ScanResult) error
	SaveError(id string, errMsg string) error
	Get(id string) (*ScanJob, error)
}
