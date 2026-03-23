package config_test

import (
	"os"
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
	os.Setenv("TEST_DEPSCOPE_TOKEN", "secret123")
	defer os.Unsetenv("TEST_DEPSCOPE_TOKEN")
	assert.Equal(t, "secret123", config.ResolveEnv("${TEST_DEPSCOPE_TOKEN}"))
	assert.Equal(t, "literal", config.ResolveEnv("literal"))
}

func TestLoadFile(t *testing.T) {
	os.Setenv("GITHUB_TOKEN", "ghp_test")
	defer os.Unsetenv("GITHUB_TOKEN")

	cfg, err := config.LoadFile("testdata/depscope.yaml")
	require.NoError(t, err)
	assert.Equal(t, 75, cfg.PassThreshold)
	assert.Equal(t, "enterprise", cfg.Profile)
	assert.Equal(t, "ghp_test", cfg.Registries.GitHubToken)
}
