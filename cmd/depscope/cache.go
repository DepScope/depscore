package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/depscope/depscope/internal/cache"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Cache management commands",
}

var cacheStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show cache statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCacheStatus(os.Stdout, cache.DefaultDir())
	},
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the cache",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := cache.NewDiskCache(cache.DefaultDir())
		if err := c.Clear(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, "Cache cleared.")
		return nil
	},
}

func init() {
	cacheCmd.AddCommand(cacheStatusCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	rootCmd.AddCommand(cacheCmd)
}

func runCacheStatus(w io.Writer, dir string) error {
	c := cache.NewDiskCache(dir)
	count, bytes, err := c.Status()
	if err != nil {
		fmt.Fprintf(w, "Error: %v\n", err)
		return nil
	}
	fmt.Fprintf(w, "Cache: %d entries, %.1f KB\n", count, float64(bytes)/1024)
	return nil
}
