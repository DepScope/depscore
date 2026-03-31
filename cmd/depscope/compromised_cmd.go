package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/scanner"
	"github.com/spf13/cobra"
)

func init() {
	compromisedCmd.Flags().String("packages", "", `comma-separated compromised packages: "axios@1.14.1,axios@0.30.4"`)
	compromisedCmd.Flags().String("file", "", "path to file with compromised packages (one per line)")
	compromisedCmd.Flags().String("db", cache.DefaultDBPath(), "path to SQLite database for logging")
	compromisedCmd.Flags().Bool("from-index", false, "query the SQLite index instead of walking the filesystem (requires prior 'depscope index' run)")
	rootCmd.AddCommand(compromisedCmd)
}

var compromisedCmd = &cobra.Command{
	Use:   "compromised [path]",
	Short: "Scan for known compromised packages in dependency trees",
	Long: `Walk a directory tree (including hidden directories) searching for npm/yarn/pnpm
manifests. For each package.json + lockfile found, check whether any direct or
transitive dependency matches a known compromised package version.

Results are printed in real-time and logged to SQLite for further analysis.

Supply compromised packages inline or via file:

  depscope compromised . --packages "axios@1.14.1,axios@0.30.4"
  depscope compromised /src --file compromised.txt

Use --from-index to query a pre-built index (much faster, all ecosystems):

  depscope index ~                                           # build index first
  depscope compromised --from-index --packages "axios@1.14.1"  # instant query

The file format is one entry per line (# for comments):

  # Exact versions
  axios@1.14.1
  axios@0.30.4
  # Semver ranges
  event-stream@>=3.3.4,<3.3.7
  ua-parser-js@^0.7.29`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runCompromised,
}

func runCompromised(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}

	// Resolve to absolute path.
	absRoot, err := resolveAbsPath(root)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Parse targets from --packages and/or --file (can combine both).
	var targets []scanner.CompromisedTarget

	if pkgs, _ := cmd.Flags().GetString("packages"); pkgs != "" {
		t, err := scanner.ParseCompromisedList(pkgs)
		if err != nil {
			return err
		}
		targets = append(targets, t...)
	}

	if file, _ := cmd.Flags().GetString("file"); file != "" {
		t, err := scanner.ParseCompromisedFile(file)
		if err != nil {
			return err
		}
		targets = append(targets, t...)
	}

	if len(targets) == 0 {
		return fmt.Errorf("specify compromised packages via --packages or --file")
	}

	dbPath, _ := cmd.Flags().GetString("db")
	fromIndex, _ := cmd.Flags().GetBool("from-index")

	var findings []scanner.Finding

	if fromIndex {
		fmt.Fprintf(os.Stderr, "Searching index for %d compromised package(s)...\n\n", len(targets))
		findings, err = scanner.ScanCompromisedFromIndex(cmd.Context(), targets, dbPath, os.Stdout)
	} else {
		fmt.Fprintf(os.Stderr, "Scanning %s for %d compromised package(s)...\n\n", absRoot, len(targets))
		findings, err = scanner.ScanCompromised(cmd.Context(), absRoot, targets, dbPath, os.Stdout)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "\nScan complete: %d finding(s)\n", len(findings))

	if len(findings) > 0 {
		return exitError{1}
	}
	return nil
}

func resolveAbsPath(p string) (string, error) {
	if p == "." || p == "" {
		return os.Getwd()
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return abs, nil
}
