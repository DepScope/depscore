package main

import (
	"fmt"
	"os"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/report"
	"github.com/depscope/depscope/internal/resolve"
	"github.com/depscope/depscope/internal/scanner"
	"github.com/spf13/cobra"
)

func init() {
	scanCmd.Flags().String("profile", "enterprise", "scoring profile: hobby, opensource, enterprise")
	scanCmd.Flags().String("config", "", "path to a depscope config file")
	scanCmd.Flags().String("output", "text", "output format: text, json, sarif")
	scanCmd.Flags().Int("depth", 10, "max dependency depth")
	scanCmd.Flags().Int("max-files", 5000, "max manifest files to fetch from remote repos")
	scanCmd.Flags().Bool("verbose", false, "verbose output")
	scanCmd.Flags().Bool("no-cve", false, "skip CVE scanning (faster, reputation-only)")
	scanCmd.Flags().StringSlice("only", nil, "filter to specific ecosystems: python, go, rust, npm, php")
	scanCmd.Flags().String("org", "", "scan all repos in a GitHub organization")
	scanCmd.Flags().String("cache-db", "", "path to SQLite cache database")
	scanCmd.Flags().StringSlice("trusted-orgs", nil, "trusted GitHub organization prefixes for org trust scoring")
	rootCmd.AddCommand(scanCmd)
}

var scanCmd = &cobra.Command{
	Use:   "scan [path-or-url]",
	Short: "Scan dependencies in a project directory or remote repository",
	Long: `Scan dependencies in a local project directory or a remote repository.

The target may be a local path (default: current directory) or a remote
repository URL. Remote URLs are resolved via the GitHub/GitLab API or
by cloning the repository over git. Set GITHUB_TOKEN / GITLAB_TOKEN
environment variables to authenticate private repositories.

Examples:
  depscope scan                                           # current directory
  depscope scan ./my-project                             # local path
  depscope scan https://github.com/org/repo              # GitHub default branch
  depscope scan https://github.com/org/repo/tree/main    # specific branch
  depscope scan https://gitlab.com/org/repo              # GitLab default branch
  depscope scan https://gitlab.com/org/repo/-/tree/v2    # specific ref
  depscope scan git@github.com:org/repo.git              # SSH URL (git clone)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runScan,
}

func runScan(cmd *cobra.Command, args []string) error {
	target := "."
	if len(args) == 1 {
		target = args[0]
	}

	cfg, err := loadConfig(cmd)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Override depth if explicitly set.
	if d, _ := cmd.Flags().GetInt("depth"); d > 0 {
		cfg.Depth = d
	}

	noCVE, _ := cmd.Flags().GetBool("no-cve")
	cacheDBPath, _ := cmd.Flags().GetString("cache-db")
	trustedOrgs, _ := cmd.Flags().GetStringSlice("trusted-orgs")

	// --org flag: scan all repos in a GitHub org and aggregate results.
	org, _ := cmd.Flags().GetString("org")
	if org != "" {
		maxFiles, _ := cmd.Flags().GetInt("max-files")
		only, _ := cmd.Flags().GetStringSlice("only")
		opts := scanner.Options{
			Profile:  cfg.Profile,
			MaxFiles: maxFiles,
			NoCVE:    noCVE,
			Only:     only,
		}
		results, err := scanner.ScanOrg(cmd.Context(), org, opts)
		if err != nil {
			return err
		}
		outputFmt, _ := cmd.Flags().GetString("output")
		anyFailed := false
		for _, r := range results {
			switch outputFmt {
			case "json":
				if err := report.WriteJSON(os.Stdout, *r); err != nil {
					return fmt.Errorf("write json: %w", err)
				}
			case "sarif":
				if err := report.WriteSARIF(os.Stdout, *r); err != nil {
					return fmt.Errorf("write sarif: %w", err)
				}
			default:
				if err := report.WriteText(os.Stdout, *r); err != nil {
					return fmt.Errorf("write text: %w", err)
				}
			}
			if !r.Passed() {
				anyFailed = true
			}
		}
		if anyFailed {
			return exitError{1}
		}
		return nil
	}

	// Use the unified crawler for both local and remote targets.
	crawlOpts := scanner.CrawlOptions{
		Profile:     cfg.Profile,
		MaxDepth:    cfg.Depth,
		NoCVE:       noCVE,
		TrustedOrgs: trustedOrgs,
		CacheDBPath: cacheDBPath,
	}

	var scanResult *core.ScanResult
	if resolve.IsRemoteURL(target) {
		crawlResult, crawlErr := scanner.CrawlURL(cmd.Context(), target, crawlOpts)
		if crawlErr != nil {
			return crawlErr
		}
		scanResult = scanner.CrawlResultToScanResult(crawlResult, cfg.Profile)
	} else {
		crawlResult, crawlErr := scanner.CrawlDir(cmd.Context(), target, crawlOpts)
		if crawlErr != nil {
			return crawlErr
		}
		scanResult = scanner.CrawlResultToScanResult(crawlResult, cfg.Profile)
	}

	// Write output.
	outputFmt, _ := cmd.Flags().GetString("output")
	switch outputFmt {
	case "json":
		if err := report.WriteJSON(os.Stdout, *scanResult); err != nil {
			return fmt.Errorf("write json: %w", err)
		}
	case "sarif":
		if err := report.WriteSARIF(os.Stdout, *scanResult); err != nil {
			return fmt.Errorf("write sarif: %w", err)
		}
	default:
		if err := report.WriteText(os.Stdout, *scanResult); err != nil {
			return fmt.Errorf("write text: %w", err)
		}
	}

	if !scanResult.Passed() {
		return exitError{1}
	}
	return nil
}

// loadConfig reads --config or --profile to build a Config.
func loadConfig(cmd *cobra.Command) (config.Config, error) {
	cfgFile, _ := cmd.Flags().GetString("config")
	if cfgFile != "" {
		return config.LoadFile(cfgFile)
	}
	profile, _ := cmd.Flags().GetString("profile")
	return config.ProfileByName(profile), nil
}
