package manifest

import (
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

type GoModParser struct{}

func NewGoModParser() *GoModParser { return &GoModParser{} }

func (p *GoModParser) Ecosystem() Ecosystem { return EcosystemGo }

func (p *GoModParser) Parse(dir string) ([]Package, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return nil, err
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
