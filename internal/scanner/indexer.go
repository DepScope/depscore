package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/manifest"
	toml "github.com/pelletier/go-toml/v2"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// IndexOptions controls the behaviour of a filesystem indexing run.
type IndexOptions struct {
	Force  bool   // ignore mtime cache and re-index everything
	Scope  string // "local", "deps", "supply-chain"
	DBPath string // SQLite database path
}

// IndexResult summarises a completed indexing run.
type IndexResult struct {
	ManifestsFound   int
	ManifestsUpdated int
	ManifestsSkipped int
	PackagesTotal    int
	PackagesNew      int
	DepsTotal        int
	Errors           int
}

// indexedPackage is an intermediate representation for a package extracted
// from a manifest file before it is persisted to the database.
type indexedPackage struct {
	ProjectID  string // e.g. "npm/axios"
	VersionKey string // e.g. "npm/axios@1.7.9"
	Name       string
	Constraint string
	DepScope   string
}

// indexedDep represents a dependency edge between two packages, extracted
// from a lockfile's dependency structure.
type indexedDep struct {
	ParentProjectID  string
	ParentVersionKey string
	ChildProjectID   string
	ChildConstraint  string
}

// ---------------------------------------------------------------------------
// Manifest detection
// ---------------------------------------------------------------------------

// knownManifests maps exact base filenames to their ecosystem.
// knownManifests maps primary manifest filenames to ecosystems.
// Lockfiles are loaded as companions when their primary manifest is parsed.
var knownManifests = map[string]string{
	"package.json":    "npm",    // lockfiles: package-lock.json, pnpm-lock.yaml
	"go.mod":          "go",     // companion: go.sum
	"Cargo.toml":      "rust",   // companion: Cargo.lock
	"pyproject.toml":  "python", // companions: poetry.lock, uv.lock
	"requirements.txt": "python", // standalone pip manifest
	"composer.json":   "php",    // companion: composer.lock
}

// detectManifestEcosystem returns the ecosystem for a known manifest file,
// identified by its relative path from the project root.
func detectManifestEcosystem(relPath string) (string, bool) {
	base := filepath.Base(relPath)

	// node_modules internal package.json files are not project manifests —
	// they are handled separately as installed packages.  However, only the
	// top-level package.json in each node_modules/<pkg>/ directory is
	// interesting.  Deeply nested lock files etc. inside node_modules should
	// be indexed, but we recognise them via the normal filename map.
	if isInNodeModules(relPath) {
		// Only accept package.json inside node_modules, nothing else.
		if base == "package.json" {
			return "npm", true
		}
		return "", false
	}

	// Exact filename match.
	if eco, ok := knownManifests[base]; ok {
		return eco, true
	}

	return "", false
}

// isInNodeModules reports whether relPath lives somewhere under a node_modules
// directory.
func isInNodeModules(relPath string) bool {
	normalized := filepath.ToSlash(relPath)
	return strings.Contains(normalized, "node_modules/")
}

// ---------------------------------------------------------------------------
// Package parsing
// ---------------------------------------------------------------------------

// parseManifestPackages extracts packages from a manifest file.  For non-code
// ecosystems (config, build, terraform, actions) it returns nil when the scope
// is "local" (no package extraction needed).
func parseManifestPackages(absPath, relPath, eco string, data []byte) []indexedPackage {
	// node_modules installed package — parse name+version from JSON directly.
	if isInNodeModules(relPath) {
		return parseNodeModulesPackage(data)
	}

	switch eco {
	case "npm":
		return parseWithEcosystemParser(absPath, "npm", manifest.NewJavaScriptParser(), data, npmFileKeys(absPath))
	case "go":
		return parseWithEcosystemParser(absPath, "go", manifest.NewGoModParser(), data, goFileKeys(absPath))
	case "python":
		return parseWithEcosystemParser(absPath, "python", manifest.NewPythonParser(), data, pythonFileKeys(absPath))
	case "rust":
		return parseWithEcosystemParser(absPath, "rust", manifest.NewRustParser(), data, rustFileKeys(absPath))
	case "php":
		return parseWithEcosystemParser(absPath, "php", manifest.NewPHPParser(), data, phpFileKeys(absPath))
	case "config", "build", "terraform", "actions":
		// No packages to extract for these ecosystems at local scope.
		return nil
	default:
		return nil
	}
}

