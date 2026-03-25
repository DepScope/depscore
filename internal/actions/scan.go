// internal/actions/scan.go
package actions

import (
	"context"
	"fmt"
	"log"

	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
)

const (
	// DefaultMaxDepth is the default recursion depth limit for resolving
	// composite actions and reusable workflows.
	DefaultMaxDepth = 10
)

// ScanWorkflows scans workflow files in dir and populates the graph with action,
// workflow, docker_image, and script_download nodes. It ties all 5 resolution
// layers together:
//  1. Parse all workflow YAML files in .github/workflows/
//  2. Resolve each action ref (tag -> SHA, fetch action.yml)
//  3. Analyze composite actions for transitive deps
//  4. Fetch bundled code (npm/Docker) from action repos
//  5. Handle reusable workflows (recursive parsing)
func ScanWorkflows(ctx context.Context, dir string, resolver *Resolver, g *graph.Graph) error {
	workflows, err := ParseWorkflowDir(dir)
	if err != nil {
		return fmt.Errorf("parse workflows: %w", err)
	}

	visited := make(map[string]bool)

	for _, wf := range workflows {
		// Create workflow node
		wfNodeID := graph.NodeID(graph.NodeWorkflow, wf.Path)
		g.AddNode(&graph.Node{
			ID:       wfNodeID,
			Type:     graph.NodeWorkflow,
			Name:     wf.Path,
			Pinning:  graph.PinningNA,
			Metadata: map[string]any{},
		})

		// Determine if this workflow has broad permissions
		permBroad := !wf.Permissions.Defined
		if wf.Permissions.Defined {
			for _, v := range wf.Permissions.Scopes {
				if v == "write" {
					permBroad = true
					break
				}
			}
		}

		// Process each action reference
		for _, ref := range wf.Actions {
			processActionRef(ctx, resolver, g, wfNodeID, ref, permBroad, visited, 0)
		}

		// Detect script downloads in run blocks
		for _, rb := range wf.RunBlocks {
			downloads := DetectScriptDownloads(rb.Content)
			for _, dl := range downloads {
				dlNodeID := graph.NodeID(graph.NodeScriptDownload, dl.URL)
				if g.Node(dlNodeID) == nil {
					score := ScoreAction(ScoringInput{IsScriptDownload: true})
					g.AddNode(&graph.Node{
						ID:      dlNodeID,
						Type:    graph.NodeScriptDownload,
						Name:    dl.URL,
						Score:   score,
						Risk:    core.RiskCritical,
						Pinning: graph.PinningUnpinned,
						Metadata: map[string]any{
							"pattern": dl.Pattern,
							"line":    dl.Line,
						},
					})
				}
				g.AddEdge(&graph.Edge{
					From: wfNodeID,
					To:   dlNodeID,
					Type: graph.EdgeDownloads,
				})
			}
		}
	}

	return nil
}

