package resolve

import (
	"context"
	"net/url"
	"strings"
)

type DetectOptions struct {
	GitHubToken string
	GitLabToken string
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
	host := extractHost(rawURL)
	switch {
	case strings.Contains(host, "github.com"):
		return NewGitHubResolver(opts.GitHubToken)
	case strings.Contains(host, "gitlab.com"):
		return NewGitLabResolver(opts.GitLabToken)
	default:
		return NewGitCloneResolver()
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

// Stub resolvers — replaced in Tasks 4-5 with real implementations.

type gitlabResolver struct{ token string }

func NewGitLabResolver(token string, opts ...Option) TypedResolver {
	return &gitlabResolver{token: token}
}
func (r *gitlabResolver) Type() string { return "gitlab" }
func (r *gitlabResolver) Resolve(ctx context.Context, url string) ([]ManifestFile, func(), error) {
	return nil, func() {}, nil
}

type gitCloneResolver struct{}

func NewGitCloneResolver() TypedResolver { return &gitCloneResolver{} }
func (r *gitCloneResolver) Type() string { return "gitclone" }
func (r *gitCloneResolver) Resolve(ctx context.Context, url string) ([]ManifestFile, func(), error) {
	return nil, func() {}, nil
}

// Option is a functional option for resolvers (e.g. override base URL in tests).
type Option func(*resolverOptions)

type resolverOptions struct {
	baseURL string
}

func WithBaseURL(url string) Option {
	return func(o *resolverOptions) { o.baseURL = url }
}