// parseNodeModulesPackage parses a node_modules/<pkg>/package.json directly.
func parseNodeModulesPackage(data []byte) []indexedPackage {
	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || pkg.Name == "" {
		return nil
	}
	projectID := "npm/" + pkg.Name
	versionKey := projectID
	if pkg.Version != "" {
		versionKey += "@" + pkg.Version
	}
	return []indexedPackage{{
		ProjectID:  projectID,
		VersionKey: versionKey,
		Name:       pkg.Name,
		Constraint: pkg.Version,
		DepScope:   "installed",
	}}
}

// parseWithEcosystemParser delegates to one of the manifest.Parser
// implementations and converts the result to indexedPackage slice.
func parseWithEcosystemParser(absPath, eco string, parser manifest.Parser, data []byte, fileKey string) []indexedPackage {
	files := map[string][]byte{fileKey: data}

	// Try to load companion files from the same directory.
	dir := filepath.Dir(absPath)
	companions := companionsFor(eco, fileKey)
	for _, c := range companions {
		cPath := filepath.Join(dir, c)
		cData, err := os.ReadFile(cPath)
		if err == nil {
			files[c] = cData
		}
	}

	pkgs, err := parser.ParseFiles(files)
	if err != nil {
		return nil
	}

	out := make([]indexedPackage, 0, len(pkgs))
	for _, p := range pkgs {
		projectID := eco + "/" + p.Name
		versionKey := projectID
		if p.ResolvedVersion != "" {
			versionKey += "@" + p.ResolvedVersion
		}
		scope := "direct"
		if p.Depth > 1 {
			scope = "transitive"
		}
		out = append(out, indexedPackage{
			ProjectID:  projectID,
			VersionKey: versionKey,
			Name:       p.Name,
			Constraint: p.Constraint,
			DepScope:   scope,
		})
	}
	return out
}

// fileKey functions map a manifest's absolute path to the key expected by
// the ecosystem parser's ParseFiles method.

func npmFileKeys(absPath string) string {
	base := filepath.Base(absPath)
	// The JS parser always wants "package.json" as key. If this IS a
	// lockfile, we still pass it under its own name.
	if base == "package.json" {
		return "package.json"
	}
	// Lock files on their own can't be parsed without a package.json, but
	// we still record the manifest.
	return base
}

func goFileKeys(absPath string) string {
	base := filepath.Base(absPath)
	if base == "go.mod" {
		return "go.mod"
	}
	return base
}

func pythonFileKeys(absPath string) string { return filepath.Base(absPath) }
func rustFileKeys(absPath string) string   { return filepath.Base(absPath) }
func phpFileKeys(absPath string) string    { return filepath.Base(absPath) }

