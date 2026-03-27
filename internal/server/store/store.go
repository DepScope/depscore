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
	// List returns all stored scan jobs in no guaranteed order.
	List() []*ScanJob
}

// GraphStore extends ScanStore with graph persistence.
type GraphStore interface {
	ScanStore
	SaveGraph(scanID string, nodes []GraphNode, edges []GraphEdge) error
	LoadGraph(scanID string) ([]GraphNode, []GraphEdge, error)
}

// GraphNode is the storage representation of a graph node.
type GraphNode struct {
	NodeID     string
	Type       string
	Name       string
	Version    string
	Ref        string
	Score      int
	Risk       string
	Pinning    string
	Metadata   map[string]any
	ProjectID  string
	VersionKey string
}

// GraphEdge is the storage representation of a graph edge.
type GraphEdge struct {
	From  string
	To    string
	Type  string
	Depth int
}
