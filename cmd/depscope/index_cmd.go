package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/scanner"
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

	indexCmd.AddCommand(indexStatusCmd)
	indexCmd.AddCommand(indexSearchCmd)
	indexCmd.AddCommand(indexListCmd)
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
