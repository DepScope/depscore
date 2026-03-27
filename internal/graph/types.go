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
	NodePrecommitHook                  // .pre-commit-config.yaml hook
	NodeTerraformModule                // Terraform/OpenTofu module
	NodeGitSubmodule                   // .gitmodules entry
	NodeDevTool                        // .tool-versions, .mise.toml entry
	NodeBuildTool                      // Makefile/Taskfile that installs things
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
	case NodePrecommitHook:
		return "precommit_hook"
	case NodeTerraformModule:
		return "terraform_module"
	case NodeGitSubmodule:
		return "git_submodule"
	case NodeDevTool:
		return "dev_tool"
	case NodeBuildTool:
		return "build_tool"
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
	EdgeUsesHook                   // workflow/repo → precommit_hook
	EdgeUsesModule                 // config → terraform_module
	EdgeIncludesSubmodule          // repo → git_submodule
	EdgeUsesTool                   // config → dev_tool
	EdgeBuiltWith                  // repo → build_tool
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
	case EdgeUsesHook:
		return "uses_hook"
	case EdgeUsesModule:
		return "uses_module"
	case EdgeIncludesSubmodule:
		return "includes_submodule"
	case EdgeUsesTool:
		return "uses_tool"
	case EdgeBuiltWith:
		return "built_with"
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
	PinningSemverRange                        // semver range constraint (e.g., ^1.2.0, ~2.3)
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
	case PinningSemverRange:
		return "semver_range"
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
	ID         string         // e.g., "package:python/litellm@1.82.8"
	Type       NodeType
	Name       string
	Version    string         // resolved version or SHA
	Ref        string         // original reference (tag, branch, constraint)
	Score      int            // 0-100 reputation score
	Risk       core.RiskLevel
	Pinning    PinningQuality
	Metadata   map[string]any // ecosystem-specific data
	ProjectID  string         // FK → cache projects.id
	VersionKey string         // FK → cache project_versions.version_key
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
