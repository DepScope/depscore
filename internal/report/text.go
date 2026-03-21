package report

import (
	"fmt"
	"io"
	"sort"

	"github.com/depscope/depscope/internal/core"
)

func WriteText(w io.Writer, result core.ScanResult) {
	fmt.Fprintf(w, "depscope scan — profile: %s (threshold: %d)\n\n", result.Profile, result.PassThreshold)

	// Build lookup map
	byName := make(map[string]core.PackageResult)
	for _, pkg := range result.Packages {
		byName[pkg.Name] = pkg
	}

	// Find root packages (not a child of any other package)
	isChild := make(map[string]bool)
	for _, children := range result.Deps {
		for _, c := range children {
			isChild[c] = true
		}
	}

	var roots []string
	for _, pkg := range result.Packages {
		if !isChild[pkg.Name] {
			roots = append(roots, pkg.Name)
		}
	}
	sort.Strings(roots)

	// If no deps map provided, fall back to flat list
	if len(result.Deps) == 0 {
		for _, pkg := range result.Packages {
			score := pkg.FinalScore()
			risk := string(pkg.FinalRisk())
			vuln := ""
			if pkg.VulnCount > 0 {
				vuln = fmt.Sprintf(" | %d CVE", pkg.VulnCount)
			}
			versionStr := formatVersion(pkg.Constraint, pkg.Version)
			fmt.Fprintf(w, "  %s %s [Score: %d | Risk: %s%s]\n", pkg.Name, versionStr, score, risk, vuln)
		}
	} else {
		// Print tree
		visited := make(map[string]bool)
		for i, root := range roots {
			isLast := i == len(roots)-1
			printTree(w, root, "", isLast, byName, result.Deps, visited)
		}
	}

	// Issues section
	if len(result.AllIssues) > 0 {
		fmt.Fprintf(w, "\nIssues:\n")
		for _, issue := range result.AllIssues {
			pkg := issue.Package
			if pkg == "" {
				pkg = "unknown"
			}
			fmt.Fprintf(w, "  [%s] %s: %s\n", issue.Severity, pkg, issue.Message)
		}
	}

	fmt.Fprintf(w, "\nScanned %d direct + %d transitive dependencies\n", result.DirectDeps, result.TransitiveDeps)

	if result.Passed() {
		fmt.Fprintf(w, "Result: PASS\n")
	} else {
		fmt.Fprintf(w, "Result: FAIL\n")
	}
}

func printTree(w io.Writer, name string, prefix string, isLast bool, byName map[string]core.PackageResult, deps map[string][]string, visited map[string]bool) {
	// Connector
	connector := "├── "
	if isLast {
		connector = "└── "
	}

	pkg, exists := byName[name]
	if !exists {
		fmt.Fprintf(w, "%s%s%s (unknown)\n", prefix, connector, name)
		return
	}

	score := pkg.FinalScore()
	risk := string(pkg.FinalRisk())
	vuln := ""
	if pkg.VulnCount > 0 {
		vuln = fmt.Sprintf(" | %d CVE", pkg.VulnCount)
	}
	versionStr := formatVersion(pkg.Constraint, pkg.Version)

	fmt.Fprintf(w, "%s%s%s %s [Score: %d | Risk: %s%s]\n",
		prefix, connector, name, versionStr, score, risk, vuln)

	// Prevent infinite loops from circular deps
	if visited[name] {
		return
	}
	visited[name] = true

	// Print children
	children := deps[name]
	sort.Strings(children)
	childPrefix := prefix
	if isLast {
		childPrefix += "    "
	} else {
		childPrefix += "│   "
	}

	for i, child := range children {
		childIsLast := i == len(children)-1
		printTree(w, child, childPrefix, childIsLast, byName, deps, visited)
	}

	delete(visited, name) // allow same package to appear in different subtrees
}

// formatVersion builds the version display string.
// When the constraint differs from the resolved version (and is not an exact pin),
// show "constraint → resolved". Otherwise just show the resolved version.
func formatVersion(constraint, version string) string {
	if constraint == "" {
		return version
	}
	// Exact pins: constraint equals "=<version>" or "==<version>" or is the bare version
	if constraint == version || constraint == "="+version || constraint == "=="+version {
		return version
	}
	return fmt.Sprintf("%s \u2192 %s", constraint, version)
}
