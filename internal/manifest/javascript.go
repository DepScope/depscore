package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type JavaScriptParser struct{}

func NewJavaScriptParser() *JavaScriptParser { return &JavaScriptParser{} }
func (p *JavaScriptParser) Ecosystem() Ecosystem { return EcosystemNPM }

func (p *JavaScriptParser) Parse(dir string) ([]Package, error) {
	// Read package.json for constraints
	pkgData, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	var pkgJSON struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(pkgData, &pkgJSON); err != nil {
		return nil, err
	}

	// Try to read package-lock.json for resolved versions
	resolved := make(map[string]string)
	lockData, err := os.ReadFile(filepath.Join(dir, "package-lock.json"))
	if err == nil {
		var lock struct {
			LockfileVersion int `json:"lockfileVersion"`
			Packages        map[string]struct {
				Version string `json:"version"`
			} `json:"packages"`
		}
		if json.Unmarshal(lockData, &lock) == nil {
			for key, val := range lock.Packages {
				name := strings.TrimPrefix(key, "node_modules/")
				resolved[name] = val.Version
			}
		}
	}

	var pkgs []Package
	for name, constraint := range pkgJSON.Dependencies {
		version := resolved[name]
		if version == "" {
			version = strings.TrimLeft(constraint, "^~>=<")
		}
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
