(function() {
  // Tab switching
  document.querySelectorAll('.tab').forEach(function(tab) {
    tab.addEventListener('click', function() {
      document.querySelectorAll('.tab').forEach(function(t) { t.classList.remove('active'); });
      document.querySelectorAll('.tab-content').forEach(function(c) { c.classList.remove('active'); });
      tab.classList.add('active');
      document.getElementById('tab-' + tab.dataset.tab).classList.add('active');
    });
  });

  // Enter key support
  var searchInput = document.getElementById('search-query');
  if (searchInput) {
    searchInput.addEventListener('keydown', function(e) {
      if (e.key === 'Enter') doSearch();
    });
  }

  // Load stats on page load
  loadStats();
})();

function loadStats() {
  fetch('/api/index/stats')
    .then(function(r) { return r.json(); })
    .then(function(data) {
      document.getElementById('stat-manifests').textContent = (data.manifests || 0).toLocaleString();
      document.getElementById('stat-packages').textContent = (data.packages || 0).toLocaleString();

      var ecoCard = document.getElementById('stat-ecosystems-card');
      if (data.ecosystems && Object.keys(data.ecosystems).length > 0) {
        // Build ecosystem tags using safe DOM methods
        while (ecoCard.firstChild) ecoCard.removeChild(ecoCard.firstChild);
        var label = document.createElement('span');
        label.className = 'stat-label';
        label.style.marginBottom = '6px';
        label.textContent = 'Ecosystems';
        ecoCard.appendChild(label);
        Object.keys(data.ecosystems).sort().forEach(function(eco) {
          var tag = document.createElement('span');
          tag.className = 'eco-tag';
          tag.textContent = eco + ' ' + data.ecosystems[eco];
          ecoCard.appendChild(tag);
        });
      }
    })
    .catch(function() {});
}

function doSearch() {
  var query = document.getElementById('search-query').value.trim();
  if (!query) return;

  fetch('/api/index/search', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({query: query})
  })
  .then(function(r) { return r.json(); })
  .then(function(data) { renderResults(data, false); })
  .catch(function(err) { console.error(err); });
}

function doCompromisedSearch() {
  var text = document.getElementById('compromised-input').value.trim();
  if (!text) return;

  var lines = text.split('\n').map(function(l) { return l.trim(); }).filter(function(l) { return l && !l.startsWith('#'); });
  if (lines.length === 0) return;

  fetch('/api/index/search', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({compromised: lines})
  })
  .then(function(r) { return r.json(); })
  .then(function(data) { renderResults(data, true); })
  .catch(function(err) { console.error(err); });
}

function renderResults(data, isCompromised) {
  var area = document.getElementById('results-area');
  var countEl = document.getElementById('results-count');
  var labelEl = document.getElementById('results-label');
  var listEl = document.getElementById('results-list');

  area.style.display = 'block';
  countEl.textContent = data.count || 0;
  labelEl.textContent = isCompromised ? 'compromised findings' : 'package references found';

  // Clear previous results
  while (listEl.firstChild) listEl.removeChild(listEl.firstChild);

  if (!data.results || data.results.length === 0) {
    var empty = document.createElement('div');
    empty.style.color = 'var(--text-secondary)';
    empty.style.padding = '20px';
    empty.textContent = 'No results found.';
    listEl.appendChild(empty);
    return;
  }

  // Group by package+version for summary
  var groups = {};
  var groupOrder = [];
  data.results.forEach(function(r) {
    var key = r.project_id + '@' + r.version;
    if (!groups[key]) {
      groups[key] = { project_id: r.project_id, version: r.version, items: [], compromised: r.compromised, matched_rule: r.matched_rule };
      groupOrder.push(key);
    }
    groups[key].items.push(r);
  });

  groupOrder.forEach(function(key) {
    var g = groups[key];
    var name = g.project_id.indexOf('/') >= 0 ? g.project_id.substring(g.project_id.indexOf('/') + 1) : g.project_id;
    var eco = g.project_id.indexOf('/') >= 0 ? g.project_id.substring(0, g.project_id.indexOf('/')) : '';

    var groupEl = document.createElement('div');
    groupEl.className = 'result-group';

    // Group header
    var header = document.createElement('div');
    header.className = 'result-group-header' + (g.compromised ? ' compromised' : '');

    var pkgSpan = document.createElement('span');
    pkgSpan.className = 'result-pkg';
    pkgSpan.textContent = name;
    header.appendChild(pkgSpan);

    var verSpan = document.createElement('span');
    verSpan.className = 'result-version';
    verSpan.textContent = g.version;
    header.appendChild(verSpan);

    if (eco) {
      var ecoSpan = document.createElement('span');
      ecoSpan.className = 'eco-tag';
      ecoSpan.textContent = eco;
      header.appendChild(ecoSpan);
    }

    var countSpan = document.createElement('span');
    countSpan.style.color = 'var(--text-secondary)';
    countSpan.style.fontSize = '13px';
    countSpan.textContent = g.items.length + ' manifest' + (g.items.length !== 1 ? 's' : '');
    header.appendChild(countSpan);

    if (g.matched_rule) {
      var matchSpan = document.createElement('span');
      matchSpan.style.color = 'var(--risk-critical)';
      matchSpan.style.fontSize = '12px';
      matchSpan.style.marginLeft = '8px';
      matchSpan.textContent = 'matched: ' + g.matched_rule;
      header.appendChild(matchSpan);
    }

    groupEl.appendChild(header);

    // Items
    g.items.forEach(function(item) {
      var card = document.createElement('div');
      card.className = 'result-card' + (item.compromised ? ' compromised' : '');
      card.addEventListener('click', function() { openManifestDetail(item.manifest_path); });

      var manifestSpan = document.createElement('span');
      manifestSpan.className = 'result-manifest';
      manifestSpan.textContent = item.manifest_path;
      card.appendChild(manifestSpan);

      if (item.constraint) {
        var constraintSpan = document.createElement('span');
        constraintSpan.className = 'dep-ver';
        constraintSpan.textContent = item.constraint;
        card.appendChild(constraintSpan);
      }

      var scopeSpan = document.createElement('span');
      scopeSpan.className = 'result-scope ' + item.dep_scope;
      scopeSpan.textContent = item.dep_scope;
      card.appendChild(scopeSpan);

      groupEl.appendChild(card);
    });

    listEl.appendChild(groupEl);
  });
}

function openManifestDetail(manifestPath) {
  var panel = document.getElementById('detail-panel');
  var title = document.getElementById('detail-title');
  var content = document.getElementById('detail-content');

  title.textContent = manifestPath;
  panel.style.display = 'block';

  // Build detail content using safe DOM methods
  while (content.firstChild) content.removeChild(content.firstChild);

  var section = document.createElement('div');
  section.className = 'detail-section';

  var h4 = document.createElement('h4');
  h4.textContent = 'Manifest Path';
  section.appendChild(h4);

  var p = document.createElement('p');
  p.style.fontFamily = 'monospace';
  p.style.fontSize = '13px';
  p.style.wordBreak = 'break-all';
  p.textContent = manifestPath;
  section.appendChild(p);

  content.appendChild(section);
}

function closeDetail() {
  document.getElementById('detail-panel').style.display = 'none';
}
