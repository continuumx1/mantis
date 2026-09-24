// graph-engine.js — the rendering/layout engine shared by index.html (the
// real app) and playground.html (the static demo). Extracted so the two
// pages can never drift apart the way they did twice before this file
// existed: every page-specific bit (login/sign-out, scenario switching, the
// "back to site" button) stays in each HTML file's own inline <script>,
// which loads this file first and calls into it.
//
// v1: replaces the old force-directed whole-cluster graph with a
// deterministic, single-screen layout (see computeClusterLayout) — every
// namespace's resources are still visible together, connected by their real
// relationships, exactly as before; the only thing that changed is *how*
// they're positioned. Each namespace's resources form their own ownership
// tree (real controlled-by edges, e.g. Deployment -> ReplicaSet -> Pod), and
// every namespace's tree is laid out side by side on one shared canvas —
// no physics simulation, no requestAnimationFrame loop, no per-frame
// recomputation. Positions are computed once per data load.

(function (global) {
  "use strict";

  const SVGNS = "http://www.w3.org/2000/svg";
  const el = (t, a) => { const e = document.createElementNS(SVGNS, t); for (const k in a) e.setAttribute(k, a[k]); return e; };
  const esc = s => String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  const css = v => getComputedStyle(document.documentElement).getPropertyValue(v).trim();
  const short = s => s.length > 22 ? s.slice(0, 21) + "…" : s;

  const CAT = { Deployment: "workload", ReplicaSet: "workload", StatefulSet: "workload", DaemonSet: "workload", CronJob: "workload", Job: "workload",
    Pod: "pod", Service: "network", Ingress: "network", ConfigMap: "config", Secret: "config", ResourceQuota: "config", LimitRange: "config",
    PersistentVolumeClaim: "storage", PersistentVolume: "storage", StorageClass: "storage", Node: "node",
    HorizontalPodAutoscaler: "autoscale", VerticalPodAutoscaler: "autoscale", NodePool: "autoscale" };
  const catColor = c => css("--cat-" + c);
  const FAMILY = { "controlled-by": "workload", "runs-on": "node", "selects": "network", "serves": "network", "routes-to": "network",
    "references": "config", "mounts": "config", "claims": "storage", "bound-to": "storage", "storage-class": "storage", "scales": "autoscale" };
  const REVERSE = { "controlled-by": "controls", "runs-on": "hosts", "selects": "selected-by", "serves": "served-by", "routes-to": "routed-from",
    "references": "used-by", "mounts": "mounted-by", "claims": "claimed-by", "bound-to": "bound-from", "storage-class": "class-of", "scales": "scaled-by" };
  const NS_PALETTE_DARK = ["#a6e22e", "#6ea8fe", "#d980fb", "#e3b341", "#34d1bf", "#f0883e", "#db61a2", "#7ee787", "#a371f7", "#4ec9b0"];
  const NS_PALETTE_LIGHT = ["#4f8a1c", "#2f6fe0", "#a23fd1", "#a06400", "#0a8f7c", "#b85c17", "#c23f82", "#1f8f4e", "#7a4fd1", "#0f8f82"];
  const nsPalette = () => document.documentElement.getAttribute("data-theme") === "light" ? NS_PALETTE_LIGHT : NS_PALETTE_DARK;

  const groupKey = n => n.ns || "__cluster";
  const cmpNode = (a, b) => a.kind.localeCompare(b.kind) || a.name.localeCompare(b.name);

  // sizeNode gives every node its box dimensions — same formula the old
  // physics view used, kept as-is since it's unrelated to layout strategy.
  function sizeNode(n) {
    n.w = Math.min(210, Math.max(104, short(n.name).length * 7 + 48));
    n.h = n.resolved === false ? 46 : 40;
    return n;
  }

  // ---- one namespace's ownership tree (internal — see computeClusterLayout,
  // the public entry point, below) ----
  // TREE_NODE_GAP_X/TREE_ROOT_GAP_X/TREE_ROW_H are the only "spacing knobs" —
  // deliberately not a full Reingold-Tilford implementation, just post-order
  // subtree-width accumulation + pre-order centering, which is enough to
  // produce a clean, non-overlapping tree for the shapes this data actually
  // has (shallow ownership chains, wide fan-outs handled by collapse).
  const TREE_NODE_GAP_X = 24, TREE_ROOT_GAP_X = 48, TREE_ROW_H = 110;
  const EMPTY_GROUP_W = 168, EMPTY_GROUP_H = 74; // placeholder box for a namespace with zero resources

  // layoutGroup lays out ONE namespace's nodes (already filtered by the
  // caller) as an ownership tree, local coordinates starting near (0,0).
  // `groupEdges` must already be filtered to edges whose endpoints are both
  // in `groupNodes` — computeClusterLayout does that once per group.
  // Returns { nodes, ownershipEdges, childrenOf, minX, minY, maxX, maxY }.
  function layoutGroup(groupNodes, groupEdges, collapsedSet) {
    const byId = {}; groupNodes.forEach(n => byId[n.id] = sizeNode({ ...n }));

    if (!groupNodes.length) {
      return { nodes: [], ownershipEdges: [], childrenOf: {}, minX: 0, minY: 0, maxX: EMPTY_GROUP_W, maxY: EMPTY_GROUP_H };
    }

    // Pick one tree parent per node — the first outgoing controlled-by edge,
    // in backend order (stable per load). Extra controlled-by edges (the
    // rare multi-owner case — see internal/graph/owner.go, which doesn't
    // filter on Controller==true) fall through and are just left as
    // ordinary secondary edges by the caller, rather than forking the tree.
    const parentOf = {}, chosen = new Set();
    groupNodes.forEach(n => {
      const outs = groupEdges.filter(e => e.from === n.id && e.type === "controlled-by");
      if (outs.length) { parentOf[n.id] = outs[0].to; chosen.add(outs[0].from + "→" + outs[0].to); }
    });

    const childrenOf = {};
    groupNodes.forEach(n => { const p = parentOf[n.id]; if (p) (childrenOf[p] = childrenOf[p] || []).push(n.id); });
    Object.keys(childrenOf).forEach(k => childrenOf[k].sort((a, b) => cmpNode(byId[a], byId[b])));

    let roots = groupNodes.filter(n => !parentOf[n.id]).map(n => n.id);
    // Guard against malformed/cyclic ownership data: anything not reachable
    // from the root set becomes its own root rather than being dropped.
    const reachable = new Set(), stack = roots.slice();
    while (stack.length) { const id = stack.pop(); if (reachable.has(id)) continue; reachable.add(id); (childrenOf[id] || []).forEach(c => stack.push(c)); }
    groupNodes.forEach(n => { if (!reachable.has(n.id)) roots.push(n.id); });
    roots = Array.from(new Set(roots)).sort((a, b) => cmpNode(byId[a], byId[b]));

    const depth = {};
    roots.forEach(r => depth[r] = 0);
    let frontier = roots.slice();
    while (frontier.length) {
      const next = [];
      frontier.forEach(id => (childrenOf[id] || []).forEach(c => { if (depth[c] === undefined) { depth[c] = depth[id] + 1; next.push(c); } }));
      frontier = next;
    }

    function countDescendants(id) {
      let n = 0; const st = (childrenOf[id] || []).slice();
      while (st.length) { const c = st.pop(); n++; st.push(...(childrenOf[c] || [])); }
      return n;
    }
    // Each subtree must be at least as wide as the node's own (name-length-
    // dependent, see sizeNode) box — a fixed constant here was the original
    // bug: long resource names render wider than any fixed guess, which let
    // adjacent leaf siblings' actual boxes overlap.
    function subtreeWidth(id) {
      const own = byId[id].w;
      if (collapsedSet.has(id)) return own;
      const kids = childrenOf[id] || [];
      if (!kids.length) return own;
      let w = 0;
      kids.forEach((k, i) => { w += (i > 0 ? TREE_NODE_GAP_X : 0) + subtreeWidth(k); });
      return Math.max(own, w);
    }
    function assignX(id, left) {
      const node = byId[id], w = subtreeWidth(id);
      node.x = left + w / 2; node.y = depth[id] * TREE_ROW_H;
      if (collapsedSet.has(id)) { node.collapsed = true; node.hiddenCount = countDescendants(id); return; }
      node.collapsed = false; node.hiddenCount = 0;
      let cursor = left;
      (childrenOf[id] || []).forEach(k => { assignX(k, cursor); cursor += subtreeWidth(k) + TREE_NODE_GAP_X; });
    }
    let cursor = 0;
    roots.forEach((r, i) => { if (i > 0) cursor += TREE_ROOT_GAP_X; assignX(r, cursor); cursor += subtreeWidth(r); });

    // Prune nodes hidden under a collapsed ancestor from the render set.
    const visible = new Set();
    function markVisible(id) { visible.add(id); if (!collapsedSet.has(id)) (childrenOf[id] || []).forEach(markVisible); }
    roots.forEach(markVisible);

    const outNodes = groupNodes.map(n => byId[n.id]).filter(n => visible.has(n.id));
    const ownershipEdges = groupEdges.filter(e => e.type === "controlled-by" && chosen.has(e.from + "→" + e.to) && visible.has(e.from) && visible.has(e.to));

    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    outNodes.forEach(n => { minX = Math.min(minX, n.x - n.w / 2); maxX = Math.max(maxX, n.x + n.w / 2); minY = Math.min(minY, n.y - n.h / 2); maxY = Math.max(maxY, n.y + n.h / 2); });

    return { nodes: outNodes, ownershipEdges, childrenOf, minX, minY, maxX, maxY };
  }

  // ---- whole-cluster layout: every namespace visible together, connected ----
  // Namespace "regions" are packed shelf-style (left to right, wrapping to a
  // new row once a row gets too wide) rather than a fixed grid — trees vary
  // hugely in size (kube-system next to a three-resource namespace), so a
  // fixed cell size would either waste space or overlap. REGION_MAX_ROW_W is
  // a soft budget, not a hard viewport limit — the canvas pans/zooms.
  const REGION_GAP_X = 56, REGION_GAP_Y = 64, REGION_PAD = 24, REGION_MAX_ROW_W = 2400;

  // computeClusterLayout is the single public layout entry point: every
  // namespace's ownership tree, side by side on one shared canvas, with
  // every other relationship (selects/serves/mounts/references/... and any
  // rare cross-namespace or multi-owner controlled-by edge) drawn as a
  // secondary connector between the two now-fixed positions — including
  // connectors that cross from one namespace's region into another's, since
  // everything lives in the same coordinate space now.
  //   collapsedIds: a Set of node ids anywhere in the cluster whose subtree
  //     should render collapsed (§5 of the plan — keeps a Deployment with 50
  //     Pods from dominating the screen). Persists across a data refresh;
  //     the caller resets it only when it chooses to (e.g. "Expand all").
  function computeClusterLayout(allNodes, allEdges, namespaceList, collapsedIds) {
    collapsedIds = collapsedIds || new Set();
    const groups = {};
    (namespaceList || []).forEach(ns => { groups[ns] = groups[ns] || []; });
    allNodes.forEach(n => { const g = groupKey(n); (groups[g] = groups[g] || []).push(n); });

    const groupKeys = Object.keys(groups).sort((a, b) => {
      if (a === "__cluster") return 1; if (b === "__cluster") return -1;
      return a.localeCompare(b);
    });

    const globalById = {};
    const regions = [];
    const ownershipEdges = [];
    let cursorX = 0, cursorY = 0, rowH = 0;

    groupKeys.forEach(g => {
      const groupNodes = groups[g];
      const idSet = new Set(groupNodes.map(n => n.id));
      const groupEdges = allEdges.filter(e => idSet.has(e.from) && idSet.has(e.to));
      const local = layoutGroup(groupNodes, groupEdges, collapsedIds);
      const w = Math.max(local.maxX - local.minX, EMPTY_GROUP_W);
      const h = Math.max(local.maxY - local.minY, EMPTY_GROUP_H);

      if (cursorX > 0 && cursorX + w > REGION_MAX_ROW_W) { cursorX = 0; cursorY += rowH + REGION_GAP_Y; rowH = 0; }
      const ox = cursorX - (isFinite(local.minX) ? local.minX : 0), oy = cursorY - (isFinite(local.minY) ? local.minY : 0);
      local.nodes.forEach(n => { n.x += ox; n.y += oy; globalById[n.id] = n; });
      local.ownershipEdges.forEach(e => ownershipEdges.push(e));

      regions.push({
        key: g, label: g === "__cluster" ? "cluster-scoped · not namespaced" : g, cluster: g === "__cluster",
        empty: local.nodes.length === 0,
        x0: cursorX - REGION_PAD, y0: cursorY - REGION_PAD, x1: cursorX + w + REGION_PAD, y1: cursorY + h + REGION_PAD,
      });

      cursorX += w + REGION_GAP_X; rowH = Math.max(rowH, h);
    });

    // Every relation NOT already drawn as an ownership-tree edge — including
    // genuinely cross-namespace edges (a shared PVC, a cross-namespace
    // NetworkPolicy target, etc.) and any rare second controlled-by edge on
    // a multi-owner node — becomes a secondary connector, drawn between
    // whatever two global positions its endpoints ended up at.
    const usedOwnershipKeys = new Set(ownershipEdges.map(e => e.from + "→" + e.to));
    const secondaryEdges = allEdges.filter(e =>
      globalById[e.from] && globalById[e.to] &&
      !(e.type === "controlled-by" && usedOwnershipKeys.has(e.from + "→" + e.to)));

    return { nodes: Object.values(globalById), ownershipEdges, secondaryEdges, regions };
  }

  global.MantisGraph = {
    SVGNS, el, esc, css, short,
    CAT, catColor, FAMILY, REVERSE, NS_PALETTE_DARK, NS_PALETTE_LIGHT, nsPalette,
    groupKey, cmpNode, sizeNode,
    computeClusterLayout,
  };
})(window);
