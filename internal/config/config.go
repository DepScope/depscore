package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/spf13/viper"
)

type CacheTTL struct {
	Metadata time.Duration
	CVE      time.Duration
}

func DefaultCacheTTL() CacheTTL {
	return CacheTTL{Metadata: 24 * time.Hour, CVE: 6 * time.Hour}
}

type VulnSources struct {
	OSV       bool
	NVD       bool
	NVDAPIKey string
}

type Registries struct {
	GitHubToken string // Deprecated: use Auth.GitHubToken instead
}

type Auth struct {
	GitHubToken    string
	GitLabToken    string
	TerraformToken string
	BitbucketToken string
}

type ConcurrencyConfig struct {
	RegistryWorkers  int // default 10
	GitCloneWorkers  int // default 3
	GitHubAPIWorkers int // default 5
}

func DefaultConcurrency() ConcurrencyConfig {
	return ConcurrencyConfig{RegistryWorkers: 10, GitCloneWorkers: 3, GitHubAPIWorkers: 5}
}

type Config struct {
	Profile           string
	PassThreshold     int
	Depth             int
	Concurrency       int
	CacheTTL          CacheTTL
	Weights           Weights
	VulnSources       VulnSources
	Registries        Registries
	TrustedOrgs       []string
	Auth              Auth
	ConcurrencyConfig ConcurrencyConfig
}

var envVarRe = regexp.MustCompile(`^\$\{(.+)\}$`)

func ResolveEnv(v string) string {
	if m := envVarRe.FindStringSubmatch(v); m != nil {
		return os.Getenv(m[1])
	}
	return v
}

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

	cfg.TrustedOrgs = v.GetStringSlice("trusted_orgs")
	cfg.Auth = Auth{
		GitHubToken:    ResolveEnv(v.GetString("auth.github_token")),
		GitLabToken:    ResolveEnv(v.GetString("auth.gitlab_token")),
		TerraformToken: ResolveEnv(v.GetString("auth.terraform_token")),
		BitbucketToken: ResolveEnv(v.GetString("auth.bitbucket_token")),
	}
	// Backward compat: populate Auth.GitHubToken from registries.github_token if auth.github_token not set
	if cfg.Auth.GitHubToken == "" && cfg.Registries.GitHubToken != "" {
		cfg.Auth.GitHubToken = cfg.Registries.GitHubToken
	}
	cfg.ConcurrencyConfig = DefaultConcurrency()
	if v.IsSet("concurrency.registry_workers") {
		cfg.ConcurrencyConfig.RegistryWorkers = v.GetInt("concurrency.registry_workers")
	}
	if v.IsSet("concurrency.git_clone_workers") {
		cfg.ConcurrencyConfig.GitCloneWorkers = v.GetInt("concurrency.git_clone_workers")
	}
	if v.IsSet("concurrency.github_api_workers") {
		cfg.ConcurrencyConfig.GitHubAPIWorkers = v.GetInt("concurrency.github_api_workers")
	}
	return cfg, nil
}

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
