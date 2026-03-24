package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
)

type BenchResult struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Score     int    `json:"score"`
	Risk      string `json:"risk"`

	// Factor availability
	HasRelease     bool `json:"has_release"`
	HasMaintainers bool `json:"has_maintainers"`
	HasDownloads   bool `json:"has_downloads"`
	HasSourceRepo  bool `json:"has_source_repo"`

	// Raw signals
	MaintainerCount  int    `json:"maintainer_count"`
	MonthlyDownloads int64  `json:"monthly_downloads"`
	ReleaseCount     int    `json:"release_count"`
	DaysSinceRelease int    `json:"days_since_release"`
	SourceRepoURL    string `json:"source_repo_url"`
	IsDeprecated     bool   `json:"is_deprecated"`

	Error string `json:"error,omitempty"`
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: benchmark <ecosystem> <packages-file>\n")
		os.Exit(1)
	}

	eco := os.Args[1]
	file := os.Args[2]

	f, err := os.Open(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", file, err)
		os.Exit(1)
	}
	defer f.Close() //nolint:errcheck

	var fetcher registry.Fetcher
	var mEco manifest.Ecosystem
	switch eco {
	case "pypi":
		fetcher = registry.NewPyPIClient()
		mEco = manifest.EcosystemPython
	case "npm":
		fetcher = registry.NewNPMClient()
		mEco = manifest.EcosystemNPM
	case "crates":
		fetcher = registry.NewCratesClient()
		mEco = manifest.EcosystemRust
	case "go":
		fetcher = registry.NewGoProxyClient()
		mEco = manifest.EcosystemGo
	case "packagist":
		fetcher = registry.NewPackagistClient()
		mEco = manifest.EcosystemPHP
	default:
		fmt.Fprintf(os.Stderr, "unknown ecosystem: %s\n", eco)
		os.Exit(1)
	}

	weights := config.Enterprise().Weights
	var results []BenchResult

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name == "" {
			continue
		}

		info, err := fetcher.Fetch(name, "")
		if err != nil {
			results = append(results, BenchResult{
				Ecosystem: eco,
				Name:      name,
				Error:     err.Error(),
			})
			fmt.Fprintf(os.Stderr, "  FAIL %s: %v\n", name, err)
			continue
		}

		pkg := manifest.Package{
			Name:           name,
			ConstraintType: manifest.ConstraintExact,
			Ecosystem:      mEco,
			Depth:          1,
		}
		fr := &registry.FetchResult{Info: info}
		scored := core.Score(pkg, fr, nil, weights)

		daysSince := 0
		if !info.LastReleaseAt.IsZero() {
			daysSince = int(time.Since(info.LastReleaseAt).Hours() / 24)
		}

		r := BenchResult{
			Ecosystem:        eco,
			Name:             name,
			Version:          info.Version,
			Score:            scored.OwnScore,
			Risk:             string(scored.OwnRisk),
			HasRelease:       !info.LastReleaseAt.IsZero(),
			HasMaintainers:   info.MaintainerCount > 0,
			HasDownloads:     info.MonthlyDownloads > 0,
			HasSourceRepo:    info.SourceRepoURL != "",
			MaintainerCount:  info.MaintainerCount,
			MonthlyDownloads: info.MonthlyDownloads,
			ReleaseCount:     info.ReleaseCount,
			DaysSinceRelease: daysSince,
			SourceRepoURL:    info.SourceRepoURL,
			IsDeprecated:     info.IsDeprecated,
		}
		results = append(results, r)
		fmt.Fprintf(os.Stderr, "  %3d %s %s\n", r.Score, r.Risk, name)
	}

	_ = json.NewEncoder(os.Stdout).Encode(results)
}
