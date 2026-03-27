package crawler

import (
	"github.com/depscope/depscope/internal/graph"
)

// FileTree represents a set of files, keyed by relative path.
type FileTree map[string][]byte

// DepSourceType identifies the kind of dependency source.
type DepSourceType int

const (
	DepSourcePackage   DepSourceType = iota // package manifest
	DepSourceAction                         // GitHub Action
	DepSourcePrecommit                      // pre-commit hook
	DepSourceTerraform                      // Terraform module
	DepSourceSubmodule                      // git submodule
	DepSourceTool                           // dev tool (.tool-versions, .mise.toml)
	DepSourceScript                         // curl|sh, wget patterns
	DepSourceBuildTool                      // Makefile, Taskfile, justfile
)

func (d DepSourceType) String() string {
	switch d {
	case DepSourcePackage:
		return "package"
	case DepSourceAction:
		return "action"
	case DepSourcePrecommit:
		return "precommit"
	case DepSourceTerraform:
		return "terraform"
	case DepSourceSubmodule:
		return "submodule"
	case DepSourceTool:
		return "tool"
	case DepSourceScript:
		return "script"
	case DepSourceBuildTool:
		return "buildtool"
	default:
		return "unknown"
	}
}

// DepRef is a dependency reference found by a resolver's Detect method.
type DepRef struct {
	Source    DepSourceType
	Name      string
	Ref       string
	Ecosystem string
	Pinning   graph.PinningQuality
}

// ProjectMeta holds metadata about a project for caching.
type ProjectMeta struct {
	SourceURL       string
	MaintainerCount int
	MaintainerNames []string
	Stars           int
	OrgName         string
}

// ResolvedDep is the result of resolving a DepRef.
type ResolvedDep struct {
	ProjectID  string
	VersionKey string
	Semver     string      // for CVE lookups, nullable
	Contents   FileTree    // for recursive scanning (nil for leaf nodes)
	Metadata   ProjectMeta
}

// CrawlResult is the output of a crawl.
type CrawlResult struct {
	Graph  *graph.Graph
	Stats  CrawlStats
	Errors []CrawlError
}

// CrawlStats tracks crawl metrics.
type CrawlStats struct {
	TotalNodes  int
	TotalEdges  int
	MaxDepthHit int
	CacheHits   int
	CacheMisses int
	ByType      map[graph.NodeType]int
}

// CrawlError records a partial failure during crawling.
type CrawlError struct {
	DepRef   DepRef
	Depth    int
	Err      error
	Resolver DepSourceType
}

func (e CrawlError) Error() string {
	return e.Err.Error()
}

// queueItem is an internal BFS queue entry.
//
//nolint:unused
type queueItem struct {
	// Exactly one of these is set:
	Contents   FileTree      // fresh scan needed
	CachedDeps []CachedChild // children already known from cache

	Depth    int
	ParentVK string // for edge construction
}

// CachedChild represents a known child dependency from the cache.
type CachedChild struct {
	ProjectID  string
	VersionKey string
	EdgeType   string
}
