// internal/actions/bundled.go
package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/depscope/depscope/internal/manifest"
)

// BundledDeps represents the bundled dependencies found in an action's repo.
type BundledDeps struct {
	NPMPackages     []manifest.Package // from package.json / package-lock.json
	Dockerfile      *DockerfileResult  // from Dockerfile (if Docker action)
	ScriptDownloads []ScriptDownload   // from composite run: steps
}

// FetchBundledDeps fetches and parses bundled code from an action's repo.
//
// For JS (node) actions: fetches package.json (and package-lock.json if available)
// at the resolved SHA. Parses them with manifest.NewJavaScriptParser().ParseFiles().
//
// For Docker actions: if runs.image starts with "Dockerfile", fetches the Dockerfile
// and parses it with ParseDockerfile(). If runs.image is a direct image ref (e.g.
// "docker://alpine:3.19"), the image reference is recorded in Dockerfile.BaseImages.
//
// For composite actions: scans all run: steps with DetectScriptDownloads().
//
// Returns a non-nil *BundledDeps on success. Returns an error only for unexpected
// failures; missing optional files (e.g. package-lock.json) are silently ignored.
func FetchBundledDeps(ctx context.Context, resolver *Resolver, ref ActionRef, resolved *ResolvedAction) (*BundledDeps, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved action is nil")
	}

	deps := &BundledDeps{}

	switch resolved.Type {
	case ActionNode:
		pkgs, err := fetchNPMDeps(ctx, resolver, ref, resolved.SHA)
		if err != nil {
			// Non-fatal: many actions don't have a package.json accessible
			// via the contents API (they bundle dist/). Return empty deps.
			return deps, nil
		}
		deps.NPMPackages = pkgs

	case ActionDocker:
		dfResult, err := fetchDockerDeps(ctx, resolver, ref, resolved)
		if err != nil {
			// Non-fatal: fall through with empty deps
			return deps, nil
		}
		deps.Dockerfile = dfResult

	case ActionComposite:
		if resolved.ActionYAML != nil {
			for _, step := range resolved.ActionYAML.Runs.Steps {
				if step.Run != "" {
					downloads := DetectScriptDownloads(step.Run)
					deps.ScriptDownloads = append(deps.ScriptDownloads, downloads...)
				}
			}
		}
	}

	return deps, nil
}

// fetchNPMDeps fetches package.json (required) and package-lock.json (optional)
// from the action repo at the resolved SHA, then parses them.
func fetchNPMDeps(ctx context.Context, resolver *Resolver, ref ActionRef, sha string) ([]manifest.Package, error) {
	pkgJSON, err := resolver.FetchFileContent(ctx, ref.Owner, ref.Repo, "package.json", sha)
	if err != nil {
		return nil, fmt.Errorf("fetch package.json: %w", err)
	}

	files := map[string][]byte{
		"package.json": pkgJSON,
	}

	// package-lock.json is optional; ignore errors
	lockJSON, err := resolver.FetchFileContent(ctx, ref.Owner, ref.Repo, "package-lock.json", sha)
	if err == nil {
		files["package-lock.json"] = lockJSON
	}

	pkgs, err := manifest.NewJavaScriptParser().ParseFiles(files)
	if err != nil {
		return nil, fmt.Errorf("parse npm manifests: %w", err)
	}
	return pkgs, nil
}

// fetchDockerDeps handles bundled deps for Docker actions.
// If runs.image starts with "Dockerfile", fetches and parses the Dockerfile.
// Otherwise (e.g. "docker://alpine:3.19" or "alpine:3.19"), records the image reference.
func fetchDockerDeps(ctx context.Context, resolver *Resolver, ref ActionRef, resolved *ResolvedAction) (*DockerfileResult, error) {
	if resolved.ActionYAML == nil {
		// docker:// inline ref — use the DockerImage from the original ref
		if ref.DockerImage != "" {
			bi := parseBaseImage(ref.DockerImage)
			return &DockerfileResult{BaseImages: []BaseImage{bi}}, nil
		}
		return &DockerfileResult{}, nil
	}

	image := resolved.ActionYAML.Runs.Image

	// If image points to a Dockerfile in the repo, fetch and parse it.
	if strings.HasPrefix(image, "Dockerfile") {
		dfContent, err := resolver.FetchFileContent(ctx, ref.Owner, ref.Repo, image, resolved.SHA)
		if err != nil {
			return nil, fmt.Errorf("fetch Dockerfile: %w", err)
		}
		return ParseDockerfile(dfContent)
	}

	// Direct image reference (e.g. "alpine:3.19" or "docker://alpine:3.19")
	imageRef := strings.TrimPrefix(image, "docker://")
	if imageRef == "" {
		return &DockerfileResult{}, nil
	}
	bi := parseBaseImage(imageRef)
	return &DockerfileResult{BaseImages: []BaseImage{bi}}, nil
}
