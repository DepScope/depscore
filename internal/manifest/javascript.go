package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type JavaScriptParser struct{}

func NewJavaScriptParser() *JavaScriptParser { return &JavaScriptParser{} }
func (p *JavaScriptParser) Ecosystem() Ecosystem { return EcosystemNPM }

func (p *JavaScriptParser) Parse(dir string) ([]Package, error) {
	// Read package.json for direct dependency constraints
	pkgData, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	var pkgJSON struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(pkgData, &pkgJSON); err != nil {
		return nil, err
	}

	// Merge deps (production only for scoring)
	directConstraints := pkgJSON.Dependencies

	// Try to read package-lock.json for full resolved tree
	lockData, err := os.ReadFile(filepath.Join(dir, "package-lock.json"))
	if err != nil {
		// Try pnpm-lock.yaml
		if _, pnpmErr := os.Stat(filepath.Join(dir, "pnpm-lock.yaml")); pnpmErr == nil {
			return p.parsePnpmLock(dir, directConstraints)
		}
		// Try bun.lock
		if _, bunErr := os.Stat(filepath.Join(dir, "bun.lock")); bunErr == nil {
			return p.parseBunLock(dir, directConstraints)
		}
		// No lockfile -- fall back to package.json only
		return p.fromPackageJSON(directConstraints)
	}

	var lock struct {
		LockfileVersion int `json:"lockfileVersion"`
		Packages        map[string]struct {
			Version      string            `json:"version"`
			Resolved     string            `json:"resolved"`
			Dependencies map[string]string `json:"dependencies"`
		} `json:"packages"`
	}
	if json.Unmarshal(lockData, &lock) != nil {
		return p.fromPackageJSON(directConstraints)
	}

	var pkgs []Package
	for key, val := range lock.Packages {
		if key == "" {
			continue // skip root package entry
		}
		name := strings.TrimPrefix(key, "node_modules/")
		// Handle scoped packages and nested node_modules
		// e.g., "node_modules/@scope/pkg" or "node_modules/a/node_modules/b"
		if strings.Contains(name, "node_modules/") {
			parts := strings.Split(name, "node_modules/")
			name = parts[len(parts)-1]
		}

		depth := 2 // transitive by default
		constraint := "=" + val.Version
		ct := ConstraintExact

		if c, ok := directConstraints[name]; ok {
			depth = 1
			constraint = c
			ct = parseNPMConstraint(c)
		}

		pkgs = append(pkgs, Package{
			Name:            name,
			ResolvedVersion: val.Version,
			Constraint:      constraint,
			ConstraintType:  ct,
			Ecosystem:       EcosystemNPM,
			Depth:           depth,
		})
	}

	return pkgs, nil
}

func (p *JavaScriptParser) fromPackageJSON(deps map[string]string) ([]Package, error) {
	var pkgs []Package
	for name, constraint := range deps {
		version := strings.TrimLeft(constraint, "^~>=<")
		pkgs = append(pkgs, Package{
			Name:            name,
			ResolvedVersion: version,
			Constraint:      constraint,
			ConstraintType:  parseNPMConstraint(constraint),
			Ecosystem:       EcosystemNPM,
			Depth:           1,
		})
	}
	return pkgs, nil
}

func parseNPMConstraint(c string) ConstraintType {
	c = strings.TrimSpace(c)
	switch {
	case strings.HasPrefix(c, "^"):
		return ConstraintMinor
	case strings.HasPrefix(c, "~"):
		return ConstraintPatch
	case c == "latest" || c == "*" || strings.HasPrefix(c, ">=") || strings.HasPrefix(c, ">"):
		return ConstraintMajor
	default:
		// bare version string like "4.17.21" = exact
		if len(c) > 0 && c[0] >= '0' && c[0] <= '9' {
			return ConstraintExact
		}
		return ConstraintMajor
	}
}

// parsePnpmLock parses a pnpm-lock.yaml file.
func (p *JavaScriptParser) parsePnpmLock(dir string, directConstraints map[string]string) ([]Package, error) {
	data, err := os.ReadFile(filepath.Join(dir, "pnpm-lock.yaml"))
	if err != nil {
		return nil, err
	}

	var lock struct {
		Packages map[string]struct {
			Dependencies map[string]string `yaml:"dependencies"`
			Dev          bool              `yaml:"dev"`
		} `yaml:"packages"`
	}
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}

	var pkgs []Package
	for key, val := range lock.Packages {
		if val.Dev {
			continue
		}
		name, version := parsePnpmKey(key)
		if name == "" {
			continue
		}

		depth := 2
		constraint := "=" + version
		ct := ConstraintExact
		if c, ok := directConstraints[name]; ok {
			depth = 1
			constraint = c
			ct = parseNPMConstraint(c)
		}

		pkgs = append(pkgs, Package{
			Name:            name,
			ResolvedVersion: version,
			Constraint:      constraint,
			ConstraintType:  ct,
			Ecosystem:       EcosystemNPM,
			Depth:           depth,
		})
	}
	return pkgs, nil
}

// parsePnpmKey parses a pnpm package key like "/express@4.18.2" or "/@scope/pkg@1.0.0".
func parsePnpmKey(key string) (name, version string) {
	key = strings.TrimPrefix(key, "/")
	idx := strings.LastIndex(key, "@")
	if idx <= 0 {
		return "", ""
	}
	return key[:idx], key[idx+1:]
}

// stripJSONComments removes single-line // comments from JSONC content.
var jsoncCommentRe = regexp.MustCompile(`(?m)^\s*//.*$`)

// parseBunLock parses a bun.lock file (JSONC format).
// bun.lock packages map keys to arrays: [resolved@version, {dependencies...}]
func (p *JavaScriptParser) parseBunLock(dir string, directConstraints map[string]string) ([]Package, error) {
	data, err := os.ReadFile(filepath.Join(dir, "bun.lock"))
	if err != nil {
		return nil, err
	}

	// Strip // comments (bun.lock is JSONC)
	cleaned := jsoncCommentRe.ReplaceAll(data, nil)

	var lock struct {
		Packages map[string][]json.RawMessage `json:"packages"`
	}
	if err := json.Unmarshal(cleaned, &lock); err != nil {
		return nil, err
	}

	var pkgs []Package
	for _, vals := range lock.Packages {
		if len(vals) == 0 {
			continue
		}
		var resolved string
		if err := json.Unmarshal(vals[0], &resolved); err != nil {
			continue
		}
		// resolved is like "express@4.18.2"
		name, version := parseBunResolved(resolved)
		if name == "" {
			continue
		}

		depth := 2
		constraint := "=" + version
		ct := ConstraintExact
		if c, ok := directConstraints[name]; ok {
			depth = 1
			constraint = c
			ct = parseNPMConstraint(c)
		}

		pkgs = append(pkgs, Package{
			Name:            name,
			ResolvedVersion: version,
			Constraint:      constraint,
			ConstraintType:  ct,
			Ecosystem:       EcosystemNPM,
			Depth:           depth,
		})
	}
	return pkgs, nil
}

// parseBunResolved parses "name@version" from a bun.lock resolved string.
func parseBunResolved(s string) (name, version string) {
	idx := strings.LastIndex(s, "@")
	if idx <= 0 {
		return "", ""
	}
	return s[:idx], s[idx+1:]
}
