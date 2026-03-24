package config

type Weights map[string]int

var factorNames = []string{
	"release_recency", "maintainer_count", "download_velocity",
	"open_issue_ratio", "org_backing", "version_pinning", "repo_health",
}

func Hobby() Config {
	return Config{
		Profile: "hobby", PassThreshold: 40, Depth: 10, Concurrency: 20,
		CacheTTL: DefaultCacheTTL(),
		Weights: Weights{
			"release_recency": 15, "maintainer_count": 5, "download_velocity": 15,
			"open_issue_ratio": 10, "org_backing": 5, "version_pinning": 25, "repo_health": 25,
		},
	}
}

func OpenSource() Config {
	return Config{
		Profile: "opensource", PassThreshold: 55, Depth: 10, Concurrency: 20,
		CacheTTL: DefaultCacheTTL(),
		Weights: Weights{
			"release_recency": 18, "maintainer_count": 12, "download_velocity": 15,
			"open_issue_ratio": 10, "org_backing": 8, "version_pinning": 17, "repo_health": 20,
		},
	}
}

func Enterprise() Config {
	return Config{
		Profile: "enterprise", PassThreshold: 70, Depth: 10, Concurrency: 20,
		CacheTTL: DefaultCacheTTL(),
		Weights: Weights{
			"release_recency": 20, "maintainer_count": 15, "download_velocity": 15,
			"open_issue_ratio": 10, "org_backing": 10, "version_pinning": 15, "repo_health": 15,
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
