// internal/discover/version_test.go
package discover

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input string
		want  Version
	}{
		{"1.82.7", Version{Major: 1, Minor: 82, Patch: 7}},
		{"0.1.0", Version{Major: 0, Minor: 1, Patch: 0}},
		{"2.0.0", Version{Major: 2, Minor: 0, Patch: 0}},
		{"1.82.7rc1", Version{Major: 1, Minor: 82, Patch: 7, Pre: "rc1"}},
		{"v1.2.3", Version{Major: 1, Minor: 2, Patch: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want.Major, got.Major)
			assert.Equal(t, tt.want.Minor, got.Minor)
			assert.Equal(t, tt.want.Patch, got.Patch)
		})
	}
}

func TestParseVersionInvalid(t *testing.T) {
	_, err := ParseVersion("not-a-version")
	assert.Error(t, err)

	_, err = ParseVersion("")
	assert.Error(t, err)
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int // -1, 0, 1
	}{
		{"1.0.0", "2.0.0", -1},
		{"1.82.7", "1.82.7", 0},
		{"1.82.8", "1.82.7", 1},
		{"1.83.0", "1.82.9", 1},
		{"0.1.0", "0.2.0", -1},
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			a, _ := ParseVersion(tt.a)
			b, _ := ParseVersion(tt.b)
			assert.Equal(t, tt.want, a.Compare(b))
		})
	}
}

func TestParseRange(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{">=1.82.7,<1.83.0", false},
		{"==1.82.8", false},
		{">=1.82.7", false},
		{"<2.0.0", false},
		{">=1.0,<2.0,!=1.5.0", false},
		{"", true},
		{"invalid", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := ParseRange(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVersionInRange(t *testing.T) {
	tests := []struct {
		version string
		rng     string
		want    bool
	}{
		{"1.82.7", ">=1.82.7,<1.83.0", true},
		{"1.82.9", ">=1.82.7,<1.83.0", true},
		{"1.83.0", ">=1.82.7,<1.83.0", false},
		{"1.82.6", ">=1.82.7,<1.83.0", false},
		{"1.82.8", "==1.82.8", true},
		{"1.82.7", "==1.82.8", false},
		{"1.0.0", ">=1.82.7", false},
		{"2.0.0", ">=1.82.7", true},
		{"1.99.0", "<2.0.0", true},
		{"2.0.0", "<2.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.version+"_in_"+tt.rng, func(t *testing.T) {
			r, err := ParseRange(tt.rng)
			require.NoError(t, err)
			v, err := ParseVersion(tt.version)
			require.NoError(t, err)
			assert.Equal(t, tt.want, r.Contains(v))
		})
	}
}

func TestConstraintOverlaps(t *testing.T) {
	tests := []struct {
		constraint string
		rng        string
		want       bool
	}{
		{">=1.80", ">=1.82.7,<1.83.0", true},           // allows versions in range
		{">=1.84", ">=1.82.7,<1.83.0", false},           // starts above range
		{"<1.82.7", ">=1.82.7,<1.83.0", false},          // ends before range
		{">=1.82.7,<1.82.9", ">=1.82.7,<1.83.0", true}, // subset
		{"==1.82.8", ">=1.82.7,<1.83.0", true},          // exact match in range
		{"==1.84.0", ">=1.82.7,<1.83.0", false},         // exact match outside
		{">=1.0", ">=1.82.7,<1.83.0", true},             // wide open
		{"~=1.82.0", ">=1.82.7,<1.83.0", true},          // compatible release overlaps
	}
	for _, tt := range tests {
		t.Run(tt.constraint+"_overlaps_"+tt.rng, func(t *testing.T) {
			r, err := ParseRange(tt.rng)
			require.NoError(t, err)
			got, err := ConstraintOverlaps(tt.constraint, r)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
