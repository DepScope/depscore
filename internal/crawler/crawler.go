package crawler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/config"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
)

const (
	defaultMaxDepth = 25
	defaultTimeout  = 10 * time.Minute
)

// CrawlerOptions configures the BFS crawl behaviour.
type CrawlerOptions struct {
	MaxDepth    int
	OwnOrgs     []string
	Concurrency config.ConcurrencyConfig
	Timeout     time.Duration
}

// Crawler performs a BFS crawl of the dependency graph.
type Crawler struct {
	cache     *cache.CacheDB
	resolvers map[DepSourceType]Resolver
	graph     *graph.Graph
	seen      map[string]string // version_key -> nodeID for dedup in THIS scan
	opts      CrawlerOptions
	mu        sync.Mutex // protects seen + graph
	errors    []CrawlError
}

// NewCrawler creates a new Crawler. If MaxDepth or Timeout are zero, defaults
// are applied (25 and 10 minutes respectively).
func NewCrawler(cacheDB *cache.CacheDB, resolvers map[DepSourceType]Resolver, opts CrawlerOptions) *Crawler {
	if opts.MaxDepth == 0 {
		opts.MaxDepth = defaultMaxDepth
	}
	if opts.Timeout == 0 {
		opts.Timeout = defaultTimeout
	}
	return &Crawler{
		cache:     cacheDB,
		resolvers: resolvers,
		graph:     graph.New(),
		seen:      make(map[string]string),
		opts:      opts,
	}
}

// resolveResult bundles the outcome of a single Resolve call.
type resolveResult struct {
	ref      DepRef
	resolved *ResolvedDep
	err      error
	source   DepSourceType
}

