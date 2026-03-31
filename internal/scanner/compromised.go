package scanner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/depscope/depscope/internal/cache"
)

// CompromisedTarget is a known-bad package with an exact version or semver range.
type CompromisedTarget struct {
	Name           string
	VersionOrRange string // "1.14.1" or "^1.14.0" or ">=0.30.0,<0.31.0"
}

// Finding records a compromised dependency detected in a manifest.
type Finding struct {
	ManifestPath string // relative to scan root, e.g. "apps/web/package.json"
	PackageName  string
	Version      string // resolved version that matched
	Constraint   string // constraint from package.json (if direct)
	Relation     string // "direct" or "indirect"
	ParentChain  string // for indirect: description of how it's pulled in
}

// ParseCompromisedList parses a comma-separated inline list: "axios@1.14.1,axios@0.30.4".
func ParseCompromisedList(s string) ([]CompromisedTarget, error) {
	var targets []CompromisedTarget
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		t, err := parseTarget(entry)
		if err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no compromised packages specified")
	}
	return targets, nil
}

// parseTarget splits on the LAST @ (scoped packages like @scope/pkg@1.0.0 have @ at start).
func parseTarget(entry string) (CompromisedTarget, error) {
	at := strings.LastIndex(entry, "@")
	if at <= 0 {
		return CompromisedTarget{}, fmt.Errorf("invalid target %q: expected name@version", entry)
	}
	return CompromisedTarget{
		Name:           entry[:at],
		VersionOrRange: entry[at+1:],
	}, nil
}

// ParseCompromisedFile reads targets from a file, one per line.
// Lines starting with # and blank lines are skipped.
func ParseCompromisedFile(path string) ([]CompromisedTarget, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open compromised file: %w", err)
	}
	defer func() { _ = f.Close() }()
	return parseCompromisedReader(f)
}

func parseCompromisedReader(r io.Reader) ([]CompromisedTarget, error) {
	var targets []CompromisedTarget
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		t, err := parseTarget(line)
		if err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no compromised packages in file")
	}
	return targets, nil
}

// ScanCompromised walks root (including hidden dirs), finds all package.json +
// lockfile pairs, checks for compromised packages, logs to SQLite, and prints
// findings to w. Returns all findings.
func ScanCompromised(ctx context.Context, root string, targets []CompromisedTarget, dbPath string, w io.Writer) ([]Finding, error) {
	// Build target lookup: name -> list of version/range strings.
	targetMap := make(map[string][]string)
	for _, t := range targets {
		targetMap[t.Name] = append(targetMap[t.Name], t.VersionOrRange)
	}

	// Open cache DB.
	var cacheDB *cache.CacheDB
	if dbPath != "" {
		var err error
		cacheDB, err = cache.NewCacheDB(dbPath)
		if err != nil {
			return nil, fmt.Errorf("open cache db: %w", err)
		}
		defer func() { _ = cacheDB.Close() }()
	}

	scanID := fmt.Sprintf("compromised-%d", time.Now().UnixNano())

	// Discover all package.json files.
	manifests, err := discoverManifests(root)
	if err != nil {
		return nil, fmt.Errorf("discover manifests: %w", err)
	}

	var allFindings []Finding

	for _, m := range manifests {
		findings := processManifest(m, targetMap, root)

		// Log to SQLite.
		if cacheDB != nil {
			logManifestToDB(cacheDB, scanID, m)
			for i := range findings {
				_ = cacheDB.AddCompromisedFinding(&cache.CompromisedFinding{
					ScanID:       scanID,
					ManifestPath: findings[i].ManifestPath,
					PackageName:  findings[i].PackageName,
					Version:      findings[i].Version,
					Constraint:   findings[i].Constraint,
					Relation:     findings[i].Relation,
					ParentChain:  findings[i].ParentChain,
				})
			}
		}

		// Print findings in real-time.
		for _, f := range findings {
			tag := "DIRECT  "
			if f.Relation == "indirect" {
				tag = "INDIRECT"
			}
			detail := ""
			if f.Constraint != "" {
				detail = fmt.Sprintf("  (constraint: %s)", f.Constraint)
			}
			if f.ParentChain != "" {
				detail = fmt.Sprintf("  (via: %s)", f.ParentChain)
			}
			_, _ = fmt.Fprintf(w, "%s  %s  %s@%s%s\n", tag, f.ManifestPath, f.PackageName, f.Version, detail)
		}

		allFindings = append(allFindings, findings...)
	}

	if cacheDB != nil {
		status, _ := cacheDB.Status()
		if status != nil {
			_, _ = fmt.Fprintf(w, "\nSQLite: %d projects, %d versions, %d dependencies logged\n",
				status.Projects, status.Versions, status.Dependencies)
		}
		cFindings, _ := cacheDB.GetCompromisedFindings(scanID)
		_, _ = fmt.Fprintf(w, "Compromised findings logged: %d (scan_id: %s)\n", len(cFindings), scanID)
	}

	return allFindings, nil
}

