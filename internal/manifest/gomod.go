package manifest

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type GoModParser struct{}

func NewGoModParser() *GoModParser { return &GoModParser{} }

func (p *GoModParser) Ecosystem() Ecosystem { return EcosystemGo }

func (p *GoModParser) Parse(dir string) ([]Package, error) {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var pkgs []Package
	inRequire := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Strip inline comments
		if idx := strings.Index(line, "//"); idx >= 0 {
			// keep the indirect marker before stripping — we detect it below
			// We'll check for "// indirect" before stripping
			_ = idx
		}

		if line == "require (" {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}

		// Single-line require directive
		if strings.HasPrefix(line, "require ") {
			rest := strings.TrimPrefix(line, "require ")
			pkg := parseGoRequireLine(strings.TrimSpace(rest))
			if pkg != nil {
				pkgs = append(pkgs, *pkg)
			}
			continue
		}

		if inRequire && line != "" {
			pkg := parseGoRequireLine(line)
			if pkg != nil {
				pkgs = append(pkgs, *pkg)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return pkgs, nil
}

// parseGoRequireLine parses a line like:
//
//	github.com/foo/bar v1.2.3
//	github.com/foo/bar v1.2.3 // indirect
func parseGoRequireLine(line string) *Package {
	// Strip inline comment
	indirect := strings.Contains(line, "// indirect")
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}

	fields := strings.Fields(line)
	if len(fields) < 2 {
		return nil
	}
	name := fields[0]
	version := fields[1]

	depth := 1
	if indirect {
		depth = 2
	}
	return &Package{
		Name:            name,
		ResolvedVersion: version,
		Constraint:      version,
		ConstraintType:  ConstraintExact,
		Ecosystem:       EcosystemGo,
		Depth:           depth,
	}
}
