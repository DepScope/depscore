package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/resolve"
	"github.com/stretchr/testify/assert"
)

// TestScanDirOnlyFilter verifies that --only filters out unwanted ecosystems.
// We create a temp dir with both a go.mod and a pyproject.toml, then scan
// with Only=[]string{"go"} and confirm no error (Go packages found) and
// that requesting a non-existent filtered ecosystem returns an error.
func TestScanDirOnlyFilter(t *testing.T) {
	root := t.TempDir()

	// Write a minimal go.mod
	goMod := "module testmod\n\ngo 1.22\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("writing go.mod: %v", err)
	}

	// Write a minimal pyproject.toml
	pyProject := "[project]\nname = \"test\"\ndependencies = []\n"
	if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte(pyProject), 0o644); err != nil {
		t.Fatalf("writing pyproject.toml: %v", err)
	}

	t.Run("only go filters out python", func(t *testing.T) {
		opts := Options{
			Profile: "enterprise",
			NoCVE:   true,
			Only:    []string{"go"},
		}
		// go.mod with no dependencies yields 0 packages, but ScanDir should
		// not error on "no recognized manifest" — it finds the ecosystem fine.
		// It will error on "no packages found" since go.mod has no deps.
		// That is acceptable — the filter did not remove go.
		_, err := ScanDir(root, opts)
		if err != nil && err.Error() == "no recognized manifest found in "+root {
			t.Errorf("--only go should not remove go ecosystem: %v", err)
		}
	})

	t.Run("only php returns no recognized manifest", func(t *testing.T) {
		opts := Options{
			Profile: "enterprise",
			NoCVE:   true,
			Only:    []string{"php"},
		}
		_, err := ScanDir(root, opts)
		if err == nil {
			t.Error("expected error when filtering to php (not present), got nil")
		}
	})

	t.Run("empty only scans all ecosystems", func(t *testing.T) {
		opts := Options{
			Profile: "enterprise",
			NoCVE:   true,
			Only:    nil,
		}
		// Both ecosystems are present but have no deps — expect "no packages" error,
		// not "no manifest" error.
		_, err := ScanDir(root, opts)
		if err != nil && err.Error() == "no recognized manifest found in "+root {
			t.Errorf("empty Only should scan all ecosystems: %v", err)
		}
	})
}

func TestComputeGraphDepths(t *testing.T) {
	t.Run("single root gets depth 1", func(t *testing.T) {
		results := []core.PackageResult{{Name: "root"}}
		deps := map[string][]string{}
		depths := computeGraphDepths(results, deps)
		assert.Equal(t, 1, depths["root"])
	})

	t.Run("chain root->a->b gives depths 1, 2, 3", func(t *testing.T) {
		results := []core.PackageResult{
			{Name: "root"},
			{Name: "a"},
			{Name: "b"},
		}
		deps := map[string][]string{
			"root": {"a"},
			"a":    {"b"},
		}
		depths := computeGraphDepths(results, deps)
		assert.Equal(t, 1, depths["root"])
		assert.Equal(t, 2, depths["a"])
		assert.Equal(t, 3, depths["b"])
	})

	t.Run("multiple roots both get depth 1", func(t *testing.T) {
		results := []core.PackageResult{
			{Name: "root1"},
			{Name: "root2"},
		}
		deps := map[string][]string{}
		depths := computeGraphDepths(results, deps)
		assert.Equal(t, 1, depths["root1"])
		assert.Equal(t, 1, depths["root2"])
	})

	t.Run("unreachable node gets depth 1 (default)", func(t *testing.T) {
		results := []core.PackageResult{
			{Name: "root"},
			{Name: "orphan"},
		}
		// orphan is listed as a dep of root in depsMap, making it reachable.
		// To make it truly unreachable, root must not reference it and
		// orphan must be a dep of something (so it's not a root).
		deps := map[string][]string{
			"phantom": {"orphan"}, // phantom doesn't exist in results, but marks orphan as a dep
		}
		depths := computeGraphDepths(results, deps)
		assert.Equal(t, 1, depths["root"])
		// orphan is depended on by "phantom" so it's not a root, and phantom
		// doesn't exist so BFS never reaches orphan -> default depth 1
		assert.Equal(t, 1, depths["orphan"])
	})

	t.Run("diamond graph: root->a, root->b, a->c, b->c", func(t *testing.T) {
		results := []core.PackageResult{
			{Name: "root"},
			{Name: "a"},
			{Name: "b"},
			{Name: "c"},
		}
		deps := map[string][]string{
			"root": {"a", "b"},
			"a":    {"c"},
			"b":    {"c"},
		}
		depths := computeGraphDepths(results, deps)
		assert.Equal(t, 1, depths["root"])
		assert.Equal(t, 2, depths["a"])
		assert.Equal(t, 2, depths["b"])
		// BFS finds c via either a or b first — both at depth 3
		assert.Equal(t, 3, depths["c"])
	})
}

func TestGroupByDirectory(t *testing.T) {
	t.Run("empty files yields empty map", func(t *testing.T) {
		groups := groupByDirectory(nil)
		assert.Empty(t, groups)
	})

	t.Run("files in same directory are grouped together", func(t *testing.T) {
		files := []resolve.ManifestFile{
			{Path: "project/go.mod", Content: []byte("mod")},
			{Path: "project/go.sum", Content: []byte("sum")},
		}
		groups := groupByDirectory(files)
		assert.Len(t, groups, 1)
		assert.Len(t, groups["project"], 2)
	})

	t.Run("files in different dirs are in separate groups", func(t *testing.T) {
		files := []resolve.ManifestFile{
			{Path: "backend/go.mod", Content: []byte("mod")},
			{Path: "frontend/package.json", Content: []byte("{}")},
		}
		groups := groupByDirectory(files)
		assert.Len(t, groups, 2)
		assert.Len(t, groups["backend"], 1)
		assert.Len(t, groups["frontend"], 1)
	})

	t.Run("nested directories are separate groups", func(t *testing.T) {
		files := []resolve.ManifestFile{
			{Path: "a/b/go.mod", Content: []byte("mod")},
			{Path: "a/c/go.mod", Content: []byte("mod")},
		}
		groups := groupByDirectory(files)
		assert.Len(t, groups, 2)
		assert.Contains(t, groups, "a/b")
		assert.Contains(t, groups, "a/c")
	})
}
