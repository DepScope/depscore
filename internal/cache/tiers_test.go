// internal/cache/tiers_test.go
package cache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheTierDurations(t *testing.T) {
	assert.Equal(t, 24*time.Hour, TTLRegistryMetadata)
	assert.Equal(t, 6*time.Hour, TTLCVEData)
	assert.Equal(t, 12*time.Hour, TTLRepoMetadata)
	assert.Equal(t, 87600*time.Hour, TTLImmutable)
	assert.Equal(t, 1*time.Hour, TTLActionRef)
	assert.Equal(t, 6*time.Hour, TTLDockerMetadata)
}

func TestCacheKeyBuilders(t *testing.T) {
	assert.Equal(t, "registry:PyPI:litellm:1.82.8", RegistryKey("PyPI", "litellm", "1.82.8"))
	assert.Equal(t, "repo:pallets/flask", RepoKey("pallets/flask"))
	assert.Equal(t, "repo:pallets/flask:abc123", RepoSHAKey("pallets/flask", "abc123"))
	assert.Equal(t, "action:actions/checkout:v4", ActionRefKey("actions/checkout", "v4"))
	assert.Equal(t, "docker:python:3.12-slim", DockerKey("python", "3.12-slim"))
	assert.Equal(t, "cve:PyPI:litellm:1.82.8", CVEKey("PyPI", "litellm", "1.82.8"))
}
