package config_test

import (
	"testing"

	"github.com/depscope/depscope/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	merged := base.WithWeights(config.Weights{"release_recency": 40}) // was 20
	total := 0
	for _, w := range merged.Weights {
		total += w
	}
	assert.Equal(t, 100, total, "merged weights must still sum to 100")
	assert.Equal(t, 40, merged.Weights["release_recency"])

	t.Run("multiple overrides", func(t *testing.T) {
		cfg := config.Enterprise()
		overridden := cfg.WithWeights(config.Weights{
			"release_recency":  30,
			"maintainer_count": 30,
		})
		sum := 0
		for _, v := range overridden.Weights {
			sum += v
		}
		assert.Equal(t, 100, sum)
		for k, v := range overridden.Weights {
			assert.GreaterOrEqual(t, v, 0, "weight %s should be non-negative", k)
		}
	})
}

func TestEnvVarResolution(t *testing.T) {
	t.Setenv("TEST_DEPSCOPE_TOKEN", "secret123")
	assert.Equal(t, "secret123", config.ResolveEnv("${TEST_DEPSCOPE_TOKEN}"))
	assert.Equal(t, "literal", config.ResolveEnv("literal"))
}

func TestLoadFile(t *testing.T) {
	cfg, err := config.LoadFile("testdata/depscope.yaml")
	require.NoError(t, err)
	assert.Equal(t, 75, cfg.PassThreshold)
	assert.Equal(t, "enterprise", cfg.Profile)
	sum := 0
	for _, v := range cfg.Weights {
		sum += v
	}
	assert.Equal(t, 100, sum, "loaded weights should sum to 100")
}

func TestProfileByName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"hobby", "hobby"},
		{"opensource", "opensource"},
		{"enterprise", "enterprise"},
		{"unknown", "enterprise"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ProfileByName(tt.name)
			assert.Equal(t, tt.want, cfg.Profile)
		})
	}
}
