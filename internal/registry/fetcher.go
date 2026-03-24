package registry

import (
	"fmt"
	"sync"

	"github.com/depscope/depscope/internal/manifest"
	"golang.org/x/sync/semaphore"
	"context"
)

// FetchResult bundles the fetched PackageInfo and any error for one package.
type FetchResult struct {
	Info *PackageInfo
	Err  error
}

// FetchersByEcosystem maps an ecosystem string to its Fetcher implementation.
type FetchersByEcosystem map[string]Fetcher

// FetchAll fetches registry metadata for every unique package in pkgs.
// Concurrency is bounded by maxConcurrent. Packages are deduplicated by
// manifest.Package.Key() so each (ecosystem, name, version) triple is fetched
// at most once.
//
// Returns a map from Package.Key() to FetchResult.
func FetchAll(
	pkgs []manifest.Package,
	fetchers FetchersByEcosystem,
	maxConcurrent int64,
) map[string]*FetchResult {
	// Deduplicate by key.
	unique := make(map[string]manifest.Package)
	for _, p := range pkgs {
		k := p.Key()
		if _, seen := unique[k]; !seen {
			unique[k] = p
		}
	}

	results := make(map[string]*FetchResult, len(unique))
	var mu sync.Mutex

	sem := semaphore.NewWeighted(maxConcurrent)
	ctx := context.Background()

	var wg sync.WaitGroup
	for key, pkg := range unique {
		key, pkg := key, pkg // capture loop vars
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sem.Acquire(ctx, 1); err != nil {
				mu.Lock()
				results[key] = &FetchResult{Err: fmt.Errorf("semaphore: %w", err)}
				mu.Unlock()
				return
			}
			defer sem.Release(1)

			result := fetchOne(pkg, fetchers)

			mu.Lock()
			results[key] = result
			mu.Unlock()
		}()
	}
	wg.Wait()

	return results
}

func fetchOne(pkg manifest.Package, fetchers FetchersByEcosystem) *FetchResult {
	ecoStr := pkg.Ecosystem.String()
	f, ok := fetchers[ecoStr]
	if !ok {
		return &FetchResult{Err: fmt.Errorf("no fetcher registered for ecosystem %q", ecoStr)}
	}

	info, err := f.Fetch(pkg.Name, pkg.ResolvedVersion)
	return &FetchResult{Info: info, Err: err}
}
