// internal/actions/resolver.go
package actions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/depscope/depscope/internal/cache"
	"go.yaml.in/yaml/v3"
)

// Resolver resolves action references to their concrete SHA and action type.
// It implements Layers 2 (tag→SHA via GitHub API) and 3 (action.yml parsing).
type Resolver struct {
	githubToken string
	httpClient  *http.Client
	cache       *cache.DiskCache
	baseURL     string // overrideable for testing; default is https://api.github.com
}

// ResolverOption is a functional option for configuring a Resolver.
type ResolverOption func(*Resolver)

// WithCache sets a custom DiskCache on the Resolver (used in tests and prod).
func WithCache(c *cache.DiskCache) ResolverOption {
	return func(r *Resolver) { r.cache = c }
}

// WithBaseURL overrides the GitHub API base URL (used for httptest mocking).
func WithBaseURL(u string) ResolverOption {
	return func(r *Resolver) { r.baseURL = u }
}

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(c *http.Client) ResolverOption {
	return func(r *Resolver) { r.httpClient = c }
}

// NewResolver creates a new Resolver with the given GitHub token and options.
func NewResolver(githubToken string, opts ...ResolverOption) *Resolver {
	r := &Resolver{
		githubToken: githubToken,
		httpClient:  http.DefaultClient,
		baseURL:     "https://api.github.com",
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ResolvedAction is the result of resolving an ActionRef.
type ResolvedAction struct {
	Ref        ActionRef
	SHA        string     // resolved commit SHA (empty for local/docker refs)
	Type       ActionType // composite, node, docker, unknown
	Pinning    PinQuality
	ActionYAML *ActionYAML // parsed action.yml contents (nil if fetch failed or not applicable)
}

// ActionYAML represents the parsed action.yml or action.yaml file.
type ActionYAML struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Runs        ActionYAMLRuns `yaml:"runs"`
}

// ActionYAMLRuns is the runs: section of an action.yml file.
type ActionYAMLRuns struct {
	Using string `yaml:"using"` // "composite", "node20", "node16", "node12", "docker"
	Main  string `yaml:"main"`  // JS entry point (node actions)
	Image string `yaml:"image"` // Docker image or "Dockerfile" (docker actions)
	Steps []struct {
		Uses string `yaml:"uses"`
		Run  string `yaml:"run"`
	} `yaml:"steps"` // composite steps
}

// Resolve resolves an ActionRef to a ResolvedAction.
//
// For local (./path) refs: returns early with Type=ActionUnknown, no SHA.
// For docker:// refs: returns early with Type=ActionDocker, no SHA.
// For GitHub refs:
//  1. If ref is a 40-char SHA, skip tag resolution.
//  2. Otherwise resolve tag/branch→SHA via GitHub API (cached with TTLActionRef).
//  3. Fetch action.yml (or action.yaml) at the resolved SHA (cached with TTLImmutable).
//  4. Parse YAML to determine ActionType.
func (r *Resolver) Resolve(ctx context.Context, ref ActionRef) (*ResolvedAction, error) {
	result := &ResolvedAction{
		Ref:     ref,
		Pinning: ClassifyPinning(ref.Ref),
	}

	// Layer 0: skip resolution for local and docker:// refs
	if ref.IsLocal() {
		result.Type = ActionUnknown
		return result, nil
	}
	if ref.IsDocker() {
		result.Type = ActionDocker
		return result, nil
	}

	// Layer 2: resolve tag/branch to SHA (skip if already a SHA)
	sha := ref.Ref
	if result.Pinning != PinSHA {
		var err error
		sha, err = r.resolveRefToSHA(ctx, ref.Owner, ref.Repo, ref.Ref)
		if err != nil {
			return nil, fmt.Errorf("resolve ref %s/%s@%s: %w", ref.Owner, ref.Repo, ref.Ref, err)
		}
	}
	result.SHA = sha

	// Layer 3: fetch and parse action.yml at the resolved SHA
	ay, err := r.fetchActionYAML(ctx, ref.Owner, ref.Repo, sha)
	if err != nil {
		// Non-fatal: action.yml may be missing (reusable workflows, etc.)
		// Leave ActionYAML nil and type as Unknown
		result.Type = ActionUnknown
		return result, nil
	}
	result.ActionYAML = ay
	result.Type = classifyActionType(ay)
	return result, nil
}

// resolveRefToSHA calls the GitHub API to resolve a tag or branch name to a commit SHA.
// Results are cached with TTLActionRef (1h).
func (r *Resolver) resolveRefToSHA(ctx context.Context, owner, repo, ref string) (string, error) {
	cacheKey := cache.ActionRefKey(owner+"/"+repo, ref)

	if r.cache != nil {
		if data, ok, _ := r.cache.Get(cacheKey); ok {
			return string(data), nil
		}
	}

	// Try tags first, then heads (branches)
	sha, err := r.fetchGitRef(ctx, owner, repo, "tags/"+ref)
	if err != nil {
		// Fall back to branches
		sha, err = r.fetchGitRef(ctx, owner, repo, "heads/"+ref)
		if err != nil {
			return "", fmt.Errorf("git ref not found for %s/%s@%s: %w", owner, repo, ref, err)
		}
	}

	if r.cache != nil {
		_ = r.cache.Set(cacheKey, []byte(sha), cache.TTLActionRef)
	}
	return sha, nil
}

// gitRefResponse is the relevant subset of the GitHub git/refs API response.
type gitRefResponse struct {
	Object struct {
		SHA string `json:"sha"`
	} `json:"object"`
}

// fetchGitRef performs a GET /repos/{owner}/{repo}/git/ref/{refPath} and returns the SHA.
func (r *Resolver) fetchGitRef(ctx context.Context, owner, repo, refPath string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/git/ref/%s", r.baseURL, owner, repo, refPath)
	body, statusCode, err := r.get(ctx, url)
	if err != nil {
		return "", err
	}
	if statusCode == http.StatusNotFound {
		return "", fmt.Errorf("ref %s not found (404)", refPath)
	}
	if statusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d fetching ref %s", statusCode, refPath)
	}

	var resp gitRefResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse git ref response: %w", err)
	}
	if resp.Object.SHA == "" {
		return "", fmt.Errorf("empty SHA in git ref response for %s", refPath)
	}
	return resp.Object.SHA, nil
}

