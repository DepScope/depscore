package resolve

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

type GitLabResolver struct {
	token   string
	baseURL string
	client  *http.Client
}

func NewGitLabResolver(token string, opts ...Option) *GitLabResolver {
	o := &resolverOptions{baseURL: "https://gitlab.com"}
	for _, opt := range opts {
		opt(o)
	}
	return &GitLabResolver{token: token, baseURL: o.baseURL, client: &http.Client{}}
}

func (r *GitLabResolver) Type() string { return "gitlab" }

func (r *GitLabResolver) Resolve(ctx context.Context, rawURL string) ([]ManifestFile, func(), error) {
	project, ref := ParseGitLabURL(rawURL)
	if project == "" {
		return nil, func() {}, fmt.Errorf("invalid GitLab URL: %s", rawURL)
	}

	encodedProject := url.PathEscape(project)

	if ref == "" {
		defaultRef, err := r.fetchDefaultBranch(ctx, encodedProject)
		if err != nil {
			return nil, func() {}, fmt.Errorf("fetch default branch: %w", err)
		}
		ref = defaultRef
	}

	var allPaths []string
	for page := 1; page <= 10; page++ {
		paths, hasMore, err := r.fetchTreePage(ctx, encodedProject, ref, page)
		if err != nil {
			return nil, func() {}, fmt.Errorf("fetch tree page %d: %w", page, err)
		}
		allPaths = append(allPaths, paths...)
		if !hasMore {
			break
		}
		if page == 10 {
			log.Printf("warning: GitLab tree for %s capped at 1000 entries; some manifests may be missed", project)
		}
	}

	var manifestPaths []string
	for _, p := range allPaths {
		if MatchesManifest(p) {
			manifestPaths = append(manifestPaths, p)
		}
	}

	var files []ManifestFile
	for _, path := range manifestPaths {
		content, err := r.fetchFileRaw(ctx, encodedProject, ref, path)
		if err != nil {
			log.Printf("warning: could not fetch %s: %v", path, err)
			continue
		}
		files = append(files, ManifestFile{Path: path, Content: content})
	}

	return files, func() {}, nil
}

func (r *GitLabResolver) fetchDefaultBranch(ctx context.Context, encodedProject string) (string, error) {
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s", r.baseURL, encodedProject)
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	r.setAuth(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitLab API %s: %d", apiURL, resp.StatusCode)
	}

	var result struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.DefaultBranch, nil
}

func (r *GitLabResolver) fetchTreePage(ctx context.Context, encodedProject, ref string, page int) ([]string, bool, error) {
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/tree?recursive=true&ref=%s&per_page=100&page=%d",
		r.baseURL, encodedProject, url.QueryEscape(ref), page)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, false, err
	}
	r.setAuth(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("GitLab API tree: %d", resp.StatusCode)
	}

	var entries []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, false, err
	}

	var paths []string
	for _, e := range entries {
		if e.Type == "blob" {
			paths = append(paths, e.Path)
		}
	}

	hasMore := len(entries) == 100
	return paths, hasMore, nil
}

func (r *GitLabResolver) fetchFileRaw(ctx context.Context, encodedProject, ref, filePath string) ([]byte, error) {
	encodedPath := url.PathEscape(filePath)
	apiURL := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/%s/raw?ref=%s",
		r.baseURL, encodedProject, encodedPath, url.QueryEscape(ref))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	r.setAuth(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitLab API file %s: %d", filePath, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (r *GitLabResolver) setAuth(req *http.Request) {
	if r.token != "" {
		req.Header.Set("PRIVATE-TOKEN", r.token)
	}
}