// Crawl performs a BFS crawl starting from root and returns the resulting graph,
// statistics, and any errors encountered.
func (c *Crawler) Crawl(ctx context.Context, root FileTree) (*CrawlResult, error) {
	// Apply timeout.
	ctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()

	queue := []queueItem{{Contents: root, Depth: 0, ParentVK: ""}}
	maxDepthHit := 0
	cacheHits := 0
	cacheMisses := 0

	for len(queue) > 0 {
		// Check context before processing each level.
		if err := ctx.Err(); err != nil {
			return c.buildResult(maxDepthHit, cacheHits, cacheMisses), nil
		}

		// Collect all items at the current depth (BFS level processing).
		currentDepth := queue[0].Depth
		var levelItems []queueItem
		var remaining []queueItem
		for _, item := range queue {
			if item.Depth == currentDepth {
				levelItems = append(levelItems, item)
			} else {
				remaining = append(remaining, item)
			}
		}
		queue = remaining

		if currentDepth > maxDepthHit {
			maxDepthHit = currentDepth
		}

		for _, item := range levelItems {
			if err := ctx.Err(); err != nil {
				return c.buildResult(maxDepthHit, cacheHits, cacheMisses), nil
			}

			if item.CachedDeps != nil {
				// Process cached children.
				children := c.processCachedDeps(item)
				queue = append(queue, children...)
				continue
			}

			if item.Contents == nil {
				continue
			}

			// Phase 1: Detect (sequential, CPU-only).
			var allRefs []depRefWithSource
			for srcType, resolver := range c.resolvers {
				if err := ctx.Err(); err != nil {
					return c.buildResult(maxDepthHit, cacheHits, cacheMisses), nil
				}
				refs, err := resolver.Detect(ctx, item.Contents)
				if err != nil {
					// Record detect error but continue with other resolvers.
					c.mu.Lock()
					c.errors = append(c.errors, CrawlError{
						Depth:    item.Depth,
						Err:      fmt.Errorf("detect: %w", err),
						Resolver: srcType,
					})
					c.mu.Unlock()
					continue
				}
				for _, ref := range refs {
					allRefs = append(allRefs, depRefWithSource{ref: ref, source: srcType})
				}
			}

			if len(allRefs) == 0 {
				continue
			}

			// Phase 2: Resolve (concurrent within the level).
			results := c.resolveRefs(ctx, allRefs, item.Depth)

			// Phase 3: Process resolve results, update graph, enqueue children.
			for _, res := range results {
				if err := ctx.Err(); err != nil {
					return c.buildResult(maxDepthHit, cacheHits, cacheMisses), nil
				}

				if res.err != nil {
					// Create error node.
					c.addErrorNode(res.ref, res.err, item.ParentVK, item.Depth+1, res.source)
					continue
				}

				if res.resolved == nil {
					continue
				}

				dep := res.resolved

				// Check if already seen (dedup by VersionKey).
				c.mu.Lock()
				existingNodeID, alreadySeen := c.seen[dep.VersionKey]
				c.mu.Unlock()

				var nodeID string
				if alreadySeen {
					// Reuse the existing node ID; just add the edge.
					nodeID = existingNodeID
				} else {
					// Create a new node.
					nodeID = c.addDepNode(dep, res.ref, item.Depth+1)
					c.mu.Lock()
					c.seen[dep.VersionKey] = nodeID
					c.mu.Unlock()
				}

				// Add edge from parent to this node.
				from := item.ParentVK
				if from == "" {
					from = "root"
				}
				c.mu.Lock()
				c.graph.AddEdge(&graph.Edge{
					From:  from,
					To:    nodeID,
					Type:  edgeTypeForSource(res.source),
					Depth: item.Depth + 1,
				})
				c.mu.Unlock()

				if alreadySeen {
					continue // already processed, skip recursion
				}

				// Check if at max depth.
				if item.Depth+1 >= c.opts.MaxDepth {
					continue
				}

				// Check cache for this version's dependencies.
				if c.cache != nil {
					cachedVersion, _ := c.cache.GetVersion(dep.ProjectID, dep.VersionKey)
					if cachedVersion != nil {
						deps, _ := c.cache.GetVersionDependencies(dep.ProjectID, dep.VersionKey)
						if len(deps) > 0 {
							cacheHits++
							var children []CachedChild
							for _, d := range deps {
								children = append(children, CachedChild{
									ProjectID:  d.ChildProjectID,
									VersionKey: d.ChildVersionConstraint,
									EdgeType:   d.DepScope,
								})
							}
							queue = append(queue, queueItem{
								CachedDeps: children,
								Depth:      item.Depth + 1,
								ParentVK:   nodeID,
							})
							continue
						}
					}
					cacheMisses++
				}

				// Enqueue for fresh scanning if contents available.
				if dep.Contents != nil {
					queue = append(queue, queueItem{
						Contents: dep.Contents,
						Depth:    item.Depth + 1,
						ParentVK: nodeID,
					})
				}

				// Cache the resolved version (best-effort).
				c.cacheVersion(dep)
			}
		}
	}

	return c.buildResult(maxDepthHit, cacheHits, cacheMisses), nil
}

// depRefWithSource pairs a DepRef with its source type.
type depRefWithSource struct {
	ref    DepRef
	source DepSourceType
}

// resolveRefs runs Resolve concurrently for all refs, returning results.
func (c *Crawler) resolveRefs(ctx context.Context, refs []depRefWithSource, depth int) []resolveResult {
	results := make([]resolveResult, len(refs))
	var wg sync.WaitGroup
	wg.Add(len(refs))

	for i, r := range refs {
		go func(idx int, drs depRefWithSource) {
			defer wg.Done()

			resolver, ok := c.resolvers[drs.source]
			if !ok {
				results[idx] = resolveResult{
					ref:    drs.ref,
					err:    fmt.Errorf("no resolver for source type %s", drs.source),
					source: drs.source,
				}
				return
			}

			resolved, err := resolver.Resolve(ctx, drs.ref)
			results[idx] = resolveResult{
				ref:      drs.ref,
				resolved: resolved,
				err:      err,
				source:   drs.source,
			}
		}(i, r)
	}

	wg.Wait()
	return results
}

