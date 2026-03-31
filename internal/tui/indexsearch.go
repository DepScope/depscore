// internal/tui/indexsearch.go
package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/depscope/depscope/internal/cache"
)

// IndexSearchModel is the bubbletea model for interactive index searching.
type IndexSearchModel struct {
	db      *cache.CacheDB
	input   textinput.Model
	results []indexSearchResult
	cursor  int
	offset  int
	detail  *indexSearchDetail // non-nil when viewing detail
	width   int
	height  int
	stats   *cache.IndexStatus
	mode    string // "search" or "detail"
}

type indexSearchResult struct {
	ProjectID string
	Name      string
	Ecosystem string
	Version   string
	Manifests int
	Score     int
	Risk      string
}

type indexSearchDetail struct {
	ProjectID  string
	Name       string
	Manifests  []cache.IndexSearchResult
	DependsOn  []cache.VersionDependency
	DependedBy []cache.VersionDependency
}

// NewIndexSearchModel creates a new index-search TUI model.
func NewIndexSearchModel(db *cache.CacheDB) IndexSearchModel {
	ti := textinput.New()
	ti.Placeholder = "Search packages (e.g., axios, lodash, requests)..."
	ti.Focus()
	ti.Width = 60

	m := IndexSearchModel{
		db:    db,
		input: ti,
		mode:  "search",
	}

	// Load initial stats from the first indexed root.
	rows, err := db.DB().Query(`SELECT DISTINCT root_path FROM index_manifests LIMIT 1`)
	if err == nil {
		defer func() { _ = rows.Close() }()
		if rows.Next() {
			var root string
			_ = rows.Scan(&root)
			m.stats, _ = db.IndexStats(root)
		}
	}

	return m
}

// Init implements tea.Model.
func (m IndexSearchModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model.
func (m IndexSearchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = msg.Width - 4
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.mode == "detail" {
				m.mode = "search"
				m.detail = nil
				return m, nil
			}
			return m, tea.Quit

		case "esc":
			if m.mode == "detail" {
				m.mode = "search"
				m.detail = nil
				return m, nil
			}
			if m.input.Value() != "" {
				m.input.SetValue("")
				m.results = nil
				return m, nil
			}
			return m, tea.Quit

		case "enter":
			if m.mode == "search" && len(m.results) > 0 {
				m.openDetail(m.results[m.cursor])
				return m, nil
			}

		case "up", "k":
			if m.mode == "search" && m.cursor > 0 {
				m.cursor--
				m.ensureVisibleIndex()
			}
			return m, nil

		case "down", "j":
			if m.mode == "search" && m.cursor < len(m.results)-1 {
				m.cursor++
				m.ensureVisibleIndex()
			}
			return m, nil

		case "tab":
			// Reserved for future mode switching.
			return m, nil
		}
	}

	if m.mode == "search" {
		var cmd tea.Cmd
		prev := m.input.Value()
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != prev {
			m.doSearch()
		}
		return m, cmd
	}

	return m, nil
}

