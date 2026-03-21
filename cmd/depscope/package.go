package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/report"
)

var packageCmd = &cobra.Command{
	Use:   "package",
	Short: "Package inspection commands",
}

var packageCheckCmd = &cobra.Command{
	Use:   "check <name==version>",
	Short: "Check a single package's reputation score",
	Args:  cobra.ExactArgs(1),
	RunE:  runPackageCheck,
}

func init() {
	packageCheckCmd.Flags().String("ecosystem", "python", "Package ecosystem: python|go|rust|npm")
	packageCheckCmd.Flags().String("profile", "enterprise", "Risk profile")
	packageCmd.AddCommand(packageCheckCmd)
	rootCmd.AddCommand(packageCmd)
}

func runPackageCheck(cmd *cobra.Command, args []string) error {
	return runPackageCheckWith(os.Stdout, nil, cmd, args)
}

func runPackageCheckWith(w io.Writer, regOverride registry.Fetcher, cmd *cobra.Command, args []string) error {
	parts := strings.SplitN(args[0], "==", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected name==version, got %q", args[0])
	}
	name, version := parts[0], parts[1]

	ecoStr, _ := cmd.Flags().GetString("ecosystem")
	eco := manifest.Ecosystem(ecoStr)
	profileName, _ := cmd.Flags().GetString("profile")
	cfg := config.ProfileByName(profileName)

	reg := regOverride
	if reg == nil {
		reg = buildRegistryFetcher(eco)
	}

	info, err := reg.Fetch(name, version)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	pkg := manifest.Package{
		Name:            name,
		ResolvedVersion: version,
		ConstraintType:  manifest.ConstraintExact,
		Ecosystem:       eco,
		Depth:           1,
	}
	fr := &core.FetchResult{Info: info}
	result := core.Score(pkg, fr, cfg.Weights)

	scanResult := core.ScanResult{
		Profile:       cfg.Profile,
		PassThreshold: cfg.PassThreshold,
		DirectDeps:    1,
		Packages:      []core.PackageResult{result},
		AllIssues:     result.Issues,
	}
	report.WriteText(w, scanResult)
	return nil
}