// companionsFor returns lockfile / supplementary filenames that a parser can
// use alongside the primary manifest.
func companionsFor(eco, primaryKey string) []string {
	switch eco {
	case "npm":
		if primaryKey == "package.json" {
			return []string{"package-lock.json", "pnpm-lock.yaml", "bun.lock"}
		}
	case "python":
		if primaryKey == "pyproject.toml" {
			return []string{"poetry.lock", "uv.lock"}
		}
	case "rust":
		if primaryKey == "Cargo.toml" {
			return []string{"Cargo.lock"}
		}
	case "php":
		if primaryKey == "composer.json" {
			return []string{"composer.lock"}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Lockfile dependency-edge extraction
// ---------------------------------------------------------------------------

// extractDependencyEdges parses a lockfile's dependency structure and returns
// edges (parent→child relationships) for storage in version_dependencies.
// It also returns any packages that must exist in project_versions as
// prerequisites for the foreign-key constraint on version_dependencies.
func extractDependencyEdges(absPath, eco string) ([]indexedDep, []indexedPackage) {
	dir := filepath.Dir(absPath)
	switch eco {
	case "npm":
		return extractNpmDeps(dir)
	case "rust":
		return extractCargoDeps(dir)
	case "python":
		return extractPoetryDeps(dir)
	default:
		return nil, nil
	}
}

// extractNpmDeps parses package-lock.json (v2/v3 format with "packages" map)
// and returns dependency edges between lockfile entries.
func extractNpmDeps(dir string) ([]indexedDep, []indexedPackage) {
	lockData, err := os.ReadFile(filepath.Join(dir, "package-lock.json"))
	if err != nil {
		return nil, nil
	}

	var lock struct {
		Packages map[string]struct {
			Version         string            `json:"version"`
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(lockData, &lock); err != nil {
		return nil, nil
	}
	if len(lock.Packages) == 0 {
		return nil, nil
	}

	// Build name→version lookup from all entries in the packages map.
	versions := map[string]string{}
	for key, entry := range lock.Packages {
		name := extractNpmName(key)
		if name != "" {
			versions[name] = entry.Version
		}
	}

	var deps []indexedDep
	var pkgs []indexedPackage

	for key, entry := range lock.Packages {
		parentName := extractNpmName(key)
		isRoot := parentName == ""
		if isRoot {
			parentName = "__root__"
		}

		parentVersion := entry.Version
		parentProjectID := "npm/" + parentName
		parentVersionKey := parentProjectID
		if parentVersion != "" {
			parentVersionKey += "@" + parentVersion
		}

		// Ensure the parent entry exists in project_versions.
		pkgs = append(pkgs, indexedPackage{
			ProjectID:  parentProjectID,
			VersionKey: parentVersionKey,
			Name:       parentName,
			Constraint: parentVersion,
			DepScope:   "lockfile",
		})

		// Collect edges from both dependencies and devDependencies.
		addEdges := func(depMap map[string]string) {
			for childName, constraint := range depMap {
				deps = append(deps, indexedDep{
					ParentProjectID:  parentProjectID,
					ParentVersionKey: parentVersionKey,
					ChildProjectID:   "npm/" + childName,
					ChildConstraint:  constraint,
				})
			}
		}
		addEdges(entry.Dependencies)
		addEdges(entry.DevDependencies)
	}

	return deps, pkgs
}

// extractNpmName extracts the package name from a node_modules path key.
// For example "node_modules/axios" → "axios",
// "node_modules/@scope/pkg" → "@scope/pkg",
// "node_modules/foo/node_modules/bar" → "bar".
// The root entry "" returns "".
func extractNpmName(key string) string {
	const prefix = "node_modules/"
	if !strings.HasPrefix(key, prefix) {
		return ""
	}
	name := key[len(prefix):]
	// Handle nested: node_modules/foo/node_modules/bar → bar
	if i := strings.LastIndex(name, prefix); i >= 0 {
		name = name[i+len(prefix):]
	}
	return name
}

// extractCargoDeps parses Cargo.lock and returns dependency edges.
// Cargo.lock uses [[package]] blocks with name, version, and dependencies.
func extractCargoDeps(dir string) ([]indexedDep, []indexedPackage) {
	lockData, err := os.ReadFile(filepath.Join(dir, "Cargo.lock"))
	if err != nil {
		return nil, nil
	}

	type cargoPackage struct {
		Name         string
		Version      string
		Dependencies []string
	}

	var packages []cargoPackage
	var current *cargoPackage
	inDeps := false

	lines := strings.Split(string(lockData), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "[[package]]" {
			if current != nil {
				packages = append(packages, *current)
			}
			current = &cargoPackage{}
			inDeps = false
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(trimmed, "name = ") {
			current.Name = unquoteTOML(trimmed[len("name = "):])
			inDeps = false
		} else if strings.HasPrefix(trimmed, "version = ") {
			current.Version = unquoteTOML(trimmed[len("version = "):])
			inDeps = false
		} else if trimmed == "dependencies = [" {
			inDeps = true
		} else if inDeps && trimmed == "]" {
			inDeps = false
		} else if inDeps && strings.HasPrefix(trimmed, "\"") {
			dep := unquoteTOML(trimmed)
			// Remove trailing comma if present.
			dep = strings.TrimSuffix(dep, ",")
			dep = strings.TrimSpace(dep)
			if dep != "" {
				current.Dependencies = append(current.Dependencies, dep)
			}
		}
	}
	if current != nil {
		packages = append(packages, *current)
	}

	var deps []indexedDep
	var pkgs []indexedPackage

	for _, pkg := range packages {
		parentProjectID := "rust/" + pkg.Name
		parentVersionKey := parentProjectID + "@" + pkg.Version

		pkgs = append(pkgs, indexedPackage{
			ProjectID:  parentProjectID,
			VersionKey: parentVersionKey,
			Name:       pkg.Name,
			Constraint: pkg.Version,
			DepScope:   "lockfile",
		})

		for _, dep := range pkg.Dependencies {
			// Dependency format is either "name" or "name version"
			parts := strings.SplitN(dep, " ", 2)
			childName := parts[0]
			childConstraint := ""
			if len(parts) > 1 {
				childConstraint = parts[1]
			}

			deps = append(deps, indexedDep{
				ParentProjectID:  parentProjectID,
				ParentVersionKey: parentVersionKey,
				ChildProjectID:   "rust/" + childName,
				ChildConstraint:  childConstraint,
			})
		}
	}

	return deps, pkgs
}

// extractPoetryDeps parses poetry.lock and returns dependency edges.
// Poetry.lock uses [[package]] blocks with name, version, and [package.dependencies].
func extractPoetryDeps(dir string) ([]indexedDep, []indexedPackage) {
	lockData, err := os.ReadFile(filepath.Join(dir, "poetry.lock"))
	if err != nil {
		return nil, nil
	}

	var lock struct {
		Package []struct {
			Name         string                 `toml:"name"`
			Version      string                 `toml:"version"`
			Dependencies map[string]interface{} `toml:"dependencies"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(lockData, &lock); err != nil {
		return nil, nil
	}

	var deps []indexedDep
	var pkgs []indexedPackage

	for _, pkg := range lock.Package {
		projectID := "python/" + pkg.Name
		versionKey := projectID + "@" + pkg.Version
		pkgs = append(pkgs, indexedPackage{
			ProjectID:  projectID,
			VersionKey: versionKey,
			Name:       pkg.Name,
			Constraint: pkg.Version,
			DepScope:   "lockfile",
		})

		for depName, constraint := range pkg.Dependencies {
			constraintStr := ""
			switch v := constraint.(type) {
			case string:
				constraintStr = v
			case map[string]interface{}:
				if ver, ok := v["version"].(string); ok {
					constraintStr = ver
				}
			}
			deps = append(deps, indexedDep{
				ParentProjectID:  projectID,
				ParentVersionKey: versionKey,
				ChildProjectID:   "python/" + depName,
				ChildConstraint:  constraintStr,
			})
		}
	}

	return deps, pkgs
}

// unquoteTOML removes surrounding double quotes from a TOML value string.
func unquoteTOML(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	// Handle trailing comma after closing quote: "value",
	if len(s) >= 3 && s[0] == '"' && s[len(s)-1] == ',' && s[len(s)-2] == '"' {
		return s[1 : len(s)-2]
	}
	return s
}

// ---------------------------------------------------------------------------
// Core indexer
// ---------------------------------------------------------------------------

// RunIndex walks the filesystem from root, discovers manifest files, parses
// packages, and stores them in SQLite. It supports incremental runs by
// comparing file mtime against the stored value (unless Force is set).
func RunIndex(ctx context.Context, root string, opts IndexOptions, w io.Writer) (*IndexResult, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root path: %w", err)
	}

	db, err := cache.NewCacheDB(opts.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open index db: %w", err)
	}
	defer func() { _ = db.Close() }()

	scope := opts.Scope
	if scope == "" {
		scope = "local"
	}

	run, err := db.StartIndexRun(root, scope)
	if err != nil {
		return nil, fmt.Errorf("start index run: %w", err)
	}

	res := &IndexResult{}
	currentPaths := make(map[string]bool) // track seen manifests for pruning

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		// Check context cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip .git directories entirely.
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		if d.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(root, path)
		eco, ok := detectManifestEcosystem(relPath)
		if !ok {
			return nil
		}

		res.ManifestsFound++
		currentPaths[path] = true

		// Get file mtime.
		info, err := d.Info()
		if err != nil {
			res.Errors++
			return nil
		}
		mtime := info.ModTime().Unix()

		// Incremental check: skip if mtime matches and not forced.
		if !opts.Force {
			existing, err := db.GetIndexManifest(path)
			if err == nil && existing != nil && existing.Mtime == mtime {
				res.ManifestsSkipped++
				_, _ = fmt.Fprintf(w, "  [skip    ] %-50s (unchanged)\n", relPath)
				return nil
			}
		}

		// Read file contents.
		data, err := os.ReadFile(path)
		if err != nil {
			res.Errors++
			return nil
		}

		// Compute sha256 checksum.
		hash := sha256.Sum256(data)
		checksum := "sha256:" + hex.EncodeToString(hash[:])

		// Parse packages.
		pkgs := parseManifestPackages(path, relPath, eco, data)

		// Upsert manifest record.
		m := &cache.IndexManifest{
			AbsPath:   path,
			RelPath:   relPath,
			RootPath:  root,
			Ecosystem: eco,
			Mtime:     mtime,
			Checksum:  checksum,
		}
		if err := db.UpsertIndexManifest(m); err != nil {
			res.Errors++
			return nil
		}

		// Clear old package links and insert fresh ones.
		if err := db.ClearManifestPackages(m.ID); err != nil {
			res.Errors++
			return nil
		}

		newCount := 0
		for _, pkg := range pkgs {
			// Upsert into projects table.
			existing, _ := db.GetProject(pkg.ProjectID)
			isNew := existing == nil
			if isNew {
				newCount++
			}
			if err := db.UpsertProject(&cache.Project{
				ID:        pkg.ProjectID,
				Ecosystem: eco,
				Name:      pkg.Name,
			}); err != nil {
				res.Errors++
				continue
			}

			// Upsert into project_versions table.
			if err := db.UpsertVersion(&cache.ProjectVersion{
				ProjectID:  pkg.ProjectID,
				VersionKey: pkg.VersionKey,
			}); err != nil {
				res.Errors++
				continue
			}

			// Link package to manifest.
			if err := db.AddManifestPackage(&cache.ManifestPackage{
				ManifestID: m.ID,
				ProjectID:  pkg.ProjectID,
				VersionKey: pkg.VersionKey,
				Constraint: pkg.Constraint,
				DepScope:   pkg.DepScope,
			}); err != nil {
				res.Errors++
				continue
			}

			res.PackagesTotal++
			if isNew {
				res.PackagesNew++
			}
		}

		// Extract dependency edges from lockfiles.
		edges, edgePkgs := extractDependencyEdges(path, eco)
		// Ensure prerequisite projects/versions exist for the dep edges.
		for _, ep := range edgePkgs {
			_ = db.UpsertProject(&cache.Project{
				ID:        ep.ProjectID,
				Ecosystem: eco,
				Name:      ep.Name,
			})
			_ = db.UpsertVersion(&cache.ProjectVersion{
				ProjectID:  ep.ProjectID,
				VersionKey: ep.VersionKey,
			})
		}
		edgeCount := 0
		for _, edge := range edges {
			if err := db.AddVersionDependency(&cache.VersionDependency{
				ParentProjectID:        edge.ParentProjectID,
				ParentVersionKey:       edge.ParentVersionKey,
				ChildProjectID:         edge.ChildProjectID,
				ChildVersionConstraint: edge.ChildConstraint,
				DepScope:               "lockfile",
			}); err != nil {
				res.Errors++
				continue
			}
			edgeCount++
		}
		res.DepsTotal += edgeCount

		res.ManifestsUpdated++
		ecoTag := fmt.Sprintf("%-8s", eco)
		depInfo := ""
		if edgeCount > 0 {
			depInfo = fmt.Sprintf(", %d dep edges", edgeCount)
		}
		_, _ = fmt.Fprintf(w, "  [%s] %-50s %d packages (%d new%s)\n", ecoTag, relPath, len(pkgs), newCount, depInfo)

		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk filesystem: %w", walkErr)
	}

	// Prune manifests that no longer exist on disk.
	pruned, err := db.PruneDeletedManifests(root, currentPaths)
	if err != nil {
		res.Errors++
	}
	_ = pruned

	// Finish the run.
	run.ManifestsFound = res.ManifestsFound
	run.ManifestsUpdated = res.ManifestsUpdated
	run.ManifestsSkipped = res.ManifestsSkipped
	run.PackagesTotal = res.PackagesTotal
	run.PackagesNew = res.PackagesNew
	run.Errors = res.Errors
	if err := db.FinishIndexRun(run); err != nil {
		return nil, fmt.Errorf("finish index run: %w", err)
	}

	_, _ = fmt.Fprintf(w, "\nDone: %d manifests scanned, %d packages indexed (%d new), %d dep edges, %d errors\n",
		res.ManifestsFound, res.PackagesTotal, res.PackagesNew, res.DepsTotal, res.Errors)
	_, _ = fmt.Fprintf(w, "Index: %s\n", opts.DBPath)

	return res, nil
}
