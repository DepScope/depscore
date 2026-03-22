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

    // Clear existing content via DOM (no innerHTML with user data)
    while (panelBody.firstChild) {
      panelBody.removeChild(panelBody.firstChild);
    }

    var h2 = document.createElement('h2');
    h2.textContent = data.name || '';
    panelBody.appendChild(h2);

    var meta = document.createElement('p');
    meta.className = 'panel-meta';
    meta.textContent = data.eco || '';
    panelBody.appendChild(meta);

    var rows = [
      ['Version',        data.version || ''],
      ['Own Score',      data.score || ''],
      ['Own Risk',       data.risk || '',       'badge risk-' + (data.risk || 'unknown').toLowerCase()],
      ['Transitive Risk',data.transitiveRisk || '', 'badge risk-' + (data.transitiveRisk || 'unknown').toLowerCase()],
      ['Constraint',     data.constraint || ''],
    ];

    rows.forEach(function (r) {
      var div = document.createElement('div');
      div.className = 'panel-row';

      var key = document.createElement('span');
      key.className = 'panel-row-key';
      key.textContent = r[0];

      var valWrap = document.createElement('span');
      valWrap.className = 'panel-row-value';

      if (r[2]) {
        // Badge variant
        var badge = document.createElement('span');
        badge.className = r[2];
        badge.textContent = r[1];
        valWrap.appendChild(badge);
      } else {
        valWrap.textContent = r[1];
      }

      div.appendChild(key);
      div.appendChild(valWrap);
      panelBody.appendChild(div);
    });

    panel.classList.remove('hidden');
    document.body.style.overflow = 'hidden';
  }

  function closePanel() {
    if (!panel) return;
    panel.classList.add('hidden');
    document.body.style.overflow = '';
  }
})();
