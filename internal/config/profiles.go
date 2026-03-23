package config

// Weights maps factor name to integer weight. All weights must sum to 100.
type Weights map[string]int

type CacheTTL struct {
	MetadataHours int
	VulnHours     int
}

type VulnSources struct {
	OSV bool
	NVD bool
}

type Registries struct {
	PyPI    string
	NPM     string
	Crates  string
	GoProxy string
}

type Config struct {
	Profile       string
	PassThreshold int
	MaxDepth      int
	Weights       Weights
	Cache         CacheTTL
	Vuln          VulnSources
	Reg           Registries
}

func Hobby() Config {
	return Config{
		Profile:       "hobby",
		PassThreshold: 40,
		MaxDepth:      10,
		Weights: Weights{
			"release_recency":   25,
			"maintainer_count":  20,
			"download_velocity": 20,
			"open_issue_ratio":  15,
			"org_backing":       5,
			"version_pinning":   10,
			"repo_health":       5,
		},
		Cache: CacheTTL{MetadataHours: 24, VulnHours: 6},
		Vuln:  VulnSources{OSV: true, NVD: false},
		Reg: Registries{
			PyPI:    "https://pypi.org",
			NPM:     "https://registry.npmjs.org",
			Crates:  "https://crates.io",
			GoProxy: "https://proxy.golang.org",
		},
	}
}

func OpenSource() Config {
	return Config{
		Profile:       "opensource",
		PassThreshold: 55,
		MaxDepth:      10,
		Weights: Weights{
			"release_recency":   20,
			"maintainer_count":  20,
			"download_velocity": 15,
			"open_issue_ratio":  10,
			"org_backing":       10,
			"version_pinning":   15,
			"repo_health":       10,
		},
		Cache: CacheTTL{MetadataHours: 24, VulnHours: 6},
		Vuln:  VulnSources{OSV: true, NVD: false},
		Reg: Registries{
			PyPI:    "https://pypi.org",
			NPM:     "https://registry.npmjs.org",
			Crates:  "https://crates.io",
			GoProxy: "https://proxy.golang.org",
		},
	}
}

func Enterprise() Config {
	return Config{
		Profile:       "enterprise",
		PassThreshold: 70,
		MaxDepth:      10,
		Weights: Weights{
			"release_recency":   20,
			"maintainer_count":  15,
			"download_velocity": 15,
			"open_issue_ratio":  10,
			"org_backing":       10,
			"version_pinning":   15,
			"repo_health":       15,
		},
		Cache: CacheTTL{MetadataHours: 24, VulnHours: 6},
		Vuln:  VulnSources{OSV: true, NVD: false},
		Reg: Registries{
			PyPI:    "https://pypi.org",
			NPM:     "https://registry.npmjs.org",
			Crates:  "https://crates.io",
			GoProxy: "https://proxy.golang.org",
		},
	}
}

func ProfileByName(name string) Config {
	switch name {
	case "hobby":
		return Hobby()
	case "opensource":
		return OpenSource()
	default:
		return Enterprise()
	}
}
