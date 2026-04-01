package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/depscope/depscope/internal/cache"
	"github.com/depscope/depscope/internal/scanner"
)

// writeJSON encodes data as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

// handleSearchPage renders the search.html template.
func (s *Server) handleSearchPage(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, r, "search.html", nil)
}

// handleIndexStats returns aggregated index statistics as JSON.
func (s *Server) handleIndexStats(w http.ResponseWriter, r *http.Request) {
	if s.cacheDBPath == "" {
		writeJSON(w, http.StatusOK, map[string]any{"error": "no index database configured"})
		return
	}
	db, err := cache.NewCacheDB(s.cacheDBPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = db.Close() }()

	// Get all roots.
	rows, err := db.DB().Query(`SELECT DISTINCT root_path FROM index_manifests`)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"manifests": 0, "packages": 0, "ecosystems": map[string]int{}})
		return
	}
	defer func() { _ = rows.Close() }()

	var allStats struct {
		Manifests   int            `json:"manifests"`
		Packages    int            `json:"packages"`
		Ecosystems  map[string]int `json:"ecosystems"`
		TopPackages []struct {
			Name  string `json:"name"`
			ID    string `json:"id"`
			Count int    `json:"count"`
		} `json:"top_packages"`
	}
	allStats.Ecosystems = make(map[string]int)

	for rows.Next() {
		var root string
		if err := rows.Scan(&root); err != nil {
			continue
		}
		stats, err := db.IndexStats(root)
		if err != nil {
			continue
		}
		allStats.Manifests += stats.ManifestCount
		allStats.Packages += stats.PackageCount
		for eco, c := range stats.EcosystemCounts {
			allStats.Ecosystems[eco] += c
		}
		for _, tp := range stats.TopPackages {
			allStats.TopPackages = append(allStats.TopPackages, struct {
				Name  string `json:"name"`
				ID    string `json:"id"`
				Count int    `json:"count"`
			}{Name: tp.Name, ID: tp.ProjectID, Count: tp.Count})
		}
	}

	// ── Risk distribution ────────────────────────────────────────────
	type riskBucket struct {
		Risk     string  `json:"risk"`
		Count    int     `json:"count"`
		AvgScore float64 `json:"avg_score"`
	}
	var riskDist []riskBucket

	riskRows, rErr := db.DB().Query(
		`SELECT json_extract(metadata, '$.risk') as risk, COUNT(*), AVG(json_extract(metadata, '$.score'))
		 FROM project_versions WHERE metadata != '' AND metadata != '{}'
		 GROUP BY risk ORDER BY AVG(json_extract(metadata, '$.score')) ASC`)
	if rErr == nil {
		defer func() { _ = riskRows.Close() }()
		for riskRows.Next() {
			var b riskBucket
			var riskNull sql.NullString
			var avgNull sql.NullFloat64
			if err := riskRows.Scan(&riskNull, &b.Count, &avgNull); err != nil {
				continue
			}
			if riskNull.Valid {
				b.Risk = riskNull.String
			} else {
				b.Risk = "UNKNOWN"
			}
			if avgNull.Valid {
				b.AvgScore = avgNull.Float64
			}
			riskDist = append(riskDist, b)
		}
	}

	// ── CVE summary ─────────────────────────────────────────────────
	type cveSummary struct {
		PackagesWithCVEs int `json:"packages_with_cves"`
		TotalCVEs        int `json:"total_cves"`
	}
	var cveSum cveSummary
	var totalCVEsNull sql.NullInt64
	_ = db.DB().QueryRow(
		`SELECT COUNT(*), SUM(json_extract(metadata, '$.cve_count'))
		 FROM project_versions WHERE metadata != '' AND json_extract(metadata, '$.cve_count') > 0`,
	).Scan(&cveSum.PackagesWithCVEs, &totalCVEsNull)
	if totalCVEsNull.Valid {
		cveSum.TotalCVEs = int(totalCVEsNull.Int64)
	}

	// ── Top riskiest packages ───────────────────────────────────────
	type riskyPkg struct {
		Name      string `json:"name"`
		Score     int    `json:"score"`
		Risk      string `json:"risk"`
		CVEs      int    `json:"cves"`
		Manifests int    `json:"manifests"`
	}
	var riskiest []riskyPkg

	rpRows, rpErr := db.DB().Query(
		`SELECT
		   pv.version_key,
		   json_extract(pv.metadata, '$.score') as score,
		   json_extract(pv.metadata, '$.risk') as risk,
		   COALESCE(json_extract(pv.metadata, '$.cve_count'), 0) as cve_count,
		   COUNT(DISTINCT mp.manifest_id) as manifest_count
		 FROM project_versions pv
		 JOIN manifest_packages mp ON mp.version_key = pv.version_key
		 WHERE pv.metadata != '' AND pv.metadata != '{}'
		 GROUP BY pv.version_key
		 ORDER BY score ASC
		 LIMIT 5`)
	if rpErr == nil {
		defer func() { _ = rpRows.Close() }()
		for rpRows.Next() {
			var p riskyPkg
			var scoreNull sql.NullInt64
			var riskNull sql.NullString
			if err := rpRows.Scan(&p.Name, &scoreNull, &riskNull, &p.CVEs, &p.Manifests); err != nil {
				continue
			}
			if scoreNull.Valid {
				p.Score = int(scoreNull.Int64)
			}
			if riskNull.Valid {
				p.Risk = riskNull.String
			}
			riskiest = append(riskiest, p)
		}
	}

	resp := map[string]any{
		"manifests":         allStats.Manifests,
		"packages":          allStats.Packages,
		"ecosystems":        allStats.Ecosystems,
		"top_packages":      allStats.TopPackages,
		"risk_distribution": riskDist,
		"cve_summary":       cveSum,
		"riskiest_packages": riskiest,
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleIndexSearch searches for packages (regular or compromised mode).
//
// POST body: {"query": "axios", "compromised": ["axios@1.14.1", "axios@0.30.4"]}
// If "compromised" is set, it filters results to only matching versions.
// If "query" is set without compromised, it does a package name search.
func (s *Server) handleIndexSearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query       string   `json:"query"`
		Compromised []string `json:"compromised"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	db, err := cache.NewCacheDB(s.cacheDBPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = db.Close() }()

	type resultEntry struct {
		ManifestPath string `json:"manifest_path"`
		ManifestID   int64  `json:"manifest_id"`
		Ecosystem    string `json:"ecosystem"`
		ProjectID    string `json:"project_id"`
		Version      string `json:"version"`
		Constraint   string `json:"constraint"`
		DepScope     string `json:"dep_scope"`
		Compromised  bool   `json:"compromised"`
		MatchedRule  string `json:"matched_rule,omitempty"`
		Score        int    `json:"score"`
		Risk         string `json:"risk"`
		CVECount     int    `json:"cve_count"`
	}

	var results []resultEntry

	if len(req.Compromised) > 0 {
		// Compromised mode: parse targets, search index, check versions.
		targets, parseErr := scanner.ParseCompromisedList(strings.Join(req.Compromised, ","))
		if parseErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": parseErr.Error()})
			return
		}

		targetMap := make(map[string][]string)
		for _, t := range targets {
			targetMap[t.Name] = append(targetMap[t.Name], t.VersionOrRange)
		}

		for name, ranges := range targetMap {
			for _, eco := range []string{"npm", "python", "go", "rust", "php"} {
				projectID := eco + "/" + name
				hits, err := db.SearchIndexByPackageName(projectID)
				if err != nil {
					continue
				}
				for _, h := range hits {
					for _, rng := range ranges {
						if scanner.SemverSatisfies(rng, h.Version) {
							entry := resultEntry{
								ManifestPath: h.ManifestRelPath,
								ManifestID:   0,
								Ecosystem:    h.Ecosystem,
								ProjectID:    h.ProjectID,
								Version:      h.Version,
								Constraint:   h.Constraint,
								DepScope:     h.DepScope,
								Compromised:  true,
								MatchedRule:  name + "@" + rng,
							}
							// Try to get enrichment data.
							ver, _ := db.GetVersion(h.ProjectID, h.VersionKey)
							if ver != nil && ver.Metadata != "" {
								var em struct {
									Score    int    `json:"score"`
									Risk     string `json:"risk"`
									CVECount int    `json:"cve_count"`
								}
								if json.Unmarshal([]byte(ver.Metadata), &em) == nil {
									entry.Score = em.Score
									entry.Risk = em.Risk
									entry.CVECount = em.CVECount
								}
							}
							results = append(results, entry)
							break
						}
					}
				}
			}
		}
	} else if req.Query != "" {
		// General search: find all manifests that reference this package name.
		query := strings.TrimSpace(req.Query)
		for _, eco := range []string{"npm", "python", "go", "rust", "php"} {
			projectID := eco + "/" + query
			hits, err := db.SearchIndexByPackageName(projectID)
			if err != nil {
				continue
			}
			for _, h := range hits {
				entry := resultEntry{
					ManifestPath: h.ManifestRelPath,
					Ecosystem:    h.Ecosystem,
					ProjectID:    h.ProjectID,
					Version:      h.Version,
					Constraint:   h.Constraint,
					DepScope:     h.DepScope,
				}
				// Try to get enrichment data.
				ver, _ := db.GetVersion(h.ProjectID, h.VersionKey)
				if ver != nil && ver.Metadata != "" {
					var em struct {
						Score    int    `json:"score"`
						Risk     string `json:"risk"`
						CVECount int    `json:"cve_count"`
					}
					if json.Unmarshal([]byte(ver.Metadata), &em) == nil {
						entry.Score = em.Score
						entry.Risk = em.Risk
						entry.CVECount = em.CVECount
					}
				}
				results = append(results, entry)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"count":   len(results),
		"results": results,
	})
}

// handleManifestDetail returns all packages in a manifest by its ID.
func (s *Server) handleManifestDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id := 0
	_, _ = fmt.Sscan(idStr, &id)

	db, err := cache.NewCacheDB(s.cacheDBPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer func() { _ = db.Close() }()

	pkgs, err := db.GetManifestPackages(int64(id))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"manifest_id": id,
		"packages":    pkgs,
		"count":       len(pkgs),
	})
}
