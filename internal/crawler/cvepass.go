// internal/crawler/cvepass.go
package crawler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/graph"
	"github.com/depscope/depscope/internal/vuln"
)

// RunCVEPass walks every node in g and enriches it with CVE information.
//
// For nodes that carry a semver version (from Node.Metadata["semver"] or from
// the VersionKey when it contains "@<version>"):
//  1. Check the CVE cache (skipped when cacheDB is nil).
//  2. On a cache miss, query the OSV client (skipped when osvClient is nil).
//  3. Store fresh findings in cache (skipped when cacheDB is nil).
//  4. Attach findings to the node as node.Metadata["cve_findings"].
//  5. Apply a severity-based score penalty.
//
// For nodes without a resolvable semver, node.Metadata["cve_status"] is set
// to "unchecked".
//
// The function is designed to be resilient: all errors are collected and
// returned without aborting the walk.
func RunCVEPass(
	ctx context.Context,
	g *graph.Graph,
	cacheDB *cache.CacheDB,
	osvClient *vuln.OSVClient,
) []CrawlError {
	var errs []CrawlError

	for _, node := range g.Nodes {
		if err := ctx.Err(); err != nil {
			break
		}

		// Resolve the semver for this node.
		semver := semverForNode(node)
		if semver == "" {
			node.Metadata["cve_status"] = "unchecked"
			continue
		}

		// Determine ecosystem + package name from node metadata or node name.
		ecosystem := ecosystemForNode(node)
		name := node.Name

		// 1. Try cache.
		var findings []vuln.Finding
		if cacheDB != nil {
			cached, err := cacheDB.GetCVECache(ecosystem, name, semver)
			if err == nil && cached != "" {
				var fs []vuln.Finding
				if json.Unmarshal([]byte(cached), &fs) == nil {
					findings = fs
				}
			}
		}

		// 2. On cache miss, query OSV.
		if findings == nil && osvClient != nil {
			queried, err := osvClient.Query(ecosystem, name, semver)
			if err != nil {
				errs = append(errs, CrawlError{
					Err: err,
				})
				// Still mark as unchecked so downstream knows we tried.
				node.Metadata["cve_status"] = "unchecked"
				continue
			}
			findings = queried

			// 3. Store in cache.
			if cacheDB != nil {
				if raw, err := json.Marshal(findings); err == nil {
					_ = cacheDB.SetCVECache(ecosystem, name, semver, string(raw))
				}
			}
		}

		// 4. Attach findings (even if empty slice — it means no CVEs found).
		if findings == nil {
			findings = []vuln.Finding{}
		}
		node.Metadata["cve_findings"] = findings

		// 5. Apply severity-based score penalty.
		if len(findings) > 0 {
			node.Score = applyFindingsPenalty(node.Score, findings)
		}
	}

	return errs
}

// semverForNode extracts a semver string from the node, or returns "".
// It first checks node.Metadata["semver"], then parses the VersionKey for
// an "@<version>" suffix.
func semverForNode(n *graph.Node) string {
	if sv, ok := n.Metadata["semver"]; ok {
		if s, ok := sv.(string); ok && s != "" {
			return s
		}
	}
	// Fall back to VersionKey: e.g. "npm/lodash@4.17.21"
	if n.VersionKey != "" {
		if i := strings.LastIndex(n.VersionKey, "@"); i != -1 {
			v := n.VersionKey[i+1:]
			if v != "" {
				return v
			}
		}
	}
	// Also check the node's Version field directly.
	if n.Version != "" {
		return n.Version
	}
	return ""
}

// ecosystemForNode returns the ecosystem string for an OSV query.
// It checks node.Metadata["ecosystem"] first, then falls back to "".
func ecosystemForNode(n *graph.Node) string {
	if eco, ok := n.Metadata["ecosystem"]; ok {
		if s, ok := eco.(string); ok {
			return s
		}
	}
	return ""
}

// applyFindingsPenalty reduces score based on vulnerability severities.
//
// Penalty per finding:
//   - CRITICAL: -15
//   - HIGH:     -10
//   - MEDIUM:    -5
//   - LOW:       -2
//   - unknown:   -5
//
// Score is clamped to [0, 100].
func applyFindingsPenalty(score int, findings []vuln.Finding) int {
	penalty := 0
	for _, f := range findings {
		switch f.Severity {
		case vuln.SeverityCritical:
			penalty += 15
		case vuln.SeverityHigh:
			penalty += 10
		case vuln.SeverityMedium:
			penalty += 5
		case vuln.SeverityLow:
			penalty += 2
		default:
			penalty += 5
		}
	}
	result := score - penalty
	if result < 0 {
		return 0
	}
	if result > 100 {
		return 100
	}
	return result
}
