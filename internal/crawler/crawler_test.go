package crawler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/core"
	"github.com/depscope/depscope/internal/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockResolver implements Resolver for testing.
type mockResolver struct {
	detectFn  func(ctx context.Context, ft FileTree) ([]DepRef, error)
	resolveFn func(ctx context.Context, ref DepRef) (*ResolvedDep, error)
}

func (m *mockResolver) Detect(ctx context.Context, ft FileTree) ([]DepRef, error) {
	if m.detectFn != nil {
		return m.detectFn(ctx, ft)
	}
	return nil, nil
}

func (m *mockResolver) Resolve(ctx context.Context, ref DepRef) (*ResolvedDep, error) {
	if m.resolveFn != nil {
		return m.resolveFn(ctx, ref)
	}
	return nil, nil
}

// TestCrawler_SingleLevel verifies that a root with 3 deps produces a graph
// with 3 nodes and 3 edges (the root node itself is not counted as a dep node;
// it's the implicit parent).
func TestCrawler_SingleLevel(t *testing.T) {
	resolver := &mockResolver{
		detectFn: func(_ context.Context, _ FileTree) ([]DepRef, error) {
			return []DepRef{
				{Source: DepSourcePackage, Name: "pkg-a", Ref: "1.0.0", Ecosystem: "npm", Pinning: graph.PinningExactVersion},
				{Source: DepSourcePackage, Name: "pkg-b", Ref: "2.0.0", Ecosystem: "npm", Pinning: graph.PinningExactVersion},
				{Source: DepSourcePackage, Name: "pkg-c", Ref: "3.0.0", Ecosystem: "npm", Pinning: graph.PinningExactVersion},
			}, nil
		},
		resolveFn: func(_ context.Context, ref DepRef) (*ResolvedDep, error) {
			return &ResolvedDep{
				ProjectID:  fmt.Sprintf("npm/%s", ref.Name),
				VersionKey: fmt.Sprintf("npm/%s@%s", ref.Name, ref.Ref),
				Semver:     ref.Ref,
				Contents:   nil, // leaf nodes
			}, nil
		},
	}

	resolvers := map[DepSourceType]Resolver{
		DepSourcePackage: resolver,
	}

	c := NewCrawler(nil, resolvers, CrawlerOptions{MaxDepth: 5})
	root := FileTree{"package.json": []byte(`{}`)}

	result, err := c.Crawl(context.Background(), root)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 3 dependency nodes (root is implicit)
	assert.Equal(t, 3, len(result.Graph.Nodes), "expected 3 nodes")
	assert.Equal(t, 3, len(result.Graph.Edges), "expected 3 edges")
	assert.Empty(t, result.Errors)
}

// TestCrawler_Dedup verifies that when two resolvers detect the same dependency
// (same VersionKey), the node appears once but two edges point to it.
func TestCrawler_Dedup(t *testing.T) {
	resolverA := &mockResolver{
		detectFn: func(_ context.Context, _ FileTree) ([]DepRef, error) {
			return []DepRef{
				{Source: DepSourcePackage, Name: "shared-dep", Ref: "1.0.0", Ecosystem: "npm", Pinning: graph.PinningExactVersion},
			}, nil
		},
		resolveFn: func(_ context.Context, ref DepRef) (*ResolvedDep, error) {
			return &ResolvedDep{
				ProjectID:  "npm/shared-dep",
				VersionKey: "npm/shared-dep@1.0.0",
				Semver:     "1.0.0",
				Contents:   nil,
			}, nil
		},
	}

	resolverB := &mockResolver{
		detectFn: func(_ context.Context, _ FileTree) ([]DepRef, error) {
			return []DepRef{
				{Source: DepSourceAction, Name: "shared-dep", Ref: "1.0.0", Ecosystem: "npm", Pinning: graph.PinningExactVersion},
			}, nil
		},
		resolveFn: func(_ context.Context, ref DepRef) (*ResolvedDep, error) {
			return &ResolvedDep{
				ProjectID:  "npm/shared-dep",
				VersionKey: "npm/shared-dep@1.0.0",
				Semver:     "1.0.0",
				Contents:   nil,
			}, nil
		},
	}

	resolvers := map[DepSourceType]Resolver{
		DepSourcePackage: resolverA,
		DepSourceAction:  resolverB,
	}

	c := NewCrawler(nil, resolvers, CrawlerOptions{MaxDepth: 5})
	root := FileTree{"package.json": []byte(`{}`)}

	result, err := c.Crawl(context.Background(), root)
	require.NoError(t, err)
	require.NotNil(t, result)

	// One node for the shared dep, but two edges (one from each resolver).
	assert.Equal(t, 1, len(result.Graph.Nodes), "expected 1 unique node")
	assert.Equal(t, 2, len(result.Graph.Edges), "expected 2 edges to same node")
}

