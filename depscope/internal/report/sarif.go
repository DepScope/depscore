package report

import (
	"io"

	"github.com/depscope/depscope/internal/core"
	sarif "github.com/owenrumney/go-sarif/v3/pkg/report/v22/sarif"
)

const (
	sarifVersion = "2.1.0"
	sarifSchema  = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json"
)

// WriteSARIF writes a SARIF 2.1.0 report of the scan result to w.
// Each HIGH or CRITICAL issue is mapped to a SARIF result with the appropriate
// level ("error" for CRITICAL, "warning" for HIGH).
func WriteSARIF(w io.Writer, result core.ScanResult) error {
	report := sarif.NewReport()
	report.Version = sarifVersion
	report.Schema = sarifSchema

	toolName := "depscope"
	infoURI := "https://github.com/depscope/depscope"
	run := sarif.NewRunWithInformationURI(toolName, infoURI)

	for _, pkg := range result.Packages {
		for _, issue := range pkg.Issues {
			if issue.Severity != core.SeverityHigh {
				continue
			}

			ruleID := "DEP-HIGH"
			msg := sarif.NewMessage().WithText(issue.Message)
			sarifResult := sarif.NewResult().
				WithRuleID(ruleID).
				WithLevel("error").
				WithMessage(msg)

			run.AddResult(sarifResult)
		}
	}

	report.AddRun(run)
	return report.Write(w)
}
