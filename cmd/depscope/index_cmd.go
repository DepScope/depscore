package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/scanner"
	"github.com/spf13/cobra"
)

func init() {
	indexCmd.Flags().Bool("force", false, "ignore mtime cache and re-index everything")
	indexCmd.Flags().String("scope", "local", "indexing scope: local, deps, supply-chain")
	indexCmd.Flags().String("db", cache.DefaultDBPath(), "path to SQLite index database")

	indexStatusCmd.Flags().String("db", cache.DefaultDBPath(), "path to SQLite index database")

	indexCmd.AddCommand(indexStatusCmd)
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