// TestCrawler_MaxDepth verifies that BFS stops at the configured max depth.
// We create a chain: root -> A -> B -> C -> D -> E (depth 5).
// With maxDepth=3, only A (depth 1), B (depth 2), and C (depth 3) should appear.
func TestCrawler_MaxDepth(t *testing.T) {
	// Chain: root -> dep-0 -> dep-1 -> dep-2 -> dep-3 -> dep-4
	resolver := &mockResolver{
		detectFn: func(_ context.Context, ft FileTree) ([]DepRef, error) {
			// Each level detects a single dep named by the "level" file.
			if lvl, ok := ft["level"]; ok {
				name := fmt.Sprintf("dep-%s", string(lvl))
				return []DepRef{
					{Source: DepSourcePackage, Name: name, Ref: "1.0.0", Ecosystem: "npm", Pinning: graph.PinningExactVersion},
				}, nil
			}
			// Root detects dep-0.
			return []DepRef{
				{Source: DepSourcePackage, Name: "dep-0", Ref: "1.0.0", Ecosystem: "npm", Pinning: graph.PinningExactVersion},
			}, nil
		},
		resolveFn: func(_ context.Context, ref DepRef) (*ResolvedDep, error) {
			// Determine the index of this dep.
			var idx int
			_, _ = fmt.Sscanf(ref.Name, "dep-%d", &idx)
			vk := fmt.Sprintf("npm/%s@1.0.0", ref.Name)

			// If idx < 4, return contents that trigger the next level.
			var contents FileTree
			if idx < 4 {
				contents = FileTree{"level": []byte(fmt.Sprintf("%d", idx+1))}
			}
			return &ResolvedDep{
				ProjectID:  fmt.Sprintf("npm/%s", ref.Name),
				VersionKey: vk,
				Semver:     "1.0.0",
				Contents:   contents,
			}, nil
		},
	}

	resolvers := map[DepSourceType]Resolver{
		DepSourcePackage: resolver,
	}

	c := NewCrawler(nil, resolvers, CrawlerOptions{MaxDepth: 3})
	root := FileTree{"package.json": []byte(`{}`)}

	result, err := c.Crawl(context.Background(), root)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Depth 1: dep-0, Depth 2: dep-1, Depth 3: dep-2 (stop here).
	assert.Equal(t, 3, len(result.Graph.Nodes), "expected 3 nodes (depth 1,2,3)")
	assert.Equal(t, 3, len(result.Graph.Edges), "expected 3 edges")

	// Verify dep-3 and dep-4 are NOT in the graph.
	for _, n := range result.Graph.Nodes {
		assert.NotContains(t, n.Name, "dep-3")
		assert.NotContains(t, n.Name, "dep-4")
	}
}

