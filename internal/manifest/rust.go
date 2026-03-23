package manifest

import (
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

type RustParser struct{}

func NewRustParser() *RustParser { return &RustParser{} }
func (p *RustParser) Ecosystem() Ecosystem { return EcosystemRust }

func (p *RustParser) Parse(dir string) ([]Package, error) {
	// Read Cargo.toml for the root package name and constraints
	tomlData, err := os.ReadFile(filepath.Join(dir, "Cargo.toml"))
	if err != nil {
		return nil, err
	}
	var manifest struct {
		Package struct {
			Name string `toml:"name"`
		} `toml:"package"`
		Dependencies map[string]any `toml:"dependencies"`
	}
	if err := toml.Unmarshal(tomlData, &manifest); err != nil {
		return nil, err
	}

	// Extract constraints from Cargo.toml
	constraints := make(map[string]string)
	for name, v := range manifest.Dependencies {
		switch val := v.(type) {
		case string:
			constraints[name] = normalizeCargo(val)
		case map[string]any:
			if version, ok := val["version"].(string); ok {
				constraints[name] = normalizeCargo(version)
			}
		}
	}

	// Read Cargo.lock for ALL packages and dependency graph
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

	// Build parents map from ALL lock packages
	parentsOf := make(map[string][]string)
	for _, pkg := range lockFile.Package {
		for _, dep := range pkg.Dependencies {
			parts := strings.SplitN(dep, " ", 2)
			depName := parts[0]
			parentsOf[depName] = append(parentsOf[depName], pkg.Name)
		}
	}

	// Build set of direct dependency names
	directDeps := make(map[string]bool)
	for name := range manifest.Dependencies {
		directDeps[name] = true
	}

	// Root package name (to skip)
	rootName := manifest.Package.Name

	// Include ALL packages from lockfile
	var pkgs []Package
	for _, pkg := range lockFile.Package {
		if pkg.Name == rootName {
			continue // skip root package
		}

		depth := 2 // transitive by default
		if directDeps[pkg.Name] {
			depth = 1
		}

		raw := constraints[pkg.Name]
		ct := ParseConstraintType(raw)
		if raw == "" {
			// Transitive dep -- pinned by lockfile
			raw = "=" + pkg.Version
			ct = ConstraintExact
		}

		pkgs = append(pkgs, Package{
			Name:            pkg.Name,
			ResolvedVersion: pkg.Version,
			Constraint:      raw,
			ConstraintType:  ct,
			Ecosystem:       EcosystemRust,
			Depth:           depth,
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
	// Bare version like "1.0" -> treat as ^1.0 (minor constraint)
	return "^" + v
}
