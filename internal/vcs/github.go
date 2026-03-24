package vcs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const githubDefaultBaseURL = "https://api.github.com"

// RepoInfo holds VCS-level health signals for a repository.
type RepoInfo struct {
	Owner            string
	Repo             string
	ContributorCount int
	OpenIssueCount   int
	ClosedIssueCount int
	StarCount        int
	LastCommitAt     time.Time
	IsArchived       bool
	HasOrgBacking    bool
}

// Client is the interface for VCS repository clients.
type Client interface {
	FetchRepo(owner, repo string) (*RepoInfo, error)
	RepoFromURL(sourceURL string) (*RepoInfo, error)
}

// GitHubClient fetches repository metadata from the GitHub REST API.
type GitHubClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// Option is a functional option for GitHubClient.
type Option func(*githubOptions)

type githubOptions struct {
	baseURL string
	token   string
}

// WithBaseURL overrides the API base URL (useful for testing).
func WithBaseURL(u string) Option {
	return func(o *githubOptions) { o.baseURL = u }
}

// WithToken sets a GitHub personal access token for authenticated requests.
func WithToken(t string) Option {
	return func(o *githubOptions) { o.token = t }
}

// NewGitHubClient constructs a new GitHub VCS client.
func NewGitHubClient(opts ...Option) *GitHubClient {
	o := &githubOptions{baseURL: githubDefaultBaseURL}
	for _, opt := range opts {
		opt(o)
	}
	return &GitHubClient{
		baseURL:    o.baseURL,
		token:      o.token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// FetchRepo retrieves repo metadata for owner/repo.
func (c *GitHubClient) FetchRepo(owner, repo string) (*RepoInfo, error) {
	repoURL := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo)
	var raw githubRepo
	if err := c.getJSON(repoURL, &raw); err != nil {
		return nil, fmt.Errorf("github: fetch repo %s/%s: %w", owner, repo, err)
	}

	info := raw.toRepoInfo(owner, repo)

	// Fetch contributor count from the contributors endpoint (per_page=1&anon=1
	// and inspect Link header would be ideal, but for simplicity we fetch one
	// page and count entries).
	contribURL := fmt.Sprintf("%s/repos/%s/%s/contributors?per_page=100&anon=0", c.baseURL, owner, repo)
	var contributors []githubContributor
	if err := c.getJSON(contribURL, &contributors); err == nil {
		info.ContributorCount = len(contributors)
	}

	return info, nil
}

// RepoFromURL parses a GitHub URL and calls FetchRepo.
func (c *GitHubClient) RepoFromURL(sourceURL string) (*RepoInfo, error) {
	owner, repo, err := parseGitHubURL(sourceURL)
	if err != nil {
		return nil, err
	}
	return c.FetchRepo(owner, repo)
}

// ---- helpers ---------------------------------------------------------------

func (c *GitHubClient) getJSON(rawURL string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s returned %d", rawURL, resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// parseGitHubURL extracts owner and repo from various GitHub URL forms.
func parseGitHubURL(rawURL string) (owner, repo string, err error) {
	// Normalise git+ prefix and .git suffix.
	rawURL = strings.TrimPrefix(rawURL, "git+")
	rawURL = strings.TrimSuffix(rawURL, ".git")

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("vcs: invalid URL %q: %w", rawURL, err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("vcs: cannot extract owner/repo from %q", rawURL)
	}

	return parts[0], parts[1], nil
}

// ---- raw JSON shapes -------------------------------------------------------

type githubRepo struct {
	FullName        string      `json:"full_name"`
	Owner           githubOwner `json:"owner"`
	OpenIssuesCount int         `json:"open_issues_count"`
	StargazersCount int         `json:"stargazers_count"`
	Archived        bool        `json:"archived"`
	PushedAt        string      `json:"pushed_at"`
}

type githubOwner struct {
	Type string `json:"type"`
}

type githubContributor struct {
	Login string `json:"login"`
}

func (r githubRepo) toRepoInfo(owner, repo string) *RepoInfo {
	info := &RepoInfo{
		Owner:          owner,
		Repo:           repo,
		OpenIssueCount: r.OpenIssuesCount,
		StarCount:      r.StargazersCount,
		IsArchived:     r.Archived,
		HasOrgBacking:  strings.EqualFold(r.Owner.Type, "Organization"),
	}

	if t, err := time.Parse(time.RFC3339, r.PushedAt); err == nil {
		info.LastCommitAt = t
	}

	return info
}
