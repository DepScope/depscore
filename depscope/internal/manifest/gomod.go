package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

type GoModParser struct{}

func NewGoModParser() *GoModParser { return &GoModParser{} }

func (p *GoModParser) Ecosystem() Ecosystem { return EcosystemGo }

func (p *GoModParser) ParseFiles(files map[string][]byte) ([]Package, error) {
	data, ok := files["go.mod"]
	if !ok {
		return nil, fmt.Errorf("go.mod not found in files")
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, req := range f.Require {
		depth := 1
		if req.Indirect {
			depth = 2
		}
		pkgs = append(pkgs, Package{
			Name:            req.Mod.Path,
			ResolvedVersion: req.Mod.Version,
			Constraint:      req.Mod.Version,
			ConstraintType:  ConstraintExact,
			Ecosystem:       EcosystemGo,
			Depth:           depth,
		})
	}
	return pkgs, nil
}

func (p *GoModParser) Parse(dir string) ([]Package, error) {
	files := make(map[string][]byte)
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && name == "go.mod" {
			return nil, err
		}
		if err == nil {
			files[name] = data
		}
	}
	return p.ParseFiles(files)
}
