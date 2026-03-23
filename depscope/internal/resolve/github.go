package resolve

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

const DefaultMaxFiles = 5000

type GitHubResolver struct {
	token    string
	baseURL  string
	maxFiles int
	client   *http.Client
}

func NewGitHubResolver(token string, opts ...Option) *GitHubResolver {
	o := &resolverOptions{baseURL: "https://api.github.com"}
	for _, opt := range opts {
		opt(o)
	}
	mf := o.maxFiles
	if mf <= 0 {
		mf = DefaultMaxFiles
	}
	return &GitHubResolver{token: token, baseURL: o.baseURL, maxFiles: mf, client: &http.Client{}}
}

func (r *GitHubResolver) Type() string { return "github" }

func (r *GitHubResolver) Resolve(ctx context.Context, rawURL string) ([]ManifestFile, func(), error) {
	owner, repo, ref := ParseGitHubURL(rawURL)
	if owner == "" || repo == "" {
		return nil, func() {}, fmt.Errorf("invalid GitHub URL: %s", rawURL)
	}

	if ref == "" {
		defaultRef, err := r.fetchDefaultBranch(ctx, owner, repo)
		if err != nil {
			return nil, func() {}, fmt.Errorf("fetch default branch: %w", err)
		}
		ref = defaultRef
	}

	treePaths, err := r.fetchTree(ctx, owner, repo, ref)
	if err != nil {
		// If ref was explicitly provided and 404s, fall back to default branch.
		if strings.Contains(err.Error(), "404") && ref != "" {
			log.Printf("warning: ref %q not found for %s/%s, falling back to default branch", ref, owner, repo)
			defaultRef, defaultErr := r.fetchDefaultBranch(ctx, owner, repo)
			if defaultErr != nil {
				return nil, func() {}, fmt.Errorf("fetch tree: %w (fallback also failed: %v)", err, defaultErr)
			}
			ref = defaultRef
			treePaths, err = r.fetchTree(ctx, owner, repo, ref)
			if err != nil {
				return nil, func() {}, fmt.Errorf("fetch tree: %w", err)
			}
		} else {
			return nil, func() {}, fmt.Errorf("fetch tree: %w", err)
		}
	}

	var manifestPaths []string
	for _, p := range treePaths {
		if MatchesManifest(p) {
			manifestPaths = append(manifestPaths, p)
		}
	}

	// Cap the number of files to fetch
	if len(manifestPaths) > r.maxFiles {
		log.Printf("warning: found %d manifest files, capping at %d", len(manifestPaths), r.maxFiles)
		manifestPaths = manifestPaths[:r.maxFiles]
	}

	var files []ManifestFile
	for _, path := range manifestPaths {
		content, err := r.fetchFileContent(ctx, owner, repo, ref, path)
		if err != nil {
			log.Printf("warning: could not fetch %s: %v", path, err)
			continue
		}
		files = append(files, ManifestFile{Path: path, Content: content})
	}

	return files, func() {}, nil
}

func (r *GitHubResolver) fetchDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", r.baseURL, owner, repo)
	var result struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := r.getJSON(ctx, url, &result); err != nil {
		return "", err
	}
	return result.DefaultBranch, nil
}

func (r *GitHubResolver) fetchTree(ctx context.Context, owner, repo, ref string) ([]string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", r.baseURL, owner, repo, url.PathEscape(ref))
	var result struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := r.getJSON(ctx, url, &result); err != nil {
		return nil, err
	}
	if result.Truncated {
		log.Printf("warning: GitHub tree for %s/%s is truncated (>100k entries); some manifests may be missed", owner, repo)
	}
	var paths []string
	for _, entry := range result.Tree {
		if entry.Type == "blob" {
			paths = append(paths, entry.Path)
		}
	}
	return paths, nil
}

func (r *GitHubResolver) fetchFileContent(ctx context.Context, owner, repo, ref, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", r.baseURL, owner, repo, path, url.QueryEscape(ref))
	var result struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := r.getJSON(ctx, url, &result); err != nil {
		return nil, err
	}
	if result.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected encoding %q for %s", result.Encoding, path)
	}
	// GitHub API returns base64 with embedded newlines; strip them before decoding.
	clean := strings.ReplaceAll(result.Content, "\n", "")
	return base64.StdEncoding.DecodeString(clean)
}

func (r *GitHubResolver) getJSON(ctx context.Context, url string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB cap for error bodies
		return fmt.Errorf("GitHub API %s: %d %s", url, resp.StatusCode, string(body))
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 100<<20)).Decode(target) // 100 MB cap for tree responses
}
