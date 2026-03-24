/* depscope — vanilla JS */
(function () {
  'use strict';

  /* =========================================================
     Table Sorting
     ========================================================= */
  var table = document.getElementById('pkg-table');
  if (table) {
    var tbody = table.querySelector('tbody');
    var headers = table.querySelectorAll('th.sortable');
    var sortCol = null;
    var sortAsc = true;

    headers.forEach(function (th) {
      th.addEventListener('click', function () {
        var col = th.dataset.col;
        if (sortCol === col) {
          sortAsc = !sortAsc;
        } else {
          sortCol = col;
          sortAsc = true;
        }
        headers.forEach(function (h) {
          h.classList.remove('sort-asc', 'sort-desc');
        });
        th.classList.add(sortAsc ? 'sort-asc' : 'sort-desc');
        sortTable(col, sortAsc);
      });
    });

    function sortTable(col, asc) {
      var rows = Array.from(tbody.querySelectorAll('tr.pkg-row'));
      rows.sort(function (a, b) {
        var av = getCellValue(a, col);
        var bv = getCellValue(b, col);
        if (!isNaN(av) && !isNaN(bv)) {
          return asc ? av - bv : bv - av;
        }
        return asc
          ? String(av).localeCompare(String(bv))
          : String(bv).localeCompare(String(av));
      });
      rows.forEach(function (r) { tbody.appendChild(r); });
    }

    function getCellValue(row, col) {
      switch (col) {
        case 'name':       return row.dataset.name || '';
        case 'version':    return row.dataset.version || '';
        case 'score':      return parseFloat(row.dataset.score) || 0;
        case 'risk':       return riskOrder(row.dataset.risk);
        case 'transitive': return riskOrder(row.dataset.transitiveRisk);
        case 'constraint': return row.dataset.constraint || '';
        default:           return '';
      }
    }

    function riskOrder(r) {
      var order = { CRITICAL: 0, HIGH: 1, MEDIUM: 2, LOW: 3, UNKNOWN: 4 };
      return order[r] !== undefined ? order[r] : 99;
    }
  }

  /* =========================================================
     Side Panel
     ========================================================= */
  var panel    = document.getElementById('panel');
  var panelBody = document.getElementById('panel-body');

  if (panel && panelBody) {
    var overlay = panel.querySelector('.panel-overlay');
    var closeBtn = panel.querySelector('.panel-close');

    // Open panel on row click (delegated so it works for dynamically added dep-rows too)
    // Skip clicks on the expand ▶ button.
    var tbody = panel ? document.querySelector('#pkg-table tbody') : null;
    if (tbody) {
      tbody.addEventListener('click', function (e) {
        if (e.target.closest('.pkg-expand')) return; // handled by tree expand
        var row = e.target.closest('tr.pkg-row');
        if (row && !row.classList.contains('dep-empty')) {
          openPanel(row.dataset);
        }
      });
    }

    // Close on overlay click or close button
    if (overlay)  overlay.addEventListener('click', closePanel);
    if (closeBtn) closeBtn.addEventListener('click', closePanel);

    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') closePanel();
    });
  }

  function openPanel(data) {
    if (!panel || !panelBody) return;

    // Clear existing content
    while (panelBody.firstChild) {
      panelBody.removeChild(panelBody.firstChild);
    }

    // Show loading state
    var loading = document.createElement('p');
    loading.textContent = 'Loading...';
    loading.className = 'panel-loading';
    panelBody.appendChild(loading);
    panel.classList.remove('hidden');
    document.body.style.overflow = 'hidden';

    // Fetch full detail from API
    var eco = encodeURIComponent(data.eco || '');
    var name = data.name || '';
    var version = data.version || '';
    var url = '/api/package/' + eco + '/' + name;
    if (version) url += '/' + encodeURIComponent(version);

    fetch(url)
      .then(function (resp) { return resp.json(); })
      .then(function (pkg) { renderPanel(pkg); })
      .catch(function () { renderPanelFallback(data); });
  }

  function registryURL(eco, name, version) {
    switch (eco.toLowerCase()) {
      case 'python': return 'https://pypi.org/project/' + name + (version ? '/' + version : '') + '/';
      case 'npm':    return 'https://www.npmjs.com/package/' + name + (version ? '/v/' + version : '');
      case 'crates.io': case 'rust': return 'https://crates.io/crates/' + name + (version ? '/' + version : '');
      case 'go':     return 'https://pkg.go.dev/' + name + (version ? '@' + version : '');
      default:       return null;
    }
  }

  // Checks that passed (no issues found for these)
  var allChecks = [
    'vulnerability',
    'release_recency',
    'maintainer_count',
    'download_velocity',
    'version_pinning',
    'org_backing',
    'open_issue_ratio',
    'repo_health'
  ];

  var checkLabels = {
    'vulnerability':     'Known vulnerabilities (CVE)',
    'release_recency':   'Release recency',
    'maintainer_count':  'Maintainer count',
    'download_velocity': 'Download velocity',
    'version_pinning':   'Version pinning',
    'org_backing':       'Organization backing',
    'open_issue_ratio':  'Open issue ratio',
    'repo_health':       'Repository health'
  };

  // Map issue messages to their check category
  function issueToCheck(msg) {
    msg = msg.toLowerCase();
    if (msg.indexOf('cve:') >= 0 || msg.indexOf('ghsa-') >= 0 || msg.indexOf('pysec-') >= 0) return 'vulnerability';
    if (msg.indexOf('release') >= 0 || msg.indexOf('stale') >= 0) return 'release_recency';
    if (msg.indexOf('maintainer') >= 0 || msg.indexOf('bus-factor') >= 0) return 'maintainer_count';
    if (msg.indexOf('download') >= 0) return 'download_velocity';
    if (msg.indexOf('constraint') >= 0 || msg.indexOf('version') >= 0 || msg.indexOf('pinning') >= 0) return 'version_pinning';
    if (msg.indexOf('org') >= 0 || msg.indexOf('backing') >= 0 || msg.indexOf('individual') >= 0) return 'org_backing';
    if (msg.indexOf('issue ratio') >= 0) return 'open_issue_ratio';
    if (msg.indexOf('commit') >= 0 || msg.indexOf('archived') >= 0 || msg.indexOf('repo') >= 0) return 'repo_health';
    return null;
  }

  function renderPanel(pkg) {
    while (panelBody.firstChild) panelBody.removeChild(panelBody.firstChild);

    // Header
    var h2 = document.createElement('h2');
    h2.textContent = pkg.name;
    panelBody.appendChild(h2);

    var meta = document.createElement('p');
    meta.className = 'panel-meta';
    meta.textContent = pkg.ecosystem + (pkg.version ? ' · ' + pkg.version : '');
    panelBody.appendChild(meta);

    // Registry link
    var regURL = registryURL(pkg.ecosystem, pkg.name, pkg.version);
    if (regURL) {
      var link = document.createElement('a');
      link.href = regURL;
      link.target = '_blank';
      link.rel = 'noopener';
      link.className = 'panel-registry-link';
      link.textContent = 'View on ' + registryName(pkg.ecosystem) + ' \u2197';
      panelBody.appendChild(link);
    }

    // Score section
    var scoreSection = document.createElement('div');
    scoreSection.className = 'panel-section';

    var scoreRow = document.createElement('div');
    scoreRow.className = 'panel-score-row';
    var scoreLabel = document.createElement('span');
    scoreLabel.textContent = 'Score: ' + pkg.score;
    var scoreBadge = document.createElement('span');
    scoreBadge.className = 'badge risk-' + String(pkg.risk).toLowerCase();
    scoreBadge.textContent = pkg.risk;
    scoreRow.appendChild(scoreLabel);
    scoreRow.appendChild(scoreBadge);
    scoreSection.appendChild(scoreRow);

    if (pkg.transitiveScore !== pkg.score) {
      var transRow = document.createElement('div');
      transRow.className = 'panel-score-row';
      var transLabel = document.createElement('span');
      transLabel.textContent = 'Transitive: ' + pkg.transitiveScore;
      var transBadge = document.createElement('span');
      transBadge.className = 'badge risk-' + String(pkg.transitiveRisk).toLowerCase();
      transBadge.textContent = pkg.transitiveRisk;
      transRow.appendChild(transLabel);
      transRow.appendChild(transBadge);
      scoreSection.appendChild(transRow);
    }

    panelBody.appendChild(scoreSection);

    // Info rows
    var infoRows = [
      ['Constraint', pkg.constraintType],
      ['Depth', pkg.depth === 1 ? 'Direct dependency' : 'Transitive (depth ' + pkg.depth + ')'],
    ];
    var infoSection = document.createElement('div');
    infoSection.className = 'panel-section';
    infoRows.forEach(function (r) {
      var div = document.createElement('div');
      div.className = 'panel-row';
      var key = document.createElement('span');
      key.className = 'panel-row-key';
      key.textContent = r[0];
      var val = document.createElement('span');
      val.className = 'panel-row-value';
      val.textContent = r[1];
      div.appendChild(key);
      div.appendChild(val);
      infoSection.appendChild(div);
    });
    panelBody.appendChild(infoSection);

    // Checks: show passed and failed
    var failedChecks = {};
    if (pkg.issues) {
      pkg.issues.forEach(function (iss) {
        var check = issueToCheck(iss.Message);
        if (check) failedChecks[check] = iss;
      });
    }

    var checksHeader = document.createElement('h3');
    checksHeader.textContent = 'Reputation Checks';
    panelBody.appendChild(checksHeader);

    var checksList = document.createElement('ul');
    checksList.className = 'panel-checks-list';
    allChecks.forEach(function (check) {
      var li = document.createElement('li');
      li.className = 'panel-check';
      var icon = document.createElement('span');
      var label = document.createElement('span');
      label.className = 'panel-check-label';
      label.textContent = checkLabels[check];

      if (failedChecks[check]) {
        var sev = String(failedChecks[check].Severity).toLowerCase();
        if (sev === 'info') {
          icon.className = 'check-icon check-info';
          icon.textContent = '\u2139 ';
        } else {
          icon.className = 'check-icon check-fail';
          icon.textContent = '\u2717 ';
        }
        var detail = document.createElement('span');
        detail.className = 'panel-check-detail';
        detail.textContent = ' — ' + failedChecks[check].Message;
        li.appendChild(icon);
        li.appendChild(label);
        li.appendChild(detail);
      } else {
        icon.className = 'check-icon check-pass';
        icon.textContent = '\u2713 ';
        li.appendChild(icon);
        li.appendChild(label);
      }
      checksList.appendChild(li);
    });
    panelBody.appendChild(checksList);

    // Vulnerabilities (CVEs)
    if (pkg.vulnerabilities && pkg.vulnerabilities.length > 0) {
      var vulnHeader = document.createElement('h3');
      vulnHeader.textContent = 'Vulnerabilities (' + pkg.vulnerabilities.length + ')';
      panelBody.appendChild(vulnHeader);

      var vulnList = document.createElement('ul');
      vulnList.className = 'panel-vuln-list';
      pkg.vulnerabilities.forEach(function (v) {
        var li = document.createElement('li');
        li.className = 'panel-vuln';
        var id = document.createElement('a');
        id.className = 'vuln-id';
        id.textContent = v.id;
        id.href = 'https://osv.dev/vulnerability/' + encodeURIComponent(v.id);
        id.target = '_blank';
        id.rel = 'noopener';
        var sev = document.createElement('span');
        sev.className = 'badge sev-' + (v.severity || 'medium').toLowerCase();
        sev.textContent = v.severity || 'UNKNOWN';
        var summary = document.createElement('span');
        summary.className = 'vuln-summary';
        summary.textContent = v.summary;
        li.appendChild(sev);
        li.appendChild(document.createTextNode(' '));
        li.appendChild(id);
        li.appendChild(document.createElement('br'));
        li.appendChild(summary);
        vulnList.appendChild(li);
      });
      panelBody.appendChild(vulnList);
    }

    // Remaining issues not mapped to checks
    var unmappedIssues = (pkg.issues || []).filter(function (iss) {
      return !issueToCheck(iss.Message);
    });
    if (unmappedIssues.length > 0) {
      var otherHeader = document.createElement('h3');
      otherHeader.textContent = 'Other Issues';
      panelBody.appendChild(otherHeader);

      var issueList = document.createElement('ul');
      issueList.className = 'panel-issue-list';
      unmappedIssues.forEach(function (iss) {
        var li = document.createElement('li');
        li.className = 'panel-issue';
        var sev = document.createElement('span');
        sev.className = 'badge sev-' + String(iss.Severity).toLowerCase();
        sev.textContent = iss.Severity;
        var msg = document.createElement('span');
        msg.textContent = ' ' + iss.Message;
        li.appendChild(sev);
        li.appendChild(msg);
        issueList.appendChild(li);
      });
      panelBody.appendChild(issueList);
    }
  }

  function registryName(eco) {
    switch (eco.toLowerCase()) {
      case 'python': return 'PyPI';
      case 'npm': return 'npm';
      case 'crates.io': case 'rust': return 'crates.io';
      case 'go': return 'pkg.go.dev';
      default: return 'registry';
    }
  }

  function renderPanelFallback(data) {
    while (panelBody.firstChild) panelBody.removeChild(panelBody.firstChild);
    var h2 = document.createElement('h2');
    h2.textContent = data.name || '';
    panelBody.appendChild(h2);
    var p = document.createElement('p');
    p.textContent = 'Could not load package details.';
    panelBody.appendChild(p);
  }

  function closePanel() {
    if (!panel) return;
    panel.classList.add('hidden');
    document.body.style.overflow = '';
  }

  /* =========================================================
     Recursive Dependency Tree (expand ▶ in table)
     ========================================================= */
  if (table) {
    table.addEventListener('click', function (e) {
      var expand = e.target.closest('.pkg-expand');
      if (!expand) return;
      e.stopPropagation(); // don't open the side panel

      var row = expand.closest('tr.pkg-row');
      if (!row || row.classList.contains('dep-empty')) return;

      // Toggle: if already expanded, collapse
      if (row.classList.contains('expanded')) {
        collapseRow(row);
        return;
      }

      // Expand: fetch deps and insert sub-rows
      var eco = row.dataset.eco;
      var name = row.dataset.name;
      var version = row.dataset.version;
      var depth = parseInt(row.dataset.depth || '0', 10);
      var url = '/api/package/' + encodeURIComponent(eco) + '/' + name;
      if (version) url += '/' + encodeURIComponent(version);

      expand.textContent = '\u25BC'; // ▼
      row.classList.add('expanded');

      fetch(url)
        .then(function (r) { return r.json(); })
        .then(function (pkg) {
          if (!pkg.dependsOn || pkg.dependsOn.length === 0) {
            // No deps — show a leaf indicator
            var noDep = document.createElement('tr');
            noDep.className = 'dep-row dep-empty';
            noDep.dataset.parentName = name;
            var indent = '';
            for (var j = 0; j <= depth; j++) indent += '\u00A0\u00A0\u00A0';
            noDep.innerHTML = '<td colspan="6" class="dep-leaf">' + indent + '\u2514 no dependencies</td>';
            row.parentNode.insertBefore(noDep, row.nextSibling);
            return;
          }
          // Insert a sub-row for each dependency (reverse so insertion order is correct)
          var depNames = pkg.dependsOn.slice().reverse();
          depNames.forEach(function (depName) {
            // Look up the dep's score from the main table data attributes
            var depRow = findRowByName(depName);
            var depScore = depRow ? depRow.dataset.score : '?';
            var depRisk = depRow ? depRow.dataset.risk : 'UNKNOWN';
            var depVersion = depRow ? depRow.dataset.version : '';

            var sub = createDepRow(depName, depVersion, depScore, depRisk, depth + 1, eco);
            sub.dataset.parentName = name;
            row.parentNode.insertBefore(sub, row.nextSibling);
          });
        })
        .catch(function () {
          expand.textContent = '\u25B6'; // reset to ▶
          row.classList.remove('expanded');
        });
    });
  }

  function createDepRow(name, version, score, risk, depth, eco) {
    var tr = document.createElement('tr');
    tr.className = 'pkg-row dep-row';
    tr.dataset.name = name;
    tr.dataset.version = version || '';
    tr.dataset.eco = eco;
    tr.dataset.score = score;
    tr.dataset.risk = risk;
    tr.dataset.depth = depth;

    var indent = '';
    for (var i = 0; i < depth; i++) indent += '\u00A0\u00A0\u00A0';

    var riskLower = String(risk).toLowerCase();

    tr.innerHTML =
      '<td class="pkg-name">' + indent + '<span class="pkg-expand pkg-expand-sub" title="Show dependencies">&#9654;</span> ' + escapeHtml(name) + '</td>' +
      '<td class="pkg-version">' + escapeHtml(version) + '</td>' +
      '<td class="pkg-score">' + escapeHtml(String(score)) + '</td>' +
      '<td><span class="badge risk-' + riskLower + '">' + escapeHtml(String(risk)) + '</span></td>' +
      '<td></td>' +
      '<td></td>';
    return tr;
  }

  function collapseRow(row) {
    var expand = row.querySelector('.pkg-expand');
    if (expand) expand.textContent = '\u25B6'; // ▶
    row.classList.remove('expanded');

    // Remove all sub-rows inserted after this row (they have higher depth
    // or are dep-empty rows). Stop when we hit a non-dep-row.
    var rowDepth = parseInt(row.dataset.depth || '0', 10);
    var next = row.nextElementSibling;
    while (next && next.classList.contains('dep-row')) {
      var toRemove = next;
      next = next.nextElementSibling;
      toRemove.remove();
    }
  }

  function findRowByName(name) {
    if (!table) return null;
    var rows = table.querySelectorAll('tr.pkg-row');
    for (var i = 0; i < rows.length; i++) {
      if (rows[i].dataset.name === name) return rows[i];
    }
    return null;
  }

  function escapeHtml(str) {
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  /* =========================================================
     Issue Severity Filter
     ========================================================= */
  var activeFilter = null;
  var filters = document.querySelectorAll('.issue-filter');
  var issueItems = document.querySelectorAll('.issue-item');

  filters.forEach(function (badge) {
    badge.style.cursor = 'pointer';
    badge.addEventListener('click', function (e) {
      e.preventDefault();
      e.stopPropagation();
      var sev = badge.dataset.severity;

      if (activeFilter === sev) {
        // Toggle off — show all
        activeFilter = null;
        filters.forEach(function (f) { f.classList.remove('filter-inactive'); });
        issueItems.forEach(function (li) { li.style.display = ''; });
      } else {
        // Filter to this severity
        activeFilter = sev;
        filters.forEach(function (f) {
          f.classList.toggle('filter-inactive', f.dataset.severity !== sev);
        });
        issueItems.forEach(function (li) {
          li.style.display = li.classList.contains('sev-' + sev) ? '' : 'none';
        });
      }
    });
  });
})();
