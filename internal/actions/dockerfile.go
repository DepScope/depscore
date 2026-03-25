// internal/actions/dockerfile.go
package actions

import (
	"regexp"
	"strings"
)

// BaseImage represents a parsed FROM instruction in a Dockerfile.
type BaseImage struct {
	Image  string // e.g., "python"
	Tag    string // e.g., "3.12-slim"
	Digest string // e.g., "sha256:abc123"
	Alias  string // AS alias from multi-stage builds
}

// DockerfileResult holds the parsed contents of a Dockerfile.
type DockerfileResult struct {
	BaseImages    []BaseImage
	HasPipInstall bool
	HasNpmInstall bool
}

var (
	// FROM image:tag AS alias
	fromRegex = regexp.MustCompile(`(?i)^FROM\s+(\S+?)(?:\s+AS\s+(\S+))?$`)
	// image@sha256:digest
	digestRegex = regexp.MustCompile(`^([^@]+)@(.+)$`)
	// image:tag
	tagRegex = regexp.MustCompile(`^([^:]+):(.+)$`)
)

// ParseDockerfile parses a Dockerfile and extracts base images and notable patterns.
func ParseDockerfile(content []byte) (*DockerfileResult, error) {
	result := &DockerfileResult{}
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check FROM lines
		if m := fromRegex.FindStringSubmatch(line); m != nil {
			imageRef := m[1]
			alias := ""
			if len(m) > 2 {
				alias = m[2]
			}
			bi := parseBaseImage(imageRef)
			bi.Alias = alias
			result.BaseImages = append(result.BaseImages, bi)
		}

		// Check RUN lines for pip install / npm install
		if regexp.MustCompile(`(?i)^RUN\b`).MatchString(line) {
			if strings.Contains(line, "pip install") {
				result.HasPipInstall = true
			}
			if strings.Contains(line, "npm install") {
				result.HasNpmInstall = true
			}
		}
	}

	return result, nil
}

func parseBaseImage(ref string) BaseImage {
	// Check for digest: image@sha256:...
	if m := digestRegex.FindStringSubmatch(ref); m != nil {
		img := m[1]
		digest := m[2]
		return BaseImage{Image: img, Digest: digest}
	}

	// Check for tag: image:tag
	if m := tagRegex.FindStringSubmatch(ref); m != nil {
		return BaseImage{Image: m[1], Tag: m[2]}
	}

	// No tag or digest
	return BaseImage{Image: ref}
}