// addDepNode adds a node for a resolved dependency to the graph.
// Returns the node ID.
func (c *Crawler) addDepNode(dep *ResolvedDep, ref DepRef, depth int) string {
	nodeType := nodeTypeForSource(ref.Source)
	nodeID := graph.NodeID(nodeType, dep.VersionKey)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Only add the node if it doesn't already exist.
	if existing := c.graph.Node(nodeID); existing == nil {
		c.graph.AddNode(&graph.Node{
			ID:         nodeID,
			Type:       nodeType,
			Name:       ref.Name,
			Version:    dep.Semver,
			Ref:        ref.Ref,
			Score:      0,
			Risk:       core.RiskUnknown,
			Pinning:    ref.Pinning,
			Metadata:   make(map[string]any),
			ProjectID:  dep.ProjectID,
			VersionKey: dep.VersionKey,
		})
	}
	return nodeID
}

// addErrorNode creates a node representing a resolution failure.
func (c *Crawler) addErrorNode(ref DepRef, resolveErr error, parentVK string, depth int, source DepSourceType) {
	nodeType := nodeTypeForSource(ref.Source)
	errorID := graph.NodeID(nodeType, fmt.Sprintf("error/%s@%s", ref.Name, ref.Ref))

	c.mu.Lock()
	defer c.mu.Unlock()

	c.graph.AddNode(&graph.Node{
		ID:      errorID,
		Type:    nodeType,
		Name:    ref.Name,
		Version: ref.Ref,
		Ref:     ref.Ref,
		Score:   0,
		Risk:    core.RiskCritical,
		Pinning: ref.Pinning,
		Metadata: map[string]any{
			"error": resolveErr.Error(),
		},
	})

	from := parentVK
	if from == "" {
		from = "root"
	}
	c.graph.AddEdge(&graph.Edge{
		From:  from,
		To:    errorID,
		Type:  edgeTypeForSource(source),
		Depth: depth,
	})

	c.errors = append(c.errors, CrawlError{
		DepRef:   ref,
		Depth:    depth,
		Err:      resolveErr,
		Resolver: source,
	})
}

// processCachedDeps adds nodes/edges from cached dependencies and returns
// queue items for further processing.
func (c *Crawler) processCachedDeps(item queueItem) []queueItem {
	var children []queueItem

	for _, child := range item.CachedDeps {
		// Determine edge type from the cached scope.
		edgeType := edgeTypeFromString(child.EdgeType)

		nodeID := graph.NodeID(graph.NodePackage, child.VersionKey)

		c.mu.Lock()
		// Add node if not present.
		if existing := c.graph.Node(nodeID); existing == nil {
			c.graph.AddNode(&graph.Node{
				ID:         nodeID,
				Type:       graph.NodePackage,
				Name:       child.ProjectID,
				Version:    child.VersionKey,
				Score:      0,
				Risk:       core.RiskUnknown,
				Metadata:   make(map[string]any),
				ProjectID:  child.ProjectID,
				VersionKey: child.VersionKey,
			})
		}

		// Add edge from parent.
		c.graph.AddEdge(&graph.Edge{
			From:  item.ParentVK,
			To:    nodeID,
			Type:  edgeType,
			Depth: item.Depth + 1,
		})

		_, alreadySeen := c.seen[child.VersionKey]
		if !alreadySeen {
			c.seen[child.VersionKey] = nodeID
		}
		c.mu.Unlock()

		if alreadySeen || item.Depth+1 >= c.opts.MaxDepth {
			continue
		}

		// Check cache for this child's dependencies.
		if c.cache != nil {
			deps, _ := c.cache.GetVersionDependencies(child.ProjectID, child.VersionKey)
			if len(deps) > 0 {
				var grandchildren []CachedChild
				for _, d := range deps {
					grandchildren = append(grandchildren, CachedChild{
						ProjectID:  d.ChildProjectID,
						VersionKey: d.ChildVersionConstraint,
						EdgeType:   d.DepScope,
					})
				}
				children = append(children, queueItem{
					CachedDeps: grandchildren,
					Depth:      item.Depth + 1,
					ParentVK:   nodeID,
				})
			}
		}
	}

	return children
}

