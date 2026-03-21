package report

import (
	"fmt"
	"io"

	"github.com/depscope/depscope/internal/core"
	"github.com/olekukonko/tablewriter"
)

func WriteText(w io.Writer, result core.ScanResult) {
	fmt.Fprintf(w, "depscope scan — profile: %s (threshold: %d)\n\n",
		result.Profile, result.PassThreshold)

	table := tablewriter.NewTable(w)
	table.Header("Package", "Version", "Depth", "Score", "Risk", "Constraint")

	for _, pkg := range result.Packages {
		score := pkg.FinalScore()
		risk := string(pkg.FinalRisk())
		table.Append([]string{
			pkg.Name,
			pkg.Version,
			fmt.Sprintf("%d", pkg.Depth),
			fmt.Sprintf("%d", score),
			risk,
			pkg.ConstraintType,
		})
	}
	table.Render()

	if len(result.AllIssues) > 0 {
		fmt.Fprintf(w, "\nIssues:\n")
		for _, issue := range result.AllIssues {
			fmt.Fprintf(w, "  [%s] %s: %s\n", issue.Severity, issue.Package, issue.Message)
		}
	}

	fmt.Fprintf(w, "\nScanned %d direct + %d transitive dependencies\n",
		result.DirectDeps, result.TransitiveDeps)

	if result.Passed() {
		fmt.Fprintf(w, "Result: PASS\n")
	} else {
		fmt.Fprintf(w, "Result: FAIL\n")
	}
}
