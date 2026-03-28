// internal/crawler/scorepass.go
package crawler

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
)

// maxScoreConcurrency limits how many registry/VCS fetches run in parallel.
const maxScoreConcurrency = 10

// RunScorePass walks all nodes in the graph and computes reputation scores
// using registry metadata and VCS health data.
func RunScorePass(ctx context.Context, g *graph.Graph, cacheDB *cache.CacheDB) []CrawlError {
	cfg := config.ProfileByName("enterprise")
	weights := cfg.Weights

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

	// Collect nodes to score (skip root node).
	var nodes []*graph.Node
	for _, n := range g.Nodes {
		if n.ID == RootNodeID {
			continue
		}
		nodes = append(nodes, n)
	}

	var (
		mu   sync.Mutex
		errs []CrawlError
		wg   sync.WaitGroup
		sem  = make(chan struct{}, maxScoreConcurrency)
	)

	// Cache VCS lookups by repo URL to avoid redundant API calls.
	repoCache := make(map[string]*vcs.RepoInfo)
	var repoCacheMu sync.Mutex

	for _, node := range nodes {
		if err := ctx.Err(); err != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{} // acquire semaphore slot
		go func(n *graph.Node) {
			defer wg.Done()
			defer func() { <-sem }() // release semaphore slot

			if err := ctx.Err(); err != nil {
				return
			}

			switch n.Type {
			case graph.NodePackage:
				scoreErr := scorePackageNode(n, fetchers, vcsClient, weights, &repoCacheMu, repoCache)
				if scoreErr != nil {
					mu.Lock()
					errs = append(errs, CrawlError{
						Err: fmt.Errorf("score %s: %w", n.ID, scoreErr),
					})
					mu.Unlock()
				}
			case graph.NodeAction, graph.NodePrecommitHook, graph.NodeGitSubmodule,
				graph.NodeTerraformModule:
				scoreGitNode(n, vcsClient, &repoCacheMu, repoCache)
			default:
				// Tool, Script, BuildTool — score based on pinning only.
				scorePinningNode(n)
			}
		}(node)
	}

	wg.Wait()
	return errs
}

// scorePackageNode scores a package node using registry fetchers and VCS data.
func scorePackageNode(
	n *graph.Node,
	fetchers registry.FetchersByEcosystem,
	vcsClient vcs.Client,
	weights config.Weights,
	repoCacheMu *sync.Mutex,
	repoCache map[string]*vcs.RepoInfo,
) error {
	// Extract ecosystem and name from ProjectID (format: "ecosystem/name").
	ecosystem, name := splitProjectID(n.ProjectID)
	if ecosystem == "" || name == "" {
		// Fall back to metadata if available.
		if eco, ok := n.Metadata["ecosystem"].(string); ok {
			ecosystem = eco
		}
		if ecosystem == "" {
			return fmt.Errorf("cannot determine ecosystem for node %s", n.ID)
		}
		name = n.Name
	}

	// Map ecosystem identifiers to registry keys.
	registryEco := mapEcosystemToRegistry(ecosystem)

	version := n.Version
	if version == "" {
		// Try extracting from VersionKey (format: "ecosystem/name@version").
		if i := strings.LastIndex(n.VersionKey, "@"); i != -1 {
			version = n.VersionKey[i+1:]
		}
	}

	// Build a manifest.Package for core.Score.
	pkg := manifest.Package{
		Name:            name,
		ResolvedVersion: version,
		Ecosystem:       ecosystemFromRegistryKey(registryEco),
	}

	// Fetch from registry.
	fetcher, ok := fetchers[registryEco]
	if !ok {
		// No fetcher for this ecosystem; set minimal score.
		n.Score = 0
		n.Risk = core.RiskCritical
		return nil
	}

	info, err := fetcher.Fetch(name, version)
	var fetchResult *registry.FetchResult
	if err != nil {
		fetchResult = &registry.FetchResult{Err: err}
	} else {
		fetchResult = &registry.FetchResult{Info: info}
	}

	// Fetch VCS data if available.
	var repoInfo *vcs.RepoInfo
	if fetchResult.Info != nil && fetchResult.Info.SourceRepoURL != "" {
		repoURL := fetchResult.Info.SourceRepoURL

		repoCacheMu.Lock()
		cached, hasCached := repoCache[repoURL]
		repoCacheMu.Unlock()

		if hasCached {
			repoInfo = cached
		} else {
			ri, err := vcsClient.RepoFromURL(repoURL)
			if err != nil {
				log.Printf("VCS lookup failed for %s: %v", repoURL, err)
			} else {
				repoInfo = ri
			}
			repoCacheMu.Lock()
			repoCache[repoURL] = repoInfo
			repoCacheMu.Unlock()
		}
	}

	// Score using core.Score.
	result := core.Score(pkg, fetchResult, repoInfo, weights)
	n.Score = result.OwnScore
	n.Risk = result.OwnRisk

	// Store useful metadata.
	if fetchResult.Info != nil {
		n.Metadata["maintainer_count"] = fetchResult.Info.MaintainerCount
		n.Metadata["downloads"] = fetchResult.Info.TotalDownloads
		n.Metadata["org_backing"] = fetchResult.Info.HasOrgBacking
	}

	return nil
}

