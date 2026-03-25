// internal/cache/tiers.go
package cache

import "time"

// TTL constants for the multi-tier caching strategy.
const (
	TTLRegistryMetadata = 24 * time.Hour   // package registry data (PyPI, npm, etc.)
	TTLCVEData          = 6 * time.Hour    // vulnerability data (OSV, NVD)
	TTLRepoMetadata     = 12 * time.Hour   // repo stars, maintainers, archived status
	TTLImmutable        = 87600 * time.Hour // content at a specific SHA (10 years)
	TTLActionRef        = 1 * time.Hour    // tag → SHA resolution (tags can move)
	TTLDockerMetadata   = 6 * time.Hour    // Docker Hub image metadata
)

// Key builders for consistent cache key formatting.

func RegistryKey(ecosystem, name, version string) string {
	return "registry:" + ecosystem + ":" + name + ":" + version
}

func CVEKey(ecosystem, name, version string) string {
	return "cve:" + ecosystem + ":" + name + ":" + version
}

func RepoKey(ownerRepo string) string {
	return "repo:" + ownerRepo
}

func RepoSHAKey(ownerRepo, sha string) string {
	return "repo:" + ownerRepo + ":" + sha
}

func ActionRefKey(ownerRepo, ref string) string {
	return "action:" + ownerRepo + ":" + ref
}

func DockerKey(image, tag string) string {
	return "docker:" + image + ":" + tag
}