// processActionRef resolves a single action ref and adds it to the graph.
// It recurses into composite actions and reusable workflows with depth limiting.
func processActionRef(ctx context.Context, resolver *Resolver, g *graph.Graph,
	parentNodeID string, ref ActionRef, permBroad bool,
	visited map[string]bool, depth int) {

	if depth >= DefaultMaxDepth {
		return
	}

	// Handle docker:// references as docker_image nodes
	if ref.IsDocker() {
		addDockerImageNode(g, parentNodeID, ref.DockerImage)
		return
	}

	// Skip local actions (can't resolve remotely)
	if ref.IsLocal() {
		return
	}

	// Construct a unique node ID for this action
	actionNodeID := graph.NodeID(graph.NodeAction, ref.FullName()+"@"+ref.Ref)

	// Cycle detection
	if visited[actionNodeID] {
		// Still add the edge even if we've already processed this node
		addEdgeIfNew(g, parentNodeID, actionNodeID, edgeTypeForParent(g, parentNodeID))
		return
	}
	visited[actionNodeID] = true

	// Resolve the action reference (Layer 2 + 3)
	resolved, err := resolver.Resolve(ctx, ref)
	if err != nil {
		log.Printf("warning: resolve %s: %v", ref.FullName(), err)
		// Create a node anyway with unknown type and low score
		g.AddNode(&graph.Node{
			ID:      actionNodeID,
			Type:    graph.NodeAction,
			Name:    ref.FullName(),
			Ref:     ref.Ref,
			Score:   0,
			Risk:    core.RiskCritical,
			Pinning: pinQualityToGraphPinning(ClassifyPinning(ref.Ref)),
			Metadata: map[string]any{
				"error": err.Error(),
			},
		})
		addEdgeIfNew(g, parentNodeID, actionNodeID, edgeTypeForParent(g, parentNodeID))
		return
	}

	// Score the action
	scoring := ScoringInput{
		Pinning:          resolved.Pinning,
		FirstParty:       ref.IsFirstParty(),
		BundledMinScore:  100,
		PermissionsBroad: permBroad,
	}
	score := ScoreAction(scoring)

	// Create action node
	pinning := pinQualityToGraphPinning(resolved.Pinning)
	version := resolved.SHA
	if version == "" {
		version = ref.Ref
	}

	g.AddNode(&graph.Node{
		ID:      actionNodeID,
		Type:    graph.NodeAction,
		Name:    ref.FullName(),
		Version: version,
		Ref:     ref.Ref,
		Score:   score,
		Risk:    core.RiskLevelFromScore(score),
		Pinning: pinning,
		Metadata: map[string]any{
			"action_type": resolved.Type.String(),
			"first_party": ref.IsFirstParty(),
			"sha":         resolved.SHA,
		},
	})

	// Add edge from parent to action
	addEdgeIfNew(g, parentNodeID, actionNodeID, edgeTypeForParent(g, parentNodeID))

	// Layer 3: handle composite actions (transitive deps)
	if resolved.Type == ActionComposite && resolved.ActionYAML != nil {
		compositeRefs := ExtractCompositeActions(resolved.ActionYAML)
		for _, cRef := range compositeRefs {
			processActionRef(ctx, resolver, g, actionNodeID, cRef, permBroad, visited, depth+1)
		}
	}

	// Layer 4: fetch bundled dependencies
	bundled, err := FetchBundledDeps(ctx, resolver, ref, resolved)
	if err != nil {
		log.Printf("warning: bundled deps for %s: %v", ref.FullName(), err)
	}
	if bundled != nil {
		// Add npm package nodes
		for _, pkg := range bundled.NPMPackages {
			pkgNodeID := graph.NodeID(graph.NodePackage, pkg.Key())
			if g.Node(pkgNodeID) == nil {
				g.AddNode(&graph.Node{
					ID:      pkgNodeID,
					Type:    graph.NodePackage,
					Name:    pkg.Name,
					Version: pkg.ResolvedVersion,
					Pinning: graph.PinningNA,
					Metadata: map[string]any{
						"ecosystem": string(pkg.Ecosystem),
						"bundled":   true,
					},
				})
			}
			g.AddEdge(&graph.Edge{
				From:  actionNodeID,
				To:    pkgNodeID,
				Type:  graph.EdgeBundles,
				Depth: depth + 1,
			})
		}

		// Add Docker image nodes from Dockerfile analysis
		if bundled.Dockerfile != nil {
			for _, bi := range bundled.Dockerfile.BaseImages {
				imageRef := bi.Image
				if bi.Tag != "" {
					imageRef += ":" + bi.Tag
				} else if bi.Digest != "" {
					imageRef += "@" + bi.Digest
				}
				addDockerImageNode(g, actionNodeID, imageRef)
			}
		}

		// Add script download nodes from composite run: steps
		for _, dl := range bundled.ScriptDownloads {
			dlNodeID := graph.NodeID(graph.NodeScriptDownload, dl.URL)
			if g.Node(dlNodeID) == nil {
				g.AddNode(&graph.Node{
					ID:      dlNodeID,
					Type:    graph.NodeScriptDownload,
					Name:    dl.URL,
					Score:   0,
					Risk:    core.RiskCritical,
					Pinning: graph.PinningUnpinned,
					Metadata: map[string]any{
						"pattern": dl.Pattern,
						"line":    dl.Line,
					},
				})
			}
			g.AddEdge(&graph.Edge{
				From: actionNodeID,
				To:   dlNodeID,
				Type: graph.EdgeDownloads,
			})
		}
	}

	// Layer 5: handle reusable workflows
	if ref.IsReusableWorkflow() && resolved.SHA != "" {
		reusableWfNodeID := graph.NodeID(graph.NodeWorkflow, ref.FullName()+"@"+ref.Ref)
		if !visited[reusableWfNodeID] {
			visited[reusableWfNodeID] = true

			// Fetch the reusable workflow file content
			wfPath := ref.Path
			content, err := resolver.FetchFileContent(ctx, ref.Owner, ref.Repo, wfPath, resolved.SHA)
			if err != nil {
				log.Printf("warning: fetch reusable workflow %s: %v", ref.FullName(), err)
			} else {
				wf, err := ParseWorkflow(content, ref.FullName())
				if err != nil {
					log.Printf("warning: parse reusable workflow %s: %v", ref.FullName(), err)
				} else {
					g.AddNode(&graph.Node{
						ID:      reusableWfNodeID,
						Type:    graph.NodeWorkflow,
						Name:    ref.FullName(),
						Version: resolved.SHA,
						Ref:     ref.Ref,
						Pinning: pinning,
						Metadata: map[string]any{
							"reusable": true,
						},
					})

					g.AddEdge(&graph.Edge{
						From: parentNodeID,
						To:   reusableWfNodeID,
						Type: graph.EdgeTriggers,
					})

					// Process actions within the reusable workflow
					for _, subRef := range wf.Actions {
						processActionRef(ctx, resolver, g, reusableWfNodeID, subRef, permBroad, visited, depth+1)
					}

					// Detect script downloads in reusable workflow run blocks
					for _, rb := range wf.RunBlocks {
						downloads := DetectScriptDownloads(rb.Content)
						for _, dl := range downloads {
							dlNodeID := graph.NodeID(graph.NodeScriptDownload, dl.URL)
							if g.Node(dlNodeID) == nil {
								g.AddNode(&graph.Node{
									ID:      dlNodeID,
									Type:    graph.NodeScriptDownload,
									Name:    dl.URL,
									Score:   0,
									Risk:    core.RiskCritical,
									Pinning: graph.PinningUnpinned,
									Metadata: map[string]any{
										"pattern": dl.Pattern,
										"line":    dl.Line,
									},
								})
							}
							g.AddEdge(&graph.Edge{
								From: reusableWfNodeID,
								To:   dlNodeID,
								Type: graph.EdgeDownloads,
							})
						}
					}
				}
			}
		}
	}
}