// TestCrawler_CacheHit verifies that when a version exists in cache with known
// deps, Resolve is NOT called for that version but its children still appear.
func TestCrawler_CacheHit(t *testing.T) {
	// Setup a real CacheDB in a temp directory.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-cache.db")
	cacheDB, err := cache.NewCacheDB(dbPath)
	require.NoError(t, err)
	defer func() { _ = cacheDB.Close() }()

	// Pre-populate cache: project "npm/cached-pkg" with version "npm/cached-pkg@1.0.0"
	// has a child dependency "npm/child-a".
	err = cacheDB.UpsertProject(&cache.Project{
		ID:        "npm/cached-pkg",
		Ecosystem: "npm",
		Name:      "cached-pkg",
	})
	require.NoError(t, err)

	err = cacheDB.UpsertVersion(&cache.ProjectVersion{
		ProjectID:  "npm/cached-pkg",
		VersionKey: "npm/cached-pkg@1.0.0",
		Metadata:   "{}",
	})
	require.NoError(t, err)

	err = cacheDB.AddVersionDependency(&cache.VersionDependency{
		ParentProjectID:        "npm/cached-pkg",
		ParentVersionKey:       "npm/cached-pkg@1.0.0",
		ChildProjectID:         "npm/child-a",
		ChildVersionConstraint: "npm/child-a@2.0.0",
		DepScope:               "depends_on",
	})
	require.NoError(t, err)

	// Also add child-a as a project+version so it's in cache (for graph node creation).
	err = cacheDB.UpsertProject(&cache.Project{
		ID:        "npm/child-a",
		Ecosystem: "npm",
		Name:      "child-a",
	})
	require.NoError(t, err)

	err = cacheDB.UpsertVersion(&cache.ProjectVersion{
		ProjectID:  "npm/child-a",
		VersionKey: "npm/child-a@2.0.0",
		Metadata:   "{}",
	})
	require.NoError(t, err)

	resolveCallCount := 0
	resolver := &mockResolver{
		detectFn: func(_ context.Context, _ FileTree) ([]DepRef, error) {
			return []DepRef{
				{Source: DepSourcePackage, Name: "cached-pkg", Ref: "1.0.0", Ecosystem: "npm", Pinning: graph.PinningExactVersion},
			}, nil
		},
		resolveFn: func(_ context.Context, ref DepRef) (*ResolvedDep, error) {
			resolveCallCount++
			return &ResolvedDep{
				ProjectID:  fmt.Sprintf("npm/%s", ref.Name),
				VersionKey: fmt.Sprintf("npm/%s@%s", ref.Name, ref.Ref),
				Semver:     ref.Ref,
				Contents:   nil,
			}, nil
		},
	}

	resolvers := map[DepSourceType]Resolver{
		DepSourcePackage: resolver,
	}

	c := NewCrawler(cacheDB, resolvers, CrawlerOptions{MaxDepth: 5})
	root := FileTree{"package.json": []byte(`{}`)}

	result, err := c.Crawl(context.Background(), root)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Resolve was called for "cached-pkg" (it was detected fresh from root),
	// but the cached version's children (child-a) were loaded from cache,
	// so Resolve was NOT called for child-a.
	assert.Equal(t, 1, resolveCallCount, "Resolve should be called once (for cached-pkg, not for child-a)")

	// Both cached-pkg and child-a should appear in the graph.
	assert.Equal(t, 2, len(result.Graph.Nodes), "expected 2 nodes (cached-pkg + child-a)")
	assert.GreaterOrEqual(t, result.Stats.CacheHits, 1, "expected at least 1 cache hit")
}

// TestCrawler_ErrorNode verifies that when a resolver.Resolve returns an error,
// an error node with Risk=CRITICAL is added to the graph.
func TestCrawler_ErrorNode(t *testing.T) {
	resolver := &mockResolver{
		detectFn: func(_ context.Context, _ FileTree) ([]DepRef, error) {
			return []DepRef{
				{Source: DepSourcePackage, Name: "broken-pkg", Ref: "1.0.0", Ecosystem: "npm", Pinning: graph.PinningExactVersion},
				{Source: DepSourcePackage, Name: "good-pkg", Ref: "2.0.0", Ecosystem: "npm", Pinning: graph.PinningExactVersion},
			}, nil
		},
		resolveFn: func(_ context.Context, ref DepRef) (*ResolvedDep, error) {
			if ref.Name == "broken-pkg" {
				return nil, errors.New("network timeout")
			}
			return &ResolvedDep{
				ProjectID:  fmt.Sprintf("npm/%s", ref.Name),
				VersionKey: fmt.Sprintf("npm/%s@%s", ref.Name, ref.Ref),
				Semver:     ref.Ref,
				Contents:   nil,
			}, nil
		},
	}

	resolvers := map[DepSourceType]Resolver{
		DepSourcePackage: resolver,
	}

	c := NewCrawler(nil, resolvers, CrawlerOptions{MaxDepth: 5})
	root := FileTree{"package.json": []byte(`{}`)}

	result, err := c.Crawl(context.Background(), root)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have 2 nodes: one error node for broken-pkg, one normal for good-pkg.
	assert.Equal(t, 2, len(result.Graph.Nodes), "expected 2 nodes")

	// Find the error node.
	var errorNode *graph.Node
	for _, n := range result.Graph.Nodes {
		if n.Risk == core.RiskCritical && n.Metadata != nil {
			if _, ok := n.Metadata["error"]; ok {
				errorNode = n
				break
			}
		}
	}
	require.NotNil(t, errorNode, "expected an error node with Risk=CRITICAL")
	assert.Equal(t, "network timeout", errorNode.Metadata["error"])
	assert.Equal(t, 1, len(result.Errors), "expected 1 crawl error")
}

