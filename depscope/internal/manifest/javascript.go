package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// JavaScriptParser parses Node.js manifest files: package.json and package-lock.json.
type JavaScriptParser struct{}

func NewJavaScriptParser() *JavaScriptParser { return &JavaScriptParser{} }

func (p *JavaScriptParser) Ecosystem() Ecosystem { return EcosystemNPM }

// ParseFiles implements the Parser interface for in-memory file content.
func (p *JavaScriptParser) ParseFiles(files map[string][]byte) ([]Package, error) {
	pkgData, ok := files["package.json"]
	if !ok {
		return nil, fmt.Errorf("package.json not found in files")
	}
	constraints, err := parsePackageJSONBytes(pkgData)
	if err != nil {
		return nil, fmt.Errorf("parsing package.json: %w", err)
	}

	resolved := make(map[string]string)
	if lockData, ok := files["package-lock.json"]; ok {
		resolved, err = parsePackageLockJSONBytes(lockData)
		if err != nil {
			return nil, fmt.Errorf("parsing package-lock.json: %w", err)
		}
	}

	var pkgs []Package
	for name, constraint := range constraints {
		pkgs = append(pkgs, Package{
			Name:            name,
			ResolvedVersion: resolved[name],
			Constraint:      constraint,
			ConstraintType:  npmConstraintType(constraint),
			Ecosystem:       EcosystemNPM,
			Depth:           1,
		})
	}
	return pkgs, nil
}

// Parse implements the Parser interface. It reads package.json for constraints
// and package-lock.json (lockfileVersion 3) for resolved versions.
func (p *JavaScriptParser) Parse(dir string) ([]Package, error) {
	files := make(map[string][]byte)
	pkgData, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, fmt.Errorf("parsing package.json: %w", err)
	}
	files["package.json"] = pkgData

	lockData, err := os.ReadFile(filepath.Join(dir, "package-lock.json"))
	if err == nil {
		files["package-lock.json"] = lockData
	}

	return p.ParseFiles(files)
}

// npmConstraintType maps an npm version string to a ConstraintType.
// Rules:
//   - "^X.Y.Z"      → minor
//   - "~X.Y.Z"      → patch
//   - ">=X.Y"       → major
//   - "X.Y.Z" (bare)→ exact
func npmConstraintType(constraint string) ConstraintType {
	c := strings.TrimSpace(constraint)
	switch {
	case strings.HasPrefix(c, "^"):
		return ConstraintMinor
	case strings.HasPrefix(c, "~"):
		return ConstraintPatch
	case strings.HasPrefix(c, ">=") || strings.HasPrefix(c, ">"):
		return ConstraintMajor
	default:
		// Bare version (e.g. "4.17.21") or "*" / "latest"
		return ConstraintExact
	}
}

// parsePackageJSONBytes reads the dependencies map from package.json bytes.
func parsePackageJSONBytes(data []byte) (map[string]string, error) {
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	if pkg.Dependencies == nil {
		return make(map[string]string), nil
	}
	return pkg.Dependencies, nil
}

// packageLockEntry is one entry in the "packages" map of package-lock.json v3.
type packageLockEntry struct {
	Version string `json:"version"`
}

// parsePackageLockJSONBytes reads resolved versions from a lockfileVersion 3
// package-lock.json bytes. Keys in "packages" are "node_modules/{name}".
func parsePackageLockJSONBytes(data []byte) (map[string]string, error) {
	var lock struct {
		Packages map[string]packageLockEntry `json:"packages"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, err
	}

	resolved := make(map[string]string)
	const prefix = "node_modules/"
	for key, entry := range lock.Packages {
		if strings.HasPrefix(key, prefix) {
			name := key[len(prefix):]
			resolved[name] = entry.Version
		}
	}
	return resolved, nil
}

