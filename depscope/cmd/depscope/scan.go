package main

import (
	"fmt"
	"os"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/report"
	"github.com/spf13/cobra"
)

func init() {
	scanCmd.Flags().String("profile", "enterprise", "scoring profile: hobby, opensource, enterprise")
	scanCmd.Flags().String("config", "", "path to a depscope config file")
	scanCmd.Flags().String("output", "text", "output format: text, json, sarif")
	scanCmd.Flags().Int("depth", 0, "max dependency depth (0 = use profile default)")
	scanCmd.Flags().Bool("verbose", false, "verbose output")
	rootCmd.AddCommand(scanCmd)
}

var scanCmd = &cobra.Command{
	Use:   "scan [dir]",
	Short: "Scan dependencies in a project directory",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runScan,
}

func runScan(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	cfg, err := loadConfig(cmd)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Override depth if explicitly set.
	if d, _ := cmd.Flags().GetInt("depth"); d > 0 {
		cfg.Depth = d
	}

	// Detect ecosystem and parse manifest.
	eco, err := manifest.DetectEcosystem(dir)
	if err != nil {
		return fmt.Errorf("detect ecosystem: %w", err)
	}

	parser := manifest.ParserFor(eco)
	pkgs, err := parser.Parse(dir)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	// Build fetchers map for the detected ecosystem.
	fetchers := buildFetchers(eco)

	// Fetch registry data for all packages.
	fetchResults := registry.FetchAll(pkgs, fetchers, int64(cfg.Concurrency))

	// Score each package.
	scored := make([]core.PackageResult, 0, len(pkgs))
	for _, pkg := range pkgs {
		fr := fetchResults[pkg.Key()]
		result := core.Score(pkg, fr, cfg.Weights)
		scored = append(scored, result)
	}

	// Propagate transitive risk.
	depsMap := manifest.BuildDepsMap(pkgs)
	scored = core.Propagate(scored, depsMap)

	// Count direct vs transitive.
	directCount, transitiveCount := 0, 0
	for _, pkg := range pkgs {
		if pkg.Depth <= 1 {
			directCount++
		} else {
			transitiveCount++
		}
	}

	// Collect all issues.
	var allIssues []core.Issue
	for _, r := range scored {
		allIssues = append(allIssues, r.Issues...)
	}

	scanResult := core.ScanResult{
		Profile:        cfg.Profile,
		PassThreshold:  cfg.PassThreshold,
		DirectDeps:     directCount,
		TransitiveDeps: transitiveCount,
		Packages:       scored,
		AllIssues:      allIssues,
	}

	// Write output.
	outputFmt, _ := cmd.Flags().GetString("output")
	switch outputFmt {
	case "json":
		if err := report.WriteJSON(os.Stdout, scanResult); err != nil {
			return fmt.Errorf("write json: %w", err)
		}
	case "sarif":
		if err := report.WriteSARIF(os.Stdout, scanResult); err != nil {
			return fmt.Errorf("write sarif: %w", err)
		}
	default:
		if err := report.WriteText(os.Stdout, scanResult); err != nil {
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

// buildFetchers constructs the FetchersByEcosystem map for the given ecosystem.
func buildFetchers(eco manifest.Ecosystem) registry.FetchersByEcosystem {
	fetchers := registry.FetchersByEcosystem{}
	switch eco {
	case manifest.EcosystemPython:
		fetchers["PyPI"] = registry.NewPyPIClient()
	case manifest.EcosystemNPM:
		fetchers["npm"] = registry.NewNPMClient()
	case manifest.EcosystemRust:
		fetchers["crates.io"] = registry.NewCratesClient()
	case manifest.EcosystemGo:
		fetchers["Go"] = registry.NewGoProxyClient()
	}
	return fetchers
}
