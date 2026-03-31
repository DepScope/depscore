package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
	"github.com/depscope/depscope/internal/vuln"
)

// EnrichOptions configures the enrich command.
type EnrichOptions struct {
	DBPath string
	Force  bool // re-enrich packages that already have metadata
	NoCVE  bool // skip CVE lookups
}

// EnrichResult holds the summary of an enrichment run.
type EnrichResult struct {
	Total     int
	Enriched  int
	Skipped   int
	Errors    int
	CVEsFound int
}

// EnrichMetadata is the JSON stored in project_versions.metadata.
type EnrichMetadata struct {
	Score           int          `json:"score"`
	Risk            string       `json:"risk"`
	MaintainerCount int          `json:"maintainer_count,omitempty"`
	Downloads       int64        `json:"downloads,omitempty"`
	OrgBacking      bool         `json:"org_backing,omitempty"`
	CVECount        int          `json:"cve_count"`
	CVEs            []EnrichCVE  `json:"cves,omitempty"`
	EnrichedAt      time.Time    `json:"enriched_at"`
}

// EnrichCVE represents a single CVE in the enrichment metadata.
type EnrichCVE struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Summary  string `json:"summary,omitempty"`
}

// RunEnrich enriches all unique packages in the index with reputation scores and CVE data.
func RunEnrich(ctx context.Context, opts EnrichOptions, w io.Writer) (*EnrichResult, error) {
	db, err := cache.NewCacheDB(opts.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Query all unique packages from the index.
	rows, err := db.DB().Query(
		`SELECT DISTINCT mp.project_id, mp.version_key
		 FROM manifest_packages mp
		 ORDER BY mp.project_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("query packages: %w", err)
	}

	type pkgEntry struct {
		projectID  string
		versionKey string
	}
	var packages []pkgEntry
	for rows.Next() {
		var e pkgEntry
		if err := rows.Scan(&e.projectID, &e.versionKey); err != nil {
			_ = rows.Close()
			return nil, err
		}
		packages = append(packages, e)
	}
	_ = rows.Close()

	result := &EnrichResult{Total: len(packages)}
	_, _ = fmt.Fprintf(w, "Enriching %d unique packages...\n\n", len(packages))

	// Build registry fetchers.
	fetchers := registry.FetchersByEcosystem{
		"PyPI":      registry.NewPyPIClient(),
		"npm":       registry.NewNPMClient(),
		"crates.io": registry.NewCratesClient(),
		"Go":        registry.NewGoProxyClient(),
		"Packagist": registry.NewPackagistClient(),
	}

	// Build VCS client.
	var vcsClient vcs.Client
	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken != "" {
		vcsClient = vcs.NewGitHubClient(vcs.WithToken(ghToken))
	} else {
		vcsClient = vcs.NewGitHubClient()
	}

	// Build OSV client for CVE lookups.
	var osvClient *vuln.OSVClient
	if !opts.NoCVE {
		osvClient = vuln.NewOSVClient()
	}

	// Cache VCS lookups to avoid redundant API calls.
	repoCache := make(map[string]*vcs.RepoInfo)
	var repoCacheMu sync.Mutex

	// Process with concurrency limit.
	sem := make(chan struct{}, 5) // conservative: 5 concurrent API calls
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, pkg := range packages {
		if ctx.Err() != nil {
			break
		}

		// Skip if already enriched (unless --force).
		if !opts.Force {
			var metadata string
			err := db.DB().QueryRow(
				`SELECT metadata FROM project_versions WHERE project_id = ? AND version_key = ?`,
				pkg.projectID, pkg.versionKey,
			).Scan(&metadata)
			if err == nil && metadata != "" && metadata != "{}" {
				result.Skipped++
				continue
			}
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, p pkgEntry) {
			defer wg.Done()
			defer func() { <-sem }()

			meta := enrichPackage(ctx, p.projectID, p.versionKey, fetchers, vcsClient, osvClient, &repoCacheMu, repoCache)

			// Store metadata.
			metaJSON, _ := json.Marshal(meta)
			if err := db.UpsertVersion(&cache.ProjectVersion{
				ProjectID:  p.projectID,
				VersionKey: p.versionKey,
				Metadata:   string(metaJSON),
			}); err != nil {
				mu.Lock()
				result.Errors++
				mu.Unlock()
				return
			}

			mu.Lock()
			result.Enriched++
			result.CVEsFound += meta.CVECount
			mu.Unlock()

			// Print progress for every package.
			cveInfo := ""
			if meta.CVECount > 0 {
				cveInfo = fmt.Sprintf("  %d CVE(s)", meta.CVECount)
			}
			_, _ = fmt.Fprintf(w, "  [%3d/%d] %-40s score:%-3d risk:%-8s%s\n",
				idx+1, len(packages), truncateStr(p.versionKey, 40), meta.Score, meta.Risk, cveInfo)
		}(i, pkg)
	}

	wg.Wait()

	_, _ = fmt.Fprintf(w, "\nDone: %d enriched, %d skipped, %d errors, %d CVEs found\n",
		result.Enriched, result.Skipped, result.Errors, result.CVEsFound)

	return result, nil
}

func enrichPackage(
	ctx context.Context,
	projectID, versionKey string,
	fetchers registry.FetchersByEcosystem,
	vcsClient vcs.Client,
	osvClient *vuln.OSVClient,
	repoCacheMu *sync.Mutex,
	repoCache map[string]*vcs.RepoInfo,
) EnrichMetadata {
	meta := EnrichMetadata{EnrichedAt: time.Now()}

	// Parse ecosystem and name from projectID (e.g. "npm/axios" -> "npm", "axios").
	eco, name := splitEnrichProjectID(projectID)
	if eco == "" || name == "" {
		meta.Risk = "UNKNOWN"
		return meta
	}

	// Extract version from versionKey (e.g. "npm/axios@1.7.2" -> "1.7.2").
	version := ""
	if i := strings.LastIndex(versionKey, "@"); i >= 0 {
		version = versionKey[i+1:]
	}

	// Map ecosystem to registry key (e.g. "python" -> "PyPI").
	registryEco := mapEnrichEcosystem(eco)

	// Score via registry.
	cfg := config.ProfileByName("enterprise")
	fetcher, ok := fetchers[registryEco]
	if !ok {
		meta.Score = 50
		meta.Risk = "UNKNOWN"
	} else {
		info, err := fetcher.Fetch(name, version)
		if err != nil {
			meta.Score = 0
			meta.Risk = string(core.RiskCritical)
		} else {
			// Get VCS data if available.
			var repoInfo *vcs.RepoInfo
			if info != nil && info.SourceRepoURL != "" {
				repoURL := info.SourceRepoURL
				repoCacheMu.Lock()
				cached, hasCached := repoCache[repoURL]
				repoCacheMu.Unlock()
				if hasCached {
					repoInfo = cached
				} else {
					ri, _ := vcsClient.RepoFromURL(repoURL)
					repoInfo = ri
					repoCacheMu.Lock()
					repoCache[repoURL] = repoInfo
					repoCacheMu.Unlock()
				}
			}

			pkg := manifest.Package{
				Name:            name,
				ResolvedVersion: version,
				Ecosystem:       ecosystemFromEnrichKey(registryEco),
			}
			scoreResult := core.Score(pkg, &registry.FetchResult{Info: info}, repoInfo, cfg.Weights)
			meta.Score = scoreResult.OwnScore
			meta.Risk = string(scoreResult.OwnRisk)

			if info != nil {
				meta.MaintainerCount = info.MaintainerCount
				meta.Downloads = info.TotalDownloads
				meta.OrgBacking = info.HasOrgBacking
			}
		}
	}

	// CVE lookup via OSV.
	if osvClient != nil && version != "" {
		osvEco := registryEco
		findings, err := osvClient.Query(osvEco, name, version)
		if err == nil {
			meta.CVECount = len(findings)
			for _, f := range findings {
				meta.CVEs = append(meta.CVEs, EnrichCVE{
					ID:       f.ID,
					Severity: string(f.Severity),
					Summary:  f.Summary,
				})
			}
		}
	}

	return meta
}

func splitEnrichProjectID(pid string) (eco, name string) {
	i := strings.Index(pid, "/")
	if i < 0 {
		return "", pid
	}
	return pid[:i], pid[i+1:]
}

func mapEnrichEcosystem(eco string) string {
	switch strings.ToLower(eco) {
	case "python":
		return "PyPI"
	case "npm":
		return "npm"
	case "rust":
		return "crates.io"
	case "go":
		return "Go"
	case "php":
		return "Packagist"
	default:
		return eco
	}
}

func ecosystemFromEnrichKey(key string) manifest.Ecosystem {
	switch key {
	case "PyPI":
		return manifest.EcosystemPython
	case "npm":
		return manifest.EcosystemNPM
	case "crates.io":
		return manifest.EcosystemRust
	case "Go":
		return manifest.EcosystemGo
	case "Packagist":
		return manifest.EcosystemPHP
	default:
		return manifest.Ecosystem(key)
	}
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
