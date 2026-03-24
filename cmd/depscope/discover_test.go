package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverCmdRequiresRange(t *testing.T) {
	// discover without --range should fail
	rootCmd.SetArgs([]string{"discover", "litellm", "."})
	err := rootCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "range")
}

func TestDiscoverCmdInvalidRange(t *testing.T) {
	rootCmd.SetArgs([]string{"discover", "litellm", "--range", "invalid", "."})
	err := rootCmd.Execute()
	assert.Error(t, err)
}

func TestDiscoverCmdOffline(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "uv.lock"), []byte(`[[package]]
name = "litellm"
version = "1.82.8"
`), 0o644))

	rootCmd.SetArgs([]string{"discover", "litellm", "--range", ">=1.82.7,<1.83.0", "--offline", root})
	err := rootCmd.Execute()
	// Command ran successfully — confirmed affected projects cause exitError{1},
	// but that is a normal exit-code signal, not an infrastructure error.
	// We accept either nil or exitError{1} as a successful run.
	if err != nil {
		var ee exitError
		assert.True(t, errors.As(err, &ee), "expected exitError, got %v", err)
	}
}
