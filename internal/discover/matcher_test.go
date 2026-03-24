package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchPackageInFiles(t *testing.T) {
	root := t.TempDir()
	uvLock := "[[package]]\nname = \"litellm\"\nversion = \"1.82.8\"\n\n[[package]]\nname = \"requests\"\nversion = \"2.31.0\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "uv.lock"), []byte(uvLock), 0o644))
	pyproject := "[project]\ndependencies = [\"requests>=2.0\"]\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte(pyproject), 0o644))

	project := ProjectInfo{
		Dir:           root,
		ManifestFiles: []string{"uv.lock", "pyproject.toml"},
	}

	matched := MatchPackageInProject("litellm", project)
	assert.True(t, matched.Bool())
	assert.Contains(t, matched.Files, "uv.lock")
	assert.NotContains(t, matched.Files, "pyproject.toml")
}

func TestMatchPackageNotFound(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\nrequire github.com/gin-gonic/gin v1.8.0\n"), 0o644))
	project := ProjectInfo{Dir: root, ManifestFiles: []string{"go.mod"}}

	matched := MatchPackageInProject("litellm", project)
	assert.False(t, matched.Bool())
}

func TestMatchPackageCaseInsensitive(t *testing.T) {
	root := t.TempDir()
	content := "[project]\ndependencies = [\"LiteLLM>=1.80\"]\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte(content), 0o644))
	project := ProjectInfo{Dir: root, ManifestFiles: []string{"pyproject.toml"}}

	matched := MatchPackageInProject("litellm", project)
	assert.True(t, matched.Bool())
}