// addDockerImageNode creates a docker_image node and connects it via pulls_image edge.
func addDockerImageNode(g *graph.Graph, parentNodeID, imageRef string) {
	nodeID := graph.NodeID(graph.NodeDockerImage, imageRef)

	if g.Node(nodeID) == nil {
		bi := parseBaseImage(imageRef)

		// Score the docker image
		dockerScoring := DockerScoringInput{
			PinningDigest: bi.Digest != "",
			ExactTag:      bi.Tag != "" && bi.Tag != "latest",
		}
		score := ScoreDockerImage(dockerScoring)

		g.AddNode(&graph.Node{
			ID:      nodeID,
			Type:    graph.NodeDockerImage,
			Name:    bi.Image,
			Version: bi.Tag,
			Score:   score,
			Risk:    core.RiskLevelFromScore(score),
			Pinning: dockerPinningQuality(bi),
			Metadata: map[string]any{
				"tag":    bi.Tag,
				"digest": bi.Digest,
			},
		})
	}

	g.AddEdge(&graph.Edge{
		From: parentNodeID,
		To:   nodeID,
		Type: graph.EdgePullsImage,
	})
}

// addEdgeIfNew adds an edge to the graph. We allow duplicate edges since the
// graph's adjacency list handles this; the edge list records all references.
func addEdgeIfNew(g *graph.Graph, from, to string, edgeType graph.EdgeType) {
	g.AddEdge(&graph.Edge{
		From: from,
		To:   to,
		Type: edgeType,
	})
}

// edgeTypeForParent returns the appropriate edge type based on parent node type.
func edgeTypeForParent(g *graph.Graph, parentNodeID string) graph.EdgeType {
	parent := g.Node(parentNodeID)
	if parent != nil && parent.Type == graph.NodeWorkflow {
		return graph.EdgeUsesAction
	}
	// Action -> action (composite transitive dep) is still uses_action
	return graph.EdgeUsesAction
}

// pinQualityToGraphPinning converts an actions.PinQuality to a graph.PinningQuality.
func pinQualityToGraphPinning(p PinQuality) graph.PinningQuality {
	switch p {
	case PinSHA:
		return graph.PinningSHA
	case PinExactVersion:
		return graph.PinningExactVersion
	case PinMajorTag:
		return graph.PinningMajorTag
	case PinBranch:
		return graph.PinningBranch
	case PinUnpinned:
		return graph.PinningUnpinned
	default:
		return graph.PinningNA
	}
}

// dockerPinningQuality determines the pinning quality for a Docker image.
func dockerPinningQuality(bi BaseImage) graph.PinningQuality {
	if bi.Digest != "" {
		return graph.PinningDigest
	}
	if bi.Tag != "" && bi.Tag != "latest" {
		return graph.PinningExactVersion
	}
	if bi.Tag == "latest" || bi.Tag == "" {
		return graph.PinningUnpinned
	}
	return graph.PinningNA
}
