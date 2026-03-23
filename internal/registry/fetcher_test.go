package registry_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/depscope/depscope/internal/registry"
	"github.com/depscope/depscope/internal/vcs"
	"github.com/depscope/depscope/internal/vuln"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Blank imports to satisfy the compiler for packages referenced only by type.
var _ vcs.Client
var _ vuln.Source

type stubFetcher struct {
	fn func(name, version string) (*registry.PackageInfo, error)
}

func (s *stubFetcher) Fetch(name, version string) (*registry.PackageInfo, error) {
	return s.fn(name, version)
}
func (s *stubFetcher) Ecosystem() string { return "test" }

func TestFetchAllDeduplicates(t *testing.T) {
	pkgs := []manifest.Package{
		{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython},
		{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython},
		{Name: "urllib3", ResolvedVersion: "2.0.7", Ecosystem: manifest.EcosystemPython},
	}
	var fetchCount atomic.Int32
	stub := &stubFetcher{fn: func(name, version string) (*registry.PackageInfo, error) {
		fetchCount.Add(1)
		return &registry.PackageInfo{Name: name}, nil
	}}
	results, err := registry.FetchAll(context.Background(), pkgs, stub, nil, nil,
		registry.FetchOptions{Concurrency: 5})
	require.NoError(t, err)
	assert.Equal(t, int32(2), fetchCount.Load(), "duplicate package should be fetched only once")
	assert.Len(t, results, 2)
}

func TestFetchAllContinuesOnError(t *testing.T) {
	pkgs := []manifest.Package{
		{Name: "good", ResolvedVersion: "1.0.0", Ecosystem: manifest.EcosystemPython},
		{Name: "bad", ResolvedVersion: "1.0.0", Ecosystem: manifest.EcosystemPython},
	}
	stub := &stubFetcher{fn: func(name, version string) (*registry.PackageInfo, error) {
		if name == "bad" {
			return nil, errors.New("registry unavailable")
		}
		return &registry.PackageInfo{Name: name}, nil
	}}
	results, err := registry.FetchAll(context.Background(), pkgs, stub, nil, nil,
		registry.FetchOptions{Concurrency: 5})
	require.NoError(t, err, "FetchAll should not fail when one package errors")
	assert.NotNil(t, results["python/good@1.0.0"].Info)
	assert.NotNil(t, results["python/bad@1.0.0"].Err)
}
