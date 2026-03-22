package registry

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubFetcher is a test double that records every Fetch call.
type stubFetcher struct {
	calls    atomic.Int64
	ecosystem string
	failFor  map[string]bool // name → should error
}

func newStubFetcher(eco string) *stubFetcher {
	return &stubFetcher{ecosystem: eco, failFor: map[string]bool{}}
}

func (s *stubFetcher) Ecosystem() string { return s.ecosystem }

func (s *stubFetcher) Fetch(name, version string) (*PackageInfo, error) {
	s.calls.Add(1)
	if s.failFor[name] {
		return nil, fmt.Errorf("stub: forced error for %s", name)
	}
	return &PackageInfo{
		Name:      name,
		Version:   version,
		Ecosystem: s.ecosystem,
	}, nil
}

func TestFetchAll_Basic(t *testing.T) {
	stub := newStubFetcher("PyPI")
	fetchers := FetchersByEcosystem{"PyPI": stub}

	pkgs := []manifest.Package{
		{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython},
		{Name: "flask", ResolvedVersion: "2.3.0", Ecosystem: manifest.EcosystemPython},
	}

	results := FetchAll(pkgs, fetchers, 4)

	assert.Len(t, results, 2)
	assert.Equal(t, int64(2), stub.calls.Load())

	k1 := pkgs[0].Key()
	require.NotNil(t, results[k1])
	require.NoError(t, results[k1].Err)
	assert.Equal(t, "requests", results[k1].Info.Name)
}

func TestFetchAll_Deduplication(t *testing.T) {
	stub := newStubFetcher("PyPI")
	fetchers := FetchersByEcosystem{"PyPI": stub}

	// Same package listed three times.
	pkgs := []manifest.Package{
		{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython},
		{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython},
		{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython},
	}

	results := FetchAll(pkgs, fetchers, 4)

	// Only one unique key, only one fetch call.
	assert.Len(t, results, 1)
	assert.Equal(t, int64(1), stub.calls.Load())
}

func TestFetchAll_NoFetcherForEcosystem(t *testing.T) {
	fetchers := FetchersByEcosystem{} // no fetchers at all

	pkgs := []manifest.Package{
		{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython},
	}

	results := FetchAll(pkgs, fetchers, 4)

	k := pkgs[0].Key()
	require.NotNil(t, results[k])
	require.Error(t, results[k].Err)
	assert.Contains(t, results[k].Err.Error(), "no fetcher registered")
}

func TestFetchAll_FetcherError(t *testing.T) {
	stub := newStubFetcher("PyPI")
	stub.failFor["broken"] = true
	fetchers := FetchersByEcosystem{"PyPI": stub}

	pkgs := []manifest.Package{
		{Name: "broken", ResolvedVersion: "1.0.0", Ecosystem: manifest.EcosystemPython},
		{Name: "good", ResolvedVersion: "1.0.0", Ecosystem: manifest.EcosystemPython},
	}

	results := FetchAll(pkgs, fetchers, 4)

	assert.Len(t, results, 2)

	brokenKey := pkgs[0].Key()
	require.NotNil(t, results[brokenKey])
	require.Error(t, results[brokenKey].Err)

	goodKey := pkgs[1].Key()
	require.NotNil(t, results[goodKey])
	require.NoError(t, results[goodKey].Err)
}

func TestFetchAll_SemaphoreLimitsConcurrency(t *testing.T) {
	// Use a stub that counts concurrent calls.
	var concurrent atomic.Int64
	var maxConcurrent atomic.Int64

	stub := &concurrencyTrackingFetcher{
		concurrent:    &concurrent,
		maxConcurrent: &maxConcurrent,
	}
	fetchers := FetchersByEcosystem{"PyPI": stub}

	const n = 20
	pkgs := make([]manifest.Package, n)
	for i := range pkgs {
		pkgs[i] = manifest.Package{
			Name:            fmt.Sprintf("pkg%d", i),
			ResolvedVersion: "1.0.0",
			Ecosystem:       manifest.EcosystemPython,
		}
	}

	const limit = int64(3)
	FetchAll(pkgs, fetchers, limit)

	assert.LessOrEqual(t, maxConcurrent.Load(), limit)
}

type concurrencyTrackingFetcher struct {
	concurrent    *atomic.Int64
	maxConcurrent *atomic.Int64
}

func (f *concurrencyTrackingFetcher) Ecosystem() string { return "PyPI" }

func (f *concurrencyTrackingFetcher) Fetch(name, version string) (*PackageInfo, error) {
	cur := f.concurrent.Add(1)
	defer f.concurrent.Add(-1)

	// Track maximum observed concurrency.
	for {
		old := f.maxConcurrent.Load()
		if cur <= old {
			break
		}
		if f.maxConcurrent.CompareAndSwap(old, cur) {
			break
		}
	}

	return &PackageInfo{Name: name, Version: version, Ecosystem: "PyPI"}, nil
}

func TestFetchAll_MultipleEcosystems(t *testing.T) {
	pyStub := newStubFetcher("PyPI")
	npmStub := newStubFetcher("npm")

	fetchers := FetchersByEcosystem{
		"PyPI": pyStub,
		"npm":  npmStub,
	}

	pkgs := []manifest.Package{
		{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython},
		{Name: "express", ResolvedVersion: "4.18.2", Ecosystem: manifest.EcosystemNPM},
	}

	results := FetchAll(pkgs, fetchers, 4)

	assert.Len(t, results, 2)
	assert.Equal(t, int64(1), pyStub.calls.Load())
	assert.Equal(t, int64(1), npmStub.calls.Load())

	for _, r := range results {
		require.NoError(t, r.Err)
	}
}