// cacheVersion stores a resolved dependency in the cache (best-effort).
func (c *Crawler) cacheVersion(dep *ResolvedDep) {
	if c.cache == nil {
		return
	}
	_ = c.cache.UpsertProject(&cache.Project{
		ID:        dep.ProjectID,
		Ecosystem: dep.Metadata.OrgName,
		Name:      dep.ProjectID,
	})
	_ = c.cache.UpsertVersion(&cache.ProjectVersion{
		ProjectID:  dep.ProjectID,
		VersionKey: dep.VersionKey,
		Metadata:   "",
	})
}

// buildResult constructs the CrawlResult from the current state.
func (c *Crawler) buildResult(maxDepthHit, cacheHits, cacheMisses int) *CrawlResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	byType := make(map[graph.NodeType]int)
	for _, n := range c.graph.Nodes {
		byType[n.Type]++
	}

	return &CrawlResult{
		Graph: c.graph,
		Stats: CrawlStats{
			TotalNodes:  len(c.graph.Nodes),
			TotalEdges:  len(c.graph.Edges),
			MaxDepthHit: maxDepthHit,
			CacheHits:   cacheHits,
			CacheMisses: cacheMisses,
			ByType:      byType,
		},
		Errors: c.errors,
	}
}

// nodeTypeForSource maps a DepSourceType to a graph.NodeType.
func nodeTypeForSource(src DepSourceType) graph.NodeType {
	switch src {
	case DepSourcePackage:
		return graph.NodePackage
	case DepSourceAction:
		return graph.NodeAction
	case DepSourcePrecommit:
		return graph.NodePrecommitHook
	case DepSourceTerraform:
		return graph.NodeTerraformModule
	case DepSourceSubmodule:
		return graph.NodeGitSubmodule
	case DepSourceTool:
		return graph.NodeDevTool
	case DepSourceScript:
		return graph.NodeScriptDownload
	case DepSourceBuildTool:
		return graph.NodeBuildTool
	default:
		return graph.NodePackage
	}
}

// edgeTypeForSource maps a DepSourceType to a graph.EdgeType.
func edgeTypeForSource(src DepSourceType) graph.EdgeType {
	switch src {
	case DepSourcePackage:
		return graph.EdgeDependsOn
	case DepSourceAction:
		return graph.EdgeUsesAction
	case DepSourcePrecommit:
		return graph.EdgeUsesHook
	case DepSourceTerraform:
		return graph.EdgeUsesModule
	case DepSourceSubmodule:
		return graph.EdgeIncludesSubmodule
	case DepSourceTool:
		return graph.EdgeUsesTool
	case DepSourceScript:
		return graph.EdgeDownloads
	case DepSourceBuildTool:
		return graph.EdgeBuiltWith
	default:
		return graph.EdgeDependsOn
	}
}

// edgeTypeFromString parses an edge type string back into a graph.EdgeType.
func edgeTypeFromString(s string) graph.EdgeType {
	switch s {
	case "depends_on":
		return graph.EdgeDependsOn
	case "hosted_at":
		return graph.EdgeHostedAt
	case "uses_action":
		return graph.EdgeUsesAction
	case "bundles":
		return graph.EdgeBundles
	case "triggers":
		return graph.EdgeTriggers
	case "resolves_to":
		return graph.EdgeResolvesTo
	case "pulls_image":
		return graph.EdgePullsImage
	case "downloads":
		return graph.EdgeDownloads
	case "uses_hook":
		return graph.EdgeUsesHook
	case "uses_module":
		return graph.EdgeUsesModule
	case "includes_submodule":
		return graph.EdgeIncludesSubmodule
	case "uses_tool":
		return graph.EdgeUsesTool
	case "built_with":
		return graph.EdgeBuiltWith
	default:
		return graph.EdgeDependsOn
	}
}
