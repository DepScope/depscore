package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/depscope/depscope/internal/cache"
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
	scanCmd.Flags().String("manifest", "", "Explicit manifest file to scan (e.g., poetry.lock)")
	scanCmd.Flags().Bool("verbose", false, "Show detailed package metadata")
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
func scanDeps(w io.Writer, dir string, cfg config.Config, outputFmt string, manifestFile string, verbose bool, regOverride registry.Fetcher) error {
	// Warn if GITHUB_TOKEN is not set
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "warning: GITHUB_TOKEN not set — repo health scoring disabled")
	}

	var eco manifest.Ecosystem
	var pkgs []manifest.Package
	var err error

	if manifestFile != "" {
		// Explicit manifest file: detect parser from filename
		eco, pkgs, err = parseManifestFile(manifestFile)
		if err != nil {
			return fmt.Errorf("manifest parse: %w", err)
		}
	} else {
		eco, err = manifest.DetectEcosystem(dir)
		if err != nil {
			return fmt.Errorf("manifest detection: %w", err)
		}

		pkgs, err = manifest.ParserFor(eco).Parse(dir)
		if err != nil {
			return fmt.Errorf("manifest parse: %w", err)
		}
	}

	_ = verbose // reserved for future detailed output

	reg := regOverride
	if reg == nil {
		reg = buildRegistryFetcher(eco)
		// Wrap with disk cache
		diskCache := cache.NewDiskCache(cache.DefaultDir())
		reg = registry.NewCachedFetcher(reg, diskCache, cfg.Cache.MetadataHours)
	}

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

	// Build dependency graph and compute correct depths before scoring
	deps := manifest.BuildDepsMap(pkgs)
	pkgs = manifest.ComputeDepths(pkgs, deps)

	// Filter by MaxDepth if configured
	maxDepthReached := false
	if cfg.MaxDepth > 0 {
		var filtered []manifest.Package
		for _, pkg := range pkgs {
			if pkg.Depth <= cfg.MaxDepth {
				filtered = append(filtered, pkg)
			} else {
				maxDepthReached = true
			}
		}
		pkgs = filtered
	}

	if maxDepthReached {
		fmt.Fprintf(os.Stderr, "warning: depth limit %d reached — some transitive dependencies were excluded\n", cfg.MaxDepth)
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
			coreFR = &core.FetchResult{Info: regFR.Info, RepoInfo: regFR.RepoInfo, Vulns: regFR.Vulns, Err: regFR.Err}
		}
		results = append(results, core.Score(pkg, coreFR, cfg.Weights))
	}

	results = core.Propagate(results, deps)

	scanResult := core.ScanResult{
		Profile:         cfg.Profile,
		PassThreshold:   cfg.PassThreshold,
		DirectDeps:      directCount,
		TransitiveDeps:  transitiveCount,
		MaxDepthReached: maxDepthReached,
		Packages:        results,
		Deps:            deps,
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

// parseManifestFile detects the ecosystem from a manifest filename and parses it.
func parseManifestFile(path string) (manifest.Ecosystem, []manifest.Package, error) {
	base := filepath.Base(path)
	dir := filepath.Dir(path)
	switch {
	case base == "poetry.lock" || base == "uv.lock" || base == "requirements.txt":
		p := manifest.NewPythonParser()
		pkgs, err := p.ParseFile(path)
		return manifest.EcosystemPython, pkgs, err
	case base == "package-lock.json" || base == "package.json" || base == "pnpm-lock.yaml" || base == "bun.lock":
		p := manifest.NewJavaScriptParser()
		pkgs, err := p.Parse(dir)
		return manifest.EcosystemNPM, pkgs, err
	case base == "go.mod":
		p := manifest.NewGoModParser()
		pkgs, err := p.Parse(dir)
		return manifest.EcosystemGo, pkgs, err
	case base == "Cargo.toml" || base == "Cargo.lock":
		p := manifest.NewRustParser()
		pkgs, err := p.Parse(dir)
		return manifest.EcosystemRust, pkgs, err
	default:
		return "", nil, fmt.Errorf("unrecognized manifest file: %s", base)
	}
}

func runScan(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	// Handle git URLs
	if isGitURL(dir) {
		tmpDir, err := cloneRepo(dir)
		if err != nil {
			return fmt.Errorf("clone %s: %w", dir, err)
		}
		defer os.RemoveAll(tmpDir)
		dir = tmpDir
	}

	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	if depthFlag, _ := cmd.Flags().GetInt("depth"); depthFlag > 0 {
		cfg.MaxDepth = depthFlag
	}

	outputFmt, _ := cmd.Flags().GetString("output")
	manifestFile, _ := cmd.Flags().GetString("manifest")
	verbose, _ := cmd.Flags().GetBool("verbose")
	return scanDeps(os.Stdout, dir, cfg, outputFmt, manifestFile, verbose, nil)
}

func isGitURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "git@")
}

func cloneRepo(url string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "depscope-scan-*")
	if err != nil {
		return "", err
	}

	cmd := exec.Command("git", "clone", "--depth=1", url, tmpDir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("git clone failed: %w", err)
	}
	return tmpDir, nil
}
