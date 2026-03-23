package vcs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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

type Client interface {
	FetchRepo(owner, repo string) (*RepoInfo, error)
	RepoFromURL(sourceURL string) (*RepoInfo, error)
}

type vcsOption func(*vcsOptions)

type vcsOptions struct {
	baseURL    string
	httpClient *http.Client
}

func WithBaseURL(url string) vcsOption {
	return func(o *vcsOptions) { o.baseURL = url }
}

type GitHubClient struct {
	token string
	opts  vcsOptions
}

func NewGitHubClient(token string, opts ...vcsOption) *GitHubClient {
	c := &GitHubClient{
		token: token,
		opts: vcsOptions{
			baseURL:    "https://api.github.com",
			httpClient: http.DefaultClient,
		},
	}
	for _, o := range opts {
		o(&c.opts)
	}
	return c
}

func (c *GitHubClient) get(path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET",
		c.opts.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.opts.httpClient.Do(req)
}

func (c *GitHubClient) FetchRepo(owner, repo string) (*RepoInfo, error) {
	resp, err := c.get(fmt.Sprintf("/repos/%s/%s", owner, repo))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		FullName   string `json:"full_name"`
		Archived   bool   `json:"archived"`
		OpenIssues int    `json:"open_issues_count"`
		StarCount  int    `json:"stargazers_count"`
		PushedAt   string `json:"pushed_at"`
		Owner      struct {
			Type string `json:"type"`
		} `json:"owner"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	info := &RepoInfo{
		Owner:          owner,
		Repo:           repo,
		IsArchived:     data.Archived,
		OpenIssueCount: data.OpenIssues,
		StarCount:      data.StarCount,
		HasOrgBacking:  data.Owner.Type == "Organization",
	}
	if t, err := time.Parse(time.RFC3339, data.PushedAt); err == nil {
		info.LastCommitAt = t
	}

	// Get contributor count via X-Total-Count header
	cResp, err := c.get(fmt.Sprintf("/repos/%s/%s/contributors?per_page=1&anon=true", owner, repo))
	if err == nil {
		defer cResp.Body.Close()
		if total := cResp.Header.Get("X-Total-Count"); total != "" {
			if n, err := strconv.Atoi(total); err == nil {
				info.ContributorCount = n
			}
		}
		// Fallback: count items in response
		if info.ContributorCount == 0 {
			var items []json.RawMessage
			if json.NewDecoder(cResp.Body).Decode(&items) == nil {
				info.ContributorCount = len(items)
			}
		}
	}

	return info, nil
}

func (c *GitHubClient) RepoFromURL(sourceURL string) (*RepoInfo, error) {
	// Parse github.com/owner/repo from URL
	sourceURL = strings.TrimSuffix(sourceURL, ".git")
	parts := strings.Split(strings.TrimPrefix(sourceURL, "https://github.com/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("not a GitHub URL: %s", sourceURL)
	}
	return c.FetchRepo(parts[0], parts[1])
}
