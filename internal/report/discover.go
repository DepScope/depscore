package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/depscope/depscope/internal/discover"
)

// WriteDiscoverText writes a human-readable discover report grouped by status.
func WriteDiscoverText(w io.Writer, result *discover.DiscoverResult) error {
	p := func(format string, args ...any) { fmt.Fprintf(w, format, args...) } //nolint:errcheck

	p("Package: %s | Range: %s\n\n", result.Package, result.Range)

	buckets := map[discover.Status][]discover.ProjectMatch{
		discover.StatusConfirmed:    {},
		discover.StatusPotentially:  {},
		discover.StatusUnresolvable: {},
		discover.StatusSafe:         {},
	}
	for _, m := range result.Matches {
		buckets[m.Status] = append(buckets[m.Status], m)
	}

	labels := []struct {
		status discover.Status
		icon   string
		label  string
	}{
		{discover.StatusConfirmed, "\U0001F534", "CONFIRMED AFFECTED"},
		{discover.StatusPotentially, "\U0001F7E1", "POTENTIALLY AFFECTED"},
		{discover.StatusUnresolvable, "\U0001F535", "UNRESOLVABLE"},
		{discover.StatusSafe, "\U0001F7E2", "SAFE"},
	}

	for _, l := range labels {
		matches := buckets[l.status]
		if len(matches) == 0 {
			continue
		}
		p("%s %s (%d projects)\n", l.icon, l.label, len(matches))
		for _, m := range matches {
			p("  %s\n", m.Project)
			p("    Source: %s\n", m.Source)
			if m.Version != "" {
				p("    Installed: %s %s\n", result.Package, m.Version)
			}
			if m.Constraint != "" {
				p("    Constraint: %s %s\n", result.Package, m.Constraint)
			}
			if m.Depth != "" {
				depthStr := m.Depth
				if len(m.DependencyPath) > 1 {
					chain := ""
					for i, dp := range m.DependencyPath {
						if i > 0 {
							chain += " \u2192 "
						}
						chain += dp
					}
					depthStr += " (via " + chain + ")"
				}
				p("    Depth: %s\n", depthStr)
			}
			if m.Reason != "" {
				p("    Reason: %s\n", m.Reason)
			}
			p("\n")
		}
	}

	s := result.Summary()
	p("Summary: %d confirmed, %d potentially, %d unresolvable, %d safe (%d total)\n",
		s.Confirmed, s.Potentially, s.Unresolvable, s.Safe, s.Total)

	return nil
}

// WriteDiscoverJSON writes a JSON-encoded discover report.
func WriteDiscoverJSON(w io.Writer, result *discover.DiscoverResult) error {
	type jsonMatch struct {
		Status         string   `json:"status"`
		Project        string   `json:"project"`
		Source         string   `json:"source"`
		Version        string   `json:"version,omitempty"`
		Constraint     string   `json:"constraint,omitempty"`
		Depth          string   `json:"depth,omitempty"`
		DependencyPath []string `json:"dependency_path,omitempty"`
		Reason         string   `json:"reason,omitempty"`
	}

	matches := make([]jsonMatch, len(result.Matches))
	for i, m := range result.Matches {
		matches[i] = jsonMatch{
			Status:         m.Status.String(),
			Project:        m.Project,
			Source:         m.Source,
			Version:        m.Version,
			Constraint:     m.Constraint,
			Depth:          m.Depth,
			DependencyPath: m.DependencyPath,
			Reason:         m.Reason,
		}
	}

	s := result.Summary()
	out := struct {
		Package string      `json:"package"`
		Range   string      `json:"range"`
		Results []jsonMatch `json:"results"`
		Summary struct {
			Confirmed    int `json:"confirmed"`
			Potentially  int `json:"potentially"`
			Unresolvable int `json:"unresolvable"`
			Safe         int `json:"safe"`
			Total        int `json:"total"`
		} `json:"summary"`
	}{
		Package: result.Package,
		Range:   result.Range,
		Results: matches,
	}
	out.Summary.Confirmed = s.Confirmed
	out.Summary.Potentially = s.Potentially
	out.Summary.Unresolvable = s.Unresolvable
	out.Summary.Safe = s.Safe
	out.Summary.Total = s.Total

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
