package resolve_test

import (
	"testing"

	"github.com/depscope/depscope/internal/resolve"
	"github.com/stretchr/testify/assert"
)

func TestIsRemoteURL(t *testing.T) {
	tests := []struct {
		input  string
		remote bool
	}{
		{"https://github.com/psf/requests", true},
		{"http://github.com/psf/requests", true},
		{"ssh://git@github.com/psf/requests.git", true},
		{"git@github.com:psf/requests.git", true},
		{".", false},
		{"/home/user/project", false},
		{"./relative/path", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.remote, resolve.IsRemoteURL(tt.input), tt.input)
	}
}

func TestDetectResolver(t *testing.T) {
	tests := []struct {
		url          string
		expectedType string
	}{
		{"https://github.com/psf/requests", "github"},
		{"https://github.com/psf/requests/tree/v2.31.0", "github"},
		{"https://gitlab.com/org/project", "gitlab"},
		{"https://gitlab.com/group/subgroup/project/-/tree/main", "gitlab"},
		{"https://bitbucket.org/org/repo", "gitclone"},
		{"git@custom.host:org/repo.git", "gitclone"},
		{"ssh://git@example.com/org/repo.git", "gitclone"},
	}
	for _, tt := range tests {
		r := resolve.DetectResolver(tt.url, resolve.DetectOptions{})
		assert.Equal(t, tt.expectedType, r.Type(), tt.url)
	}
}

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		url   string
		owner string
		repo  string
		ref   string
	}{
		{"https://github.com/psf/requests", "psf", "requests", ""},
		{"https://github.com/psf/requests/tree/v2.31.0", "psf", "requests", "v2.31.0"},
		{"https://github.com/psf/requests/tree/feature/branch", "psf", "requests", "feature/branch"},
		{"https://github.com/psf/requests.git", "psf", "requests", ""},
	}
	for _, tt := range tests {
		owner, repo, ref := resolve.ParseGitHubURL(tt.url)
		assert.Equal(t, tt.owner, owner, tt.url)
		assert.Equal(t, tt.repo, repo, tt.url)
		assert.Equal(t, tt.ref, ref, tt.url)
	}
}

func TestParseGitLabURL(t *testing.T) {
	tests := []struct {
		url     string
		project string
		ref     string
	}{
		{"https://gitlab.com/org/project", "org/project", ""},
		{"https://gitlab.com/group/subgroup/project", "group/subgroup/project", ""},
		{"https://gitlab.com/org/project/-/tree/main", "org/project", "main"},
		{"https://gitlab.com/group/sub/project/-/tree/v1.0", "group/sub/project", "v1.0"},
	}
	for _, tt := range tests {
		project, ref := resolve.ParseGitLabURL(tt.url)
		assert.Equal(t, tt.project, project, tt.url)
		assert.Equal(t, tt.ref, ref, tt.url)
	}
}