// ScanCompromisedFromIndex queries the SQLite index for compromised packages
// instead of walking the filesystem. Much faster — requires a prior `depscope index` run.
func ScanCompromisedFromIndex(ctx context.Context, targets []CompromisedTarget, dbPath string, w io.Writer) ([]Finding, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("--db is required with --from-index")
	}

	cacheDB, err := cache.NewCacheDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open cache db: %w", err)
	}
	defer func() { _ = cacheDB.Close() }()

	// Build target lookup.
	targetMap := make(map[string][]string)
	for _, t := range targets {
		targetMap[t.Name] = append(targetMap[t.Name], t.VersionOrRange)
	}

	scanID := fmt.Sprintf("compromised-idx-%d", time.Now().UnixNano())
	var allFindings []Finding

	for name, ranges := range targetMap {
		// Determine ecosystem prefix — for now support npm, python, go, rust, php.
		for _, eco := range []string{"npm", "python", "go", "rust", "php"} {
			projectID := eco + "/" + name

			results, err := cacheDB.SearchIndexByPackageName(projectID)
			if err != nil {
				return nil, fmt.Errorf("search index for %s: %w", projectID, err)
			}

			for _, r := range results {
				for _, rng := range ranges {
					if !semverSatisfies(rng, r.Version) {
						continue
					}

					relation := "direct"
					if r.DepScope == "transitive" || r.DepScope == "installed" {
						relation = "indirect"
					}

					f := Finding{
						ManifestPath: r.ManifestRelPath,
						PackageName:  name,
						Version:      r.Version,
						Constraint:   r.Constraint,
						Relation:     relation,
					}

					tag := "DIRECT  "
					if relation == "indirect" {
						tag = "INDIRECT"
					}
					detail := ""
					if r.Constraint != "" {
						detail = fmt.Sprintf("  (constraint: %s)", r.Constraint)
					}
					_, _ = fmt.Fprintf(w, "%s  %s  %s@%s%s\n", tag, f.ManifestPath, f.PackageName, f.Version, detail)

					// Log to DB.
					_ = cacheDB.AddCompromisedFinding(&cache.CompromisedFinding{
						ScanID:       scanID,
						ManifestPath: f.ManifestPath,
						PackageName:  f.PackageName,
						Version:      f.Version,
						Constraint:   f.Constraint,
						Relation:     f.Relation,
					})

					allFindings = append(allFindings, f)
					break // one match per range is enough
				}
			}
		}
	}

	cFindings, _ := cacheDB.GetCompromisedFindings(scanID)
	_, _ = fmt.Fprintf(w, "\nCompromised findings: %d (from index, scan_id: %s)\n", len(cFindings), scanID)

	return allFindings, nil
}

// manifestInfo holds parsed data for one package.json + its lockfile.
type manifestInfo struct {
	relDir      string            // e.g., "apps/web"
	pkgJSONPath string            // absolute path to package.json
	directDeps  map[string]string // name -> constraint from package.json
	resolved    map[string]string // name -> exact version from lockfile
}

