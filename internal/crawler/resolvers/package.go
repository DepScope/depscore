package resolvers

import (
	"context"
	"path/filepath"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/manifest"
)

// manifestFiles lists all known manifest filenames we look for.
var manifestFiles = map[string]bool{
	"go.mod":            true,
	"go.sum":            true,
	"package.json":      true,
	"package-lock.json": true,
	"pnpm-lock.yaml":    true,
	"bun.lock":          true,
	"pyproject.toml":    true,
	"requirements.txt":  true,
	"poetry.lock":       true,
	"uv.lock":           true,
	"Cargo.toml":        true,
	"Cargo.lock":        true,
	"composer.json":     true,
	"composer.lock":     true,
}

// PackageResolver detects and resolves package manager dependencies.
type PackageResolver struct{}

// NewPackageResolver returns a new PackageResolver.
func NewPackageResolver() *PackageResolver { return &PackageResolver{} }

// Detect scans the FileTree for known manifest files and uses the manifest
// package to parse them into dependency references.
func (r *PackageResolver) Detect(_ context.Context, contents crawler.FileTree) ([]crawler.DepRef, error) {
	// Collect manifest filenames present in the tree.
	var found []string
	for path := range contents {
		base := filepath.Base(path)
		if manifestFiles[base] {
			found = append(found, path)
		}
	}
	if len(found) == 0 {
		return nil, nil
	}

	// Group files by ecosystem and parse.
	eco, err := manifest.DetectEcosystemFromFiles(found)
	if err != nil {
		return nil, nil // unrecognized — not an error, just nothing to detect
	}

	parser := manifest.ParserFor(eco)
	if parser == nil {
		return nil, nil
	}

	// Build a fileMap with base-name keys (what ParseFiles expects).
	fileMap := make(map[string][]byte)
	for path, data := range contents {
		base := filepath.Base(path)
		if manifestFiles[base] {
			fileMap[base] = data
		}
	}

	pkgs, err := parser.ParseFiles(fileMap)
	if err != nil {
		return nil, nil // malformed manifest — gracefully return empty
	}

	var refs []crawler.DepRef
	for _, pkg := range pkgs {
		refs = append(refs, crawler.DepRef{
			Source:    crawler.DepSourcePackage,
			Name:      pkg.Name,
			Ref:       pkg.Constraint,
			Ecosystem: string(pkg.Ecosystem),
			Pinning:   graph.PinningNA,
		})
	}
	return refs, nil
}

// Resolve constructs a ResolvedDep from the DepRef without network calls.
func (r *PackageResolver) Resolve(_ context.Context, ref crawler.DepRef) (*crawler.ResolvedDep, error) {
	projectID := ref.Ecosystem + "/" + ref.Name
	versionKey := ref.Ecosystem + "/" + ref.Name + "@" + ref.Ref
	return &crawler.ResolvedDep{
		ProjectID:  projectID,
		VersionKey: versionKey,
		Contents:   nil,
	}, nil
}
