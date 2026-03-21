package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheStatusNoCache(t *testing.T) {
	var stdout bytes.Buffer
	err := runCacheStatus(&stdout, t.TempDir())
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "0 entries")
}
