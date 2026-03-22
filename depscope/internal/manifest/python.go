package manifest

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// PythonParser parses Python manifest files: requirements.txt, poetry.lock, uv.lock.
type PythonParser struct{}

func NewPythonParser() *PythonParser { return &PythonParser{} }

func (p *PythonParser) Ecosystem() Ecosystem { return EcosystemPython }

// ParseFiles implements the Parser interface for in-memory file content.
// It dispatches based on which keys are present, preferring uv.lock > poetry.lock > requirements.txt.
func (p *PythonParser) ParseFiles(files map[string][]byte) ([]Package, error) {
	if data, ok := files["uv.lock"]; ok {
		return parseUVLockBytes(data)
	}
	if data, ok := files["poetry.lock"]; ok {
		return parsePoetryLockBytes(data)
	}
	if data, ok := files["requirements.txt"]; ok {
		return parseRequirementsTxtBytes(data)
	}
	if data, ok := files["pyproject.toml"]; ok {
		return parsePyprojectTomlBytes(data)
	}
	return nil, fmt.Errorf("no recognized Python manifest file found in provided files")
}

// Parse implements the Parser interface. It selects the best available manifest in dir.
func (p *PythonParser) Parse(dir string) ([]Package, error) {
	for _, name := range []string{"uv.lock", "poetry.lock", "requirements.txt"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return p.ParseFile(path)
		}
	}
	return nil, nil
}

// ParseFile parses a single Python manifest file, dispatching on its base name.
func (p *PythonParser) ParseFile(path string) ([]Package, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	base := filepath.Base(path)
	switch base {
	case "requirements.txt":
		return parseRequirementsTxtBytes(data)
	case "poetry.lock":
		return parsePoetryLockBytes(data)
	case "uv.lock":
		return parseUVLockBytes(data)
	default:
		return nil, nil
	}
}

// --- requirements.txt parser ---

func parseRequirementsTxtBytes(data []byte) ([]Package, error) {
	var pkgs []Package
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip inline comments
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}

		name, constraint := splitRequirement(line)
		if name == "" {
			continue
		}

		ct := ParseConstraintType(constraint)
		resolved := ""
		if ct == ConstraintExact {
			resolved = strings.TrimLeft(constraint, "=")
		}

		pkgs = append(pkgs, Package{
			Name:            name,
			Constraint:      constraint,
			ConstraintType:  ct,
			ResolvedVersion: resolved,
			Ecosystem:       EcosystemPython,
			Depth:           1,
		})
	}
	return pkgs, scanner.Err()
}

// splitRequirement splits a requirement line like "requests==2.31.0" into
// name="requests" and constraint="==2.31.0".
func splitRequirement(line string) (name, constraint string) {
	// Operators to try, longest first so ">=" isn't mistaken for ">"
	ops := []string{"===", "~=", "==", "!=", ">=", "<=", ">", "<", "~"}
	for _, op := range ops {
		if i := strings.Index(line, op); i >= 0 {
			return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i:])
		}
	}
	// No operator — bare package name
	return strings.TrimSpace(line), ""
}

// --- pyproject.toml parser ---

func parsePyprojectTomlBytes(data []byte) ([]Package, error) {
	var proj struct {
		Project struct {
			Dependencies []string `toml:"dependencies"`
		} `toml:"project"`
		Tool struct {
			Poetry struct {
				Dependencies map[string]any `toml:"dependencies"`
			} `toml:"poetry"`
		} `toml:"tool"`
	}
	if err := toml.Unmarshal(data, &proj); err != nil {
		return nil, err
	}

	var pkgs []Package

	// PEP 621: [project.dependencies] — list of requirement strings like "requests>=2.0"
	for _, req := range proj.Project.Dependencies {
		name, constraint := splitRequirement(req)
		if name == "" {
			continue
		}
		ct := ParseConstraintType(constraint)
		resolved := ""
		if ct == ConstraintExact {
			resolved = strings.TrimLeft(constraint, "=")
		}
		pkgs = append(pkgs, Package{
			Name: strings.ToLower(name), Constraint: constraint,
			ConstraintType: ct, ResolvedVersion: resolved,
			Ecosystem: EcosystemPython, Depth: 1,
		})
	}

	// Poetry: [tool.poetry.dependencies] — map of name → version string or table
	if len(pkgs) == 0 {
		for name, val := range proj.Tool.Poetry.Dependencies {
			if strings.ToLower(name) == "python" {
				continue
			}
			constraint := ""
			switch v := val.(type) {
			case string:
				constraint = v
			case map[string]any:
				if ver, ok := v["version"].(string); ok {
					constraint = ver
				}
			}
			ct := ParseConstraintType(constraint)
			pkgs = append(pkgs, Package{
				Name: strings.ToLower(name), Constraint: constraint,
				ConstraintType: ct, Ecosystem: EcosystemPython, Depth: 1,
			})
		}
	}

	return pkgs, nil
}

// --- poetry.lock parser ---

func parsePoetryLockBytes(data []byte) ([]Package, error) {
	// Build a map of package name -> dependency names (children).
	// poetry.lock [package.dependencies] keys are the child package names.
	type rawPkg struct {
		Name         string         `toml:"name"`
		Version      string         `toml:"version"`
		Dependencies map[string]any `toml:"dependencies"`
	}
	var rawLock struct {
		Package []rawPkg `toml:"package"`
	}
	if err := toml.Unmarshal(data, &rawLock); err != nil {
		return nil, err
	}

	// children[parent] = list of direct child names
	children := make(map[string][]string)
	for _, pkg := range rawLock.Package {
		name := strings.ToLower(pkg.Name)
		for depName := range pkg.Dependencies {
			children[name] = append(children[name], strings.ToLower(depName))
		}
	}

	// Build parents map: parents[child] = list of parents
	parents := make(map[string][]string)
	for parent, deps := range children {
		for _, child := range deps {
			parents[child] = append(parents[child], parent)
		}
	}

	var pkgs []Package
	for _, pkg := range rawLock.Package {
		name := strings.ToLower(pkg.Name)
		depth := 1
		if len(parents[name]) > 0 {
			depth = 2
		}
		pkgs = append(pkgs, Package{
			Name:            name,
			ResolvedVersion: pkg.Version,
			Constraint:      pkg.Version,
			ConstraintType:  ConstraintExact,
			Ecosystem:       EcosystemPython,
			Depth:           depth,
			Parents:         parents[name],
		})
	}
	return pkgs, nil
}

// --- uv.lock parser ---

type uvLockFile struct {
	Package []uvPackage `toml:"package"`
}

type uvPackage struct {
	Name    string `toml:"name"`
	Version string `toml:"version"`
}

func parseUVLockBytes(data []byte) ([]Package, error) {
	var lock uvLockFile
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}

	var pkgs []Package
	for _, pkg := range lock.Package {
		pkgs = append(pkgs, Package{
			Name:            strings.ToLower(pkg.Name),
			ResolvedVersion: pkg.Version,
			Constraint:      pkg.Version,
			ConstraintType:  ConstraintExact,
			Ecosystem:       EcosystemPython,
			Depth:           1,
		})
	}
	return pkgs, nil
}
