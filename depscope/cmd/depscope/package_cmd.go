package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/spf13/cobra"
)

func init() {
	packageCheckCmd.Flags().String("ecosystem", "PyPI", "ecosystem: PyPI, npm, crates.io, Go")
	packageCmd.AddCommand(packageCheckCmd)
	rootCmd.AddCommand(packageCmd)
}

var packageCmd = &cobra.Command{
	Use:   "package",
	Short: "Package-level commands",
}

var packageCheckCmd = &cobra.Command{
	Use:   "check <name>@<version>",
	Short: "Fetch and score a single package",
	Args:  cobra.ExactArgs(1),
	RunE:  runPackageCheck,
}

func runPackageCheck(cmd *cobra.Command, args []string) error {
	spec := args[0]
	name, version, err := parseNameVersion(spec)
	if err != nil {
		return err
	}

	eco, _ := cmd.Flags().GetString("ecosystem")

	fetcher, err := fetcherForEcosystem(eco)
	if err != nil {
		return err
	}

	info, err := fetcher.Fetch(name, version)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", spec, err)
	}

	fr := &registry.FetchResult{Info: info}
	pkg := manifest.Package{
		Name:            name,
		ResolvedVersion: version,
		Ecosystem:       manifest.Ecosystem(strings.ToLower(eco)),
		ConstraintType:  manifest.ConstraintExact,
		Depth:           1,
	}

	cfg := config.Enterprise()
	result := core.Score(pkg, fr, cfg.Weights)

	fmt.Fprintf(os.Stdout, "Package:  %s@%s\n", result.Name, result.Version)
	fmt.Fprintf(os.Stdout, "Score:    %d\n", result.OwnScore)
	fmt.Fprintf(os.Stdout, "Risk:     %s\n", result.OwnRisk)

	if len(result.Issues) > 0 {
		fmt.Fprintln(os.Stdout, "Issues:")
		for _, iss := range result.Issues {
			fmt.Fprintf(os.Stdout, "  [%s] %s\n", iss.Severity, iss.Message)
		}
	}

	return nil
}

// parseNameVersion splits "name@version" into its two parts.
func parseNameVersion(spec string) (name, version string, err error) {
	idx := strings.LastIndex(spec, "@")
	if idx <= 0 {
		return "", "", fmt.Errorf("invalid package spec %q: expected name@version", spec)
	}
	return spec[:idx], spec[idx+1:], nil
}

// fetcherForEcosystem returns the appropriate Fetcher for the ecosystem string.
func fetcherForEcosystem(eco string) (registry.Fetcher, error) {
	switch eco {
	case "PyPI":
		return registry.NewPyPIClient(), nil
	case "npm":
		return registry.NewNPMClient(), nil
	case "crates.io":
		return registry.NewCratesClient(), nil
	case "Go":
		return registry.NewGoProxyClient(), nil
	default:
		return nil, fmt.Errorf("unknown ecosystem %q: use PyPI, npm, crates.io, or Go", eco)
	}
}
