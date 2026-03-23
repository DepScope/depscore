package cache_test

import (
	"os"
	"testing"
	"time"

	"github.com/depscope/depscope/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetAndGet(t *testing.T) {
	c := cache.NewDiskCache(t.TempDir())
	require.NoError(t, c.Set("key1", []byte(`{"hello":"world"}`), time.Hour))
	data, ok, err := c.Get("key1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, `{"hello":"world"}`, string(data))
}

func TestExpiredEntryMiss(t *testing.T) {
	c := cache.NewDiskCache(t.TempDir())
	require.NoError(t, c.Set("key2", []byte("data"), -time.Second))
	_, ok, err := c.Get("key2")
	require.NoError(t, err)
	assert.False(t, ok, "expired entry should be a cache miss")
}

func TestMissingKeyMiss(t *testing.T) {
	c := cache.NewDiskCache(t.TempDir())
	_, ok, err := c.Get("nonexistent")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestClearRemovesAllEntries(t *testing.T) {
	dir := t.TempDir()
	c := cache.NewDiskCache(dir)
	require.NoError(t, c.Set("k1", []byte("a"), time.Hour))
	require.NoError(t, c.Set("k2", []byte("b"), time.Hour))
	require.NoError(t, c.Clear())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestStatus(t *testing.T) {
	c := cache.NewDiskCache(t.TempDir())
	require.NoError(t, c.Set("k1", []byte("hello"), time.Hour))
	count, size, err := c.Status()
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Greater(t, size, int64(0))
}
