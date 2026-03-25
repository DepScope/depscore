// internal/graph/types.go
package graph

import "github.com/depscope/depscope/internal/core"

// NodeType identifies what kind of supply chain entity a node represents.
type NodeType int

const (
	NodePackage        NodeType = iota // versioned software dependency
	NodeRepo                           // source code repository
	NodeAction                         // CI/CD action reference
	NodeWorkflow                       // workflow file
	NodeDockerImage                    // container base image
	NodeScriptDownload                 // curl/wget binary in CI steps
	// Future: NodeHook, NodeTerraformModule, NodeGitSubmodule, NodeBuildTool, NodeOSPackage, NodeVendoredCode
)

func (t NodeType) String() string {
	switch t {
	case NodePackage:
		return "package"
	case NodeRepo:
		return "repo"
	case NodeAction:
		return "action"
	case NodeWorkflow:
		return "workflow"
	case NodeDockerImage:
		return "docker_image"
	case NodeScriptDownload:
		return "script_download"
	default:
		return "unknown"
	}
}

// EdgeType identifies the relationship between two nodes.
type EdgeType int

const (
	EdgeDependsOn  EdgeType = iota // package → package
	EdgeHostedAt                   // package → repo
	EdgeUsesAction                 // workflow → action
	EdgeBundles                    // action → package
	EdgeTriggers                   // workflow → workflow
	EdgeResolvesTo                 // action → repo (tag→SHA)
	EdgePullsImage                 // workflow/action → docker_image
	EdgeDownloads                  // workflow → script_download
	// Future: EdgeUsesHook, EdgeUsesModule, EdgeIncludesSubmodule, EdgeBuiltWith, EdgeInstallsOSPkg, EdgeVendors, EdgeAttests
)

func (t EdgeType) String() string {
	switch t {
	case EdgeDependsOn:
		return "depends_on"
	case EdgeHostedAt:
		return "hosted_at"
	case EdgeUsesAction:
		return "uses_action"
	case EdgeBundles:
		return "bundles"
	case EdgeTriggers:
		return "triggers"
	case EdgeResolvesTo:
		return "resolves_to"
	case EdgePullsImage:
		return "pulls_image"
	case EdgeDownloads:
		return "downloads"
	default:
		return "unknown"
	}
}

// PinningQuality describes how securely a dependency reference is pinned.
type PinningQuality int

const (
	PinningSHA          PinningQuality = iota // immutable hash
	PinningDigest                             // Docker image digest
	PinningExactVersion                       // exact version tag (e.g., v4.2.0)
	PinningMajorTag                           // major version tag (e.g., v4)
	PinningBranch                             // branch name (e.g., main)
	PinningUnpinned                           // no version reference
	PinningNA                                 // not applicable (packages use constraints)
)

func (p PinningQuality) String() string {
	switch p {
	case PinningSHA:
		return "sha"
	case PinningDigest:
		return "digest"
	case PinningExactVersion:
		return "exact_version"
	case PinningMajorTag:
		return "major_tag"
	case PinningBranch:
		return "branch"
	case PinningUnpinned:
		return "unpinned"
	case PinningNA:
		return "n/a"
	default:
		return "unknown"
	}
}

// Node represents a single entity in the supply chain graph.
type Node struct {
	ID       string         // e.g., "package:python/litellm@1.82.8"
	Type     NodeType
	Name     string
	Version  string         // resolved version or SHA
	Ref      string         // original reference (tag, branch, constraint)
	Score    int            // 0-100 reputation score
	Risk     core.RiskLevel
	Pinning  PinningQuality
	Metadata map[string]any // ecosystem-specific data
}

// Edge represents a relationship between two nodes.
type Edge struct {
	From  string // NodeID
	To    string // NodeID
	Type  EdgeType
	Depth int // distance from root
}

// NodeID constructs a canonical node identifier.
func NodeID(nodeType NodeType, key string) string {
	return nodeType.String() + ":" + key
}