// TestCrawler_ContextCancellation verifies that context cancellation stops the crawl.
func TestCrawler_ContextCancellation(t *testing.T) {
	resolver := &mockResolver{
		detectFn: func(_ context.Context, _ FileTree) ([]DepRef, error) {
			return []DepRef{
				{Source: DepSourcePackage, Name: "slow-pkg", Ref: "1.0.0", Ecosystem: "npm"},
			}, nil
		},
		resolveFn: func(ctx context.Context, _ DepRef) (*ResolvedDep, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	resolvers := map[DepSourceType]Resolver{
		DepSourcePackage: resolver,
	}

	c := NewCrawler(nil, resolvers, CrawlerOptions{MaxDepth: 5, Timeout: 5 * time.Second})
	root := FileTree{"package.json": []byte(`{}`)}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately.
	cancel()

	result, err := c.Crawl(ctx, root)
	// Should return gracefully (possibly with error or partial result).
	if err != nil {
		assert.True(t, errors.Is(err, context.Canceled), "expected context.Canceled error")
	} else {
		// If no error, result should be valid (possibly empty).
		require.NotNil(t, result)
	}
}

// TestCrawler_NilCache verifies the crawler works with a nil cache.
func TestCrawler_NilCache(t *testing.T) {
	resolver := &mockResolver{
		detectFn: func(_ context.Context, _ FileTree) ([]DepRef, error) {
			return []DepRef{
				{Source: DepSourcePackage, Name: "pkg-a", Ref: "1.0.0", Ecosystem: "npm"},
			}, nil
		},
		resolveFn: func(_ context.Context, ref DepRef) (*ResolvedDep, error) {
			return &ResolvedDep{
				ProjectID:  fmt.Sprintf("npm/%s", ref.Name),
				VersionKey: fmt.Sprintf("npm/%s@%s", ref.Name, ref.Ref),
				Semver:     ref.Ref,
			}, nil
		},
	}

	c := NewCrawler(nil, map[DepSourceType]Resolver{DepSourcePackage: resolver}, CrawlerOptions{})
	root := FileTree{"package.json": []byte(`{}`)}

	result, err := c.Crawl(context.Background(), root)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, len(result.Graph.Nodes))
}

// TestCrawler_ErrorNodeMetadata verifies that when a resolver returns an error,
// the resulting error node has Risk=CRITICAL, Metadata["error"] contains the
// error message, and the error appears in CrawlResult.Errors.
func TestCrawler_ErrorNodeMetadata(t *testing.T) {
	resolveErr := errors.New("connection refused: registry.npmjs.org")

	resolver := &mockResolver{
		detectFn: func(_ context.Context, _ FileTree) ([]DepRef, error) {
			return []DepRef{
				{Source: DepSourcePackage, Name: "failing-pkg", Ref: "1.0.0", Ecosystem: "npm", Pinning: graph.PinningExactVersion},
			}, nil
		},
		resolveFn: func(_ context.Context, ref DepRef) (*ResolvedDep, error) {
			return nil, resolveErr
		},
	}

	resolvers := map[DepSourceType]Resolver{
		DepSourcePackage: resolver,
	}

	c := NewCrawler(nil, resolvers, CrawlerOptions{MaxDepth: 5})
	root := FileTree{"package.json": []byte(`{}`)}

	result, err := c.Crawl(context.Background(), root)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have exactly 1 error node.
	require.Equal(t, 1, len(result.Graph.Nodes), "expected 1 error node")

	// Find the error node and verify its properties.
	var errorNode *graph.Node
	for _, n := range result.Graph.Nodes {
		errorNode = n
		break
	}
	require.NotNil(t, errorNode)

	// Risk must be CRITICAL.
	assert.Equal(t, core.RiskCritical, errorNode.Risk, "error node Risk should be CRITICAL")

	// Metadata["error"] must contain the error message.
	require.NotNil(t, errorNode.Metadata, "error node should have Metadata")
	errMsg, ok := errorNode.Metadata["error"]
	require.True(t, ok, "error node Metadata should contain 'error' key")
	assert.Equal(t, resolveErr.Error(), errMsg, "error message should match")

	// The error must appear in CrawlResult.Errors.
	require.Len(t, result.Errors, 1, "expected 1 CrawlError")
	assert.Contains(t, result.Errors[0].Err.Error(), "connection refused")
}

// Ensure temp dir cleanup
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
