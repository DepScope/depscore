package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/report"
	"github.com/depscope/depscope/internal/vcs"
	"github.com/depscope/depscope/internal/vuln"
)

var scanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Scan a project's dependencies for supply chain risk",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runScan,
}

func init() {
	scanCmd.Flags().String("profile", "enterprise", "Risk profile: hobby|opensource|enterprise")
	scanCmd.Flags().String("config", "", "Path to depscope.yaml config file")
	scanCmd.Flags().String("output", "text", "Output format: text|json|sarif")
	scanCmd.Flags().Int("depth", 0, "Max dependency depth (0 = profile default)")
	rootCmd.AddCommand(scanCmd)
}

func loadConfig(cmd *cobra.Command) (config.Config, error) {
	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath != "" {
		return config.LoadFile(cfgPath)
	}
	profileName, _ := cmd.Flags().GetString("profile")
	return config.ProfileByName(profileName), nil
}

func buildRegistryFetcher(eco manifest.Ecosystem) registry.Fetcher {
	switch eco {
	case manifest.EcosystemPython:
		return registry.NewPyPIClient()
	case manifest.EcosystemNPM:
		return registry.NewNPMClient()
	case manifest.EcosystemRust:
		return registry.NewCratesClient()
	case manifest.EcosystemGo:
		return registry.NewGoProxyClient()
	default:
		return registry.NewPyPIClient()
	}
}

func buildVulnSources(cfg config.Config) []vuln.Source {
	var sources []vuln.Source
	if cfg.Vuln.OSV {
		sources = append(sources, vuln.NewOSVClient())
	}
	if cfg.Vuln.NVD {
		apiKey := os.Getenv("NVD_API_KEY")
		sources = append(sources, vuln.NewNVDClient(apiKey))
	}
	return sources
}

// scanDeps is the core scan logic, extracted for testability.
// It accepts an io.Writer and optional registry.Fetcher override (for testing).
func scanDeps(w io.Writer, dir string, cfg config.Config, outputFmt string, regOverride registry.Fetcher) error {
	eco, err := manifest.DetectEcosystem(dir)
	if err != nil {
		return fmt.Errorf("manifest detection: %w", err)
	}

	pkgs, err := manifest.ParserFor(eco).Parse(dir)
	if err != nil {
		return fmt.Errorf("manifest parse: %w", err)
	}

	reg := regOverride
	if reg == nil {
		reg = buildRegistryFetcher(eco)
	}

	token := os.Getenv("GITHUB_TOKEN")
	var vcsClient vcs.Client
	if token != "" {
		vcsClient = vcs.NewGitHubClient(token)
	}

	vulnSources := buildVulnSources(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fetchResults, err := registry.FetchAll(ctx, pkgs, reg, vcsClient, vulnSources,
		registry.FetchOptions{Concurrency: 20})
	if err != nil {
		return err
	}

	// Count direct vs transitive
	directCount, transitiveCount := 0, 0
	for _, pkg := range pkgs {
		if pkg.Depth <= 1 {
			directCount++
		} else {
			transitiveCount++
		}
	}

	var results []core.PackageResult
	for _, pkg := range pkgs {
		regFR := fetchResults[pkg.Key()]
		var coreFR *core.FetchResult
		if regFR != nil {
			coreFR = &core.FetchResult{Info: regFR.Info, RepoInfo: regFR.RepoInfo, Err: regFR.Err}
		}
		results = append(results, core.Score(pkg, coreFR, cfg.Weights))
	}

	deps := manifest.BuildDepsMap(pkgs)
	results = core.Propagate(results, deps)

	scanResult := core.ScanResult{
		Profile:        cfg.Profile,
		PassThreshold:  cfg.PassThreshold,
		DirectDeps:     directCount,
		TransitiveDeps: transitiveCount,
		Packages:       results,
	}
	for _, r := range results {
		scanResult.AllIssues = append(scanResult.AllIssues, r.Issues...)
	}

	switch outputFmt {
	case "json":
		if err := report.WriteJSON(w, scanResult); err != nil {
			return fmt.Errorf("write JSON report: %w", err)
		}
	case "sarif":
		if err := report.WriteSARIF(w, scanResult); err != nil {
			return fmt.Errorf("write SARIF report: %w", err)
		}
	default:
		report.WriteText(w, scanResult)
	}

	if !scanResult.Passed() {
		return exitError{1}
	}
	return nil
}

func runScan(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	outputFmt, _ := cmd.Flags().GetString("output")
	return scanDeps(os.Stdout, dir, cfg, outputFmt, nil)
}
