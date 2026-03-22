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
    var version = encodeURIComponent(data.version || '');
    var url = '/api/package/' + eco + '/' + name + '/' + version;

    fetch(url)
      .then(function (resp) { return resp.json(); })
      .then(function (pkg) { renderPanel(pkg); })
      .catch(function () { renderPanelFallback(data); });
  }

  function renderPanel(pkg) {
    while (panelBody.firstChild) panelBody.removeChild(panelBody.firstChild);

    // Header
    var h2 = document.createElement('h2');
    h2.textContent = pkg.name;
    panelBody.appendChild(h2);

    var meta = document.createElement('p');
    meta.className = 'panel-meta';
    meta.textContent = pkg.ecosystem + ' · ' + (pkg.version || 'unknown version');
    panelBody.appendChild(meta);

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

    if (pkg.transitiveRisk && pkg.transitiveRisk !== pkg.risk) {
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
      ['Depends on', pkg.dependsOn + ' packages'],
      ['Depended on by', pkg.dependedOn + ' packages'],
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

    // Issues
    if (pkg.issues && pkg.issues.length > 0) {
      var issueHeader = document.createElement('h3');
      issueHeader.textContent = 'Issues (' + pkg.issues.length + ')';
      panelBody.appendChild(issueHeader);

      var issueList = document.createElement('ul');
      issueList.className = 'panel-issue-list';
      pkg.issues.forEach(function (iss) {
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
    } else {
      var noIssues = document.createElement('p');
      noIssues.className = 'panel-no-issues';
      noIssues.textContent = 'No issues found';
      panelBody.appendChild(noIssues);
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
