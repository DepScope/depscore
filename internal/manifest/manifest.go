package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Ecosystem string

const (
	EcosystemPython  Ecosystem = "python"
	EcosystemGo      Ecosystem = "go"
	EcosystemRust    Ecosystem = "rust"
	EcosystemNPM     Ecosystem = "npm"
	EcosystemPHP     Ecosystem = "php"
	EcosystemActions Ecosystem = "actions"
)

type ConstraintType string

const (
	ConstraintExact ConstraintType = "exact"
	ConstraintPatch ConstraintType = "patch"
	ConstraintMinor ConstraintType = "minor"
	ConstraintMajor ConstraintType = "major"
)

type Package struct {
	Name            string
	ResolvedVersion string
	Constraint      string
	ConstraintType  ConstraintType
	Ecosystem       Ecosystem
	Depth           int
	Parents         []string
}

func (p Package) Key() string {
	return string(p.Ecosystem) + "/" + p.Name + "@" + p.ResolvedVersion
}

type Parser interface {
	Parse(dir string) ([]Package, error)
	ParseFiles(files map[string][]byte) ([]Package, error)
	Ecosystem() Ecosystem
}

var ecosystemFiles = []struct {
	file      string
	ecosystem Ecosystem
}{
	{"go.mod", EcosystemGo},
	{"Cargo.toml", EcosystemRust},
	{"package.json", EcosystemNPM},
	{"composer.json", EcosystemPHP},
	{"uv.lock", EcosystemPython},
	{"poetry.lock", EcosystemPython},
	{"pyproject.toml", EcosystemPython},
	{"requirements.txt", EcosystemPython},
}

func DetectEcosystem(dir string) (Ecosystem, error) {
	for _, ef := range ecosystemFiles {
		if _, err := os.Stat(filepath.Join(dir, ef.file)); err == nil {
			return ef.ecosystem, nil
		}
	}
	return "", fmt.Errorf("no recognized manifest found in %s", dir)
}

// DetectAllEcosystems returns all ecosystems detected in dir (deduped).
func DetectAllEcosystems(dir string) []Ecosystem {
	seen := make(map[Ecosystem]bool)
	var result []Ecosystem
	for _, ef := range ecosystemFiles {
		if _, err := os.Stat(filepath.Join(dir, ef.file)); err == nil {
			if !seen[ef.ecosystem] {
				seen[ef.ecosystem] = true
				result = append(result, ef.ecosystem)
			}
		}
	}

	// Detect GitHub Actions: check if .github/workflows/ directory exists
	workflowDir := filepath.Join(dir, ".github", "workflows")
	if info, err := os.Stat(workflowDir); err == nil && info.IsDir() {
		if !seen[EcosystemActions] {
			seen[EcosystemActions] = true
			result = append(result, EcosystemActions)
		}
	}

	return result
}

func DetectEcosystemFromFiles(filenames []string) (Ecosystem, error) {
	nameSet := make(map[string]bool)
	for _, f := range filenames {
		nameSet[filepath.Base(f)] = true
	}
	for _, ef := range ecosystemFiles {
		if nameSet[ef.file] {
			return ef.ecosystem, nil
		}
	}
	return "", fmt.Errorf("no recognized manifest in files: %v", filenames)
}

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
	case EcosystemPHP:
		return NewPHPParser()
	default:
		panic("unknown ecosystem: " + string(eco))
	}
}

// String returns the canonical ecosystem name as used by registry APIs.
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
	case EcosystemPHP:
		return "Packagist"
	case EcosystemActions:
		return "GitHub Actions"
	default:
		return string(e)
	}
}

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
		seen := make(map[string]map[string]bool) // parent → set of children
		for _, p := range pkgs {
			for _, parent := range p.Parents {
				if seen[parent] == nil {
					seen[parent] = make(map[string]bool)
				}
				if !seen[parent][p.Name] {
					seen[parent][p.Name] = true
					deps[parent] = append(deps[parent], p.Name)
				}
			}
		}
		return deps
	}

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
