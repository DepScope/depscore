package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/scanner"
	"github.com/depscope/depscope/internal/tui"
	"github.com/spf13/cobra"
)

func init() {
	indexCmd.Flags().Bool("force", false, "ignore mtime cache and re-index everything")
	indexCmd.Flags().String("scope", "local", "indexing scope: local, deps, supply-chain")
	indexCmd.Flags().String("db", cache.DefaultDBPath(), "path to SQLite index database")

	indexStatusCmd.Flags().String("db", cache.DefaultDBPath(), "path to SQLite index database")
	indexSearchCmd.Flags().String("db", cache.DefaultDBPath(), "path to SQLite index database")
	indexListCmd.Flags().String("db", cache.DefaultDBPath(), "path to SQLite index database")
	indexListCmd.Flags().String("ecosystem", "", "filter by ecosystem (npm, go, python, rust, php)")

	indexExploreCmd.Flags().String("db", cache.DefaultDBPath(), "path to SQLite index database")

	indexEnrichCmd.Flags().String("db", cache.DefaultDBPath(), "path to SQLite index database")
	indexEnrichCmd.Flags().Bool("force", false, "re-enrich packages that already have scores")
	indexEnrichCmd.Flags().Bool("no-cve", false, "skip CVE lookups (faster, reputation-only)")

	indexReportCmd.Flags().String("db", cache.DefaultDBPath(), "path to SQLite index database")
	indexReportCmd.Flags().String("ecosystem", "", "filter by ecosystem (npm, go, python, rust, php)")

	indexCmd.AddCommand(indexStatusCmd)
	indexCmd.AddCommand(indexSearchCmd)
	indexCmd.AddCommand(indexListCmd)
	indexCmd.AddCommand(indexExploreCmd)
	indexCmd.AddCommand(indexEnrichCmd)
	indexCmd.AddCommand(indexReportCmd)
	rootCmd.AddCommand(indexCmd)
}

var indexCmd = &cobra.Command{
	Use:   "index [path]",
	Short: "Index manifest files and packages in a project directory",
	Long: `Walk a project directory, discover manifest files (package.json, go.mod,
Cargo.toml, etc.), parse their dependencies, and store the results in a
local SQLite index for fast querying.

The index supports incremental updates — only files whose mtime has changed
since the last run are re-parsed (use --force to override).

Examples:
  depscope index                     # index current directory
  depscope index ./my-project        # index a specific path
  depscope index --force .           # re-index everything
  depscope index status              # show index statistics`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runIndex,
}

var indexStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Show index statistics",
	SilenceUsage: true,
	RunE:         runIndexStatus,
}

var indexSearchCmd = &cobra.Command{
	Use:   "search <package-name>",
	Short: "Search for a package in the index",
	Long: `Find all manifests that reference a given package name.

Examples:
  depscope index search axios
  depscope index search @scope/ui
  depscope index search golang.org/x/sync`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runIndexSearch,
}

var indexListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all indexed manifests and their package counts",
	Long: `Show every indexed manifest file with its ecosystem and package count.

Examples:
  depscope index list
  depscope index list --ecosystem npm`,
	SilenceUsage: true,
	RunE:         runIndexList,
}

func runIndex(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	force, _ := cmd.Flags().GetBool("force")
	scope, _ := cmd.Flags().GetString("scope")
	dbPath, _ := cmd.Flags().GetString("db")

	// Ensure the database parent directory exists.
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Indexing %s (scope: %s, force: %v)...\n", absRoot, scope, force)

	opts := scanner.IndexOptions{
		Force:  force,
		Scope:  scope,
		DBPath: dbPath,
	}

	result, err := scanner.RunIndex(cmd.Context(), absRoot, opts, os.Stdout)
	if err != nil {
		return err
	}

	if result.Errors > 0 {
		fmt.Fprintf(os.Stderr, "Warning: %d errors occurred during indexing\n", result.Errors)
	}

	return nil
}

