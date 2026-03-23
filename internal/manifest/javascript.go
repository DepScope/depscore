package manifest

import (
	"bufio"
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
	} else if lockData, ok := files["pnpm-lock.yaml"]; ok {
		resolved, err = parsePnpmLockYAML(lockData)
		if err != nil {
			return nil, fmt.Errorf("parsing pnpm-lock.yaml: %w", err)
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

	for _, lockName := range []string{"package-lock.json", "pnpm-lock.yaml", "bun.lock"} {
		lockData, err := os.ReadFile(filepath.Join(dir, lockName))
		if err == nil {
			files[lockName] = lockData
			break // use the first lockfile found
		}
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

// parsePackageJSONBytes reads dependencies + devDependencies from package.json bytes.
func parsePackageJSONBytes(data []byte) (map[string]string, error) {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	merged := make(map[string]string)
	for k, v := range pkg.Dependencies {
		merged[k] = v
	}
	for k, v := range pkg.DevDependencies {
		if _, exists := merged[k]; !exists {
			merged[k] = v
		}
	}
	return merged, nil
}

// parsePnpmLockYAML extracts resolved versions from a pnpm-lock.yaml file.
// Packages are listed under "packages:" with keys like "'@babel/parser@7.29.0':" or "'express@4.18.2':".
// We parse the key to extract name@version without a full YAML parser.
func parsePnpmLockYAML(data []byte) (map[string]string, error) {
	resolved := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	inPackages := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "packages:" {
			inPackages = true
			continue
		}
		if inPackages {
			// New top-level section ends "packages:"
			if len(line) > 0 && line[0] != ' ' && line[0] != '\'' {
				break
			}
			// Package entries look like: "  '@babel/parser@7.29.0':" or "  'express@4.18.2':"
			trimmed := strings.TrimSpace(line)
			if !strings.HasSuffix(trimmed, ":") {
				continue
			}
			trimmed = strings.TrimSuffix(trimmed, ":")
			trimmed = strings.Trim(trimmed, "'\"")
			// Find last @ that separates name from version (scoped packages have @ at start)
			if lastAt := strings.LastIndex(trimmed, "@"); lastAt > 0 {
				name := trimmed[:lastAt]
				version := trimmed[lastAt+1:]
				resolved[name] = version
			}
		}
	}
	return resolved, scanner.Err()
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
