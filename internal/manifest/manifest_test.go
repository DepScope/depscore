package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectEcosystem(t *testing.T) {
	tests := []struct {
		file     string
		expected manifest.Ecosystem
	}{
		{"go.mod", manifest.EcosystemGo},
		{"Cargo.toml", manifest.EcosystemRust},
		{"package.json", manifest.EcosystemNPM},
		{"requirements.txt", manifest.EcosystemPython},
		{"poetry.lock", manifest.EcosystemPython},
		{"uv.lock", manifest.EcosystemPython},
	}
	for _, tt := range tests {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, tt.file), []byte(""), 0o644))
		eco, err := manifest.DetectEcosystem(dir)
		require.NoError(t, err, tt.file)
		assert.Equal(t, tt.expected, eco, tt.file)
	}
}

func TestParseConstraintType(t *testing.T) {
	tests := []struct {
		constraint string
		expected   manifest.ConstraintType
	}{
		{"==1.2.3", manifest.ConstraintExact},
		{"=1.2.3", manifest.ConstraintExact},
		{"~=1.2.3", manifest.ConstraintPatch},
		{"~1.2", manifest.ConstraintPatch},
		{"^1.2.3", manifest.ConstraintMinor},
		{">=1.2,<2.0", manifest.ConstraintMinor},
		{">=1.0.0", manifest.ConstraintMajor},
		{"*", manifest.ConstraintMajor},
		{"latest", manifest.ConstraintMajor},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, manifest.ParseConstraintType(tt.constraint), tt.constraint)
	}
}

func TestPackageKey(t *testing.T) {
	p := manifest.Package{Name: "requests", ResolvedVersion: "2.31.0", Ecosystem: manifest.EcosystemPython}
	assert.Equal(t, "python/requests@2.31.0", p.Key())
}

func TestBuildDepsMap(t *testing.T) {
	pkgs := []manifest.Package{
		{Name: "a", Depth: 1, Ecosystem: manifest.EcosystemGo},
		{Name: "b", Depth: 2, Ecosystem: manifest.EcosystemGo, Parents: []string{"a"}},
		{Name: "c", Depth: 2, Ecosystem: manifest.EcosystemGo, Parents: []string{"a"}},
	}
	deps := manifest.BuildDepsMap(pkgs)
	assert.ElementsMatch(t, []string{"b", "c"}, deps["a"])
}

func TestBuildDepsMapFallback(t *testing.T) {
	// When Parents is empty (e.g. go.mod flat list), all depth-1 get all depth-2 as children.
	pkgs := []manifest.Package{
		{Name: "direct1", Depth: 1, Ecosystem: manifest.EcosystemGo},
		{Name: "indirect1", Depth: 2, Ecosystem: manifest.EcosystemGo},
	}
	deps := manifest.BuildDepsMap(pkgs)
	assert.Contains(t, deps["direct1"], "indirect1")
}
