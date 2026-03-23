package report

import (
	"fmt"
	"io"

	"github.com/depscope/depscope/internal/core"
	goSarif "github.com/owenrumney/go-sarif/v3/pkg/report/v210/sarif"
)

func strPtr(s string) *string { return &s }

func WriteSARIF(w io.Writer, result core.ScanResult) error {
	run := goSarif.NewRun()

	tool := goSarif.NewTool()
	driver := goSarif.NewToolComponent()
	driver.Name = strPtr("depscope")
	tool.Driver = driver
	run.Tool = tool

	rulesSeen := make(map[string]bool)
	for _, issue := range result.AllIssues {
		if issue.Severity != core.SeverityHigh && issue.Severity != core.SeverityCritical {
			continue
		}

		ruleID := fmt.Sprintf("SC%03d", len(rulesSeen)+1)
		if !rulesSeen[issue.Message] {
			rulesSeen[issue.Message] = true
			rule := goSarif.NewReportingDescriptor()
			rule.ID = strPtr(ruleID)
			rule.Name = strPtr(issue.Message)
			if driver.Rules == nil {
				driver.Rules = []*goSarif.ReportingDescriptor{}
			}
			driver.Rules = append(driver.Rules, rule)
		}

		level := "warning"
		if issue.Severity == core.SeverityCritical {
			level = "error"
		}

		sarifResult := goSarif.NewResult()
		sarifResult.Level = level
		msg := goSarif.NewMessage()
		msg.Text = strPtr(fmt.Sprintf("%s: %s", issue.Package, issue.Message))
		sarifResult.Message = msg
		run.Results = append(run.Results, sarifResult)
	}

	report := goSarif.NewReport()
	report.Runs = append(report.Runs, run)
	return report.Write(w)
}
