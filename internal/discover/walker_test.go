// internal/discover/walker_test.go
package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkProjects(t *testing.T) {
	// Create temp dir structure:
	// root/
	//   project-a/pyproject.toml
	//   project-b/package.json
	//   project-c/go.mod
	//   empty-dir/
	//   node_modules/some-pkg/package.json  (should be ignored)
	root := t.TempDir()

	dirs := []string{
		"project-a", "project-b", "project-c", "empty-dir",
		"node_modules/some-pkg",
	}
	for _, d := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(root, d), 0o755))
	}
	files := map[string]string{
		"project-a/pyproject.toml":          "[project]\n",
		"project-b/package.json":            "{}",
		"project-c/go.mod":                  "module example\n",
		"node_modules/some-pkg/package.json": "{}",
	}
	for path, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(content), 0o644))
	}

	projects, err := WalkProjects(root, 10, "")
	require.NoError(t, err)

	// Should find 3 projects, not the node_modules one
	assert.Len(t, projects, 3)

	paths := make([]string, len(projects))
	for i, p := range projects {
		paths[i] = p.Dir
	}
	assert.Contains(t, paths, filepath.Join(root, "project-a"))
	assert.Contains(t, paths, filepath.Join(root, "project-b"))
	assert.Contains(t, paths, filepath.Join(root, "project-c"))
}

func TestWalkProjectsMaxDepth(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(deep, "go.mod"), []byte("module x\n"), 0o644))

	// Depth 2 should NOT find it (4 levels deep)
	projects, err := WalkProjects(root, 2, "")
	require.NoError(t, err)
	assert.Len(t, projects, 0)

	// Depth 5 should find it
	projects, err = WalkProjects(root, 5, "")
	require.NoError(t, err)
	assert.Len(t, projects, 1)
}

func TestWalkProjectsEcosystemFilter(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "py"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "js"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "py", "pyproject.toml"), []byte("[project]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "js", "package.json"), []byte("{}"), 0o644))

	projects, err := WalkProjects(root, 10, "python")
	require.NoError(t, err)
	assert.Len(t, projects, 1)
	assert.Equal(t, filepath.Join(root, "py"), projects[0].Dir)
}

func TestReadProjectList(t *testing.T) {
	root := t.TempDir()
	// Create two project dirs
	for _, name := range []string{"proj1", "proj2"} {
		dir := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644))
	}

	// Write project list file
	listContent := filepath.Join(root, "proj1") + "\n" +
		"# comment\n" +
		"\n" +
		filepath.Join(root, "proj2") + "\n"
	listFile := filepath.Join(root, "projects.txt")
	require.NoError(t, os.WriteFile(listFile, []byte(listContent), 0o644))

	projects, err := ReadProjectList(listFile, "")
	require.NoError(t, err)
	assert.Len(t, projects, 2)
}
