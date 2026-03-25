// internal/actions/types_test.go
package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActionTypeString(t *testing.T) {
	assert.Equal(t, "composite", ActionComposite.String())
	assert.Equal(t, "node", ActionNode.String())
	assert.Equal(t, "docker", ActionDocker.String())
}

func TestParseActionRef(t *testing.T) {
	tests := []struct {
		input string
		want  ActionRef
	}{
		{"actions/checkout@v4", ActionRef{Owner: "actions", Repo: "checkout", Ref: "v4", Path: ""}},
		{"actions/checkout@abc123def", ActionRef{Owner: "actions", Repo: "checkout", Ref: "abc123def", Path: ""}},
		{"org/repo/sub/path@v1", ActionRef{Owner: "org", Repo: "repo", Ref: "v1", Path: "sub/path"}},
		{"docker://alpine:3.19", ActionRef{DockerImage: "alpine:3.19"}},
		{"./local-action", ActionRef{LocalPath: "./local-action"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseActionRef(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestActionRefIsFirstParty(t *testing.T) {
	assert.True(t, ActionRef{Owner: "actions", Repo: "checkout"}.IsFirstParty())
	assert.True(t, ActionRef{Owner: "github", Repo: "codeql-action"}.IsFirstParty())
	assert.False(t, ActionRef{Owner: "some-org", Repo: "deploy"}.IsFirstParty())
}

func TestClassifyPinning(t *testing.T) {
	tests := []struct {
		ref  string
		want PinQuality
	}{
		{"abc123def456abc123def456abc123def456abcdef", PinSHA},
		{"v4.2.0", PinExactVersion},
		{"v4", PinMajorTag},
		{"main", PinBranch},
		{"", PinUnpinned},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifyPinning(tt.ref))
		})
	}
}