// discoverManifests walks root including hidden dirs and returns parsed manifest info.
func discoverManifests(root string) ([]manifestInfo, error) {
	var manifests []manifestInfo

	skipDirsCompromised := map[string]bool{
		".git":         true,
		"node_modules": true,
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirsCompromised[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "package.json" {
			return nil
		}

		dir := filepath.Dir(path)
		relDir, _ := filepath.Rel(root, dir)
		relDir = filepath.ToSlash(relDir)
		if relDir == "." {
			relDir = ""
		}

		// Parse package.json for direct deps.
		pkgData, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		directDeps := parseDirectDeps(pkgData)

		// Parse lockfile for resolved versions.
		resolved := make(map[string]string)
		for _, lockName := range []string{"package-lock.json", "pnpm-lock.yaml"} {
			lockPath := filepath.Join(dir, lockName)
			lockData, readErr := os.ReadFile(lockPath)
			if readErr != nil {
				continue
			}
			switch lockName {
			case "package-lock.json":
				resolved = parseLockfileResolved(lockData)
			case "pnpm-lock.yaml":
				resolved = parsePnpmResolved(lockData)
			}
			break
		}

		manifests = append(manifests, manifestInfo{
			relDir:      relDir,
			pkgJSONPath: path,
			directDeps:  directDeps,
			resolved:    resolved,
		})

		return nil
	})

	return manifests, err
}

// parseDirectDeps extracts dependencies + devDependencies from package.json.
func parseDirectDeps(data []byte) map[string]string {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}
	merged := make(map[string]string, len(pkg.Dependencies)+len(pkg.DevDependencies))
	for k, v := range pkg.Dependencies {
		merged[k] = v
	}
	for k, v := range pkg.DevDependencies {
		if _, exists := merged[k]; !exists {
			merged[k] = v
		}
	}
	return merged
}

// parseLockfileResolved extracts name -> version from package-lock.json v3.
func parseLockfileResolved(data []byte) map[string]string {
	var lock struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil
	}
	resolved := make(map[string]string)
	const prefix = "node_modules/"
	for key, entry := range lock.Packages {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		name := key[len(prefix):]
		// Handle scoped packages in nested node_modules.
		if i := strings.LastIndex(name, "node_modules/"); i >= 0 {
			name = name[i+len("node_modules/"):]
		}
		if entry.Version != "" {
			resolved[name] = entry.Version
		}
	}
	return resolved
}

// parsePnpmResolved extracts name -> version from pnpm-lock.yaml.
func parsePnpmResolved(data []byte) map[string]string {
	resolved := make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	inPackages := false
	for sc.Scan() {
		line := sc.Text()
		if line == "packages:" {
			inPackages = true
			continue
		}
		if inPackages {
			if len(line) > 0 && line[0] != ' ' && line[0] != '\'' {
				break
			}
			trimmed := strings.TrimSpace(line)
			if !strings.HasSuffix(trimmed, ":") {
				continue
			}
			trimmed = strings.TrimSuffix(trimmed, ":")
			trimmed = strings.Trim(trimmed, "'\"")
			if lastAt := strings.LastIndex(trimmed, "@"); lastAt > 0 {
				resolved[trimmed[:lastAt]] = trimmed[lastAt+1:]
			}
		}
	}
	return resolved
}

