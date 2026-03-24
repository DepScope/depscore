package report

import (
	"fmt"
	"io"

	"github.com/depscope/depscope/internal/core"
	"github.com/olekukonko/tablewriter"
)

// WriteText writes a human-readable text report of the scan result to w.
// It renders a table of packages followed by any issues, and a final
// PASS or FAIL verdict.
func WriteText(w io.Writer, result core.ScanResult) error {
	table := tablewriter.NewWriter(w)
	table.Header([]string{"Package", "Version", "Score", "Risk", "Transitive Risk", "Constraint"})

	for _, pkg := range result.Packages {
		row := []string{
			pkg.Name,
			pkg.Version,
			fmt.Sprintf("%d", pkg.OwnScore),
			string(pkg.OwnRisk),
			string(pkg.TransitiveRisk),
			pkg.ConstraintType,
		}
		if err := table.Append(row); err != nil {
			return fmt.Errorf("report: appending table row: %w", err)
		}
	}

	if err := table.Render(); err != nil {
		return fmt.Errorf("report: rendering table: %w", err)
	}

	// Risk paths: show worst dependency chains
	if len(result.RiskPaths) > 0 {
		fmt.Fprintln(w)                                       //nolint:errcheck
		fmt.Fprintln(w, "Risk Paths (worst dependency chains):") //nolint:errcheck
		for i, rp := range result.RiskPaths {
			chain := ""
			for j, name := range rp.Chain {
				if j > 0 {
					chain += " → "
				}
				chain += name
			}
			fmt.Fprintf(w, "  %d. %s [score: %d, %s]\n", i+1, chain, rp.EndScore, rp.EndRisk) //nolint:errcheck
			fmt.Fprintf(w, "     %s\n", rp.Reason)                                          //nolint:errcheck
		}
	}

	// Suspicious indicators: supply chain anomalies
	if len(result.Suspicious) > 0 {
		fmt.Fprintln(w)                          //nolint:errcheck
		fmt.Fprintln(w, "Supply Chain Warnings:") //nolint:errcheck
		for _, s := range result.Suspicious {
			fmt.Fprintf(w, "  [%s] %s: %s\n", s.Severity, s.Package, s.Message) //nolint:errcheck
		}
	}

	// Print issues below
	if len(result.AllIssues) > 0 {
		fmt.Fprintln(w)           //nolint:errcheck
		fmt.Fprintln(w, "Issues:") //nolint:errcheck
		for _, issue := range result.AllIssues {
			if issue.Severity == core.SeverityInfo {
				continue // skip INFO in CLI output to reduce noise
			}
			fmt.Fprintf(w, "  [%s] %s: %s\n", issue.Severity, issue.Package, issue.Message) //nolint:errcheck
		}
	}

	fmt.Fprintln(w)              //nolint:errcheck
	if result.Passed() {
		fmt.Fprintln(w, "Result: PASS") //nolint:errcheck
	} else {
		fmt.Fprintln(w, "Result: FAIL") //nolint:errcheck
	}

	return nil
}
