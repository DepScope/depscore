package resolvers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"

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

// depsFileKey is a synthetic filename used to pass dependency children
// through the FileTree for recursive crawling.
const depsFileKey = "__depscope_deps__.json"

// PackageResolver detects and resolves package manager dependencies.
// It uses BuildDepsMap to build a parent→children graph and only returns
// direct deps from Detect. Resolve returns synthetic FileTrees for packages
// with children, enabling the crawler's BFS to produce a proper tree.
type PackageResolver struct {
	mu       sync.Mutex
	depsMap  map[string][]manifest.Package // parent name → child packages
	allPkgs  map[string]manifest.Package   // name → package (for lookup)
}

// NewPackageResolver returns a new PackageResolver.
func NewPackageResolver() *PackageResolver {
	return &PackageResolver{
		depsMap: make(map[string][]manifest.Package),
		allPkgs: make(map[string]manifest.Package),
	}
}

// Detect scans the FileTree for known manifest files and uses the manifest
// package to parse them into dependency references.
// Only returns direct dependencies (depth ≤ 1). Transitive deps are returned
// via Resolve's synthetic FileTree.
func (r *PackageResolver) Detect(_ context.Context, contents crawler.FileTree) ([]crawler.DepRef, error) {
	// Check for synthetic deps file (recursive call from Resolve).
	if data, ok := contents[depsFileKey]; ok {
		return r.detectFromSynthetic(data)
	}

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

	// Build the dependency map and store packages for Resolve.
	depsMap := manifest.BuildDepsMap(pkgs)
	r.mu.Lock()
	for _, pkg := range pkgs {
		r.allPkgs[pkg.Name] = pkg
	}
	for parent, children := range depsMap {
		var childPkgs []manifest.Package
		for _, childName := range children {
			if p, ok := r.allPkgs[childName]; ok {
				childPkgs = append(childPkgs, p)
			}
		}
		r.depsMap[parent] = childPkgs
	}
	r.mu.Unlock()

	// Only return direct deps (depth ≤ 1, or packages not depended on by others).
	isDep := make(map[string]bool)
	for _, children := range depsMap {
		for _, child := range children {
			isDep[child] = true
		}
	}

	var refs []crawler.DepRef
	for _, pkg := range pkgs {
		if isDep[pkg.Name] {
			continue // skip transitive deps — they'll come from Resolve
		}
		refs = append(refs, pkgToDepRef(pkg))
	}

	// If no dep map info (no lockfile), all packages are direct.
	if len(depsMap) == 0 {
		refs = nil
		for _, pkg := range pkgs {
			refs = append(refs, pkgToDepRef(pkg))
		}
	}

	return refs, nil
}

// detectFromSynthetic parses synthetic dep refs from a JSON deps file.
func (r *PackageResolver) detectFromSynthetic(data []byte) ([]crawler.DepRef, error) {
	var refs []crawler.DepRef
	if err := json.Unmarshal(data, &refs); err != nil {
		return nil, nil
	}
	return refs, nil
}

// Resolve constructs a ResolvedDep from the DepRef.
// If the package has children in the dep map, returns a synthetic FileTree
// so the crawler recurses into them.
func (r *PackageResolver) Resolve(_ context.Context, ref crawler.DepRef) (*crawler.ResolvedDep, error) {
	projectID := ref.Ecosystem + "/" + ref.Name
	version := ref.Ref
	versionKey := ref.Ecosystem + "/" + ref.Name + "@" + version

	dep := &crawler.ResolvedDep{
		ProjectID:  projectID,
		VersionKey: versionKey,
		Semver:     version,
		Contents:   nil,
	}

	// Check if this package has children in the dep map.
	r.mu.Lock()
	children := r.depsMap[ref.Name]
	r.mu.Unlock()

	if len(children) > 0 {
		// Create synthetic FileTree with children as JSON DepRefs.
		var childRefs []crawler.DepRef
		for _, child := range children {
			childRefs = append(childRefs, pkgToDepRef(child))
		}
		data, err := json.Marshal(childRefs)
		if err == nil {
			dep.Contents = crawler.FileTree{depsFileKey: data}
		}
	}

	return dep, nil
}

func pkgToDepRef(pkg manifest.Package) crawler.DepRef {
	version := pkg.ResolvedVersion
	if version == "" {
		version = pkg.Constraint
	}
	pinning := graph.PinningNA
	if pkg.ResolvedVersion != "" {
		pinning = graph.PinningExactVersion
	} else if pkg.Constraint != "" {
		pinning = graph.PinningSemverRange
	}
	return crawler.DepRef{
		Source:    crawler.DepSourcePackage,
		Name:      pkg.Name,
		Ref:       version,
		Ecosystem: string(pkg.Ecosystem),
		Pinning:   pinning,
	}
}
