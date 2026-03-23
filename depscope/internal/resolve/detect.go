package resolve

import (
	"net/url"
	"strings"
)

type DetectOptions struct {
	GitHubToken string
	GitLabToken string
	MaxFiles    int // max manifest files to fetch per resolve (0 = default 5000)
}

func IsRemoteURL(arg string) bool {
	for _, prefix := range []string{"http://", "https://", "ssh://", "git@"} {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

type TypedResolver interface {
	Resolver
	Type() string
}

func DetectResolver(rawURL string, opts DetectOptions) TypedResolver {
	var resolveOpts []Option
	if opts.MaxFiles > 0 {
		resolveOpts = append(resolveOpts, WithMaxFiles(opts.MaxFiles))
	}
	host := extractHost(rawURL)
	switch {
	case strings.Contains(host, "github.com"):
		return NewGitHubResolver(opts.GitHubToken, resolveOpts...)
	case strings.Contains(host, "gitlab.com"):
		return NewGitLabResolver(opts.GitLabToken, resolveOpts...)
	default:
		return NewGitCloneResolver(resolveOpts...)
	}
}

func ParseGitHubURL(rawURL string) (owner, repo, ref string) {
	rawURL = strings.TrimSuffix(rawURL, ".git")
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return
	}
	owner, repo = parts[0], parts[1]
	if len(parts) >= 4 && parts[2] == "tree" {
		ref = strings.Join(parts[3:], "/")
	}
	return
}

func ParseGitLabURL(rawURL string) (project, ref string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	path := strings.Trim(u.Path, "/")
	if idx := strings.Index(path, "/-/tree/"); idx >= 0 {
		project = path[:idx]
		ref = path[idx+len("/-/tree/"):]
		return
	}
	project = path
	return
}

func extractHost(rawURL string) string {
	if strings.HasPrefix(rawURL, "git@") {
		parts := strings.SplitN(rawURL, ":", 2)
		if len(parts) == 2 {
			return strings.TrimPrefix(parts[0], "git@")
		}
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// Option is a functional option for resolvers (e.g. override base URL in tests).
type Option func(*resolverOptions)

type resolverOptions struct {
	baseURL  string
	maxFiles int
}

func WithBaseURL(url string) Option {
	return func(o *resolverOptions) { o.baseURL = url }
}

func WithMaxFiles(n int) Option {
	return func(o *resolverOptions) { o.maxFiles = n }
}
