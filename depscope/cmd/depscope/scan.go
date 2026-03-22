package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/report"
	"github.com/depscope/depscope/internal/resolve"
	"github.com/spf13/cobra"
)

func init() {
	scanCmd.Flags().String("profile", "enterprise", "scoring profile: hobby, opensource, enterprise")
	scanCmd.Flags().String("config", "", "path to a depscope config file")
	scanCmd.Flags().String("output", "text", "output format: text, json, sarif")
	scanCmd.Flags().Int("depth", 10, "max dependency depth")
	scanCmd.Flags().Int("max-files", 5000, "max manifest files to fetch from remote repos")
	scanCmd.Flags().Bool("verbose", false, "verbose output")
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

	var pkgs []manifest.Package

	if resolve.IsRemoteURL(target) {
		// Remote URL path
		maxFiles, _ := cmd.Flags().GetInt("max-files")
		resolver := resolve.DetectResolver(target, resolve.DetectOptions{
			GitHubToken: os.Getenv("GITHUB_TOKEN"),
			GitLabToken: os.Getenv("GITLAB_TOKEN"),
			MaxFiles:    maxFiles,
		})
		files, cleanup, err := resolver.Resolve(cmd.Context(), target)
		defer cleanup()
		if err != nil {
			return fmt.Errorf("resolve remote: %w", err)
		}

		// Group files by directory and parse each group
		groups := groupByDirectory(files)
		for dir, group := range groups {
			filenames := make([]string, 0, len(group))
			fileMap := make(map[string][]byte)
			for _, f := range group {
				name := filepath.Base(f.Path)
				filenames = append(filenames, name)
				fileMap[name] = f.Content
			}
			eco, err := manifest.DetectEcosystemFromFiles(filenames)
			if err != nil {
				log.Printf("warning: skipping %s: %v", dir, err)
				continue
			}
			parsed, err := manifest.ParserFor(eco).ParseFiles(fileMap)
			if err != nil {
				return fmt.Errorf("parse %s: %w", dir, err)
			}
			pkgs = append(pkgs, parsed...)
		}
	} else {
		// Local path
		eco, err := manifest.DetectEcosystem(target)
		if err != nil {
			return fmt.Errorf("detect ecosystem: %w", err)
		}
		pkgs, err = manifest.ParserFor(eco).Parse(target)
		if err != nil {
			return fmt.Errorf("parse manifest: %w", err)
		}
	}

	// Determine ecosystem for fetchers. When scanning multiple groups we may
	// have packages from different ecosystems; for now we build fetchers for
	// all supported ecosystems.
	fetchers := buildAllFetchers()

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

// buildAllFetchers returns a FetchersByEcosystem map populated with all
// supported registry clients. Used when scanning remote repos that may
// contain multiple ecosystems.
func buildAllFetchers() registry.FetchersByEcosystem {
	return registry.FetchersByEcosystem{
		"PyPI":      registry.NewPyPIClient(),
		"npm":       registry.NewNPMClient(),
		"crates.io": registry.NewCratesClient(),
		"Go":        registry.NewGoProxyClient(),
	}
}

// groupByDirectory groups ManifestFiles by their parent directory.
func groupByDirectory(files []resolve.ManifestFile) map[string][]resolve.ManifestFile {
	groups := make(map[string][]resolve.ManifestFile)
	for _, f := range files {
		dir := filepath.Dir(f.Path)
		groups[dir] = append(groups[dir], f)
	}
	return groups
}