func runIndexStatus(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")

	// Check if the database file exists.
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "No index database found at %s\n", dbPath)
		fmt.Fprintf(os.Stderr, "Run 'depscope index' to create one.\n")
		return nil
	}

	db, err := cache.NewCacheDB(dbPath)
	if err != nil {
		return fmt.Errorf("open index db: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Query distinct root paths from the index.
	rows, err := db.DB().Query(`SELECT DISTINCT root_path FROM index_manifests ORDER BY root_path`)
	if err != nil {
		return fmt.Errorf("query root paths: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roots []string
	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			return fmt.Errorf("scan root path: %w", err)
		}
		roots = append(roots, root)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate root paths: %w", err)
	}

	if len(roots) == 0 {
		fmt.Printf("Index database: %s\n", dbPath)
		fmt.Println("No indexed projects found.")
		return nil
	}

	fmt.Printf("Index database: %s\n\n", dbPath)

	for i, root := range roots {
		stats, err := db.IndexStats(root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to get stats for %s: %v\n", root, err)
			continue
		}

		if i > 0 {
			fmt.Println()
		}

		fmt.Printf("Root: %s\n", root)

		if stats.LastRun != nil {
			fmt.Printf("  Last run:    %s (scope: %s)\n",
				stats.LastRun.FinishedAt.Format("2006-01-02 15:04:05"), stats.LastRun.Scope)
		}

		fmt.Printf("  Manifests:   %d\n", stats.ManifestCount)
		fmt.Printf("  Packages:    %d (unique)\n", stats.PackageCount)
		fmt.Printf("  Dependencies: %d (edges)\n", stats.DependencyCount)

		if len(stats.EcosystemCounts) > 0 {
			fmt.Printf("  Ecosystems:\n")
			// Sort ecosystem names for deterministic output.
			ecos := make([]string, 0, len(stats.EcosystemCounts))
			for eco := range stats.EcosystemCounts {
				ecos = append(ecos, eco)
			}
			sort.Strings(ecos)
			for _, eco := range ecos {
				fmt.Printf("    %-12s %d manifests\n", eco, stats.EcosystemCounts[eco])
			}
		}

		if len(stats.TopPackages) > 0 {
			limit := 5
			if len(stats.TopPackages) < limit {
				limit = len(stats.TopPackages)
			}
			fmt.Printf("  Top %d packages:\n", limit)
			for _, pkg := range stats.TopPackages[:limit] {
				fmt.Printf("    %-40s (%d manifests)\n", pkg.ProjectID, pkg.Count)
			}
		}
	}

	// Enrichment stats (across all roots).
	var enriched int
	var avgScore float64
	var withCVEs int
	enrichRows, err := db.DB().Query(
		`SELECT COUNT(*), AVG(json_extract(metadata, '$.score')),
		        SUM(CASE WHEN json_extract(metadata, '$.cve_count') > 0 THEN 1 ELSE 0 END)
		 FROM project_versions WHERE metadata != '' AND metadata != '{}'`)
	if err == nil {
		defer func() { _ = enrichRows.Close() }()
		if enrichRows.Next() {
			var avgScoreNull sql.NullFloat64
			var withCVEsNull sql.NullInt64
			_ = enrichRows.Scan(&enriched, &avgScoreNull, &withCVEsNull)
			if avgScoreNull.Valid {
				avgScore = avgScoreNull.Float64
			}
			if withCVEsNull.Valid {
				withCVEs = int(withCVEsNull.Int64)
			}
		}
	}
	fmt.Println()
	if enriched > 0 {
		fmt.Printf("  Enriched:    %d packages (avg score: %.0f)\n", enriched, avgScore)
		if withCVEs > 0 {
			fmt.Printf("  With CVEs:   %d packages\n", withCVEs)
		}
	} else {
		fmt.Printf("  Enrichment:  not yet run (use 'depscope index enrich')\n")
	}

	return nil
}

func runIndexSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	dbPath, _ := cmd.Flags().GetString("db")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("no index database at %s — run 'depscope index' first", dbPath)
	}

	db, err := cache.NewCacheDB(dbPath)
	if err != nil {
		return fmt.Errorf("open index db: %w", err)
	}
	defer func() { _ = db.Close() }()

	found := 0
	for _, eco := range []string{"npm", "python", "go", "rust", "php"} {
		projectID := eco + "/" + query
		results, err := db.SearchIndexByPackageName(projectID)
		if err != nil {
			continue
		}
		for _, r := range results {
			if found == 0 {
				fmt.Printf("%-10s %-20s %-60s %s\n", "ECO", "VERSION", "MANIFEST", "SCOPE")
				fmt.Println(strings.Repeat("-", 110))
			}
			found++
			fmt.Printf("%-10s %-20s %-60s %s\n", r.Ecosystem, r.Version, r.ManifestRelPath, r.DepScope)
		}
	}

	if found == 0 {
		fmt.Printf("No results for %q in the index.\n", query)
		fmt.Println("Make sure you've run 'depscope index <path>' first.")
		return nil
	}

	fmt.Printf("\n%d result(s)\n", found)

	// Show dependency edges for the first matching ecosystem.
	for _, eco := range []string{"npm", "python", "go", "rust", "php"} {
		projectID := eco + "/" + query

		// "Depends on" — what this package depends on.
		results, err := db.SearchIndexByPackageName(projectID)
		if err != nil || len(results) == 0 {
			continue
		}
		// Use the first version found for the dependency lookup.
		versionKey := results[0].VersionKey
		children, err := db.GetVersionDependencies(projectID, versionKey)
		if err == nil && len(children) > 0 {
			fmt.Printf("\nDependencies of %s:\n", versionKey)
			for _, c := range children {
				childName := c.ChildProjectID
				if idx := strings.Index(childName, "/"); idx >= 0 {
					childName = childName[idx+1:]
				}
				fmt.Printf("  → %-20s (%s)\n", childName, c.ChildVersionConstraint)
			}
		}

		// "Depended on by" — what depends on this package.
		parents, err := db.FindDependents(projectID)
		if err == nil && len(parents) > 0 {
			fmt.Printf("\nDepended on by:\n")
			for _, p := range parents {
				parentName := p.ParentProjectID
				if idx := strings.Index(parentName, "/"); idx >= 0 {
					parentName = parentName[idx+1:]
				}
				fmt.Printf("  ← %-20s (%s)\n", parentName, p.ChildVersionConstraint)
			}
		}
		break
	}

	return nil
}

