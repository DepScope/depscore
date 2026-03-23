package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PHPParser parses PHP manifest files: composer.json and composer.lock.
type PHPParser struct{}

func NewPHPParser() *PHPParser { return &PHPParser{} }

func (p *PHPParser) Ecosystem() Ecosystem { return EcosystemPHP }

// ParseFiles implements the Parser interface for in-memory file content.
func (p *PHPParser) ParseFiles(files map[string][]byte) ([]Package, error) {
	composerData, ok := files["composer.json"]
	if !ok {
		return nil, fmt.Errorf("composer.json not found in files")
	}
	constraints, err := parseComposerJSONBytes(composerData)
	if err != nil {
		return nil, fmt.Errorf("parsing composer.json: %w", err)
	}

	resolved := make(map[string]string)
	parents := make(map[string][]string)
	if lockData, ok := files["composer.lock"]; ok {
		resolved, parents, err = parseComposerLockBytes(lockData)
		if err != nil {
			return nil, fmt.Errorf("parsing composer.lock: %w", err)
		}
	}

	var pkgs []Package
	for name, constraint := range constraints {
		depth := 1
		if len(parents[name]) > 0 {
			depth = 2
		}
		pkgs = append(pkgs, Package{
			Name:            name,
			ResolvedVersion: resolved[name],
			Constraint:      constraint,
			ConstraintType:  phpConstraintType(constraint),
			Ecosystem:       EcosystemPHP,
			Depth:           depth,
			Parents:         parents[name],
		})
	}

	// Include packages from composer.lock that are transitive (not in composer.json directly)
	for name := range resolved {
		if _, inConstraints := constraints[name]; !inConstraints {
			depth := 2
			if len(parents[name]) == 0 {
				depth = 1
			}
			pkgs = append(pkgs, Package{
				Name:            name,
				ResolvedVersion: resolved[name],
				Constraint:      resolved[name],
				ConstraintType:  ConstraintExact,
				Ecosystem:       EcosystemPHP,
				Depth:           depth,
				Parents:         parents[name],
			})
		}
	}

	return pkgs, nil
}

// Parse implements the Parser interface. It reads composer.json for constraints
// and composer.lock for resolved versions and dependency relationships.
func (p *PHPParser) Parse(dir string) ([]Package, error) {
	files := make(map[string][]byte)
	composerData, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	if err != nil {
		return nil, fmt.Errorf("parsing composer.json: %w", err)
	}
	files["composer.json"] = composerData

	lockData, err := os.ReadFile(filepath.Join(dir, "composer.lock"))
	if err == nil {
		files["composer.lock"] = lockData
	}

	return p.ParseFiles(files)
}

// phpConstraintType maps a Composer version requirement string to a ConstraintType.
// Rules:
//   - "^X.Y"         → minor (caret = compatible with)
//   - "~X.Y"         → patch (tilde = next significant release)
//   - "X.Y.Z" (bare) → exact
//   - ">=X.Y" / ">X.Y" / "*" → major
func phpConstraintType(constraint string) ConstraintType {
	c := strings.TrimSpace(constraint)
	switch {
	case strings.HasPrefix(c, "^"):
		return ConstraintMinor
	case strings.HasPrefix(c, "~"):
		return ConstraintPatch
	case strings.HasPrefix(c, ">=") || strings.HasPrefix(c, ">") || c == "*":
		return ConstraintMajor
	default:
		// Bare version like "1.2.3"
		return ConstraintExact
	}
}

// isSkippedEntry returns true for entries that should be skipped:
// php, ext-*, lib-* (language/extension requirements, not packages).
func isSkippedEntry(name string) bool {
	return name == "php" ||
		strings.HasPrefix(name, "ext-") ||
		strings.HasPrefix(name, "lib-")
}

// composerJSON is the structure of composer.json relevant fields.
type composerJSON struct {
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
}

// parseComposerJSONBytes reads require + require-dev from composer.json bytes,
// skipping php, ext-*, and lib-* entries.
func parseComposerJSONBytes(data []byte) (map[string]string, error) {
	var cj composerJSON
	if err := json.Unmarshal(data, &cj); err != nil {
		return nil, err
	}

	constraints := make(map[string]string)
	for name, ver := range cj.Require {
		if isSkippedEntry(name) {
			continue
		}
		constraints[name] = ver
	}
	for name, ver := range cj.RequireDev {
		if isSkippedEntry(name) {
			continue
		}
		if _, exists := constraints[name]; !exists {
			constraints[name] = ver
		}
	}
	return constraints, nil
}

// composerLockPackage is one entry in the packages/packages-dev array of composer.lock.
type composerLockPackage struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Require map[string]string `json:"require"`
}

// composerLock is the structure of composer.lock relevant fields.
type composerLock struct {
	Packages    []composerLockPackage `json:"packages"`
	PackagesDev []composerLockPackage `json:"packages-dev"`
}

// parseComposerLockBytes reads packages + packages-dev from composer.lock bytes,
// returning:
//   - resolved: map of package name → resolved version (v prefix stripped)
//   - parents:  map of package name → list of packages that depend on it
func parseComposerLockBytes(data []byte) (resolved map[string]string, parents map[string][]string, err error) {
	var lock composerLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, nil, err
	}

	resolved = make(map[string]string)
	parents = make(map[string][]string)

	allPkgs := append(lock.Packages, lock.PackagesDev...)
	for _, pkg := range allPkgs {
		version := strings.TrimPrefix(pkg.Version, "v")
		resolved[pkg.Name] = version

		for dep := range pkg.Require {
			if isSkippedEntry(dep) {
				continue
			}
			parents[dep] = append(parents[dep], pkg.Name)
		}
	}

	return resolved, parents, nil
}
