package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/depscope/depscope/internal/core"
)

// ScanOrg scans all repositories in a GitHub organization.
// Lists repos via GitHub API, then calls ScanURL for each.
func ScanOrg(ctx context.Context, org string, opts Options) ([]*core.ScanResult, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN required for org scanning")
	}

	repos, err := listOrgRepos(ctx, org, token)
	if err != nil {
		return nil, fmt.Errorf("listing org repos: %w", err)
	}

	var results []*core.ScanResult
	for _, repo := range repos {
		url := fmt.Sprintf("https://github.com/%s/%s", org, repo)
		log.Printf("scanning %s...", url)
		result, err := ScanURL(ctx, url, opts)
		if err != nil {
			log.Printf("warning: %s: %v", url, err)
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

// listOrgRepos lists all repos in an org via GitHub API (paginated).
func listOrgRepos(ctx context.Context, org, token string) ([]string, error) {
	return listOrgReposFromBase(ctx, org, token, "https://api.github.com")
}

// listOrgReposFromBase is the testable version that accepts a custom base URL.
func listOrgReposFromBase(ctx context.Context, org, token, baseURL string) ([]string, error) {
	var allRepos []string
	page := 1
	for {
		url := fmt.Sprintf("%s/orgs/%s/repos?per_page=100&page=%d&type=all", baseURL, org, page)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Accept", "application/vnd.github+json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("GitHub API returned %d for org %s", resp.StatusCode, org)
		}

		var repos []struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			return nil, err
		}
		if len(repos) == 0 {
			break
		}
		for _, r := range repos {
			allRepos = append(allRepos, r.Name)
		}
		page++
	}
	return allRepos, nil
}
