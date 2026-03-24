// internal/registry/deps_test.go
package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPyPIFetchDependencies(t *testing.T) {
	// Mock PyPI response with requires_dist
	resp := map[string]any{
		"info": map[string]any{
			"name":    "langchain",
			"version": "0.1.0",
			"requires_dist": []string{
				"litellm (>=1.82.0)",
				"requests (>=2.0)",
				"pydantic (>=1.0) ; extra == \"extended\"",
			},
		},
		"releases": map[string]any{},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewPyPIClient(WithBaseURL(server.URL))
	deps, err := client.FetchDependencies("langchain", "0.1.0")
	require.NoError(t, err)
	assert.Len(t, deps, 2) // pydantic is an extra, should be excluded
	assert.Equal(t, "litellm", deps[0].Name)
	assert.Equal(t, ">=1.82.0", deps[0].Constraint)
	assert.Equal(t, "requests", deps[1].Name)
}

func TestNPMFetchDependencies(t *testing.T) {
	resp := map[string]any{
		"name":    "some-pkg",
		"version": "1.0.0",
		"dependencies": map[string]string{
			"litellm": "^1.82.0",
			"express": "^4.18.0",
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewNPMClient(WithBaseURL(server.URL))
	deps, err := client.FetchDependencies("some-pkg", "1.0.0")
	require.NoError(t, err)
	assert.Len(t, deps, 2)

	names := make(map[string]bool)
	for _, d := range deps {
		names[d.Name] = true
	}
	assert.True(t, names["litellm"])
	assert.True(t, names["express"])
}