func runIndexList(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")
	ecoFilter, _ := cmd.Flags().GetString("ecosystem")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("no index database at %s — run 'depscope index' first", dbPath)
	}

	db, err := cache.NewCacheDB(dbPath)
	if err != nil {
		return fmt.Errorf("open index db: %w", err)
	}
	defer func() { _ = db.Close() }()

	q := `SELECT im.rel_path, im.ecosystem, COUNT(mp.project_id) as pkg_count
	      FROM index_manifests im
	      LEFT JOIN manifest_packages mp ON mp.manifest_id = im.id`
	var qArgs []any
	if ecoFilter != "" {
		q += ` WHERE im.ecosystem = ?`
		qArgs = append(qArgs, ecoFilter)
	}
	q += ` GROUP BY im.id ORDER BY im.ecosystem, im.rel_path`

	rows, err := db.DB().Query(q, qArgs...)
	if err != nil {
		return fmt.Errorf("query manifests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	fmt.Printf("%-10s %6s  %s\n", "ECO", "PKGS", "MANIFEST")
	fmt.Println(strings.Repeat("-", 90))

	count := 0
	for rows.Next() {
		var relPath, eco string
		var pkgCount int
		if err := rows.Scan(&relPath, &eco, &pkgCount); err != nil {
			return err
		}
		fmt.Printf("%-10s %6d  %s\n", eco, pkgCount, relPath)
		count++
	}

	fmt.Printf("\n%d manifest(s)\n", count)
	return nil
}

var indexEnrichCmd = &cobra.Command{
	Use:   "enrich",
	Short: "Enrich indexed packages with reputation scores and CVE data",
	Long: `Query package registries (npm, PyPI, crates.io, Go proxy, Packagist) and
OSV.dev to add reputation scores and CVE data to every indexed package.

This is resumable — packages already enriched are skipped unless --force is set.
Set GITHUB_TOKEN for better VCS scoring (higher rate limits).

Examples:
  depscope index enrich              # enrich all indexed packages
  depscope index enrich --force      # re-enrich everything
  depscope index enrich --no-cve     # reputation only, skip CVE lookups`,
	SilenceUsage: true,
	RunE:         runIndexEnrich,
}

func runIndexEnrich(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")
	force, _ := cmd.Flags().GetBool("force")
	noCVE, _ := cmd.Flags().GetBool("no-cve")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("no index database at %s — run 'depscope index' first", dbPath)
	}

	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken == "" {
		fmt.Fprintln(os.Stderr, "Tip: set GITHUB_TOKEN for better VCS scoring and higher rate limits")
	}

	result, err := scanner.RunEnrich(cmd.Context(), scanner.EnrichOptions{
		DBPath: dbPath,
		Force:  force,
		NoCVE:  noCVE,
	}, os.Stdout)
	if err != nil {
		return err
	}

	if result.CVEsFound > 0 {
		fmt.Fprintf(os.Stderr, "\nWarning: %d CVE(s) found across indexed packages\n", result.CVEsFound)
	}
	return nil
}

var indexReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a supply chain risk report from the enriched index",
	Long: `Produces a comprehensive risk report showing risk distribution, top vulnerable
packages, CVE summary, ecosystem breakdown, and most exposed manifests.

Examples:
  depscope index report
  depscope index report --ecosystem npm
  depscope index report --ecosystem go
  depscope index report --db /path/to/index.db`,
	SilenceUsage: true,
	RunE:         runIndexReport,
}

var indexExploreCmd = &cobra.Command{
	Use:          "explore",
	Short:        "Interactive index search TUI",
	SilenceUsage: true,
	RunE:         runIndexExplore,
}

func runIndexExplore(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("no index database at %s — run 'depscope index' first", dbPath)
	}

	db, err := cache.NewCacheDB(dbPath)
	if err != nil {
		return fmt.Errorf("open index db: %w", err)
	}
	// Don't defer close — the TUI needs the DB alive during execution.

	model := tui.NewIndexSearchModel(db)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		_ = db.Close()
		return fmt.Errorf("TUI error: %w", err)
	}
	_ = db.Close()
	return nil
}

// ---------------------------------------------------------------------------
// report subcommand
// ---------------------------------------------------------------------------

func runIndexReport(cmd *cobra.Command, args []string) error {
	dbPath, _ := cmd.Flags().GetString("db")
	ecoFilter, _ := cmd.Flags().GetString("ecosystem")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("no index database at %s — run 'depscope index' first", dbPath)
	}

	db, err := cache.NewCacheDB(dbPath)
	if err != nil {
		return fmt.Errorf("open index db: %w", err)
	}
	defer func() { _ = db.Close() }()

	raw := db.DB()

	// Build ecosystem filter args for parameterized queries.
	var ecoArgs []any
	if ecoFilter != "" {
		ecoFilter = strings.ToLower(ecoFilter)
		ecoArgs = []any{ecoFilter}
	}

	// ── Overview counts ─────────────────────────────────────────────────
	var manifestCount, packageCount, enrichedCount int
	var avgScore sql.NullFloat64

	if ecoFilter != "" {
		_ = raw.QueryRow(`SELECT COUNT(*) FROM index_manifests WHERE ecosystem = ?`, ecoFilter).Scan(&manifestCount)
		_ = raw.QueryRow(`SELECT COUNT(DISTINCT mp.project_id) FROM manifest_packages mp JOIN projects p ON p.id = mp.project_id WHERE p.ecosystem = ?`, ecoFilter).Scan(&packageCount)
		_ = raw.QueryRow(
			`SELECT COUNT(*), AVG(json_extract(pv.metadata, '$.score'))
			 FROM project_versions pv JOIN projects p ON p.id = pv.project_id
			 WHERE pv.metadata != '' AND pv.metadata != '{}' AND p.ecosystem = ?`, ecoFilter,
		).Scan(&enrichedCount, &avgScore)
	} else {
		_ = raw.QueryRow(`SELECT COUNT(*) FROM index_manifests`).Scan(&manifestCount)
		_ = raw.QueryRow(`SELECT COUNT(DISTINCT project_id) FROM manifest_packages`).Scan(&packageCount)
		_ = raw.QueryRow(
			`SELECT COUNT(*), AVG(json_extract(metadata, '$.score'))
			 FROM project_versions WHERE metadata != '' AND metadata != '{}'`,
		).Scan(&enrichedCount, &avgScore)
	}

	if enrichedCount == 0 {
		fmt.Println("No enriched packages found. Run 'depscope index enrich' first.")
		return nil
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	line := strings.Repeat("=", 59)

	fmt.Println(line)
	fmt.Println("  DEPSCOPE SUPPLY CHAIN RISK REPORT")
	fmt.Printf("  Generated: %s\n", now)
	fmt.Printf("  Index: %s\n", dbPath)
	if ecoFilter != "" {
		fmt.Printf("  Filter: %s\n", ecoFilter)
	}
	fmt.Println(line)

	enrichPct := 0
	if packageCount > 0 {
		enrichPct = enrichedCount * 100 / packageCount
	}
	avgScoreInt := 0
	if avgScore.Valid {
		avgScoreInt = int(avgScore.Float64)
	}

	fmt.Println()
	fmt.Println("OVERVIEW")
	fmt.Printf("  Manifests:    %s\n", fmtCount(manifestCount))
	fmt.Printf("  Packages:     %s unique\n", fmtCount(packageCount))
	fmt.Printf("  Enriched:     %s (%d%%)\n", fmtCount(enrichedCount), enrichPct)
	fmt.Printf("  Avg Score:    %d\n", avgScoreInt)

	// ── Risk distribution ───────────────────────────────────────────────
	type riskBucket struct {
		risk     string
		count    int
		avgScore float64
	}

	riskQuery := `SELECT json_extract(pv.metadata, '$.risk') as risk,
		        COUNT(*) as cnt,
		        AVG(json_extract(pv.metadata, '$.score')) as avg_score
		 FROM project_versions pv`
	if ecoFilter != "" {
		riskQuery += ` JOIN projects p ON p.id = pv.project_id
		 WHERE pv.metadata != '' AND pv.metadata != '{}' AND p.ecosystem = ?
		 GROUP BY risk`
	} else {
		riskQuery += ` WHERE pv.metadata != '' AND pv.metadata != '{}'
		 GROUP BY risk`
	}
	riskRows, err := raw.Query(riskQuery, ecoArgs...)
	if err != nil {
		return fmt.Errorf("query risk distribution: %w", err)
	}
	defer func() { _ = riskRows.Close() }()

	var buckets []riskBucket
	maxBucketCount := 0
	for riskRows.Next() {
		var b riskBucket
		var as sql.NullFloat64
		var riskNull sql.NullString
		if err := riskRows.Scan(&riskNull, &b.count, &as); err != nil {
			continue
		}
		if riskNull.Valid {
			b.risk = riskNull.String
		} else {
			b.risk = "UNKNOWN"
		}
		if as.Valid {
			b.avgScore = as.Float64
		}
		buckets = append(buckets, b)
		if b.count > maxBucketCount {
			maxBucketCount = b.count
		}
	}

	// Sort: CRITICAL, HIGH, MEDIUM, LOW, others.
	riskOrder := map[string]int{"CRITICAL": 0, "HIGH": 1, "MEDIUM": 2, "LOW": 3}
	sort.Slice(buckets, func(i, j int) bool {
		oi, ok := riskOrder[buckets[i].risk]
		if !ok {
			oi = 4
		}
		oj, ok2 := riskOrder[buckets[j].risk]
		if !ok2 {
			oj = 4
		}
		return oi < oj
	})

	fmt.Println()
	fmt.Println("RISK DISTRIBUTION")
	for _, b := range buckets {
		pct := 0.0
		if enrichedCount > 0 {
			pct = float64(b.count) * 100.0 / float64(enrichedCount)
		}
		fmt.Printf("  %-9s %s  %s (%.1f%%)\n",
			b.risk, riskBar(b.count, maxBucketCount, 20), fmtCount(b.count), pct)
	}

	// ── CVE summary ─────────────────────────────────────────────────────
	var pkgsWithCVEs int
	var totalCVEs sql.NullInt64
	if ecoFilter != "" {
		_ = raw.QueryRow(
			`SELECT COUNT(*), SUM(json_extract(pv.metadata, '$.cve_count'))
			 FROM project_versions pv JOIN projects p ON p.id = pv.project_id
			 WHERE pv.metadata != '' AND json_extract(pv.metadata, '$.cve_count') > 0 AND p.ecosystem = ?`, ecoFilter,
		).Scan(&pkgsWithCVEs, &totalCVEs)
	} else {
		_ = raw.QueryRow(
			`SELECT COUNT(*), SUM(json_extract(metadata, '$.cve_count'))
			 FROM project_versions
			 WHERE metadata != '' AND json_extract(metadata, '$.cve_count') > 0`,
		).Scan(&pkgsWithCVEs, &totalCVEs)
	}

	fmt.Println()
	fmt.Println("VULNERABILITIES")
	fmt.Printf("  Packages with CVEs:  %d\n", pkgsWithCVEs)
	if totalCVEs.Valid {
		fmt.Printf("  Total CVEs:          %d\n", totalCVEs.Int64)
	}

	// CVE severity breakdown — parse from metadata JSON.
	type cveSeverityCounts struct {
		critical, high, medium, low int
	}
	var sevCounts cveSeverityCounts

	sevQuery := `SELECT pv.metadata FROM project_versions pv`
	if ecoFilter != "" {
		sevQuery += ` JOIN projects p ON p.id = pv.project_id
		 WHERE pv.metadata != '' AND json_extract(pv.metadata, '$.cve_count') > 0 AND p.ecosystem = ?`
	} else {
		sevQuery += ` WHERE pv.metadata != '' AND json_extract(pv.metadata, '$.cve_count') > 0`
	}
	sevRows, err := raw.Query(sevQuery, ecoArgs...)
	if err == nil {
		defer func() { _ = sevRows.Close() }()
		for sevRows.Next() {
			var meta string
			if err := sevRows.Scan(&meta); err != nil {
				continue
			}
			var parsed struct {
				CVEs []struct {
					Severity string `json:"severity"`
				} `json:"cves"`
			}
			if json.Unmarshal([]byte(meta), &parsed) == nil {
				for _, c := range parsed.CVEs {
					switch strings.ToUpper(c.Severity) {
					case "CRITICAL":
						sevCounts.critical++
					case "HIGH":
						sevCounts.high++
					case "MEDIUM":
						sevCounts.medium++
					case "LOW":
						sevCounts.low++
					}
				}
			}
		}
	}

	if sevCounts.critical+sevCounts.high+sevCounts.medium+sevCounts.low > 0 {
		fmt.Printf("    CRITICAL:  %d\n", sevCounts.critical)
		fmt.Printf("    HIGH:      %d\n", sevCounts.high)
		fmt.Printf("    MEDIUM:    %d\n", sevCounts.medium)
		fmt.Printf("    LOW:       %d\n", sevCounts.low)
	}

	// ── Top 15 riskiest packages ────────────────────────────────────────
	type riskyPkg struct {
		projectID     string
		versionKey    string
		score         int
		risk          string
		cveCount      int
		manifestCount int
	}

	riskyQuery := `SELECT
		   pv.project_id,
		   pv.version_key,
		   json_extract(pv.metadata, '$.score') as score,
		   json_extract(pv.metadata, '$.risk') as risk,
		   COALESCE(json_extract(pv.metadata, '$.cve_count'), 0) as cve_count,
		   COUNT(DISTINCT mp.manifest_id) as manifest_count
		 FROM project_versions pv
		 JOIN manifest_packages mp ON mp.version_key = pv.version_key`
	if ecoFilter != "" {
		riskyQuery += ` JOIN projects p ON p.id = pv.project_id
		 WHERE pv.metadata != '' AND pv.metadata != '{}' AND p.ecosystem = ?`
	} else {
		riskyQuery += ` WHERE pv.metadata != '' AND pv.metadata != '{}'`
	}
	riskyQuery += ` GROUP BY pv.version_key ORDER BY score ASC LIMIT 15`
	riskyRows, err := raw.Query(riskyQuery, ecoArgs...)
	if err == nil {
		defer func() { _ = riskyRows.Close() }()

		fmt.Println()
		fmt.Println("TOP 15 RISKIEST PACKAGES")
		fmt.Printf("  %-6s %-10s %5s  %-36s %s\n", "Score", "Risk", "CVEs", "Package", "Used by")
		fmt.Printf("  %s\n", strings.Repeat("-", 68))

		for riskyRows.Next() {
			var p riskyPkg
			var scoreNull sql.NullInt64
			var riskNull sql.NullString
			if err := riskyRows.Scan(&p.projectID, &p.versionKey, &scoreNull, &riskNull, &p.cveCount, &p.manifestCount); err != nil {
				continue
			}
			if scoreNull.Valid {
				p.score = int(scoreNull.Int64)
			}
			if riskNull.Valid {
				p.risk = riskNull.String
			}
			label := p.versionKey
			if len(label) > 36 {
				label = label[:33] + "..."
			}
			fmt.Printf("  %4d   %-10s %3d   %-36s %d manifests\n",
				p.score, p.risk, p.cveCount, label, p.manifestCount)
		}
	}

	// ── Most vulnerable packages (by CVE count) ─────────────────────────
	vulnQuery := `SELECT
		   pv.version_key,
		   json_extract(pv.metadata, '$.cve_count') as cve_count,
		   json_extract(pv.metadata, '$.score') as score,
		   pv.metadata
		 FROM project_versions pv`
	if ecoFilter != "" {
		vulnQuery += ` JOIN projects p ON p.id = pv.project_id
		 WHERE json_extract(pv.metadata, '$.cve_count') > 0 AND p.ecosystem = ?`
	} else {
		vulnQuery += ` WHERE json_extract(pv.metadata, '$.cve_count') > 0`
	}
	vulnQuery += ` ORDER BY cve_count DESC LIMIT 10`
	vulnRows, err := raw.Query(vulnQuery, ecoArgs...)
	if err == nil {
		defer func() { _ = vulnRows.Close() }()

		fmt.Println()
		fmt.Println("MOST VULNERABLE PACKAGES (by CVE count)")
		fmt.Printf("  %5s  %-40s %s\n", "CVEs", "Package", "Severity")
		fmt.Printf("  %s\n", strings.Repeat("-", 62))

		for vulnRows.Next() {
			var vk string
			var cveCount int
			var scoreNull sql.NullInt64
			var metaStr string
			if err := vulnRows.Scan(&vk, &cveCount, &scoreNull, &metaStr); err != nil {
				continue
			}
			// Parse CVE severities from metadata.
			var parsed struct {
				CVEs []struct {
					Severity string `json:"severity"`
				} `json:"cves"`
			}
			sevSummary := ""
			if json.Unmarshal([]byte(metaStr), &parsed) == nil && len(parsed.CVEs) > 0 {
				sc := map[string]int{}
				for _, c := range parsed.CVEs {
					sc[strings.ToUpper(c.Severity)]++
				}
				var parts []string
				for _, sev := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"} {
					if n, ok := sc[sev]; ok && n > 0 {
						parts = append(parts, fmt.Sprintf("%d %s", n, sev))
					}
				}
				sevSummary = strings.Join(parts, ", ")
			}
			label := vk
			if len(label) > 40 {
				label = label[:37] + "..."
			}
			fmt.Printf("  %3d   %-40s %s\n", cveCount, label, sevSummary)
		}
	}

	// ── Ecosystem breakdown ─────────────────────────────────────────────
	ecoRows, err := raw.Query(
		`SELECT
		   p.ecosystem,
		   COUNT(DISTINCT pv.version_key) as pkg_count,
		   AVG(json_extract(pv.metadata, '$.score')) as avg_score,
		   SUM(CASE WHEN json_extract(pv.metadata, '$.risk') = 'CRITICAL' THEN 1 ELSE 0 END) as critical,
		   SUM(CASE WHEN json_extract(pv.metadata, '$.risk') = 'HIGH' THEN 1 ELSE 0 END) as high
		 FROM project_versions pv
		 JOIN projects p ON p.id = pv.project_id
		 WHERE pv.metadata != '' AND pv.metadata != '{}'
		 GROUP BY p.ecosystem
		 ORDER BY pkg_count DESC`)
	if err == nil {
		defer func() { _ = ecoRows.Close() }()

		fmt.Println()
		fmt.Println("ECOSYSTEM BREAKDOWN")
		fmt.Printf("  %-12s %8s  %9s  %8s  %6s\n", "Ecosystem", "Packages", "Avg Score", "CRITICAL", "HIGH")
		fmt.Printf("  %s\n", strings.Repeat("-", 53))

		for ecoRows.Next() {
			var eco string
			var pkgCount, critical, high int
			var as sql.NullFloat64
			if err := ecoRows.Scan(&eco, &pkgCount, &as, &critical, &high); err != nil {
				continue
			}
			avgS := 0
			if as.Valid {
				avgS = int(as.Float64)
			}
			fmt.Printf("  %-12s %8s  %9d  %8d  %6d\n",
				eco, fmtCount(pkgCount), avgS, critical, high)
		}
	}

	// ── Most exposed manifests ──────────────────────────────────────────
	exposedQuery := `SELECT
		   im.rel_path,
		   COUNT(CASE WHEN json_extract(pv.metadata, '$.risk') IN ('CRITICAL', 'HIGH') THEN 1 END) as risky_count,
		   MIN(json_extract(pv.metadata, '$.score')) as worst_score
		 FROM index_manifests im
		 JOIN manifest_packages mp ON mp.manifest_id = im.id
		 JOIN project_versions pv ON pv.version_key = mp.version_key
		 WHERE pv.metadata != '' AND pv.metadata != '{}'`
	if ecoFilter != "" {
		exposedQuery += ` AND im.ecosystem = ?`
	}
	exposedQuery += ` GROUP BY im.id HAVING risky_count > 0
		 ORDER BY risky_count DESC, worst_score ASC LIMIT 15`
	exposedRows, err := raw.Query(exposedQuery, ecoArgs...)
	if err == nil {
		defer func() { _ = exposedRows.Close() }()

		fmt.Println()
		fmt.Println("MOST EXPOSED MANIFESTS (highest risk concentration)")
		fmt.Printf("  %-11s %-52s %s\n", "Risk Score", "Manifest", "Risky Deps")
		fmt.Printf("  %s\n", strings.Repeat("-", 68))

		for exposedRows.Next() {
			var relPath string
			var riskyCount int
			var worstScore sql.NullInt64
			if err := exposedRows.Scan(&relPath, &riskyCount, &worstScore); err != nil {
				continue
			}
			ws := 0
			if worstScore.Valid {
				ws = int(worstScore.Int64)
			}
			label := relPath
			if len(label) > 52 {
				label = label[:49] + "..."
			}
			fmt.Printf("  %6d      %-52s %d HIGH+\n", ws, label, riskyCount)
		}
	}

	fmt.Println()
	return nil
}

// riskBar renders a horizontal bar chart segment.
func riskBar(count, maxCount int, width int) string {
	if maxCount == 0 {
		return strings.Repeat("░", width)
	}
	filled := (count * width) / maxCount
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// fmtCount formats an integer with thousand-separating commas.
func fmtCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
