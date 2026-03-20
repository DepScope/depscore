package config

import (
	"fmt"
	"os"
	"regexp"

	"github.com/spf13/viper"
)

var envVarRe = regexp.MustCompile(`^\$\{(.+)\}$`)

// ResolveEnv replaces "${VAR}" with the value of the environment variable VAR.
// Returns the string unchanged if it does not match the pattern.
func ResolveEnv(v string) string {
	if m := envVarRe.FindStringSubmatch(v); m != nil {
		return os.Getenv(m[1])
	}
	return v
}

// LoadFile reads a YAML config file via Viper and merges it onto the named profile.
// The profile field in the file determines the base profile; explicit fields override it.
func LoadFile(path string) (Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}

	profileName := v.GetString("profile")
	cfg := ProfileByName(profileName)

	if v.IsSet("pass_threshold") {
		cfg.PassThreshold = v.GetInt("pass_threshold")
	}
	if v.IsSet("depth") {
		cfg.Depth = v.GetInt("depth")
	}
	if v.IsSet("concurrency") {
		cfg.Concurrency = v.GetInt("concurrency")
	}
	if v.IsSet("weights") {
		overrides := Weights{}
		for _, k := range factorNames {
			if v.IsSet("weights." + k) {
				overrides[k] = v.GetInt("weights." + k)
			}
		}
		if len(overrides) > 0 {
			cfg = cfg.WithWeights(overrides)
		}
	}

	cfg.VulnSources = VulnSources{
		OSV:       v.GetBool("vuln_sources.osv"),
		NVD:       v.GetBool("vuln_sources.nvd"),
		NVDAPIKey: ResolveEnv(v.GetString("vuln_sources.nvd_api_key")),
	}
	cfg.Registries = Registries{
		GitHubToken: ResolveEnv(v.GetString("registries.github_token")),
	}
	return cfg, nil
}

// WithWeights applies partial weight overrides and renormalizes all weights to sum to 100.
func (c Config) WithWeights(overrides Weights) Config {
	merged := make(Weights, len(c.Weights))
	for k, v := range overrides {
		merged[k] = v
	}

	overriddenSum := 0
	for _, v := range overrides {
		overriddenSum += v
	}

	baseRemaining := 0
	for k, v := range c.Weights {
		if _, ok := overrides[k]; !ok {
			baseRemaining += v
		}
	}

	remaining := 100 - overriddenSum
	for k, v := range c.Weights {
		if _, ok := overrides[k]; ok {
			continue
		}
		if baseRemaining == 0 {
			merged[k] = 0
		} else {
			merged[k] = int(float64(v) / float64(baseRemaining) * float64(remaining))
		}
	}

	// Fix rounding drift: assign remainder to first non-overridden factor
	total := 0
	for _, v := range merged {
		total += v
	}
	if diff := 100 - total; diff != 0 {
		for k := range c.Weights {
			if _, ok := overrides[k]; !ok {
				merged[k] += diff
				break
			}
		}
	}

	c.Weights = merged
	return c
}
