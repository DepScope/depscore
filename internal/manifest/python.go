package manifest

import (
	"bufio"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type PythonParser struct{}

func NewPythonParser() *PythonParser { return &PythonParser{} }
func (p *PythonParser) Ecosystem() Ecosystem { return EcosystemPython }

func (p *PythonParser) Parse(dir string) ([]Package, error) {
	for _, try := range []struct{ file string }{
		{"uv.lock"}, {"poetry.lock"}, {"requirements.txt"},
	} {
		path := dir + "/" + try.file
		if _, err := os.Stat(path); err == nil {
			return p.ParseFile(path)
		}
	}
	return nil, os.ErrNotExist
}

func (p *PythonParser) ParseFile(path string) ([]Package, error) {
	if strings.HasSuffix(path, "requirements.txt") {
		return p.parseRequirements(path)
	}
	if strings.HasSuffix(path, "poetry.lock") {
		return p.parsePoetryLock(path)
	}
	return p.parseUVLock(path)
}

func (p *PythonParser) parseRequirements(path string) ([]Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var pkgs []Package
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, constraint := splitRequirement(line)
		pkgs = append(pkgs, Package{
			Name: name, Constraint: constraint,
			ConstraintType: ParseConstraintType(constraint),
			Ecosystem: EcosystemPython, Depth: 1,
		})
	}
	return pkgs, scanner.Err()
}

func splitRequirement(line string) (name, constraint string) {
	for i, ch := range line {
		if ch == '=' || ch == '>' || ch == '<' || ch == '~' || ch == '!' {
			return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i:])
		}
	}
	return strings.TrimSpace(line), ""
}

func (p *PythonParser) parsePoetryLock(path string) ([]Package, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock struct {
		Package []struct {
			Name         string         `toml:"name"`
			Version      string         `toml:"version"`
			Dependencies map[string]any `toml:"dependencies"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	parentsOf := make(map[string][]string)
	for _, pkg := range lock.Package {
		for depName := range pkg.Dependencies {
			child := strings.ToLower(depName)
			parentsOf[child] = append(parentsOf[child], pkg.Name)
		}
	}
	var pkgs []Package
	for _, pkg := range lock.Package {
		pkgs = append(pkgs, Package{
			Name: pkg.Name, ResolvedVersion: pkg.Version,
			Constraint: "==" + pkg.Version, ConstraintType: ConstraintExact,
			Ecosystem: EcosystemPython, Depth: 1,
			Parents: parentsOf[pkg.Name],
		})
	}
	return pkgs, nil
}

func (p *PythonParser) parseUVLock(path string) ([]Package, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock struct {
		Package []struct {
			Name    string `toml:"name"`
			Version string `toml:"version"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, pkg := range lock.Package {
		pkgs = append(pkgs, Package{
			Name: pkg.Name, ResolvedVersion: pkg.Version,
			Constraint: "==" + pkg.Version, ConstraintType: ConstraintExact,
			Ecosystem: EcosystemPython, Depth: 1,
		})
	}
	return pkgs, nil
}