// contentsResponse is the relevant subset of the GitHub contents API response.
type contentsResponse struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// fetchActionYAML fetches and base64-decodes action.yml (falling back to action.yaml)
// from the given repo at the given SHA. Results are cached with TTLImmutable (10y).
func (r *Resolver) fetchActionYAML(ctx context.Context, owner, repo, sha string) (*ActionYAML, error) {
	for _, filename := range []string{"action.yml", "action.yaml"} {
		ay, err := r.fetchContentsFile(ctx, owner, repo, filename, sha)
		if err == nil {
			return ay, nil
		}
		// If 404, try the other filename; otherwise propagate
		if !strings.Contains(err.Error(), "404") {
			return nil, err
		}
	}
	return nil, fmt.Errorf("neither action.yml nor action.yaml found for %s/%s@%s", owner, repo, sha)
}

// fetchContentsFile fetches a single file from the GitHub contents API and parses it
// as an ActionYAML. The result is cached with TTLImmutable since content at a SHA
// never changes.
func (r *Resolver) fetchContentsFile(ctx context.Context, owner, repo, filename, sha string) (*ActionYAML, error) {
	cacheKey := cache.RepoSHAKey(owner+"/"+repo, sha+":"+filename)

	if r.cache != nil {
		if data, ok, _ := r.cache.Get(cacheKey); ok {
			var ay ActionYAML
			if err := yaml.Unmarshal(data, &ay); err == nil {
				return &ay, nil
			}
		}
	}

	url := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", r.baseURL, owner, repo, filename, sha)
	body, statusCode, err := r.get(ctx, url)
	if err != nil {
		return nil, err
	}
	if statusCode == http.StatusNotFound {
		return nil, fmt.Errorf("404: %s not found", filename)
	}
	if statusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching %s", statusCode, filename)
	}

	var cr contentsResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return nil, fmt.Errorf("parse contents response: %w", err)
	}

	var rawYAML []byte
	if cr.Encoding == "base64" {
		// GitHub wraps lines with newlines; strip them before decoding
		clean := strings.ReplaceAll(cr.Content, "\n", "")
		rawYAML, err = base64.StdEncoding.DecodeString(clean)
		if err != nil {
			return nil, fmt.Errorf("base64 decode %s: %w", filename, err)
		}
	} else {
		rawYAML = []byte(cr.Content)
	}

	var ay ActionYAML
	if err := yaml.Unmarshal(rawYAML, &ay); err != nil {
		return nil, fmt.Errorf("parse %s YAML: %w", filename, err)
	}

	if r.cache != nil {
		_ = r.cache.Set(cacheKey, rawYAML, cache.TTLImmutable)
	}
	return &ay, nil
}

// get performs an authenticated GET request and returns the body, status code, and error.
func (r *Resolver) get(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}
	if r.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.githubToken)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("HTTP GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response body: %w", err)
	}
	return body, resp.StatusCode, nil
}

// classifyActionType derives the ActionType from a parsed ActionYAML.
func classifyActionType(ay *ActionYAML) ActionType {
	if ay == nil {
		return ActionUnknown
	}
	using := strings.ToLower(ay.Runs.Using)
	switch {
	case using == "composite":
		return ActionComposite
	case strings.HasPrefix(using, "node"):
		return ActionNode
	case using == "docker":
		return ActionDocker
	default:
		return ActionUnknown
	}
}
