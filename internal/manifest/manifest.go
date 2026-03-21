package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Ecosystem string

const (
	EcosystemPython Ecosystem = "python"
	EcosystemGo     Ecosystem = "go"
	EcosystemRust   Ecosystem = "rust"
	EcosystemNPM    Ecosystem = "npm"
)

// String returns the OSV-compatible ecosystem name.
func (e Ecosystem) String() string {
	switch e {
	case EcosystemPython:
		return "PyPI"
	case EcosystemGo:
		return "Go"
	case EcosystemRust:
		return "crates.io"
	case EcosystemNPM:
		return "npm"
	default:
		return string(e)
	}
}

type ConstraintType string

const (
	ConstraintExact ConstraintType = "exact"
	ConstraintPatch ConstraintType = "patch"
	ConstraintMinor ConstraintType = "minor"
	ConstraintMajor ConstraintType = "major"
)

// Package is one dependency entry, merged from manifest (constraints) and lockfile (resolved version).
type Package struct {
	Name            string
	ResolvedVersion string
	Constraint      string
	ConstraintType  ConstraintType
	Ecosystem       Ecosystem
	Depth           int      // 1 = direct dep, 2+ = transitive
	Parents         []string // names of packages that directly depend on this one
}

// Key returns a unique string identifier for this package within a scan.
func (p Package) Key() string {
	return string(p.Ecosystem) + "/" + p.Name + "@" + p.ResolvedVersion
}

// Parser reads a project directory and returns all packages (direct + transitive).
type Parser interface {
	Parse(dir string) ([]Package, error)
	Ecosystem() Ecosystem
}

var ecosystemFiles = []struct {
	file      string
	ecosystem Ecosystem
}{
	{"go.mod", EcosystemGo},
	{"Cargo.toml", EcosystemRust},
	{"package.json", EcosystemNPM},
	{"uv.lock", EcosystemPython},
	{"poetry.lock", EcosystemPython},
	{"requirements.txt", EcosystemPython},
}

// DetectEcosystem scans dir for known manifest files and returns the ecosystem.
func DetectEcosystem(dir string) (Ecosystem, error) {
	for _, ef := range ecosystemFiles {
		if _, err := os.Stat(filepath.Join(dir, ef.file)); err == nil {
			return ef.ecosystem, nil
		}
	}
	return "", fmt.Errorf("no recognized manifest found in %s", dir)
}

// ParserFor returns the concrete parser for the given ecosystem.
func ParserFor(eco Ecosystem) Parser {
	switch eco {
	case EcosystemPython:
		return NewPythonParser()
	case EcosystemGo:
		return NewGoModParser()
	case EcosystemRust:
		return NewRustParser()
	case EcosystemNPM:
		return NewJavaScriptParser()
	default:
		panic("unknown ecosystem: " + string(eco))
	}
}

// BuildDepsMap builds a map of package name → list of direct dependency names.
// Uses Package.Parents when available. Falls back to a flat two-level structure
// (all depth-1 packages depend on all depth-2 packages) when Parents is empty.
func BuildDepsMap(pkgs []Package) map[string][]string {
	deps := make(map[string][]string)

	hasParentInfo := false
	for _, p := range pkgs {
		if len(p.Parents) > 0 {
			hasParentInfo = true
			break
		}
	}

	if hasParentInfo {
		for _, p := range pkgs {
			for _, parent := range p.Parents {
				deps[parent] = append(deps[parent], p.Name)
			}
		}
		return deps
	}

	// Fallback: flat two-level (all direct deps depend on all indirect deps)
	var direct, indirect []string
	for _, p := range pkgs {
		if p.Depth <= 1 {
			direct = append(direct, p.Name)
		} else {
			indirect = append(indirect, p.Name)
		}
	}
	for _, d := range direct {
		deps[d] = append(deps[d], indirect...)
	}
	return deps
}

// ComputeDepths assigns correct depth values based on the dependency graph.
// Direct deps (no parents in the graph) get depth=1, their deps get depth=2, etc.
func ComputeDepths(pkgs []Package, deps map[string][]string) []Package {
	// Find root packages (depth=1): those that appear as direct deps (not as children of other packages)
	isChild := make(map[string]bool)
	for _, children := range deps {
		for _, c := range children {
			isChild[c] = true
		}
	}

	// BFS from roots to assign depths
	depth := make(map[string]int)
	queue := []string{}
	for i := range pkgs {
		if !isChild[pkgs[i].Name] {
			depth[pkgs[i].Name] = 1
			queue = append(queue, pkgs[i].Name)
		}
	}

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for _, child := range deps[name] {
			if _, ok := depth[child]; !ok {
				depth[child] = depth[name] + 1
				queue = append(queue, child)
			}
		}
	}

	// Apply computed depths
	for i := range pkgs {
		if d, ok := depth[pkgs[i].Name]; ok {
			pkgs[i].Depth = d
		}
	}
	return pkgs
}

// ParseConstraintType classifies a raw version constraint string.
func ParseConstraintType(constraint string) ConstraintType {
	c := strings.TrimSpace(constraint)
	switch {
	case strings.HasPrefix(c, "==") || (strings.HasPrefix(c, "=") && !strings.HasPrefix(c, "=>")):
		return ConstraintExact
	case strings.HasPrefix(c, "~=") || strings.HasPrefix(c, "~"):
		return ConstraintPatch
	case strings.HasPrefix(c, "^") || (strings.HasPrefix(c, ">=") && strings.Contains(c, "<")):
		return ConstraintMinor
	default:
		return ConstraintMajor
	}
}
