package crawler_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/depscope/depscope/internal/crawler"
	"github.com/depscope/depscope/internal/crawler/resolvers"
	"github.com/depscope/depscope/internal/graph"
)

// BenchmarkCrawl_SmallProject benchmarks crawling a small project
// with go.mod + 1 workflow + pre-commit + tools (no network).
func BenchmarkCrawl_SmallProject(b *testing.B) {
	tree := crawler.FileTree{
		"go.mod": []byte(`module example.com/small
go 1.22
require github.com/stretchr/testify v1.9.0
`),
		".github/workflows/ci.yml": []byte(`name: CI
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11
      - uses: actions/setup-go@v5
      - run: go test ./...
`),
		".pre-commit-config.yaml": []byte(`repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.6.0
    hooks:
      - id: trailing-whitespace
`),
		".tool-versions": []byte(`golang 1.22.0
nodejs 20.11.0
`),
	}

	allResolvers := buildAllResolvers()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := crawler.NewCrawler(nil, allResolvers, crawler.CrawlerOptions{MaxDepth: 5})
		_, _ = c.Crawl(context.Background(), tree)
	}
}

// BenchmarkCrawl_MediumProject benchmarks a project with multiple ecosystems,
// 5 workflows, lockfile with transitive deps.
func BenchmarkCrawl_MediumProject(b *testing.B) {
	tree := buildMediumProject()
	allResolvers := buildAllResolvers()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := crawler.NewCrawler(nil, allResolvers, crawler.CrawlerOptions{MaxDepth: 10})
		_, _ = c.Crawl(context.Background(), tree)
	}
}

// BenchmarkCrawl_LargeTree benchmarks crawling with synthetic deep transitive deps
// to stress BFS dedup and queue handling.
func BenchmarkCrawl_LargeTree(b *testing.B) {
	// Build a package.json + lock with 100 transitive deps in a chain
	tree := buildLargeTree(100)
	allResolvers := buildAllResolvers()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := crawler.NewCrawler(nil, allResolvers, crawler.CrawlerOptions{MaxDepth: 25})
		_, _ = c.Crawl(context.Background(), tree)
	}
}

// BenchmarkDetect_AllResolvers benchmarks running all 8 resolvers' Detect phase
// on a realistic FileTree.
func BenchmarkDetect_AllResolvers(b *testing.B) {
	tree := buildMediumProject()
	allResolvers := buildAllResolvers()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, r := range allResolvers {
			_, _ = r.Detect(ctx, tree)
		}
	}
}

// BenchmarkDedup benchmarks the dedup mechanism with many duplicate deps.
func BenchmarkDedup(b *testing.B) {
	// Create a resolver that returns the same 10 deps every time
	mockR := &benchMockResolver{
		refs: make([]crawler.DepRef, 10),
	}
	for i := 0; i < 10; i++ {
		mockR.refs[i] = crawler.DepRef{
			Source:    crawler.DepSourcePackage,
			Name:      fmt.Sprintf("pkg-%d", i),
			Ref:       "1.0.0",
			Ecosystem: "npm",
			Pinning:   graph.PinningExactVersion,
		}
	}

	rs := map[crawler.DepSourceType]crawler.Resolver{
		crawler.DepSourcePackage: mockR,
	}

	tree := crawler.FileTree{"package.json": []byte(`{}`)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := crawler.NewCrawler(nil, rs, crawler.CrawlerOptions{MaxDepth: 3})
		_, _ = c.Crawl(context.Background(), tree)
	}
}

// BenchmarkPinningScore benchmarks the pinning score calculation.
func BenchmarkPinningScore(b *testing.B) {
	g := graph.New()
	g.AddNode(&graph.Node{
		ID: crawler.RootNodeID, Type: graph.NodeRepo,
		Name: "root", Metadata: make(map[string]any),
	})
	for i := 0; i < 100; i++ {
		pinning := graph.PinningQuality(i % 8)
		g.AddNode(&graph.Node{
			ID:       fmt.Sprintf("dev_tool:tool-%d@1.0.0", i),
			Type:     graph.NodeDevTool,
			Name:     fmt.Sprintf("tool-%d", i),
			Pinning:  pinning,
			Metadata: make(map[string]any),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		crawler.RunScorePass(context.Background(), g, nil)
	}
}

// --- helpers ---

func buildAllResolvers() map[crawler.DepSourceType]crawler.Resolver {
	return map[crawler.DepSourceType]crawler.Resolver{
		crawler.DepSourcePackage:   resolvers.NewPackageResolver(),
		crawler.DepSourceAction:    resolvers.NewActionResolver(),
		crawler.DepSourcePrecommit: resolvers.NewPrecommitResolver(),
		crawler.DepSourceSubmodule: resolvers.NewSubmoduleResolver(),
		crawler.DepSourceTerraform: resolvers.NewTerraformResolver(),
		crawler.DepSourceTool:      resolvers.NewToolResolver(),
		crawler.DepSourceScript:    resolvers.NewScriptResolver(),
		crawler.DepSourceBuildTool: resolvers.NewBuildToolResolver(),
	}
}

func buildMediumProject() crawler.FileTree {
	return crawler.FileTree{
		"go.mod": []byte(`module example.com/medium
go 1.22
require (
	github.com/spf13/cobra v1.8.0
	github.com/stretchr/testify v1.9.0
	golang.org/x/sync v0.6.0
)
`),
		".github/workflows/ci.yml": []byte(`name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go test ./...
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: golangci/golangci-lint-action@v7
`),
		".github/workflows/release.yml": []byte(`name: Release
on:
  push:
    tags: ['v*']
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: goreleaser/goreleaser-action@v6
`),
		".pre-commit-config.yaml": []byte(`repos:
  - repo: https://github.com/golangci/golangci-lint
    rev: v2.1.0
    hooks:
      - id: golangci-lint
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v5.0.0
    hooks:
      - id: trailing-whitespace
      - id: end-of-file-fixer
`),
		".tool-versions": []byte(`golang 1.22.0
nodejs 20.11.0
terraform 1.7.3
`),
		"Makefile": []byte(`install:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh
	go install github.com/goreleaser/goreleaser@latest
`),
		"main.tf": []byte(`module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.1.0"
}
`),
	}
}

func buildLargeTree(n int) crawler.FileTree {
	// Build a package.json with n deps as a flat list
	deps := "{"
	for i := 0; i < n; i++ {
		if i > 0 {
			deps += ","
		}
		deps += fmt.Sprintf(`"pkg-%d":"1.0.%d"`, i, i)
	}
	deps += "}"

	return crawler.FileTree{
		"package.json": []byte(fmt.Sprintf(`{"name":"large","dependencies":%s}`, deps)),
	}
}

type benchMockResolver struct {
	refs []crawler.DepRef
}

func (m *benchMockResolver) Detect(_ context.Context, _ crawler.FileTree) ([]crawler.DepRef, error) {
	return m.refs, nil
}

func (m *benchMockResolver) Resolve(_ context.Context, ref crawler.DepRef) (*crawler.ResolvedDep, error) {
	return &crawler.ResolvedDep{
		ProjectID:  "npm/" + ref.Name,
		VersionKey: "npm/" + ref.Name + "@" + ref.Ref,
		Semver:     ref.Ref,
	}, nil
}
