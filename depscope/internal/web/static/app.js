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

    // Open panel on row click
    var rows = document.querySelectorAll('tr.pkg-row');
    rows.forEach(function (row) {
      row.addEventListener('click', function () {
        openPanel(row.dataset);
      });
    });

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
    'release_recency',
    'maintainer_count',
    'download_velocity',
    'version_pinning',
    'org_backing',
    'open_issue_ratio',
    'repo_health'
  ];

  var checkLabels = {
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
        icon.className = 'check-icon check-fail';
        icon.textContent = '\u2717 ';
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
})();
