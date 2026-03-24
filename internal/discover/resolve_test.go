// internal/discover/resolve_test.go
package discover

import (
	"testing"

	"github.com/depscope/depscope/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDepFetcher implements registry.DependencyFetcher for testing.
type mockDepFetcher struct {
	deps map[string][]registry.Dependency // key: "name@version"
}

func (m *mockDepFetcher) Fetch(name, version string) (*registry.PackageInfo, error) {
	return &registry.PackageInfo{Name: name, Version: version}, nil
}
func (m *mockDepFetcher) Ecosystem() string { return "PyPI" }
func (m *mockDepFetcher) FetchDependencies(name, version string) ([]registry.Dependency, error) {
	key := name + "@" + version
	return m.deps[key], nil
}

func TestResolveTransitiveFindsTarget(t *testing.T) {
	fetcher := &mockDepFetcher{
		deps: map[string][]registry.Dependency{
			"langchain@0.1.0": {
				{Name: "litellm", Constraint: ">=1.82.0"},
				{Name: "requests", Constraint: ">=2.0"},
			},
			"litellm@1.82.8": {},
			"requests@2.31.0": {},
		},
	}

	// Direct deps of the project being checked
	directDeps := []DepEntry{
		{Name: "langchain", Version: "0.1.0"},
	}

	result, err := ResolveTransitive("litellm", directDeps, fetcher, 10)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "litellm", result.Name)
	assert.Equal(t, ">=1.82.0", result.Constraint)
	assert.Equal(t, []string{"langchain", "litellm"}, result.Path)
}

func TestResolveTransitiveNotFound(t *testing.T) {
	fetcher := &mockDepFetcher{
		deps: map[string][]registry.Dependency{
			"requests@2.31.0": {
				{Name: "urllib3", Constraint: ">=1.26"},
			},
			"urllib3@1.26.0": {},
		},
	}

	directDeps := []DepEntry{
		{Name: "requests", Version: "2.31.0"},
	}

	result, err := ResolveTransitive("litellm", directDeps, fetcher, 10)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestResolveTransitiveMaxDepth(t *testing.T) {
	// Chain: a → b → c → litellm, but max depth = 2 → shouldn't find it
	fetcher := &mockDepFetcher{
		deps: map[string][]registry.Dependency{
			"a@1.0.0": {{Name: "b", Constraint: ">=1.0"}},
			"b@1.0.0": {{Name: "c", Constraint: ">=1.0"}},
			"c@1.0.0": {{Name: "litellm", Constraint: ">=1.82.0"}},
		},
	}

	directDeps := []DepEntry{{Name: "a", Version: "1.0.0"}}

	result, err := ResolveTransitive("litellm", directDeps, fetcher, 2)
	require.NoError(t, err)
	assert.Nil(t, result)
}
