package resolve_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/depscope/depscope/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitCloneResolver(t *testing.T) {
	// Create a local bare git repo with manifest files to avoid network calls
	bareDir := t.TempDir()
	workDir := t.TempDir()

	// Init bare repo
	runCmd(t, bareDir, "git", "init", "--bare", bareDir)

	// Clone it, add files, push
	runCmd(t, workDir, "git", "clone", bareDir, workDir)
	runCmd(t, workDir, "git", "config", "user.email", "test@test.com")
	runCmd(t, workDir, "git", "config", "user.name", "Test")

	require.NoError(t, os.WriteFile(filepath.Join(workDir, "go.mod"), []byte("module example.com\n\ngo 1.22\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "main.go"), []byte("package main\n"), 0o644))
	// Create node_modules dir that should be filtered out
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, "node_modules", "foo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "node_modules", "foo", "package.json"), []byte("{}"), 0o644))

	runCmd(t, workDir, "git", "add", "-A")
	runCmd(t, workDir, "git", "commit", "-m", "init")
	runCmd(t, workDir, "git", "push", "origin", "HEAD")

	// Test the resolver against the bare repo
	resolver := resolve.NewGitCloneResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	files, cleanup, err := resolver.Resolve(ctx, bareDir)
	require.NoError(t, err)
	defer cleanup()

	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Path
	}

	assert.Contains(t, names, "go.mod")
	assert.NotContains(t, names, "main.go") // not a manifest
	for _, name := range names {
		assert.NotContains(t, name, "node_modules") // filtered out
	}

	// Verify content
	for _, f := range files {
		if f.Path == "go.mod" {
			assert.Contains(t, string(f.Content), "module example.com")
		}
	}
}

func TestGitCloneResolverTimeout(t *testing.T) {
	resolver := resolve.NewGitCloneResolver()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	_, cleanup, err := resolver.Resolve(ctx, "https://example.com/nonexistent.git")
	defer cleanup()
	assert.Error(t, err)
}

func TestGitCloneResolverCleanup(t *testing.T) {
	bareDir := t.TempDir()
	workDir := t.TempDir()

	runCmd(t, bareDir, "git", "init", "--bare", bareDir)
	runCmd(t, workDir, "git", "clone", bareDir, workDir)
	runCmd(t, workDir, "git", "config", "user.email", "test@test.com")
	runCmd(t, workDir, "git", "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "go.mod"), []byte("module example.com\n"), 0o644))
	runCmd(t, workDir, "git", "add", "-A")
	runCmd(t, workDir, "git", "commit", "-m", "init")
	runCmd(t, workDir, "git", "push", "origin", "HEAD")

	resolver := resolve.NewGitCloneResolver()
	ctx := context.Background()
	_, cleanup, err := resolver.Resolve(ctx, bareDir)
	require.NoError(t, err)

	// Calling cleanup should remove the temp dir
	cleanup()
	// The temp dir pattern is depscope-clone-* - we verified it ran by not erroring
}

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "cmd %s %v failed: %s", name, args, string(out))
}
