// report-layout.js — lays out the topology diagrams for an Infrastructure
// Report using graph-engine.js, the same deterministic, overlap-free layout
// the Topology page uses. It only computes geometry: mantis-report receives
// IDs and coordinates, checks every resource and relationship against the
// engine's snapshot, and draws the PDF itself. DOM-free, so it is tested in
// Node (test/ui).
(function (root, factory) {
  const api = factory(root.MantisGraph || (typeof require === "function" ? require("./graph-engine.js") : null));
  if (typeof module === "object" && module.exports) module.exports = api;
  root.MantisReportLayout = api;
})(typeof self !== "undefined" ? self : globalThis, function (G) {
  "use strict";

  // A single diagram is used while it stays readable: at most FULL_MAX_CARDS
  // cards and small enough to draw at full size within the PDF page limit
  // (14,400 pt, at 0.75 pt per layout px). Beyond that the report becomes an
  // overview plus one diagram per namespace.
  const FULL_MAX_CARDS = 600;
  const MAX_LAYOUT_PX = (14400 - 200) / 0.75;
  // A namespace with more resources than this keeps large fan-outs folded
  // ("48 Pods") on its diagram; the inventory still lists every one.
  const NAMESPACE_UNFOLD_MAX = 900;
  const RELATIONS = ["ownership", "networking", "configuration", "storage", "security", "autoscaling"];

  function baseState() {
    const st = G.createViewState();
    st.view = "namespace";
    st.relations.namespace = new Set(RELATIONS);
    return st;
  }

  function serialize(P, L, meta) {
    const groups = L.groups.map(g => ({ key: g.key, x0: g.x0, y0: g.y0, x1: g.x1, y1: g.y1, collapsed: !!g.collapsed }));
    const nodes = [];
    let folded = 0;
    P.vnodes.forEach(v => {
      const p = L.pos.get(v.vid);
      if (!p) return;
      const n = { vid: v.vid, type: v.type, group: v.group, x: p.x, y: p.y, w: p.w, h: p.h };
      if (v.type === "resource") {
        n.id = v.vid;
        const badges = (v.badges || []).map(b => (b.kind === "node" ? "on " : b.kind === "ns" ? "ns " : "") + b.text);
        if (badges.length) n.badges = badges;
      } else if (v.type === "aggregate") {
        n.members = v.members.slice();
        folded += v.members.length;
      }
      nodes.push(n);
    });
    const edges = [];
    P.vedges.forEach(e => {
      const pts = L.routes.get(e.key);
      if (!pts || pts.length < 2) return;
      edges.push({ from: e.from, to: e.to, cross: !!e.cross, keys: e.canonical.map(c => c.key), points: pts.map(p => [p.x, p.y]) });
    });
    const b = L.bounds;
    return Object.assign({ groups, nodes, edges, bounds: [b.x0, b.y0, b.x1, b.y1], folded }, meta);
  }

  // build returns { revision, pages } for the store's current graph.
  function build(store) {
    const pages = [];
    // 1. Try one complete diagram.
    const st = baseState();
    st.unfoldAll = true;
    let P = G.project(store, st);
    P.groups.forEach(g => st.expanded.namespace.set(g.key, true));
    P = G.project(store, st);
    let L = G.computeLayout(P);
    const w = L.bounds.x1 - L.bounds.x0, h = L.bounds.y1 - L.bounds.y0;
    if (P.vnodes.size <= FULL_MAX_CARDS && w <= MAX_LAYOUT_PX && h <= MAX_LAYOUT_PX) {
      pages.push(serialize(P, L, { title: "Complete cluster", kind: "full" }));
      return { revision: store.revision, pages, partitioned: false };
    }
    // 2. Overview: every namespace as a card, relationships between them bundled.
    const ov = baseState();
    P = G.project(store, ov);
    P.groups.forEach(g => ov.expanded.namespace.set(g.key, false));
    P = G.project(store, ov);
    L = G.computeLayout(P);
    pages.push(serialize(P, L, { title: "Cluster overview by namespace", kind: "overview" }));
    // 3. One diagram per namespace (with the cluster-scoped resources it uses).
    store.namespaces().forEach(ns => {
      const ids = store.byNs.get(ns) || [];
      if (!ids.length) return;
      const s = baseState();
      s.namespaces = new Set([ns]);
      s.unfoldAll = ids.length <= NAMESPACE_UNFOLD_MAX;
      let Q = G.project(store, s);
      Q.groups.forEach(g => s.expanded.namespace.set(g.key, true));
      Q = G.project(store, s);
      pages.push(serialize(Q, G.computeLayout(Q), { title: "Namespace " + ns, kind: "namespace", namespace: ns }));
    });
    return { revision: store.revision, pages, partitioned: true };
  }

  return { build, FULL_MAX_CARDS };
});
