package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testFetcher struct{}

func (f *testFetcher) Fetch(name, version string) (*registry.PackageInfo, error) {
	return &registry.PackageInfo{
		Name:             name,
		Version:          version,
		MaintainerCount:  3,
		HasOrgBacking:    true,
		LastReleaseAt:    time.Now().Add(-30 * 24 * time.Hour),
		MonthlyDownloads: 1_000_000,
		SourceRepoURL:    "https://github.com/psf/requests",
	}, nil
}
func (f *testFetcher) Ecosystem() string { return "python" }

func TestScanCommandOutputsTable(t *testing.T) {
	cfg := config.Hobby()
	var stdout bytes.Buffer
	err := scanDeps(&stdout, "testdata/fixture-python", cfg, "text", &testFetcher{})
	// May return exitError{1} if below threshold -- that's OK
	if err != nil {
		var ee exitError
		require.ErrorAs(t, err, &ee)
	}
	out := stdout.String()
	assert.Contains(t, out, "requests")
	assert.Contains(t, out, "urllib3")
}

func TestIsGitURL(t *testing.T) {
	assert.True(t, isGitURL("https://github.com/user/repo"))
	assert.True(t, isGitURL("http://github.com/user/repo"))
	assert.True(t, isGitURL("git@github.com:user/repo.git"))
	assert.False(t, isGitURL("."))
	assert.False(t, isGitURL("/home/user/project"))
	assert.False(t, isGitURL("./relative/path"))
}

func TestScanCommandJSON(t *testing.T) {
	cfg := config.Hobby()
	var stdout bytes.Buffer
	err := scanDeps(&stdout, "testdata/fixture-python", cfg, "json", &testFetcher{})
	if err != nil {
		var ee exitError
		require.ErrorAs(t, err, &ee)
	}
	assert.True(t, json.Valid(stdout.Bytes()), "output should be valid JSON")
}