// scoreGitNode scores a git-based node (action, hook, submodule, terraform)
// using VCS repo health data and pinning quality.
func scoreGitNode(
	n *graph.Node,
	vcsClient vcs.Client,
	repoCacheMu *sync.Mutex,
	repoCache map[string]*vcs.RepoInfo,
) {
	// Start with pinning-based score.
	baseScore := pinningScore(n.Pinning) + 20
	if baseScore > 100 {
		baseScore = 100
	}

	// Try to get repo info from ProjectID (expected format: owner/repo or github.com/owner/repo).
	repoURL := n.ProjectID
	if repoURL == "" {
		n.Score = baseScore
		n.Risk = core.RiskLevelFromScore(n.Score)
		return
	}

	// Normalize to a GitHub URL if it looks like owner/repo.
	if !strings.Contains(repoURL, "://") && !strings.HasPrefix(repoURL, "github.com/") {
		repoURL = "https://github.com/" + repoURL
	} else if strings.HasPrefix(repoURL, "github.com/") {
		repoURL = "https://" + repoURL
	}

	repoCacheMu.Lock()
	cached, hasCached := repoCache[repoURL]
	repoCacheMu.Unlock()

	var repoInfo *vcs.RepoInfo
	if hasCached {
		repoInfo = cached
	} else {
		ri, err := vcsClient.RepoFromURL(repoURL)
		if err != nil {
			log.Printf("VCS lookup failed for %s: %v", repoURL, err)
		} else {
			repoInfo = ri
		}
		repoCacheMu.Lock()
		repoCache[repoURL] = repoInfo
		repoCacheMu.Unlock()
	}

	if repoInfo == nil {
		n.Score = baseScore
		n.Risk = core.RiskLevelFromScore(n.Score)
		return
	}

	// Compute a simplified score: pinning + stars + maintainer count + archived status.
	score := pinningScore(n.Pinning)

	// Stars contribution (up to 20 points).
	switch {
	case repoInfo.StarCount >= 10000:
		score += 20
	case repoInfo.StarCount >= 1000:
		score += 15
	case repoInfo.StarCount >= 100:
		score += 10
	case repoInfo.StarCount >= 10:
		score += 5
	}

	// Maintainer contribution (up to 20 points).
	switch {
	case repoInfo.ContributorCount >= 20:
		score += 20
	case repoInfo.ContributorCount >= 5:
		score += 15
	case repoInfo.ContributorCount >= 2:
		score += 10
	default:
		score += 5
	}

	// Archived penalty.
	if repoInfo.IsArchived {
		score -= 30
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	n.Score = score
	n.Risk = core.RiskLevelFromScore(n.Score)
	n.Metadata["stars"] = repoInfo.StarCount
	n.Metadata["contributors"] = repoInfo.ContributorCount
	n.Metadata["archived"] = repoInfo.IsArchived
}

// scorePinningNode scores a tool/script/buildtool node based on pinning quality only.
func scorePinningNode(n *graph.Node) {
	n.Score = pinningScore(n.Pinning)
	n.Risk = core.RiskLevelFromScore(n.Score)
}

// pinningScore returns a score based on pinning quality.
func pinningScore(p graph.PinningQuality) int {
	switch p {
	case graph.PinningSHA:
		return 100
	case graph.PinningDigest:
		return 100
	case graph.PinningExactVersion:
		return 85
	case graph.PinningSemverRange:
		return 70
	case graph.PinningMajorTag:
		return 40
	case graph.PinningBranch:
		return 20
	case graph.PinningUnpinned:
		return 0
	case graph.PinningNA:
		return 50 // neutral
	default:
		return 0
	}
}

// splitProjectID splits a ProjectID of the form "ecosystem/name" into its parts.
// For Go modules like "Go/github.com/foo/bar", it returns ("Go", "github.com/foo/bar").
func splitProjectID(projectID string) (ecosystem, name string) {
	if projectID == "" {
		return "", ""
	}
	i := strings.Index(projectID, "/")
	if i < 0 {
		return projectID, ""
	}
	return projectID[:i], projectID[i+1:]
}

// mapEcosystemToRegistry maps various ecosystem identifiers to registry keys.
func mapEcosystemToRegistry(eco string) string {
	switch strings.ToLower(eco) {
	case "pypi", "python":
		return "PyPI"
	case "npm":
		return "npm"
	case "crates.io", "rust":
		return "crates.io"
	case "go":
		return "Go"
	case "packagist", "php":
		return "Packagist"
	default:
		return eco
	}
}

// ecosystemFromRegistryKey maps a registry key back to a manifest.Ecosystem.
func ecosystemFromRegistryKey(key string) manifest.Ecosystem {
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
