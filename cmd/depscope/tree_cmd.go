package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/report"
	"github.com/depscope/depscope/internal/scanner"
	"github.com/spf13/cobra"
)

func init() {
	treeCmd.Flags().Int("depth", 0, "max tree depth (0 = unlimited)")
	treeCmd.Flags().StringSlice("type", nil, "filter by node type: package, action, workflow, docker, hook, terraform, submodule, tool, build")
	treeCmd.Flags().StringSlice("risk", nil, "filter by risk level: LOW, MEDIUM, HIGH, CRITICAL")
	treeCmd.Flags().Int("collapse", 0, "auto-collapse subtrees deeper than this (0 = no collapse)")
	treeCmd.Flags().Bool("json", false, "output as JSON")
	treeCmd.Flags().String("cache-db", "", "path to SQLite cache database")
	treeCmd.Flags().StringSlice("trusted-orgs", nil, "GitHub orgs to treat as trusted")
	treeCmd.Flags().Bool("no-cve", false, "skip CVE scanning (faster, reputation-only)")
	rootCmd.AddCommand(treeCmd)
}

var treeCmd = &cobra.Command{
	Use:   "tree [path]",
	Short: "Show dependency tree as ASCII/Unicode output",
	Long: `Scan a project directory and display the supply chain dependency tree
as a formatted ASCII/Unicode tree.

The tree shows each dependency with its type, name, version, reputation
score, and mutability status. Use --depth, --type, and --risk flags
to filter the output.

Examples:
  depscope tree .                         # full tree for current dir
  depscope tree . --depth 2               # limit to 2 levels
  depscope tree . --type action,workflow   # only show actions/workflows
  depscope tree . --risk HIGH,CRITICAL    # only show risky nodes
  depscope tree . --json                  # JSON output
  depscope tree . --collapse 3            # collapse subtrees deeper than 3`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE:         runTree,
}

func runTree(cmd *cobra.Command, args []string) error {
	target := "."
	if len(args) == 1 {
		target = args[0]
	}

	cacheDB, _ := cmd.Flags().GetString("cache-db")
	trustedOrgs, _ := cmd.Flags().GetStringSlice("trusted-orgs")
	noCVE, _ := cmd.Flags().GetBool("no-cve")

	crawlOpts := scanner.CrawlOptions{
		NoCVE:       noCVE,
		TrustedOrgs: trustedOrgs,
		CacheDBPath: cacheDB,
	}

	fmt.Fprintln(os.Stderr, "Scanning...") //nolint:errcheck
	result, err := scanner.CrawlDir(context.Background(), target, crawlOpts)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	// Parse tree options.
	depth, _ := cmd.Flags().GetInt("depth")
	collapse, _ := cmd.Flags().GetInt("collapse")
	jsonOut, _ := cmd.Flags().GetBool("json")
	typeStrs, _ := cmd.Flags().GetStringSlice("type")
	riskStrs, _ := cmd.Flags().GetStringSlice("risk")

	var typeFilter []graph.NodeType
	for _, ts := range typeStrs {
		if nt, ok := parseNodeType(ts); ok {
			typeFilter = append(typeFilter, nt)
		}
	}

	// Uppercase risk values.
	var riskFilter []string
	for _, r := range riskStrs {
		riskFilter = append(riskFilter, strings.ToUpper(r))
	}

	opts := report.TreeOptions{
		MaxDepth:   depth,
		TypeFilter: typeFilter,
		RiskFilter: riskFilter,
		CollapseAt: collapse,
		JSON:       jsonOut,
	}

	output := report.RenderTree(result.Graph, opts)
	fmt.Fprint(os.Stdout, output) //nolint:errcheck
	return nil
}

func parseNodeType(s string) (graph.NodeType, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "package":
		return graph.NodePackage, true
	case "action":
		return graph.NodeAction, true
	case "workflow":
		return graph.NodeWorkflow, true
	case "docker", "docker_image":
		return graph.NodeDockerImage, true
	case "script", "script_download":
		return graph.NodeScriptDownload, true
	case "hook", "precommit_hook", "precommit":
		return graph.NodePrecommitHook, true
	case "terraform", "terraform_module":
		return graph.NodeTerraformModule, true
	case "submodule", "git_submodule":
		return graph.NodeGitSubmodule, true
	case "tool", "dev_tool":
		return graph.NodeDevTool, true
	case "build", "build_tool":
		return graph.NodeBuildTool, true
	default:
		return 0, false
	}
}
