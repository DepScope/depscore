package registry

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/depscope/depscope/internal/cache"
)

// CachedFetcher wraps a Fetcher with disk caching.
type CachedFetcher struct {
	inner Fetcher
	cache *cache.DiskCache
	ttl   time.Duration
}

// NewCachedFetcher wraps a fetcher with disk-based caching.
func NewCachedFetcher(inner Fetcher, c *cache.DiskCache, ttlHours int) *CachedFetcher {
	return &CachedFetcher{
		inner: inner,
		cache: c,
		ttl:   time.Duration(ttlHours) * time.Hour,
	}
}

func (f *CachedFetcher) Ecosystem() string { return f.inner.Ecosystem() }

func (f *CachedFetcher) Fetch(name, version string) (*PackageInfo, error) {
	key := fmt.Sprintf("registry/%s/%s@%s", f.inner.Ecosystem(), name, version)

	// Try cache first
	if data, ok, err := f.cache.Get(key); err == nil && ok {
		var info PackageInfo
		if json.Unmarshal(data, &info) == nil {
			return &info, nil
		}
	}

	// Cache miss — fetch from network
	info, err := f.inner.Fetch(name, version)
	if err != nil {
		return nil, err
	}

	// Store in cache (best-effort)
	if data, err := json.Marshal(info); err == nil {
		_ = f.cache.Set(key, data, f.ttl)
	}

	return info, nil
}
