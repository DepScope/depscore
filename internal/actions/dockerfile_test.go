// internal/actions/dockerfile_test.go
package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDockerfile(t *testing.T) {
	content := `FROM python:3.12-slim AS builder
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt
RUN npm install
FROM node:20-alpine
COPY --from=builder /app /app
`
	result, err := ParseDockerfile([]byte(content))
	require.NoError(t, err)

	// Two FROM images
	assert.Len(t, result.BaseImages, 2)
	assert.Equal(t, "python", result.BaseImages[0].Image)
	assert.Equal(t, "3.12-slim", result.BaseImages[0].Tag)
	assert.Equal(t, "builder", result.BaseImages[0].Alias)
	assert.Equal(t, "node", result.BaseImages[1].Image)
	assert.Equal(t, "20-alpine", result.BaseImages[1].Tag)

	// Detects pip install and npm install
	assert.True(t, result.HasPipInstall)
	assert.True(t, result.HasNpmInstall)
}

func TestParseDockerfileDigest(t *testing.T) {
	content := `FROM alpine@sha256:abc123def456`
	result, err := ParseDockerfile([]byte(content))
	require.NoError(t, err)
	assert.Equal(t, "alpine", result.BaseImages[0].Image)
	assert.Equal(t, "sha256:abc123def456", result.BaseImages[0].Digest)
}
