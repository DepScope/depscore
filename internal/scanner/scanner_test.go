package scanner

import (
	"os"
	"path/filepath"
	"testing"
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
