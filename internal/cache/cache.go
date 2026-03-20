package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type entry struct {
	ExpiresAt time.Time `json:"expires_at"`
	Data      []byte    `json:"data"`
}

type DiskCache struct{ dir string }

func NewDiskCache(dir string) *DiskCache { return &DiskCache{dir: dir} }

func DefaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "depscope")
}

func (c *DiskCache) path(key string) string {
	h := sha256.Sum256([]byte(key))
	return filepath.Join(c.dir, fmt.Sprintf("%x.json", h))
}

func (c *DiskCache) Get(key string) ([]byte, bool, error) {
	data, err := os.ReadFile(c.path(key))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, false, nil
	}
	if time.Now().After(e.ExpiresAt) {
		_ = os.Remove(c.path(key))
		return nil, false, nil
	}
	return e.Data, true, nil
}

func (c *DiskCache) Set(key string, data []byte, ttl time.Duration) error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(entry{ExpiresAt: time.Now().Add(ttl), Data: data})
	if err != nil {
		return err
	}
	return os.WriteFile(c.path(key), b, 0o644)
}

func (c *DiskCache) Clear() error {
	entries, err := os.ReadDir(c.dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		_ = os.Remove(filepath.Join(c.dir, e.Name()))
	}
	return nil
}

func (c *DiskCache) Status() (count int, bytes int64, err error) {
	entries, err := os.ReadDir(c.dir)
	if os.IsNotExist(err) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		if info, _ := e.Info(); info != nil {
			bytes += info.Size()
		}
		count++
	}
	return count, bytes, nil
}
