package discover

import (
	"testing"

	"github.com/depscope/depscope/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyConfirmedFromLockfile(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)
	pkg := manifest.Package{Name: "litellm", ResolvedVersion: "1.82.8", Depth: 1}
	match := Classify(pkg, r, false)
	assert.Equal(t, StatusConfirmed, match.Status)
	assert.Equal(t, "1.82.8", match.Version)
	assert.Equal(t, "direct", match.Depth)
}

func TestClassifySafeFromLockfile(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)
	pkg := manifest.Package{Name: "litellm", ResolvedVersion: "1.83.1", Depth: 1}
	match := Classify(pkg, r, false)
	assert.Equal(t, StatusSafe, match.Status)
}

func TestClassifyPotentiallyFromConstraint(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)
	pkg := manifest.Package{Name: "litellm", Constraint: ">=1.80", Depth: 1}
	match := Classify(pkg, r, false)
	assert.Equal(t, StatusPotentially, match.Status)
}

func TestClassifySafeFromConstraint(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)
	pkg := manifest.Package{Name: "litellm", Constraint: ">=1.84", Depth: 1}
	match := Classify(pkg, r, false)
	assert.Equal(t, StatusSafe, match.Status)
}

func TestClassifyUnresolvable(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)
	pkg := manifest.Package{Name: "litellm", Depth: 1}
	match := Classify(pkg, r, true)
	assert.Equal(t, StatusUnresolvable, match.Status)
}

func TestClassifyTransitive(t *testing.T) {
	r, err := ParseRange(">=1.82.7,<1.83.0")
	require.NoError(t, err)
	pkg := manifest.Package{Name: "litellm", ResolvedVersion: "1.82.8", Depth: 2, Parents: []string{"langchain"}}
	match := Classify(pkg, r, false)
	assert.Equal(t, StatusConfirmed, match.Status)
	assert.Equal(t, "transitive", match.Depth)
	assert.Equal(t, []string{"langchain", "litellm"}, match.DependencyPath)
}
