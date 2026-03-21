package registry

import (
	"context"
	"sync"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/vcs"
	"github.com/depscope/depscope/internal/vuln"
)

// FetchResult holds the combined results of fetching package info,
// repository info, and vulnerability findings for a single package.
type FetchResult struct {
	Info     *PackageInfo
	RepoInfo *vcs.RepoInfo
	Vulns    []vuln.Finding
	Err      error
}

// FetchOptions configures the parallel fetch behaviour.
type FetchOptions struct {
	Concurrency int
}

// FetchAll takes a slice of packages, deduplicates them by Key(), and
// fetches registry info, VCS metadata, and vulnerability findings in
// parallel. Results are keyed by pkg.Key(). Errors for individual
// packages are stored in FetchResult.Err; FetchAll itself only returns
// an error when the entire operation cannot proceed.
func FetchAll(
	ctx context.Context,
	pkgs []manifest.Package,
	reg Fetcher,
	vcsClient vcs.Client,
	vulnSources []vuln.Source,
	opts FetchOptions,
) (map[string]*FetchResult, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 20
	}

	// Use a buffered channel as a counting semaphore.
	sem := make(chan struct{}, opts.Concurrency)

	// Deduplicate packages by key.
	unique := make(map[string]manifest.Package)
	for _, p := range pkgs {
		unique[p.Key()] = p
	}

	results := make(map[string]*FetchResult, len(unique))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for key, pkg := range unique {
		wg.Add(1)
		go func(key string, pkg manifest.Package) {
			defer wg.Done()

			// Acquire semaphore slot, respecting context cancellation.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				results[key] = &FetchResult{Err: ctx.Err()}
				mu.Unlock()
				return
			}
			defer func() { <-sem }()

			res := &FetchResult{}
			info, err := reg.Fetch(pkg.Name, pkg.ResolvedVersion)
			if err != nil {
				res.Err = err
			} else {
				res.Info = info
				if vcsClient != nil && info.SourceRepoURL != "" {
					if repoInfo, err := vcsClient.RepoFromURL(info.SourceRepoURL); err == nil {
						res.RepoInfo = repoInfo
					}
				}
				for _, vs := range vulnSources {
					if findings, err := vs.Query(string(pkg.Ecosystem), pkg.Name, pkg.ResolvedVersion); err == nil {
						res.Vulns = append(res.Vulns, findings...)
					}
				}
			}

			mu.Lock()
			results[key] = res
			mu.Unlock()
		}(key, pkg)
	}
	wg.Wait()
	return results, nil
}
