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

		res.ManifestsUpdated++
		ecoTag := fmt.Sprintf("%-8s", eco)
		_, _ = fmt.Fprintf(w, "  [%s] %-50s %d packages (%d new)\n", ecoTag, relPath, len(pkgs), newCount)

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

	_, _ = fmt.Fprintf(w, "\nDone: %d manifests scanned, %d packages indexed (%d new), %d errors\n",
		res.ManifestsFound, res.PackagesTotal, res.PackagesNew, res.Errors)
	_, _ = fmt.Fprintf(w, "Index: %s\n", opts.DBPath)

	return res, nil
}
