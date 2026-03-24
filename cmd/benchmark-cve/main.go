package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vuln"
)

type CVEResult struct {
	Ecosystem  string          `json:"ecosystem"`
	Name       string          `json:"name"`
	Version    string          `json:"version"`
	VulnCount  int             `json:"vuln_count"`
	Critical   int             `json:"critical"`
	High       int             `json:"high"`
	Medium     int             `json:"medium"`
	Low        int             `json:"low"`
	Vulns      []VulnSummary   `json:"vulns,omitempty"`
	Error      string          `json:"error,omitempty"`
}

type VulnSummary struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: benchmark-cve <ecosystem> <packages-file>\n")
		fmt.Fprintf(os.Stderr, "  ecosystem: pypi, npm, crates, go, packagist\n")
		os.Exit(1)
	}

	eco := os.Args[1]
	file := os.Args[2]

	// Map CLI ecosystem name to OSV ecosystem name
	osvEco := map[string]string{
		"pypi":      "PyPI",
		"npm":       "npm",
		"crates":    "crates.io",
		"go":        "Go",
		"packagist": "Packagist",
	}[eco]
	if osvEco == "" {
		fmt.Fprintf(os.Stderr, "unknown ecosystem: %s\n", eco)
		os.Exit(1)
	}

	// Get a fetcher to resolve latest versions
	var fetcher registry.Fetcher
	switch eco {
	case "pypi":
		fetcher = registry.NewPyPIClient()
	case "npm":
		fetcher = registry.NewNPMClient()
	case "crates":
		fetcher = registry.NewCratesClient()
	case "go":
		fetcher = registry.NewGoProxyClient()
	case "packagist":
		fetcher = registry.NewPackagistClient()
	}

	f, err := os.Open(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", file, err)
		os.Exit(1)
	}
	defer f.Close()

	osvClient := vuln.NewOSVClient()
	var results []CVEResult

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name == "" {
			continue
		}

		// Query OSV for all known vulns for this package (latest version)
		// First try to get a version from the registry for more precise results
		version := ""
		if fetcher != nil {
			info, err := fetcher.Fetch(name, "")
			if err == nil && info != nil {
				version = info.Version
			}
		}

		r := CVEResult{
			Ecosystem: eco,
			Name:      name,
			Version:   version,
		}

		// If no version resolved, still query OSV without version
		// (returns all known vulns across all versions)

		findings, err := osvClient.Query(osvEco, name, version)
		if err != nil {
			r.Error = err.Error()
			results = append(results, r)
			fmt.Fprintf(os.Stderr, "  FAIL %s@%s: %v\n", name, version, err)
			continue
		}

		r.VulnCount = len(findings)
		for _, f := range findings {
			sev := string(f.Severity)
			switch sev {
			case "CRITICAL":
				r.Critical++
			case "HIGH":
				r.High++
			case "MEDIUM":
				r.Medium++
			case "LOW":
				r.Low++
			}
			summary := f.Summary
			if len(summary) > 100 {
				summary = summary[:100] + "..."
			}
			r.Vulns = append(r.Vulns, VulnSummary{
				ID:       f.ID,
				Severity: sev,
				Summary:  summary,
			})
		}

		results = append(results, r)
		if r.VulnCount > 0 {
			fmt.Fprintf(os.Stderr, "  %3d CVEs %s@%s (C=%d H=%d M=%d L=%d)\n",
				r.VulnCount, name, version, r.Critical, r.High, r.Medium, r.Low)
		} else {
			fmt.Fprintf(os.Stderr, "  --- clean %s@%s\n", name, version)
		}
	}

	json.NewEncoder(os.Stdout).Encode(results)
}