func (m *IndexSearchModel) doSearch() {
	query := strings.TrimSpace(m.input.Value())
	if query == "" {
		m.results = nil
		m.cursor = 0
		return
	}

	// Search across all ecosystems using exact project_id matching.
	seen := make(map[string]bool)
	var results []indexSearchResult

	for _, eco := range []string{"npm", "python", "go", "rust", "php"} {
		projectID := eco + "/" + query
		hits, err := m.db.SearchIndexByPackageName(projectID)
		if err != nil || len(hits) == 0 {
			continue
		}

		// Group by project_id + version.
		type versionGroup struct {
			version   string
			manifests int
		}
		versions := make(map[string]*versionGroup)
		for _, h := range hits {
			key := h.ProjectID + "@" + h.Version
			if versions[key] == nil {
				versions[key] = &versionGroup{version: h.Version}
			}
			versions[key].manifests++
		}

		for _, vg := range versions {
			rid := eco + "/" + query + "@" + vg.version
			if seen[rid] {
				continue
			}
			seen[rid] = true
			r := indexSearchResult{
				ProjectID: eco + "/" + query,
				Name:      query,
				Ecosystem: eco,
				Version:   vg.version,
				Manifests: vg.manifests,
			}
			// Try to get enrichment data from version metadata.
			versionKey := eco + "/" + query + "@" + vg.version
			ver, _ := m.db.GetVersion(eco+"/"+query, versionKey)
			if ver != nil && ver.Metadata != "" {
				var em struct {
					Score int    `json:"score"`
					Risk  string `json:"risk"`
				}
				if json.Unmarshal([]byte(ver.Metadata), &em) == nil {
					r.Score = em.Score
					r.Risk = em.Risk
				}
			}
			results = append(results, r)
		}
	}

	// Fall back to partial matching via LIKE query if no exact results.
	if len(results) == 0 {
		rows, err := m.db.DB().Query(
			`SELECT DISTINCT mp.project_id, COUNT(DISTINCT mp.manifest_id) as cnt
			 FROM manifest_packages mp
			 WHERE mp.project_id LIKE ?
			 GROUP BY mp.project_id
			 ORDER BY cnt DESC
			 LIMIT 50`, "%/"+query+"%",
		)
		if err == nil {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var pid string
				var cnt int
				_ = rows.Scan(&pid, &cnt)
				eco := ""
				name := pid
				if idx := strings.Index(pid, "/"); idx >= 0 {
					eco = pid[:idx]
					name = pid[idx+1:]
				}
				results = append(results, indexSearchResult{
					ProjectID: pid,
					Name:      name,
					Ecosystem: eco,
					Manifests: cnt,
				})
			}
		}
	}

	m.results = results
	m.cursor = 0
	m.offset = 0
}

func (m *IndexSearchModel) openDetail(r indexSearchResult) {
	d := &indexSearchDetail{
		ProjectID: r.ProjectID,
		Name:      r.Name,
	}

	// Get all manifests for this package.
	d.Manifests, _ = m.db.SearchIndexByPackageName(r.ProjectID)

	// Get dependency info for the first version found.
	if len(d.Manifests) > 0 {
		versionKey := d.Manifests[0].VersionKey
		d.DependsOn, _ = m.db.GetVersionDependencies(r.ProjectID, versionKey)
		d.DependedBy, _ = m.db.FindDependents(r.ProjectID)
	}

	m.detail = d
	m.mode = "detail"
}

func (m *IndexSearchModel) ensureVisibleIndex() {
	contentHeight := m.indexContentHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+contentHeight {
		m.offset = m.cursor - contentHeight + 1
	}
}

func (m IndexSearchModel) indexContentHeight() int {
	h := m.height - 8
	if h < 1 {
		h = 1
	}
	return h
}

// View implements tea.Model.
func (m IndexSearchModel) View() string {
	var b strings.Builder

	// Header.
	header := styleHeader.Width(m.width).Render(" depscope index search")
	b.WriteString(header)
	b.WriteString("\n")

	if m.mode == "detail" {
		b.WriteString(m.renderIndexDetail())
	} else {
		b.WriteString(m.renderIndexSearch())
	}

	// Footer.
	var help string
	if m.mode == "detail" {
		help = " esc: back  q: quit"
	} else {
		help = " type to search  \u2191\u2193/jk: navigate  enter: details  esc/q: quit"
	}
	if m.stats != nil {
		help += fmt.Sprintf("  |  %d packages, %d manifests indexed", m.stats.PackageCount, m.stats.ManifestCount)
	}
	footer := styleFooter.Width(m.width).Render(help)
	b.WriteString(footer)

	return b.String()
}

