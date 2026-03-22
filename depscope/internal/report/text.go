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

	// Print issues below the table.
	if len(result.AllIssues) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Issues:")
		for _, issue := range result.AllIssues {
			fmt.Fprintf(w, "  [%s] %s: %s\n", issue.Severity, issue.Package, issue.Message)
		}
	}

	fmt.Fprintln(w)
	if result.Passed() {
		fmt.Fprintln(w, "Result: PASS")
	} else {
		fmt.Fprintln(w, "Result: FAIL")
	}

	return nil
}