// processManifest checks a single manifest against compromised targets.
func processManifest(m manifestInfo, targetMap map[string][]string, root string) []Finding {
	var findings []Finding
	manifestRel := filepath.ToSlash(m.relDir)
	if manifestRel != "" {
		manifestRel += "/"
	}
	manifestRel += "package.json"

	// Check all resolved versions from lockfile.
	for name, version := range m.resolved {
		ranges, ok := targetMap[name]
		if !ok {
			continue
		}
		for _, r := range ranges {
			if !semverSatisfies(r, version) {
				continue
			}
			constraint, isDirect := m.directDeps[name]
			relation := "indirect"
			if isDirect {
				relation = "direct"
			}
			f := Finding{
				ManifestPath: manifestRel,
				PackageName:  name,
				Version:      version,
				Constraint:   constraint,
				Relation:     relation,
			}
			if relation == "indirect" {
				f.ParentChain = findParentChain(m, name)
			}
			findings = append(findings, f)
			break // one match per target is enough
		}
	}

	// Check direct deps without lockfile resolution (semver range could install compromised).
	for name, constraint := range m.directDeps {
		ranges, ok := targetMap[name]
		if !ok {
			continue
		}
		// Skip if already found via lockfile.
		if _, inLock := m.resolved[name]; inLock {
			continue
		}
		for _, r := range ranges {
			// Check if the compromised version satisfies the consumer's constraint.
			compromisedVer := r
			if semverSatisfies(constraint, compromisedVer) {
				findings = append(findings, Finding{
					ManifestPath: manifestRel,
					PackageName:  name,
					Version:      compromisedVer + " (possible)",
					Constraint:   constraint,
					Relation:     "direct",
				})
				break
			}
		}
	}

	return findings
}

// findParentChain tries to identify who pulls in an indirect dependency.
// Uses a simple heuristic: list direct deps that could be the parent.
func findParentChain(m manifestInfo, target string) string {
	var parents []string
	for name := range m.directDeps {
		if name != target {
			parents = append(parents, name)
		}
	}
	if len(parents) > 3 {
		parents = parents[:3]
	}
	if len(parents) == 0 {
		return "transitive"
	}
	return strings.Join(parents, " | ") + " -> " + target
}

// logManifestToDB logs the full dependency graph of a manifest to the cache.
func logManifestToDB(db *cache.CacheDB, scanID string, m manifestInfo) {
	// Create a project for the manifest itself.
	manifestProjectID := "manifest/" + strings.ReplaceAll(m.relDir, "/", ".")
	if m.relDir == "" {
		manifestProjectID = "manifest/root"
	}
	_ = db.UpsertProject(&cache.Project{
		ID:        manifestProjectID,
		Ecosystem: "npm",
		Name:      m.relDir,
	})
	_ = db.UpsertVersion(&cache.ProjectVersion{
		ProjectID:  manifestProjectID,
		VersionKey: manifestProjectID + "@" + scanID,
	})

	// Log all resolved packages as projects + versions + dependency edges.
	for name, version := range m.resolved {
		projectID := "npm/" + name
		versionKey := projectID + "@" + version
		_ = db.UpsertProject(&cache.Project{
			ID:        projectID,
			Ecosystem: "npm",
			Name:      name,
		})
		_ = db.UpsertVersion(&cache.ProjectVersion{
			ProjectID:  projectID,
			VersionKey: versionKey,
		})

		// Dependency edge: manifest -> package.
		constraint := m.directDeps[name]
		scope := "transitive"
		if constraint != "" {
			scope = "direct"
		}
		_ = db.AddVersionDependency(&cache.VersionDependency{
			ParentProjectID:        manifestProjectID,
			ParentVersionKey:       manifestProjectID + "@" + scanID,
			ChildProjectID:         projectID,
			ChildVersionConstraint: constraint,
			DepScope:               scope,
		})
	}

	// Also log direct deps that weren't resolved.
	for name, constraint := range m.directDeps {
		if _, resolved := m.resolved[name]; resolved {
			continue
		}
		projectID := "npm/" + name
		_ = db.UpsertProject(&cache.Project{
			ID:        projectID,
			Ecosystem: "npm",
			Name:      name,
		})
		_ = db.UpsertVersion(&cache.ProjectVersion{
			ProjectID:  projectID,
			VersionKey: projectID + "@" + constraint,
		})
		_ = db.AddVersionDependency(&cache.VersionDependency{
			ParentProjectID:        manifestProjectID,
			ParentVersionKey:       manifestProjectID + "@" + scanID,
			ChildProjectID:         projectID,
			ChildVersionConstraint: constraint,
			DepScope:               "direct",
		})
	}
}
