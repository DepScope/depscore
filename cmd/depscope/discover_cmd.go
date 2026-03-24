package main

import (
	"fmt"
	"os"

	"github.com/depscope/depscope/internal/discover"
	"github.com/depscope/depscope/internal/report"
	"github.com/spf13/cobra"
)

func init() {
	discoverCmd.Flags().String("range", "", "compromised version range (required)")
	discoverCmd.Flags().String("list", "", "path to file containing project paths (one per line)")
	discoverCmd.Flags().Bool("resolve", false, "check current installable version via registry")
	discoverCmd.Flags().Bool("offline", false, "no network calls")
	discoverCmd.Flags().String("output", "text", "output format: text, json")
	discoverCmd.Flags().String("ecosystem", "", "filter to ecosystem: python, npm, rust, go, php")
	discoverCmd.Flags().Int("max-depth", 10, "max directory depth for filesystem walk")
	rootCmd.AddCommand(discoverCmd)
}

var discoverCmd = &cobra.Command{
	Use:          "discover <package> [path]",
	Short:        "Find projects affected by a compromised package",
	SilenceUsage: true,
	Long: `Search across multiple projects to find all occurrences of a package
and classify exposure against a compromised version range.

Projects are discovered via filesystem walk (default) or a project list file.
Each project is classified as: confirmed, potentially, unresolvable, or safe.

Examples:
  depscope discover litellm --range ">=1.82.7,<1.83.0" /home/me/repos
  depscope discover litellm --range ">=1.82.7,<1.83.0" --list projects.txt
  depscope discover litellm --range ">=1.82.7,<1.83.0" --offline /home/me/repos
  depscope discover litellm --range ">=1.82.7,<1.83.0" --output json /home/me/repos`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runDiscover,
}

func runDiscover(cmd *cobra.Command, args []string) error {
	pkgName := args[0]
	startPath := "."
	if len(args) >= 2 {
		startPath = args[1]
	}

	rangeStr, _ := cmd.Flags().GetString("range")
	if rangeStr == "" {
		return fmt.Errorf("--range is required")
	}

	listFile, _ := cmd.Flags().GetString("list")
	resolveFlag, _ := cmd.Flags().GetBool("resolve")
	offline, _ := cmd.Flags().GetBool("offline")
	outputFmt, _ := cmd.Flags().GetString("output")
	ecosystem, _ := cmd.Flags().GetString("ecosystem")
	maxDepth, _ := cmd.Flags().GetInt("max-depth")

	if offline && resolveFlag {
		return fmt.Errorf("--offline and --resolve are mutually exclusive")
	}

	cfg := discover.Config{
		Package:   pkgName,
		Range:     rangeStr,
		StartPath: startPath,
		ListFile:  listFile,
		Ecosystem: ecosystem,
		MaxDepth:  maxDepth,
		Resolve:   resolveFlag,
		Offline:   offline,
		Output:    outputFmt,
	}

	result, err := discover.Run(cfg)
	if err != nil {
		return err
	}

	switch outputFmt {
	case "json":
		if err := report.WriteDiscoverJSON(os.Stdout, result); err != nil {
			return fmt.Errorf("write json: %w", err)
		}
	default:
		if err := report.WriteDiscoverText(os.Stdout, result); err != nil {
			return fmt.Errorf("write text: %w", err)
		}
	}

	// Exit code 1 if any confirmed or potentially affected projects found
	summary := result.Summary()
	if summary.Confirmed > 0 || summary.Potentially > 0 {
		return exitError{1}
	}
	return nil
}
