package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/depscope/depscope/internal/cache"
	"github.com/spf13/cobra"
)

func init() {
	cachePruneCmd.Flags().String("older-than", "90d", "Remove entries older than this duration (e.g., 90d, 30d, 7d)")
	cacheCmd.AddCommand(cacheStatusCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	cacheCmd.AddCommand(cachePruneCmd)
	rootCmd.AddCommand(cacheCmd)
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Cache management commands",
}

var cacheStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print cache entry count and total size",
	RunE:  runCacheStatus,
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear all cached entries",
	RunE:  runCacheClear,
}

var cachePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old cached dependency data",
	RunE:  runCachePrune,
}

func runCacheStatus(_ *cobra.Command, _ []string) error {
	dc := cache.NewDiskCache(cache.DefaultDir())
	count, bytes, err := dc.Status()
	if err != nil {
		return fmt.Errorf("cache status: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Entries: %d\n", count)       //nolint:errcheck
	fmt.Fprintf(os.Stdout, "Size:    %d bytes\n", bytes)  //nolint:errcheck

	// Show SQLite CacheDB stats if it exists.
	dbPath := cache.DefaultDBPath()
	if _, statErr := os.Stat(dbPath); statErr == nil {
		db, dbErr := cache.NewCacheDB(dbPath)
		if dbErr == nil {
			defer func() { _ = db.Close() }()
			if status, sErr := db.Status(); sErr == nil {
				fmt.Fprintln(os.Stdout, "\nSQLite CacheDB:")                                       //nolint:errcheck
				fmt.Fprintf(os.Stdout, "  Projects:       %d\n", status.Projects)                   //nolint:errcheck
				fmt.Fprintf(os.Stdout, "  Versions:       %d\n", status.Versions)                   //nolint:errcheck
				fmt.Fprintf(os.Stdout, "  Dependencies:   %d\n", status.Dependencies)               //nolint:errcheck
				fmt.Fprintf(os.Stdout, "  CVE Entries:    %d\n", status.CVEEntries)                 //nolint:errcheck
				fmt.Fprintf(os.Stdout, "  Ref Resolutions: %d\n", status.RefResolutions)            //nolint:errcheck
			}
		}
	}

	return nil
}

func runCacheClear(_ *cobra.Command, _ []string) error {
	dc := cache.NewDiskCache(cache.DefaultDir())
	if err := dc.Clear(); err != nil {
		return fmt.Errorf("cache clear: %w", err)
	}
	fmt.Fprintln(os.Stdout, "Cache cleared.") //nolint:errcheck
	return nil
}

func runCachePrune(cmd *cobra.Command, _ []string) error {
	olderThan, _ := cmd.Flags().GetString("older-than")

	dur, err := parseDayDuration(olderThan)
	if err != nil {
		return fmt.Errorf("invalid --older-than value %q: %w", olderThan, err)
	}

	dbPath := cache.DefaultDBPath()
	if _, statErr := os.Stat(dbPath); statErr != nil {
		fmt.Fprintln(os.Stdout, "No SQLite cache database found; nothing to prune.") //nolint:errcheck
		return nil
	}

	db, err := cache.NewCacheDB(dbPath)
	if err != nil {
		return fmt.Errorf("open cache db: %w", err)
	}
	defer func() { _ = db.Close() }()

	pruned, err := db.Prune(dur)
	if err != nil {
		return fmt.Errorf("prune: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Pruned %d version(s) older than %s.\n", pruned, olderThan) //nolint:errcheck
	return nil
}

// parseDayDuration parses a duration string that supports "d" for days
// (e.g., "90d" -> 90 * 24 * time.Hour). Falls back to time.ParseDuration
// for standard Go durations (e.g., "2160h").
func parseDayDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	if strings.HasSuffix(s, "d") {
		numStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(numStr)
		if err != nil {
			return 0, fmt.Errorf("invalid day count %q: %w", numStr, err)
		}
		if days < 0 {
			return 0, fmt.Errorf("negative duration: %d days", days)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	return time.ParseDuration(s)
}
