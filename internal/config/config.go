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

// FactorNames is the canonical ordered list of scoring factor keys.
var FactorNames = []string{
	"release_recency", "maintainer_count", "download_velocity",
	"open_issue_ratio", "org_backing", "version_pinning", "repo_health",
}

// factorNames is an alias kept for internal use.
var factorNames = FactorNames

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
	if v.IsSet("max_depth") {
		cfg.MaxDepth = v.GetInt("max_depth")
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

	if v.IsSet("cache") {
		cfg.Cache = CacheTTL{
			MetadataHours: v.GetInt("cache.metadata_hours"),
			VulnHours:     v.GetInt("cache.vuln_hours"),
		}
	}
	if v.IsSet("vuln") {
		cfg.Vuln = VulnSources{
			OSV: v.GetBool("vuln.osv"),
			NVD: v.GetBool("vuln.nvd"),
		}
	}
	if v.IsSet("reg") {
		cfg.Reg = Registries{
			PyPI:    ResolveEnv(v.GetString("reg.py_pi")),
			NPM:     ResolveEnv(v.GetString("reg.npm")),
			Crates:  ResolveEnv(v.GetString("reg.crates")),
			GoProxy: ResolveEnv(v.GetString("reg.go_proxy")),
		}
	}
	return cfg, nil
}

// WithWeights applies partial weight overrides and renormalizes all weights to sum to 100.
func (c Config) WithWeights(overrides Weights) Config {
	total := 0
	for _, v := range overrides {
		total += v
	}
	if total > 100 {
		return c // overrides invalid, return unchanged
	}

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

	// Fix rounding drift: assign remainder to first non-overridden factor (deterministic order)
	sum := 0
	for _, v := range merged {
		sum += v
	}
	if diff := 100 - sum; diff != 0 {
		for _, k := range factorNames {
			if _, ok := overrides[k]; !ok {
				merged[k] += diff
				break
			}
		}
	}

	c.Weights = merged
	return c
}
