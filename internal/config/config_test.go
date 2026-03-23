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
	assert.Equal(t, 30, cfg.Weights["release_recency"])
	sum := 0
	for _, v := range cfg.Weights {
		sum += v
	}
	assert.Equal(t, 100, sum, "loaded weights should sum to 100")
	for k, v := range cfg.Weights {
		assert.GreaterOrEqual(t, v, 0, "weight %s should be non-negative", k)
	}
}

func TestFactorNamesMatchProfileKeys(t *testing.T) {
	cfg := config.Enterprise()
	// Check every factorName exists as a weight key
	for _, name := range config.FactorNames {
		_, ok := cfg.Weights[name]
		assert.True(t, ok, "factorNames contains %q but it is not a key in Enterprise weights", name)
	}
	// Check every weight key has a corresponding factorName
	nameSet := make(map[string]bool, len(config.FactorNames))
	for _, n := range config.FactorNames {
		nameSet[n] = true
	}
	for k := range cfg.Weights {
		assert.True(t, nameSet[k], "Enterprise weights contains %q but it is not in factorNames", k)
	}
}

func TestWithWeightsIgnoresUnknownKeys(t *testing.T) {
	cfg := config.Enterprise()
	overridden := cfg.WithWeights(config.Weights{
		"release_recency": 30,
		"nonexistent_key": 50, // unknown factor — should be ignored
	})
	// unknown key must not appear in result
	_, exists := overridden.Weights["nonexistent_key"]
	assert.False(t, exists, "unknown override key should be ignored")
	// sum must still be 100
	sum := 0
	for _, v := range overridden.Weights {
		sum += v
	}
	assert.Equal(t, 100, sum)
	// overriding key must be applied
	assert.Equal(t, 30, overridden.Weights["release_recency"])
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
