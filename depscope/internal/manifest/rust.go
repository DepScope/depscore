package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// RustParser parses Rust manifest files: Cargo.toml and Cargo.lock.
type RustParser struct{}

func NewRustParser() *RustParser { return &RustParser{} }

func (p *RustParser) Ecosystem() Ecosystem { return EcosystemRust }

// Parse implements the Parser interface. It reads Cargo.toml for constraints
// and Cargo.lock for resolved versions and dependency relationships.
func (p *RustParser) Parse(dir string) ([]Package, error) {
	constraints, rootName, err := parseCargoToml(filepath.Join(dir, "Cargo.toml"))
	if err != nil {
		return nil, fmt.Errorf("parsing Cargo.toml: %w", err)
	}

	resolved, parents, err := parseCargoLock(filepath.Join(dir, "Cargo.lock"), rootName)
	if err != nil {
		return nil, fmt.Errorf("parsing Cargo.lock: %w", err)
	}

	var pkgs []Package
	for name, constraint := range constraints {
		resolvedVer := resolved[name]
		depth := 1
		if len(parents[name]) > 0 {
			depth = 2
		}
		pkgs = append(pkgs, Package{
			Name:            name,
			ResolvedVersion: resolvedVer,
			Constraint:      constraint,
			ConstraintType:  cargoConstraintType(constraint),
			Ecosystem:       EcosystemRust,
			Depth:           depth,
			Parents:         parents[name],
		})
	}
	return pkgs, nil
}

// cargoConstraintType maps a Cargo version requirement string to a ConstraintType.
// Rules:
//   - "=X.Y.Z"          → exact
//   - "~X.Y" or "~X.Y.Z"→ patch
//   - "^X.Y" or "^X.Y.Z"→ minor  (also bare "X.Y" or "X.Y.Z")
//   - ">=X.Y"           → major
func cargoConstraintType(constraint string) ConstraintType {
	c := strings.TrimSpace(constraint)
	switch {
	case strings.HasPrefix(c, "=") && !strings.HasPrefix(c, "=>") && !strings.HasPrefix(c, ">="):
		return ConstraintExact
	case strings.HasPrefix(c, "~"):
		return ConstraintPatch
	case strings.HasPrefix(c, "^"):
		return ConstraintMinor
	case strings.HasPrefix(c, ">=") || strings.HasPrefix(c, ">"):
		return ConstraintMajor
	default:
		// Bare version like "1.0" or "1.35.1" — Cargo treats as ^X.Y (caret/minor)
		return ConstraintMinor
	}
}

// parseCargoToml reads [dependencies] from a Cargo.toml, returning a map of
// package name → version requirement string, plus the root package name.
func parseCargoToml(path string) (constraints map[string]string, rootName string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}

	// Use a raw map to handle mixed string/table dependency values
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, "", err
	}

	// Extract root package name
	if pkgSection, ok := raw["package"].(map[string]any); ok {
		if n, ok := pkgSection["name"].(string); ok {
			rootName = n
		}
	}

	constraints = make(map[string]string)
	depsRaw, ok := raw["dependencies"].(map[string]any)
	if !ok {
		return constraints, rootName, nil
	}

	for name, val := range depsRaw {
		switch v := val.(type) {
		case string:
			// e.g. reqwest = "=0.11.23"
			constraints[name] = v
		case map[string]any:
			// e.g. serde = { version = "1.0", features = [...] }
			if ver, ok := v["version"].(string); ok {
				constraints[name] = ver
			}
		}
	}
	return constraints, rootName, nil
}

// lockPackage is one [[package]] entry in Cargo.lock.
type lockPackage struct {
	Name         string   `toml:"name"`
	Version      string   `toml:"version"`
	Dependencies []string `toml:"dependencies"`
}

// parseCargoLock reads Cargo.lock and returns:
//   - resolved: map of package name → resolved version
//   - parents:  map of package name → list of packages that depend on it
//
// The root package (rootName) is excluded from parents as a dependency target
// so it doesn't pollute child parent lists, but its declared deps are still
// used to build the parent map.
func parseCargoLock(path string, rootName string) (resolved map[string]string, parents map[string][]string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	var lock struct {
		Package []lockPackage `toml:"package"`
	}
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, nil, err
	}

	resolved = make(map[string]string)
	parents = make(map[string][]string)

	for _, pkg := range lock.Package {
		if pkg.Name != rootName {
			resolved[pkg.Name] = pkg.Version
		}

		// Each entry in pkg.Dependencies has the format "name version"
		for _, dep := range pkg.Dependencies {
			depName := strings.SplitN(dep, " ", 2)[0]
			parents[depName] = append(parents[depName], pkg.Name)
		}
	}
	return resolved, parents, nil
}
