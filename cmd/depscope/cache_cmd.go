package main

import (
	"fmt"
	"os"

	"github.com/depscope/depscope/internal/cache"
	"github.com/spf13/cobra"
)

func init() {
	cacheCmd.AddCommand(cacheStatusCmd)
	cacheCmd.AddCommand(cacheClearCmd)
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

func runCacheStatus(_ *cobra.Command, _ []string) error {
	dc := cache.NewDiskCache(cache.DefaultDir())
	count, bytes, err := dc.Status()
	if err != nil {
		return fmt.Errorf("cache status: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Entries: %d\n", count)    //nolint:errcheck
	fmt.Fprintf(os.Stdout, "Size:    %d bytes\n", bytes) //nolint:errcheck
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
