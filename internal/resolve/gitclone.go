package resolve

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type GitCloneResolver struct {
	maxFiles int
}

func NewGitCloneResolver(opts ...Option) *GitCloneResolver {
	o := &resolverOptions{}
	for _, opt := range opts {
		opt(o)
	}
	mf := o.maxFiles
	if mf <= 0 {
		mf = DefaultMaxFiles
	}
	return &GitCloneResolver{maxFiles: mf}
}

func (r *GitCloneResolver) Type() string { return "gitclone" }

func (r *GitCloneResolver) Resolve(ctx context.Context, url string) ([]ManifestFile, func(), error) {
	tmpDir, err := os.MkdirTemp("", "depscope-clone-*")
	if err != nil {
		return nil, func() {}, fmt.Errorf("create temp dir: %w", err)
	}

	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	if _, err := exec.LookPath("git"); err != nil {
		return nil, cleanup, fmt.Errorf("git is required for scanning non-GitHub/GitLab URLs: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", url, tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, cleanup, fmt.Errorf("git clone failed: %w\n%s", err, string(output))
	}

	var files []ManifestFile
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(tmpDir, path)
		if err != nil {
			return nil
		}

		if !MatchesManifest(rel) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		files = append(files, ManifestFile{Path: rel, Content: content})
		return nil
	})
	if err != nil {
		return nil, cleanup, fmt.Errorf("walk clone dir: %w", err)
	}

	return files, cleanup, nil
}
