/* depscope — D3.js Force-Directed Graph Visualization */
(function () {
  'use strict';

  var scanId = document.getElementById('graph-canvas').dataset.scanId;
  var graphData = null;

  /* =======================================================
     Color & Shape Constants
     ======================================================= */
  var edgeColors = {
    depends_on:      '#666666',
    uses_action:     '#4488FF',
    bundles:         '#FF8800',
    pulls_image:     '#00CCCC',
    downloads:       '#FF0000',
    triggers:        '#8844FF'
  };

  function riskColor(risk) {
    switch (risk) {
      case 'CRITICAL': return '#f85149';
      case 'HIGH':     return '#db6d28';
      case 'MEDIUM':   return '#d29922';
      case 'LOW':      return '#3fb950';
      default:         return '#7d8590';
    }
  }

  function nodeShape(type) {
    var size = 150;
    switch (type) {
      case 'package':         return d3.symbol().type(d3.symbolCircle).size(size)();
      case 'action':          return d3.symbol().type(d3.symbolDiamond).size(size * 1.3)();
      case 'workflow':        return d3.symbol().type(d3.symbolSquare).size(size * 2)();
      case 'script_download': return d3.symbol().type(d3.symbolTriangle).size(size)();
      case 'docker_image':    return hexPath();
      default:                return d3.symbol().type(d3.symbolCircle).size(size)();
    }
  }

  function hexPath() {
    var r = 8;
    var pts = [];
    for (var i = 0; i < 6; i++) {
      var a = Math.PI / 3 * i - Math.PI / 6;
      pts.push([r * Math.cos(a), r * Math.sin(a)]);
    }
    return 'M' + pts.map(function (p) { return p[0] + ',' + p[1]; }).join('L') + 'Z';
  }

  /* =======================================================
     Helpers
     ======================================================= */
  function escapeHtml(str) {
    var div = document.createElement('div');
    div.textContent = str || '';
    return div.innerHTML;
  }

  function escapeAttr(str) {
    return (str || '').replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  /* =======================================================
     Initialization
     ======================================================= */
  async function init() {
    try {
      var resp = await fetch('/api/scan/' + scanId + '/graph');
      if (!resp.ok) throw new Error('Failed to load graph');
      graphData = await resp.json();
      updateStats(graphData.summary);
      updateTypeCounts(graphData);
      createSimulation(graphData);
    } catch (e) {
      var container = document.getElementById('graph-container');
      var msg = document.createElement('p');
      msg.style.cssText = 'text-align:center;padding:40px;color:#7d8590;';
      msg.textContent = 'Failed to load graph data.';
      container.appendChild(msg);
    }
  }

  function updateStats(summary) {
    var el = document.getElementById('graph-stats');
    if (!el || !summary) return;
    var parts = [summary.total_nodes + ' nodes', summary.total_edges + ' edges'];
    var byType = summary.by_type || {};
    if (byType.action)          parts.push(byType.action + ' actions');
    if (byType.workflow)        parts.push(byType.workflow + ' workflows');
    if (byType.docker_image)    parts.push(byType.docker_image + ' docker');
    if (byType.script_download) parts.push(byType.script_download + ' scripts');
    el.textContent = parts.join(' \u00b7 ');
  }

  function updateTypeCounts(data) {
    var counts = {};
    data.nodes.forEach(function (n) {
      counts[n.type] = (counts[n.type] || 0) + 1;
    });
    ['package', 'action', 'workflow', 'docker_image', 'script_download'].forEach(function (t) {
      var el = document.getElementById('count-' + t);
      if (el) el.textContent = '(' + (counts[t] || 0) + ')';
    });
  }

  /* =======================================================
     Force Simulation
     ======================================================= */
  function createSimulation(data) {
    var svg = d3.select('#graph-canvas');
    var container = document.getElementById('graph-container');
    var width = container.clientWidth;
    var height = container.clientHeight;
    svg.attr('width', width).attr('height', height);

    // Clear any previous content
    svg.selectAll('*').remove();

    // Definitions: arrow markers for each edge type
    var defs = svg.append('defs');
    Object.keys(edgeColors).forEach(function (type) {
      defs.append('marker')
        .attr('id', 'arrow-' + type)
        .attr('viewBox', '0 -5 10 10')
        .attr('refX', 18)
        .attr('refY', 0)
        .attr('markerWidth', 6)
        .attr('markerHeight', 6)
        .attr('orient', 'auto')
        .append('path')
          .attr('d', 'M0,-5L10,0L0,5')
          .attr('fill', edgeColors[type]);
    });

    var g = svg.append('g');

    // Zoom behavior
    var zoom = d3.zoom()
      .scaleExtent([0.1, 5])
      .on('zoom', function (e) { g.attr('transform', e.transform); });
    svg.call(zoom);

    // Force simulation
    var simulation = d3.forceSimulation(data.nodes)
      .force('link', d3.forceLink(data.links).id(function (d) { return d.id; }).distance(80))
      .force('charge', d3.forceManyBody().strength(-300))
      .force('center', d3.forceCenter(width / 2, height / 2))
      .force('collision', d3.forceCollide().radius(20));

    // Draw links (edges)
    var link = g.selectAll('.link')
      .data(data.links)
      .enter()
      .append('line')
        .attr('class', function (d) { return 'link link-' + d.type; })
        .attr('stroke', function (d) { return edgeColors[d.type] || '#666'; })
        .attr('stroke-dasharray', function (d) {
          if (d.type === 'bundles' || d.type === 'downloads') return '5,5';
          if (d.type === 'triggers') return '2,4';
          return null;
        })
        .attr('marker-end', function (d) { return 'url(#arrow-' + d.type + ')'; });

    // Draw nodes
    var node = g.selectAll('.node')
      .data(data.nodes)
      .enter()
      .append('path')
        .attr('d', function (d) { return nodeShape(d.type); })
        .attr('fill', function (d) { return riskColor(d.risk); })
        .attr('stroke', '#fff')
        .attr('stroke-width', 0.5)
        .attr('class', 'node')
        .on('click', function (e, d) { e.stopPropagation(); showDetail(d); })
        .on('dblclick', function (e, d) { e.stopPropagation(); zoomToNeighborhood(d); })
        .on('mouseover', function (e, d) { showTooltip(e, d); })
        .on('mouseout', function () { hideTooltip(); })
        .call(drag(simulation));

    // Node labels (only for non-package or important nodes)
    var label = g.selectAll('.node-label')
      .data(data.nodes.filter(function (d) {
        return d.type !== 'package' || d.score < 50;
      }))
      .enter()
      .append('text')
        .attr('class', 'node-label')
        .text(function (d) {
          var name = d.name || '';
          return name.length > 22 ? name.slice(0, 20) + '\u2026' : name;
        })
        .attr('font-size', 8)
        .attr('fill', '#aaa');

    // Tick handler
    simulation.on('tick', function () {
      link
        .attr('x1', function (d) { return d.source.x; })
        .attr('y1', function (d) { return d.source.y; })
        .attr('x2', function (d) { return d.target.x; })
        .attr('y2', function (d) { return d.target.y; });

      node.attr('transform', function (d) { return 'translate(' + d.x + ',' + d.y + ')'; });

      label
        .attr('x', function (d) { return d.x + 15; })
        .attr('y', function (d) { return d.y + 4; });
    });

    // Store references for filtering and controls
    window._graphRefs = {
      svg: svg,
      g: g,
      zoom: zoom,
      link: link,
      node: node,
      label: label,
      simulation: simulation,
      data: data
    };

    // Click on background clears selection
    svg.on('click', function () {
      clearHighlight();
      closeDetailPanel();
    });
  }

  /* =======================================================
     Drag Behavior
     ======================================================= */
  function drag(sim) {
    return d3.drag()
      .on('start', function (e, d) {
        if (!e.active) sim.alphaTarget(0.3).restart();
        d.fx = d.x;
        d.fy = d.y;
      })
      .on('drag', function (e, d) {
        d.fx = e.x;
        d.fy = e.y;
      })
      .on('end', function (e, d) {
        if (!e.active) sim.alphaTarget(0);
        d.fx = null;
        d.fy = null;
      });
  }

  /* =======================================================
     Tooltips
     ======================================================= */
  function showTooltip(event, d) {
    var tip = document.getElementById('tooltip');
    // Build tooltip with safe DOM methods
    while (tip.firstChild) tip.removeChild(tip.firstChild);

    var strong = document.createElement('strong');
    strong.textContent = d.name;
    tip.appendChild(strong);

    var info = document.createTextNode('Score: ' + d.score + ' \u00b7 ' + d.risk);
    tip.appendChild(info);
    tip.appendChild(document.createElement('br'));

    var typeInfo = document.createTextNode('Type: ' + d.type);
    tip.appendChild(typeInfo);

    if (d.version) {
      tip.appendChild(document.createElement('br'));
      var verInfo = document.createTextNode('Version: ' + d.version);
      tip.appendChild(verInfo);
    }

    tip.style.left = (event.clientX + 12) + 'px';
    tip.style.top = (event.clientY - 10) + 'px';
    tip.style.display = 'block';
  }

  function hideTooltip() {
    var tip = document.getElementById('tooltip');
    if (tip) tip.style.display = 'none';
  }

  /* =======================================================
     Detail Panel
     ======================================================= */
  async function showDetail(d) {
    var panel = document.getElementById('detail-panel');
    var content = document.getElementById('panel-content');

    // Show loading state using safe DOM methods
    while (content.firstChild) content.removeChild(content.firstChild);
    var loadingP = document.createElement('p');
    loadingP.className = 'panel-loading';
    loadingP.textContent = 'Loading...';
    content.appendChild(loadingP);
    panel.classList.remove('hidden');

    highlightConnected(d);

    try {
      var resp = await fetch('/api/scan/' + scanId + '/graph/node/' + encodeURIComponent(d.id));
      if (!resp.ok) throw new Error('not found');
      var data = await resp.json();
      renderNodeDetailDOM(content, data);
    } catch (e) {
      renderBasicDetailDOM(content, d);
    }
  }

  function renderNodeDetailDOM(container, data) {
    while (container.firstChild) container.removeChild(container.firstChild);

    // Title
    var h2 = document.createElement('h2');
    h2.textContent = data.name;
    container.appendChild(h2);

    // Meta
    var meta = document.createElement('p');
    meta.className = 'panel-meta';
    meta.textContent = data.type + (data.version ? ' \u00b7 ' + data.version : '');
    container.appendChild(meta);

    // Score + Risk
    var scoreSection = document.createElement('div');
    scoreSection.className = 'panel-section';
    var scoreRow = document.createElement('div');
    scoreRow.className = 'panel-score-row';
    var scoreLabel = document.createElement('span');
    scoreLabel.textContent = 'Score: ' + data.score;
    var scoreBadge = document.createElement('span');
    scoreBadge.className = 'badge risk-' + data.risk.toLowerCase();
    scoreBadge.textContent = data.risk;
    scoreRow.appendChild(scoreLabel);
    scoreRow.appendChild(scoreBadge);
    scoreSection.appendChild(scoreRow);
    container.appendChild(scoreSection);

    // Info rows
    var infoSection = document.createElement('div');
    infoSection.className = 'panel-section';
    if (data.pinning && data.pinning !== 'n/a') {
      infoSection.appendChild(createPanelRow('Pinning', data.pinning));
    }
    infoSection.appendChild(createPanelRow('ID', data.id));
    container.appendChild(infoSection);

    // Metadata
    if (data.metadata && Object.keys(data.metadata).length > 0) {
      var metaH3 = document.createElement('h3');
      metaH3.textContent = 'Metadata';
      container.appendChild(metaH3);
      var metaSection = document.createElement('div');
      metaSection.className = 'panel-section';
      Object.keys(data.metadata).forEach(function (key) {
        var val = data.metadata[key];
        if (val !== null && val !== undefined && val !== '') {
          metaSection.appendChild(createPanelRow(key, String(val)));
        }
      });
      container.appendChild(metaSection);
    }

    // Outgoing edges
    if (data.outgoing_edges && data.outgoing_edges.length > 0) {
      var outH3 = document.createElement('h3');
      outH3.textContent = 'Outgoing (' + data.outgoing_edges.length + ')';
      container.appendChild(outH3);
      var outList = document.createElement('ul');
      outList.className = 'edge-list';
      data.outgoing_edges.forEach(function (e) {
        var li = document.createElement('li');
        li.className = 'edge-item';
        li.dataset.nodeId = e.to;
        li.textContent = e.to;
        var typeSpan = document.createElement('span');
        typeSpan.className = 'edge-type';
        typeSpan.textContent = e.type;
        li.appendChild(typeSpan);
        li.addEventListener('click', function () {
          var found = window._graphRefs && window._graphRefs.data.nodes.find(function (n) { return n.id === e.to; });
          if (found) showDetail(found);
        });
        outList.appendChild(li);
      });
      container.appendChild(outList);
    }

    // Incoming edges
    if (data.incoming_edges && data.incoming_edges.length > 0) {
      var inH3 = document.createElement('h3');
      inH3.textContent = 'Incoming (' + data.incoming_edges.length + ')';
      container.appendChild(inH3);
      var inList = document.createElement('ul');
      inList.className = 'edge-list';
      data.incoming_edges.forEach(function (e) {
        var li = document.createElement('li');
        li.className = 'edge-item';
        li.dataset.nodeId = e.from;
        li.textContent = e.from;
        var typeSpan = document.createElement('span');
        typeSpan.className = 'edge-type';
        typeSpan.textContent = e.type;
        li.appendChild(typeSpan);
        li.addEventListener('click', function () {
          var found = window._graphRefs && window._graphRefs.data.nodes.find(function (n) { return n.id === e.from; });
          if (found) showDetail(found);
        });
        inList.appendChild(li);
      });
      container.appendChild(inList);
    }
  }

  function renderBasicDetailDOM(container, d) {
    while (container.firstChild) container.removeChild(container.firstChild);

    var h2 = document.createElement('h2');
    h2.textContent = d.name;
    container.appendChild(h2);

    var meta = document.createElement('p');
    meta.className = 'panel-meta';
    meta.textContent = d.type + (d.version ? ' \u00b7 ' + d.version : '');
    container.appendChild(meta);

    var scoreSection = document.createElement('div');
    scoreSection.className = 'panel-section';
    var scoreRow = document.createElement('div');
    scoreRow.className = 'panel-score-row';
    var scoreLabel = document.createElement('span');
    scoreLabel.textContent = 'Score: ' + d.score;
    var scoreBadge = document.createElement('span');
    scoreBadge.className = 'badge risk-' + d.risk.toLowerCase();
    scoreBadge.textContent = d.risk;
    scoreRow.appendChild(scoreLabel);
    scoreRow.appendChild(scoreBadge);
    scoreSection.appendChild(scoreRow);
    container.appendChild(scoreSection);
  }

  function createPanelRow(key, value) {
    var div = document.createElement('div');
    div.className = 'panel-row';
    var keySpan = document.createElement('span');
    keySpan.className = 'panel-row-key';
    keySpan.textContent = key;
    var valSpan = document.createElement('span');
    valSpan.className = 'panel-row-value';
    valSpan.textContent = value;
    div.appendChild(keySpan);
    div.appendChild(valSpan);
    return div;
  }

  function closeDetailPanel() {
    var panel = document.getElementById('detail-panel');
    if (panel) panel.classList.add('hidden');
  }

  /* =======================================================
     Highlight Connected Nodes
     ======================================================= */
  function highlightConnected(d) {
    if (!window._graphRefs) return;
    var refs = window._graphRefs;
    var connectedIds = new Set();
    connectedIds.add(d.id);

    refs.data.links.forEach(function (l) {
      var srcId = typeof l.source === 'object' ? l.source.id : l.source;
      var tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      if (srcId === d.id) connectedIds.add(tgtId);
      if (tgtId === d.id) connectedIds.add(srcId);
    });

    refs.node.classed('dimmed', function (n) { return !connectedIds.has(n.id); });
    refs.node.classed('highlighted', function (n) { return n.id === d.id; });

    refs.link.classed('dimmed', function (l) {
      var srcId = typeof l.source === 'object' ? l.source.id : l.source;
      var tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      return srcId !== d.id && tgtId !== d.id;
    });
    refs.link.classed('highlighted', function (l) {
      var srcId = typeof l.source === 'object' ? l.source.id : l.source;
      var tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      return srcId === d.id || tgtId === d.id;
    });
  }

  function clearHighlight() {
    if (!window._graphRefs) return;
    var refs = window._graphRefs;
    refs.node.classed('dimmed', false).classed('highlighted', false);
    refs.link.classed('dimmed', false).classed('highlighted', false);
  }

  /* =======================================================
     Double-Click: Zoom to Neighborhood
     ======================================================= */
  function zoomToNeighborhood(d) {
    if (!window._graphRefs) return;
    var refs = window._graphRefs;
    var container = document.getElementById('graph-container');
    var width = container.clientWidth;
    var height = container.clientHeight;

    // Find 1-hop neighbor bounding box
    var connectedIds = new Set();
    connectedIds.add(d.id);
    refs.data.links.forEach(function (l) {
      var srcId = typeof l.source === 'object' ? l.source.id : l.source;
      var tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      if (srcId === d.id) connectedIds.add(tgtId);
      if (tgtId === d.id) connectedIds.add(srcId);
    });

    var minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
    refs.data.nodes.forEach(function (n) {
      if (connectedIds.has(n.id)) {
        if (n.x < minX) minX = n.x;
        if (n.x > maxX) maxX = n.x;
        if (n.y < minY) minY = n.y;
        if (n.y > maxY) maxY = n.y;
      }
    });

    var padding = 60;
    var dx = maxX - minX + padding * 2;
    var dy = maxY - minY + padding * 2;
    var cx = (minX + maxX) / 2;
    var cy = (minY + maxY) / 2;
    var scale = Math.min(width / dx, height / dy, 3);

    var transform = d3.zoomIdentity
      .translate(width / 2, height / 2)
      .scale(scale)
      .translate(-cx, -cy);

    refs.svg.transition().duration(500).call(refs.zoom.transform, transform);
    highlightConnected(d);
  }

  /* =======================================================
     Sidebar Controls
     ======================================================= */

  // Search
  document.getElementById('node-search').addEventListener('input', function (e) {
    var q = e.target.value.toLowerCase();
    if (!window._graphRefs) return;
    var refs = window._graphRefs;

    refs.node.classed('dimmed', function (d) { return q && !d.name.toLowerCase().includes(q); });
    refs.node.classed('highlighted', function (d) { return q && d.name.toLowerCase().includes(q); });

    if (!q) {
      refs.link.classed('dimmed', false);
    } else {
      refs.link.classed('dimmed', true);
    }
  });

  // Type filters
  document.querySelectorAll('.type-filter').forEach(function (cb) {
    cb.addEventListener('change', function () {
      if (!window._graphRefs) return;
      var visible = new Set();
      document.querySelectorAll('.type-filter:checked').forEach(function (c) {
        visible.add(c.value);
      });
      var refs = window._graphRefs;

      refs.node.style('display', function (d) { return visible.has(d.type) ? null : 'none'; });
      refs.link.style('display', function (d) {
        var srcType = typeof d.source === 'object' ? d.source.type : null;
        var tgtType = typeof d.target === 'object' ? d.target.type : null;
        return (srcType && visible.has(srcType)) && (tgtType && visible.has(tgtType)) ? null : 'none';
      });
      refs.label.style('display', function (d) { return visible.has(d.type) ? null : 'none'; });
    });
  });

  // Risk filter
  document.querySelectorAll('input[name="risk"]').forEach(function (r) {
    r.addEventListener('change', function (e) {
      if (!window._graphRefs) return;
      var level = e.target.value;
      var refs = window._graphRefs;

      refs.node.classed('dimmed', function (d) {
        if (!level) return false;
        if (level === 'CRITICAL') return d.risk !== 'CRITICAL';
        return d.risk !== 'HIGH' && d.risk !== 'CRITICAL';
      });
    });
  });

  /* =======================================================
     Zoom Controls + Sidebar Toggle
     ======================================================= */
  document.getElementById('btn-fit').addEventListener('click', function () {
    if (!window._graphRefs) return;
    var refs = window._graphRefs;
    refs.svg.transition().duration(500).call(refs.zoom.transform, d3.zoomIdentity);
  });

  document.getElementById('btn-zoom-in').addEventListener('click', function () {
    if (!window._graphRefs) return;
    var refs = window._graphRefs;
    refs.svg.transition().duration(300).call(refs.zoom.scaleBy, 1.5);
  });

  document.getElementById('btn-zoom-out').addEventListener('click', function () {
    if (!window._graphRefs) return;
    var refs = window._graphRefs;
    refs.svg.transition().duration(300).call(refs.zoom.scaleBy, 0.67);
  });

  document.getElementById('btn-sidebar-toggle').addEventListener('click', function () {
    document.getElementById('sidebar').classList.toggle('collapsed');
  });

  // Close detail panel
  document.getElementById('close-panel').addEventListener('click', function () {
    closeDetailPanel();
    clearHighlight();
  });

  // Escape key closes panel
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') {
      closeDetailPanel();
      clearHighlight();
    }
  });

  // Handle window resize
  var resizeTimeout;
  window.addEventListener('resize', function () {
    clearTimeout(resizeTimeout);
    resizeTimeout = setTimeout(function () {
      if (!window._graphRefs) return;
      var container = document.getElementById('graph-container');
      var refs = window._graphRefs;
      refs.svg.attr('width', container.clientWidth).attr('height', container.clientHeight);
      refs.simulation.force('center', d3.forceCenter(container.clientWidth / 2, container.clientHeight / 2));
      refs.simulation.alpha(0.3).restart();
    }, 250);
  });

  /* =======================================================
     Blast Radius Tab Toggle
     ======================================================= */
  document.querySelectorAll('.tab-toggle .tab').forEach(function (btn) {
    btn.addEventListener('click', function () {
      document.querySelectorAll('.tab-toggle .tab').forEach(function (b) { b.classList.remove('active'); });
      document.querySelectorAll('.tab-content').forEach(function (c) { c.classList.remove('active'); });
      btn.classList.add('active');
      var tabId = 'tab-' + btn.dataset.tab;
      var tabEl = document.getElementById(tabId);
      if (tabEl) tabEl.classList.add('active');
    });
  });

  /* =======================================================
     Blast Radius: Analyze
     ======================================================= */
  function highlightBlast(affectedNodes, paths) {
    if (!window._graphRefs) return;
    var refs = window._graphRefs;
    var affectedSet = {};
    affectedNodes.forEach(function (id) { affectedSet[id] = true; });

    // Add blast-affected and blast-dimmed classes.
    refs.node.classed('blast-affected', function (d) { return !!affectedSet[d.id]; });
    refs.node.classed('blast-dimmed', function (d) { return !affectedSet[d.id]; });

    // Build path edge set for highlighting.
    var pathEdges = {};
    (paths || []).forEach(function (path) {
      for (var i = 0; i < path.length - 1; i++) {
        pathEdges[path[i] + '>' + path[i + 1]] = true;
      }
    });

    refs.link.classed('blast-path', function (l) {
      var srcId = typeof l.source === 'object' ? l.source.id : l.source;
      var tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      return !!pathEdges[srcId + '>' + tgtId];
    });
    refs.link.classed('blast-dimmed', function (l) {
      var srcId = typeof l.source === 'object' ? l.source.id : l.source;
      var tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      return !affectedSet[srcId] && !affectedSet[tgtId];
    });

    refs.label.classed('blast-dimmed', function (d) { return !affectedSet[d.id]; });
  }

  function clearBlastHighlight() {
    if (!window._graphRefs) return;
    var refs = window._graphRefs;
    refs.node.classed('blast-affected', false).classed('blast-dimmed', false);
    refs.link.classed('blast-path', false).classed('blast-dimmed', false);
    refs.label.classed('blast-dimmed', false);
  }

  function renderBlastResults(container, data) {
    while (container.firstChild) container.removeChild(container.firstChild);

    var summary = document.createElement('div');
    summary.className = 'blast-summary';
    summary.textContent = data.total_affected + ' affected node' + (data.total_affected !== 1 ? 's' : '') +
      ' \u00b7 depth ' + data.blast_depth;
    container.appendChild(summary);

    if (data.affected_nodes && data.affected_nodes.length > 0) {
      var list = document.createElement('ul');
      list.className = 'blast-node-list';
      data.affected_nodes.forEach(function (nodeId) {
        var li = document.createElement('li');
        li.className = 'blast-node-item';
        li.textContent = nodeId;
        li.addEventListener('click', function () {
          var found = window._graphRefs && window._graphRefs.data.nodes.find(function (n) { return n.id === nodeId; });
          if (found) zoomToNeighborhood(found);
        });
        list.appendChild(li);
      });
      container.appendChild(list);
    }

    if (data.paths && data.paths.length > 0) {
      var pathH4 = document.createElement('h4');
      pathH4.textContent = 'Exposure Paths (' + data.paths.length + ')';
      container.appendChild(pathH4);
      var pathList = document.createElement('ul');
      pathList.className = 'blast-path-list';
      data.paths.slice(0, 10).forEach(function (path) {
        var li = document.createElement('li');
        li.textContent = path.join(' \u2192 ');
        pathList.appendChild(li);
      });
      container.appendChild(pathList);
    }
  }

  document.getElementById('btn-blast').addEventListener('click', async function () {
    var activeTab = document.querySelector('.tab-toggle .tab.active');
    var mode = activeTab ? activeTab.dataset.tab : 'cve';
    var body;

    if (mode === 'cve') {
      var cveId = document.getElementById('blast-cve').value.trim();
      if (!cveId) return;
      body = JSON.stringify({ mode: 'cve', cve_id: cveId });
    } else {
      var pkg = document.getElementById('blast-pkg').value.trim();
      var range = document.getElementById('blast-range').value.trim();
      if (!pkg || !range) return;
      body = JSON.stringify({ mode: 'package', 'package': pkg, range: range });
    }

    var resultsEl = document.getElementById('blast-results');
    while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);
    var loading = document.createElement('p');
    loading.className = 'panel-loading';
    loading.textContent = 'Analyzing...';
    resultsEl.appendChild(loading);

    try {
      var resp = await fetch('/api/scan/' + scanId + '/blast-radius', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: body
      });
      var data = await resp.json();
      if (!resp.ok) {
        while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);
        var errP = document.createElement('p');
        errP.className = 'blast-error';
        errP.textContent = data.error || 'Analysis failed';
        resultsEl.appendChild(errP);
        return;
      }

      renderBlastResults(resultsEl, data);
      highlightBlast(data.affected_nodes || [], data.paths || []);
      document.getElementById('btn-blast-reset').style.display = 'inline-block';
    } catch (e) {
      while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);
      var errP2 = document.createElement('p');
      errP2.className = 'blast-error';
      errP2.textContent = 'Network error';
      resultsEl.appendChild(errP2);
    }
  });

  document.getElementById('btn-blast-reset').addEventListener('click', function () {
    clearBlastHighlight();
    var resultsEl = document.getElementById('blast-results');
    while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);
    document.getElementById('btn-blast-reset').style.display = 'none';
  });

  /* =======================================================
     Zero-Day Simulation
     ======================================================= */
  document.getElementById('btn-simulate').addEventListener('click', async function () {
    var pkg = document.getElementById('sim-pkg').value.trim();
    var range = document.getElementById('sim-range').value.trim();
    if (!pkg || !range) return;

    var severity = document.getElementById('sim-severity').value;
    var desc = document.getElementById('sim-desc').value.trim();

    var resultsEl = document.getElementById('sim-results');
    while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);
    var loading = document.createElement('p');
    loading.className = 'panel-loading';
    loading.textContent = 'Simulating...';
    resultsEl.appendChild(loading);

    try {
      var resp = await fetch('/api/scan/' + scanId + '/simulate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ 'package': pkg, range: range, severity: severity, description: desc })
      });
      var data = await resp.json();
      if (!resp.ok) {
        while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);
        var errP = document.createElement('p');
        errP.className = 'blast-error';
        errP.textContent = data.error || 'Simulation failed';
        resultsEl.appendChild(errP);
        return;
      }

      renderBlastResults(resultsEl, data);
      highlightBlast(data.affected_nodes || [], data.paths || []);
      document.getElementById('btn-blast-reset').style.display = 'inline-block';
    } catch (e) {
      while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);
      var errP2 = document.createElement('p');
      errP2.className = 'blast-error';
      errP2.textContent = 'Network error';
      resultsEl.appendChild(errP2);
    }
  });

  /* =======================================================
     Gap Analysis
     ======================================================= */
  var gapColors = {
    unpinned_action:  '#FF8800',
    unpinned_docker:  '#00CCCC',
    no_lockfile:      '#FFD700',
    broad_permissions: '#8844FF',
    script_download:  '#FF0000'
  };

  function highlightGaps(gaps) {
    if (!window._graphRefs) return;
    var refs = window._graphRefs;
    var gapNodeMap = {};
    gaps.forEach(function (g) { gapNodeMap[g.node_id] = g.type; });

    refs.node.classed('gap-node', function (d) { return !!gapNodeMap[d.id]; });
    refs.node.classed('blast-dimmed', function (d) { return !gapNodeMap[d.id]; });
    refs.node.style('stroke', function (d) {
      if (gapNodeMap[d.id]) return gapColors[gapNodeMap[d.id]] || '#FF8800';
      return null;
    });
    refs.node.style('stroke-width', function (d) {
      return gapNodeMap[d.id] ? '3px' : null;
    });
  }

  function clearGapHighlight() {
    if (!window._graphRefs) return;
    var refs = window._graphRefs;
    refs.node.classed('gap-node', false).classed('blast-dimmed', false);
    refs.node.style('stroke', null).style('stroke-width', null);
  }

  document.getElementById('btn-gaps').addEventListener('click', async function () {
    var resultsEl = document.getElementById('gap-results');
    while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);
    var loading = document.createElement('p');
    loading.className = 'panel-loading';
    loading.textContent = 'Analyzing gaps...';
    resultsEl.appendChild(loading);

    try {
      var resp = await fetch('/api/scan/' + scanId + '/gaps');
      var data = await resp.json();
      if (!resp.ok) {
        while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);
        var errP = document.createElement('p');
        errP.className = 'blast-error';
        errP.textContent = data.error || 'Gap analysis failed';
        resultsEl.appendChild(errP);
        return;
      }

      while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);

      // Summary.
      var summary = data.summary || {};
      var summaryDiv = document.createElement('div');
      summaryDiv.className = 'gap-summary';
      var totalGaps = (data.gaps || []).length;
      var summaryText = document.createElement('p');
      summaryText.textContent = totalGaps + ' gap' + (totalGaps !== 1 ? 's' : '') + ' found';
      summaryDiv.appendChild(summaryText);

      Object.keys(summary).forEach(function (type) {
        if (summary[type] > 0) {
          var row = document.createElement('div');
          row.className = 'gap-summary-row';
          var dot = document.createElement('span');
          dot.className = 'gap-dot';
          dot.style.background = gapColors[type] || '#888';
          row.appendChild(dot);
          var label = document.createElement('span');
          label.textContent = type.replace(/_/g, ' ') + ': ' + summary[type];
          row.appendChild(label);
          summaryDiv.appendChild(row);
        }
      });
      resultsEl.appendChild(summaryDiv);

      // Gap list.
      if (data.gaps && data.gaps.length > 0) {
        var list = document.createElement('ul');
        list.className = 'gap-list';
        data.gaps.forEach(function (gap) {
          var li = document.createElement('li');
          li.className = 'gap-item';
          var dot = document.createElement('span');
          dot.className = 'gap-dot';
          dot.style.background = gapColors[gap.type] || '#888';
          li.appendChild(dot);
          var text = document.createElement('span');
          text.textContent = gap.node_id;
          li.appendChild(text);
          var detail = document.createElement('span');
          detail.className = 'gap-detail';
          detail.textContent = gap.detail;
          li.appendChild(detail);
          li.addEventListener('click', function () {
            var found = window._graphRefs && window._graphRefs.data.nodes.find(function (n) { return n.id === gap.node_id; });
            if (found) zoomToNeighborhood(found);
          });
          list.appendChild(li);
        });
        resultsEl.appendChild(list);

        // Add reset button.
        var resetBtn = document.createElement('button');
        resetBtn.className = 'btn-secondary';
        resetBtn.textContent = 'Reset';
        resetBtn.style.marginTop = '8px';
        resetBtn.addEventListener('click', function () {
          clearGapHighlight();
          while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);
        });
        resultsEl.appendChild(resetBtn);

        highlightGaps(data.gaps);
      }
    } catch (e) {
      while (resultsEl.firstChild) resultsEl.removeChild(resultsEl.firstChild);
      var errP2 = document.createElement('p');
      errP2.className = 'blast-error';
      errP2.textContent = 'Network error';
      resultsEl.appendChild(errP2);
    }
  });

  /* =======================================================
     Start
     ======================================================= */
  // Wait for D3 to be available (CDN or fallback)
  function waitForD3(cb, attempts) {
    if (typeof d3 !== 'undefined') {
      cb();
    } else if (attempts > 0) {
      setTimeout(function () { waitForD3(cb, attempts - 1); }, 100);
    } else {
      var container = document.getElementById('graph-container');
      var msg = document.createElement('p');
      msg.style.cssText = 'text-align:center;padding:40px;color:#7d8590;';
      msg.textContent = 'Failed to load D3.js library.';
      container.appendChild(msg);
    }
  }

  waitForD3(init, 30);
})();
