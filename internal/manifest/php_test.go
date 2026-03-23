package manifest_test

import (
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPHPParser(t *testing.T) {
	p := manifest.NewPHPParser()
	pkgs, err := p.Parse("testdata/php")
	require.NoError(t, err)

	m := pkgMap(pkgs)
	// Should have 3 packages (laravel/framework, guzzlehttp/guzzle, phpunit/phpunit)
	// NOT "php" or "ext-*"
	assert.Len(t, pkgs, 3)
	assert.Contains(t, m, "laravel/framework")
	assert.Equal(t, "13.0.1", m["laravel/framework"].ResolvedVersion)
	assert.Equal(t, manifest.ConstraintMinor, m["laravel/framework"].ConstraintType)
	// guzzlehttp/guzzle should have laravel/framework as parent
	assert.Contains(t, m["guzzlehttp/guzzle"].Parents, "laravel/framework")
}

func TestPHPParserEcosystem(t *testing.T) {
	p := manifest.NewPHPParser()
	assert.Equal(t, manifest.EcosystemPHP, p.Ecosystem())
}

func TestPHPConstraintType(t *testing.T) {
	tests := []struct {
		constraint string
		expected   manifest.ConstraintType
	}{
		{"^13.0", manifest.ConstraintMinor},
		{"^7.8", manifest.ConstraintMinor},
		{"~2.1", manifest.ConstraintPatch},
		{"1.2.3", manifest.ConstraintExact},
		{">=8.0", manifest.ConstraintMajor},
		{">1.0", manifest.ConstraintMajor},
		{"*", manifest.ConstraintMajor},
	}
	for _, tt := range tests {
		p := manifest.NewPHPParser()
		files := map[string][]byte{
			"composer.json": []byte(`{"require":{"vendor/pkg":"` + tt.constraint + `"}}`),
		}
		pkgs, err := p.ParseFiles(files)
		require.NoError(t, err, tt.constraint)
		require.Len(t, pkgs, 1, tt.constraint)
		assert.Equal(t, tt.expected, pkgs[0].ConstraintType, tt.constraint)
	}
}

func TestPHPParserSkipsPhpAndExtensions(t *testing.T) {
	p := manifest.NewPHPParser()
	files := map[string][]byte{
		"composer.json": []byte(`{
			"require": {
				"php": "^8.3",
				"ext-json": "*",
				"ext-mbstring": "*",
				"lib-curl": "*",
				"vendor/package": "^1.0"
			}
		}`),
	}
	pkgs, err := p.ParseFiles(files)
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
	assert.Equal(t, "vendor/package", pkgs[0].Name)
}
