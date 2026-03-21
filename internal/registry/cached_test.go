package registry_test

import (
	"testing"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedFetcherHitsCache(t *testing.T) {
	fetchCount := 0
	inner := &stubFetcher{fn: func(name, version string) (*registry.PackageInfo, error) {
		fetchCount++
		return &registry.PackageInfo{Name: name, Version: version, MaintainerCount: 3}, nil
	}}

	c := cache.NewDiskCache(t.TempDir())
	cached := registry.NewCachedFetcher(inner, c, 24)

	// First call — cache miss
	info1, err := cached.Fetch("requests", "2.31.0")
	require.NoError(t, err)
	assert.Equal(t, "requests", info1.Name)
	assert.Equal(t, 1, fetchCount)

	// Second call — cache hit
	info2, err := cached.Fetch("requests", "2.31.0")
	require.NoError(t, err)
	assert.Equal(t, "requests", info2.Name)
	assert.Equal(t, 1, fetchCount, "second call should hit cache, not fetch again")
	assert.Equal(t, 3, info2.MaintainerCount, "cached data should preserve fields")
}