func (m IndexSearchModel) renderIndexSearch() string {
	var b strings.Builder

	// Search input.
	b.WriteString("\n")
	b.WriteString("  " + m.input.View())
	b.WriteString("\n\n")

	if len(m.results) == 0 {
		if m.input.Value() == "" {
			if m.stats != nil {
				b.WriteString(styleLabel.Render(fmt.Sprintf("  %d packages across %d manifests. Start typing to search.\n",
					m.stats.PackageCount, m.stats.ManifestCount)))
				if len(m.stats.TopPackages) > 0 {
					b.WriteString("\n")
					b.WriteString(styleLabel.Render("  Most referenced packages:\n"))
					limit := 10
					if len(m.stats.TopPackages) < limit {
						limit = len(m.stats.TopPackages)
					}
					for _, p := range m.stats.TopPackages[:limit] {
						b.WriteString(fmt.Sprintf("    %-40s %d manifests\n", p.ProjectID, p.Count))
					}
				}
			} else {
				b.WriteString(styleLabel.Render("  No index data. Run 'depscope index <path>' first.\n"))
			}
		} else {
			b.WriteString(styleLabel.Render("  No results found.\n"))
		}
		return b.String()
	}

	// Result count.
	b.WriteString(styleLabel.Render(fmt.Sprintf("  %d result(s)\n\n", len(m.results))))

	// Results list.
	contentHeight := m.indexContentHeight()
	end := m.offset + contentHeight
	if end > len(m.results) {
		end = len(m.results)
	}

	for i := m.offset; i < end; i++ {
		r := m.results[i]
		eco := fmt.Sprintf("%-8s", r.Ecosystem)
		ver := r.Version
		if ver == "" {
			ver = "-"
		}
		scoreInfo := ""
		if r.Risk != "" {
			scoreInfo = fmt.Sprintf("  score:%d %s", r.Score, r.Risk)
		}
		line := fmt.Sprintf("  [%s] %-30s %-15s  %d manifest(s)%s", eco, r.Name, ver, r.Manifests, scoreInfo)

		if i == m.cursor {
			line = styleSelected.Render(padRight(line, m.width))
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func (m IndexSearchModel) renderIndexDetail() string {
	if m.detail == nil {
		return ""
	}
	d := m.detail

	var b strings.Builder
	b.WriteString("\n")

	// Package name.
	title := fmt.Sprintf("  %s", d.ProjectID)
	b.WriteString(stylePanelTitle.Render(title))
	b.WriteString("\n\n")

	// Manifests.
	b.WriteString(styleLabel.Render(fmt.Sprintf("  Found in %d manifest(s):\n", len(d.Manifests))))
	contentHeight := m.height - 12
	shown := 0
	for _, h := range d.Manifests {
		if shown >= contentHeight/3 {
			b.WriteString(styleLabel.Render(fmt.Sprintf("    ... and %d more\n", len(d.Manifests)-shown)))
			break
		}
		scope := h.DepScope
		ver := h.Version
		if ver == "" {
			ver = "-"
		}
		line := fmt.Sprintf("    %-50s  %-12s  %s", h.ManifestRelPath, ver, scope)
		b.WriteString(line)
		b.WriteString("\n")
		shown++
	}

	// Dependencies.
	if len(d.DependsOn) > 0 {
		b.WriteString("\n")
		b.WriteString(styleLabel.Render(fmt.Sprintf("  Depends on (%d):\n", len(d.DependsOn))))
		for _, dep := range d.DependsOn {
			name := dep.ChildProjectID
			if idx := strings.Index(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			b.WriteString(fmt.Sprintf("    -> %-30s  %s\n", name, dep.ChildVersionConstraint))
		}
	}

	// Depended on by.
	if len(d.DependedBy) > 0 {
		b.WriteString("\n")
		b.WriteString(styleLabel.Render(fmt.Sprintf("  Depended on by (%d):\n", len(d.DependedBy))))
		for _, dep := range d.DependedBy {
			name := dep.ParentProjectID
			if idx := strings.Index(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			b.WriteString(fmt.Sprintf("    <- %-30s  %s\n", name, dep.ChildVersionConstraint))
		}
	}

	b.WriteString("\n")

	return b.String()
}
