// graph-engine.js — the rendering/layout engine shared by index.html (the
// real app) and playground.html (the static demo). Extracted so the two
// pages can never drift apart the way they did twice before this file
// existed: every page-specific bit (login/sign-out, scenario switching, the
// "back to site" button) stays in each HTML file's own inline <script>,
// which loads this file first and calls into it.
//
// v1: replaces the old force-directed whole-cluster graph with a
// deterministic, per-namespace ownership tree (see computeTreeLayout) plus
// a namespace-overview grid (see computeOverview) — no physics simulation,
// no requestAnimationFrame loop. Positions are computed once per data load
// or namespace switch, not recomputed every frame.

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

  // ---- namespace overview ----
  // computeOverview buckets nodes by namespace (Node.ns === "" — cluster-scoped
  // resources like Node/PersistentVolume — becomes its own "__cluster" card,
  // same convention the old region grouping used) and rolls up a health
  // summary per bucket so the overview screen can read like ArgoCD's
  // Applications grid: name, resource count, at-a-glance health.
  function computeOverview(nodes, namespaceList) {
    const groups = {};
    (namespaceList || []).forEach(ns => { groups[ns] = groups[ns] || []; });
    nodes.forEach(n => { const g = groupKey(n); (groups[g] = groups[g] || []).push(n); });
    return Object.keys(groups).map(g => {
      const members = groups[g];
      const summary = { ns: g === "__cluster" ? "" : g, cluster: g === "__cluster", count: members.length, ok: 0, warn: 0, crit: 0, unresolved: 0 };
      members.forEach(n => {
        if (n.resolved === false) summary.unresolved++;
        else if (n.status === "crit") summary.crit++;
        else if (n.status === "warn") summary.warn++;
        else if (n.status === "ok") summary.ok++;
      });
      return summary;
    }).sort((a, b) => a.cluster ? 1 : b.cluster ? -1 : a.ns.localeCompare(b.ns));
  }

  // ---- per-namespace ownership tree ----
  // TREE_NODE_GAP_X/TREE_ROOT_GAP_X/TREE_ROW_H are the only "spacing knobs" —
  // deliberately not a full Reingold-Tilford implementation, just post-order
  // subtree-width accumulation + pre-order centering, which is enough to
  // produce a clean, non-overlapping tree for the shapes this data actually
  // has (shallow ownership chains, wide fan-outs handled by collapse, not by
  // wrapping rows — see collapsedSet below).
  const TREE_NODE_W = 160, TREE_NODE_GAP_X = 24, TREE_ROOT_GAP_X = 48, TREE_ROW_H = 110;

  // computeTreeLayout builds one namespace's ownership tree from the FULL
  // edge list (not pre-filtered by the caller) — it does its own namespace
  // filtering so callers can just pass the whole /api/graph payload.
  //   namespace: "" for the cluster-scoped pseudo-namespace, else a real ns.
  //   collapsedSet: a Set of node ids whose subtree should render as a single
  //     collapsed node (see §5 of the plan — this is what keeps a Deployment
  //     with 50 Pods from blowing out the screen, instead of row-wrapping).
  // Returns { nodes, ownershipEdges, secondaryEdges, roots } — nodes carry
  // x/y (relative, origin at the first root's top-left-ish center; caller
  // positions the viewport). Cross-namespace edges are intentionally not
  // returned here — they still show up in the resource drawer's
  // Relationships tab (relHTML operates on the full graph, not the tree).
  function computeTreeLayout(allNodes, allEdges, namespace, collapsedSet) {
    collapsedSet = collapsedSet || new Set();
    const nsNodes = allNodes.filter(n => groupKey(n) === (namespace || "__cluster"));
    const byId = {}; nsNodes.forEach(n => byId[n.id] = sizeNode({ ...n }));
    const idSet = new Set(nsNodes.map(n => n.id));
    const nsEdges = allEdges.filter(e => idSet.has(e.from) && idSet.has(e.to));

    // Pick one tree parent per node — the first outgoing controlled-by edge,
    // in backend order (stable per load). Extra controlled-by edges (the
    // rare multi-owner case — see internal/graph/owner.go, which doesn't
    // filter on Controller==true) fall through into secondaryEdges below
    // rather than forking the tree.
    const parentOf = {}, chosen = new Set();
    nsNodes.forEach(n => {
      const outs = nsEdges.filter(e => e.from === n.id && e.type === "controlled-by");
      if (outs.length) { parentOf[n.id] = outs[0].to; chosen.add(outs[0].from + "→" + outs[0].to); }
    });

    const childrenOf = {};
    nsNodes.forEach(n => { const p = parentOf[n.id]; if (p) (childrenOf[p] = childrenOf[p] || []).push(n.id); });
    Object.keys(childrenOf).forEach(k => childrenOf[k].sort((a, b) => cmpNode(byId[a], byId[b])));

    let roots = nsNodes.filter(n => !parentOf[n.id]).map(n => n.id);
    // Guard against malformed/cyclic ownership data: anything not reachable
    // from the root set becomes its own root rather than being dropped.
    const reachable = new Set(), stack = roots.slice();
    while (stack.length) { const id = stack.pop(); if (reachable.has(id)) continue; reachable.add(id); (childrenOf[id] || []).forEach(c => stack.push(c)); }
    nsNodes.forEach(n => { if (!reachable.has(n.id)) roots.push(n.id); });
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
    // dependent, see sizeNode) box — using the fixed TREE_NODE_W constant
    // here instead of the real per-node width was the original bug: long
    // resource names render wider than 160px, so fixed-width spacing let
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

    // Prune nodes hidden under a collapsed ancestor from the render set (but
    // they stay reachable conceptually via hiddenCount on the ancestor).
    const visible = new Set();
    function markVisible(id) { visible.add(id); if (!collapsedSet.has(id)) (childrenOf[id] || []).forEach(markVisible); }
    roots.forEach(markVisible);

    const outNodes = nsNodes.map(n => byId[n.id]).filter(n => visible.has(n.id));
    const ownershipEdges = nsEdges.filter(e => e.type === "controlled-by" && chosen.has(e.from + "→" + e.to) && visible.has(e.from) && visible.has(e.to));
    const secondaryEdges = nsEdges.filter(e => !(e.type === "controlled-by" && chosen.has(e.from + "→" + e.to)) && visible.has(e.from) && visible.has(e.to));

    return { nodes: outNodes, ownershipEdges, secondaryEdges, roots, childrenOf };
  }

  global.MantisGraph = {
    SVGNS, el, esc, css, short,
    CAT, catColor, FAMILY, REVERSE, NS_PALETTE_DARK, NS_PALETTE_LIGHT, nsPalette,
    groupKey, cmpNode, sizeNode,
    computeOverview, computeTreeLayout,
    TREE_NODE_W, TREE_NODE_GAP_X, TREE_ROOT_GAP_X, TREE_ROW_H,
  };
})(window);
