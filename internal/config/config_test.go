package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/depscope/depscope/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnterpriseWeightsSumTo100(t *testing.T) {
	cfg := config.Enterprise()
	total := 0
	for _, w := range cfg.Weights {
		total += w
	}
	assert.Equal(t, 100, total, "enterprise weights must sum to 100")
}

func TestAllProfilesWeightsSumTo100(t *testing.T) {
	for _, cfg := range []config.Config{config.Hobby(), config.OpenSource(), config.Enterprise()} {
		total := 0
		for _, w := range cfg.Weights {
			total += w
		}
		assert.Equal(t, 100, total, "profile %s weights must sum to 100", cfg.Profile)
	}
}

func TestPartialWeightOverrideRenormalizes(t *testing.T) {
	base := config.Enterprise()
	merged := base.WithWeights(config.Weights{"release_recency": 40})
	total := 0
	for _, w := range merged.Weights {
		total += w
	}
	assert.Equal(t, 100, total, "merged weights must still sum to 100")
	assert.Equal(t, 40, merged.Weights["release_recency"])
}

func TestEnvVarResolution(t *testing.T) {
	t.Setenv("TEST_DEPSCOPE_TOKEN", "secret123")
	assert.Equal(t, "secret123", config.ResolveEnv("${TEST_DEPSCOPE_TOKEN}"))
	assert.Equal(t, "literal", config.ResolveEnv("literal"))
}

func TestLoadFile(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test")

	cfg, err := config.LoadFile("testdata/depscope.yaml")
	require.NoError(t, err)
	assert.Equal(t, 75, cfg.PassThreshold)
	assert.Equal(t, "enterprise", cfg.Profile)
	assert.Equal(t, "ghp_test", cfg.Registries.GitHubToken)
	// Backward compat: Auth.GitHubToken should be populated from registries.github_token
	assert.Equal(t, "ghp_test", cfg.Auth.GitHubToken)
}

func TestLoadFile_TrustedOrgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "depscope.yaml")
	if err := os.WriteFile(path, []byte(`
profile: enterprise
trusted_orgs:
  - github.com/my-company
  - gitlab.com/my-team
auth:
  github_token: ${GITHUB_TOKEN}
  gitlab_token: ${GITLAB_TOKEN}
concurrency:
  registry_workers: 15
  git_clone_workers: 5
  github_api_workers: 8
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TrustedOrgs) != 2 {
		t.Errorf("TrustedOrgs: got %d, want 2", len(cfg.TrustedOrgs))
	}
	if cfg.ConcurrencyConfig.RegistryWorkers != 15 {
		t.Errorf("RegistryWorkers: got %d, want 15", cfg.ConcurrencyConfig.RegistryWorkers)
	}
}
