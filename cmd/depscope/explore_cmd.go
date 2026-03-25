package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/scanner"
	"github.com/depscope/depscope/internal/tui"
	"github.com/spf13/cobra"
)

func init() {
	exploreCmd.Flags().String("profile", "enterprise", "scoring profile: hobby, opensource, enterprise")
	exploreCmd.Flags().Bool("no-cve", false, "skip CVE scanning (faster, reputation-only)")
	exploreCmd.Flags().StringSlice("only", nil, "filter to specific ecosystems")
	rootCmd.AddCommand(exploreCmd)

	// Also add --explore flag to scan command
	scanCmd.Flags().Bool("explore", false, "launch interactive TUI after scan")
}

var exploreCmd = &cobra.Command{
	Use:          "explore [path]",
	Short:        "Interactive supply chain graph explorer",
	SilenceUsage: true,
	Long: `Scan a project and launch an interactive TUI to explore the supply chain graph.

Navigate dependencies, actions, Docker images, and risk paths interactively.

Examples:
  depscope explore .                    # scan current dir, launch TUI
  depscope explore . --only actions     # actions only
  depscope explore . --no-cve           # skip CVE scanning`,
	Args: cobra.MaximumNArgs(1),
	RunE: runExplore,
}

func runExplore(cmd *cobra.Command, args []string) error {
	target := "."
	if len(args) == 1 {
		target = args[0]
	}

	profile, _ := cmd.Flags().GetString("profile")
	noCVE, _ := cmd.Flags().GetBool("no-cve")
	only, _ := cmd.Flags().GetStringSlice("only")

	opts := scanner.Options{
		Profile: profile,
		NoCVE:   noCVE,
		Only:    only,
	}

	fmt.Fprintln(os.Stderr, "Scanning...") //nolint:errcheck
	result, err := scanner.ScanDir(target, opts)
	if err != nil {
		return err
	}

	g, ok := result.Graph.(*graph.Graph)
	if !ok || g == nil {
		return fmt.Errorf("no graph data available")
	}

	model := tui.NewModel(g)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}
