// internal/actions/types.go
package actions

import (
	"regexp"
	"strings"
)

// ActionType identifies the runtime of a GitHub Action.
type ActionType int

const (
	ActionComposite ActionType = iota
	ActionNode                 // JavaScript (node12/16/20)
	ActionDocker
	ActionUnknown
)

func (t ActionType) String() string {
	switch t {
	case ActionComposite:
		return "composite"
	case ActionNode:
		return "node"
	case ActionDocker:
		return "docker"
	default:
		return "unknown"
	}
}

// ActionRef is a parsed reference to a GitHub Action from a workflow uses: field.
type ActionRef struct {
	Owner       string // e.g., "actions"
	Repo        string // e.g., "checkout"
	Ref         string // e.g., "v4", "abc123", "main"
	Path        string // e.g., "sub/path" for actions in subdirectories
	DockerImage string // non-empty if uses: docker://...
	LocalPath   string // non-empty if uses: ./local-action
}

// ParseActionRef parses a workflow uses: value into an ActionRef.
func ParseActionRef(uses string) ActionRef {
	uses = strings.TrimSpace(uses)

	// Docker reference: docker://image:tag
	if strings.HasPrefix(uses, "docker://") {
		return ActionRef{DockerImage: strings.TrimPrefix(uses, "docker://")}
	}

	// Local action: ./path
	if strings.HasPrefix(uses, "./") || strings.HasPrefix(uses, "../") {
		return ActionRef{LocalPath: uses}
	}

	// GitHub action: owner/repo(/path)@ref
	ref := ActionRef{}
	if atIdx := strings.LastIndex(uses, "@"); atIdx >= 0 {
		ref.Ref = uses[atIdx+1:]
		uses = uses[:atIdx]
	}

	parts := strings.SplitN(uses, "/", 3)
	if len(parts) >= 2 {
		ref.Owner = parts[0]
		ref.Repo = parts[1]
	}
	if len(parts) >= 3 {
		ref.Path = parts[2]
	}

	return ref
}

// IsFirstParty returns true if the action is from GitHub's official orgs.
func (r ActionRef) IsFirstParty() bool {
	return r.Owner == "actions" || r.Owner == "github"
}

// IsDocker returns true if this is a docker:// reference.
func (r ActionRef) IsDocker() bool { return r.DockerImage != "" }

// IsLocal returns true if this is a local action (./path).
func (r ActionRef) IsLocal() bool { return r.LocalPath != "" }

// IsReusableWorkflow returns true if the ref points to a .yml/.yaml file.
func (r ActionRef) IsReusableWorkflow() bool {
	return strings.HasSuffix(r.Path, ".yml") || strings.HasSuffix(r.Path, ".yaml")
}

// FullName returns owner/repo or owner/repo/path.
func (r ActionRef) FullName() string {
	if r.Path != "" {
		return r.Owner + "/" + r.Repo + "/" + r.Path
	}
	return r.Owner + "/" + r.Repo
}

// PinQuality describes how securely an action reference is pinned.
type PinQuality int

const (
	PinSHA          PinQuality = iota // 40-char hex hash
	PinExactVersion                   // vX.Y.Z
	PinMajorTag                       // vX
	PinBranch                         // main, master, etc.
	PinUnpinned                       // no ref at all
)

func (p PinQuality) String() string {
	switch p {
	case PinSHA:
		return "sha"
	case PinExactVersion:
		return "exact_version"
	case PinMajorTag:
		return "major_tag"
	case PinBranch:
		return "branch"
	case PinUnpinned:
		return "unpinned"
	default:
		return "unknown"
	}
}

var shaRegex = regexp.MustCompile(`^[0-9a-f]{40,}$`)
var exactVersionRegex = regexp.MustCompile(`^v?\d+\.\d+\.\d+`)
var majorTagRegex = regexp.MustCompile(`^v?\d+$`)

// ClassifyPinning determines the pinning quality of a ref string.
func ClassifyPinning(ref string) PinQuality {
	if ref == "" {
		return PinUnpinned
	}
	if shaRegex.MatchString(ref) {
		return PinSHA
	}
	if exactVersionRegex.MatchString(ref) {
		return PinExactVersion
	}
	if majorTagRegex.MatchString(ref) {
		return PinMajorTag
	}
	return PinBranch
}

// WorkflowFile represents a parsed GitHub Actions workflow.
type WorkflowFile struct {
	Path        string      // e.g., ".github/workflows/ci.yml"
	Actions     []ActionRef // all uses: references
	RunBlocks   []RunBlock  // all run: blocks (for script detection)
	Permissions Permissions // workflow-level permissions
}

// RunBlock is a run: step from a workflow.
type RunBlock struct {
	Content string // the shell script content
	Line    int    // line number in the workflow file (for SARIF)
}

// Permissions from the workflow's permissions: block.
type Permissions struct {
	Defined bool              // true if permissions: block exists
	Scopes  map[string]string // e.g., {"contents": "write", "id-token": "write"}
}
