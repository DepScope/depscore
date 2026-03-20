package manifest

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type RustParser struct{}

func NewRustParser() *RustParser { return &RustParser{} }
func (p *RustParser) Ecosystem() Ecosystem { return EcosystemRust }

func (p *RustParser) Parse(dir string) ([]Package, error) {
	// Read Cargo.toml for constraints
	tomlData, err := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
	if err != nil {
		return nil, err
	}
	var cargoManifest struct {
		Dependencies map[string]any `toml:"dependencies"`
	}
	if err := toml.Unmarshal(tomlData, &cargoManifest); err != nil {
		return nil, err
	}

	constraints := make(map[string]string)
	for name, v := range cargoManifest.Dependencies {
		switch val := v.(type) {
		case string:
			constraints[name] = normalizeCargo(val)
		case map[string]any:
			if version, ok := val["version"].(string); ok {
				constraints[name] = normalizeCargo(version)
			}
		}
	}

	// Read Cargo.lock for resolved versions and dependency graph
	lockData, err := os.ReadFile(filepath.Join(dir, "Cargo.lock"))
	if err != nil {
		return nil, err
	}
	var lockFile struct {
		Package []struct {
			Name         string   `toml:"name"`
			Version      string   `toml:"version"`
			Dependencies []string `toml:"dependencies"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(lockData, &lockFile); err != nil {
		return nil, err
	}

	// Build parents map: for each package's deps, mark them as having this parent
	parentsOf := make(map[string][]string)
	for _, pkg := range lockFile.Package {
		for _, dep := range pkg.Dependencies {
			// dep format: "name version"
			parts := strings.SplitN(dep, " ", 2)
			depName := parts[0]
			parentsOf[depName] = append(parentsOf[depName], pkg.Name)
		}
	}

	// Find direct dependency names (the ones in Cargo.toml [dependencies])
	depNames := make(map[string]bool)
	for n := range cargoManifest.Dependencies {
		depNames[n] = true
	}

	var pkgs []Package
	for _, pkg := range lockFile.Package {
		if !depNames[pkg.Name] {
			continue // skip root package and transitive-only deps
		}
		raw := constraints[pkg.Name]
		pkgs = append(pkgs, Package{
			Name:            pkg.Name,
			ResolvedVersion: pkg.Version,
			Constraint:      raw,
			ConstraintType:  ParseConstraintType(raw),
			Ecosystem:       EcosystemRust,
			Depth:           1,
			Parents:         parentsOf[pkg.Name],
		})
	}
	return pkgs, nil
}

// normalizeCargo converts Cargo version strings to semver constraint form.
func normalizeCargo(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "*" {
		return "*"
	}
	// Already has operator
	if strings.HasPrefix(v, "=") || strings.HasPrefix(v, "^") ||
		strings.HasPrefix(v, "~") || strings.HasPrefix(v, ">") || strings.HasPrefix(v, "<") {
		return v
	}
	// Bare version like "1.0" → treat as ^1.0 (minor constraint)
	return "^" + v
}
