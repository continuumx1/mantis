// graph-engine.js — the DOM-free core of the Mantis graph UI, shared by
// index.html (the real app), playground.html (the static demo) and the Node
// test suite (test/ui). It has three layers, each a pure function of the one
// before it:
//
//   1. Store       — the canonical resource graph exactly as mantis-engine
//                    sent it: one node per Kubernetes resource, typed edges,
//                    in/out/namespace indexes. Updated by full snapshots or by
//                    revision deltas. Never filtered, never rearranged.
//   2. Projection  — a *view* of the store: which resources are drawn, inside
//                    which visual group (namespace, worker node, workload),
//                    which are folded into aggregates ("12 Pods"), and which
//                    canonical edges are bundled into one drawn edge. Four
//                    views (namespace, node, workload, dependency) project the
//                    same store; switching never touches canonical data.
//   3. Layout      — deterministic geometry for a projection: a layered
//                    (Sugiyama-style) layout inside each group, groups packed
//                    in shelves, and cross-group edges routed orthogonally
//                    through the empty gutters between groups so they never
//                    pass through a resource card. There is no physics, no
//                    randomness and no animation loop: the same input always
//                    gives the same picture, and a group's interior is cached
//                    by content so unrelated changes never move it.
//
// Everything Kubernetes-semantic (which resources relate, and how) is decided
// by the backend. This file only groups, folds and positions what it is given.

(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  root.MantisGraph = api;
})(typeof self !== "undefined" ? self : globalThis, function () {
  "use strict";

  // ---------------------------------------------------------------------
  // Taxonomy
  // ---------------------------------------------------------------------

  // Visual category of a resource kind (drives its accent color).
  const CAT = {
    Deployment: "workload", ReplicaSet: "workload", StatefulSet: "workload", DaemonSet: "workload", CronJob: "workload", Job: "workload",
    Pod: "pod",
    Service: "network", Ingress: "network", NetworkPolicy: "network", EndpointSlice: "network",
    ConfigMap: "config", Secret: "config", ResourceQuota: "config", LimitRange: "config",
    PersistentVolumeClaim: "storage", PersistentVolume: "storage", StorageClass: "storage",
    Node: "node",
    HorizontalPodAutoscaler: "autoscale", VerticalPodAutoscaler: "autoscale", NodePool: "autoscale",
    ServiceAccount: "security", Role: "security", RoleBinding: "security", ClusterRole: "security", ClusterRoleBinding: "security",
  };
  const catOf = kind => CAT[kind] || "other";

  // Relationship categories, in legend/toolbar order.
  const CATEGORIES = ["ownership", "networking", "configuration", "storage", "scheduling", "security", "autoscaling"];
  const CATEGORY_LABEL = {
    ownership: "Ownership", networking: "Networking", configuration: "Configuration", storage: "Storage",
    scheduling: "Scheduling", security: "RBAC", autoscaling: "Autoscaling",
  };
  // Fallbacks for graphs from engines older than typed edges (e.g. the
  // captured playground fixtures): category and basis by relation type.
  const TYPE_CATEGORY = {
    "controlled-by": "ownership", "runs-on": "scheduling",
    "selects": "networking", "serves": "networking", "routes-to": "networking", "applies-to": "networking",
    "references": "configuration", "mounts": "configuration", "uses-service-account": "configuration",
    "claims": "storage", "bound-to": "storage", "storage-class": "storage",
    "binds-role": "security", "grants-to": "security", "scales": "autoscaling",
  };
  const TYPE_BASIS = {
    "controlled-by": "ownership", "selects": "selector", "applies-to": "selector", "serves": "endpoint",
  };
  const BASIS_LABEL = {
    ownership: "explicit ownership (ownerReference)",
    reference: "declared reference in the spec",
    selector: "label-selector match",
    endpoint: "EndpointSlice record (control-plane state, not observed traffic)",
    inferred: "inferred — not verified",
  };
  // Inverse verb, for "used by" lists.
  const REVERSE = {
    "controlled-by": "controls", "runs-on": "hosts", "selects": "selected by", "serves": "served by", "routes-to": "routed from",
    "references": "referenced by", "mounts": "mounted by", "claims": "claimed by", "bound-to": "bound from", "storage-class": "class of",
    "scales": "scaled by", "uses-service-account": "identity of", "binds-role": "bound by", "grants-to": "granted by", "applies-to": "governed by",
  };

  // Column-ish ordering of kinds, used for deterministic sorting everywhere.
  const KIND_ORDER = [
    "Ingress", "Service", "HorizontalPodAutoscaler", "VerticalPodAutoscaler", "CronJob", "Deployment", "StatefulSet", "DaemonSet",
    "Job", "ReplicaSet", "Pod", "NetworkPolicy", "ServiceAccount", "ConfigMap", "Secret", "PersistentVolumeClaim",
    "RoleBinding", "Role", "ResourceQuota", "LimitRange",
    "Node", "NodePool", "PersistentVolume", "StorageClass", "ClusterRoleBinding", "ClusterRole",
  ];
  const kindRank = k => { const i = KIND_ORDER.indexOf(k); return i < 0 ? KIND_ORDER.length : i; };
  const cmpStr = (a, b) => (a < b ? -1 : a > b ? 1 : 0);
  const cmpNode = (a, b) => kindRank(a.kind) - kindRank(b.kind) || cmpStr(a.name, b.name) || cmpStr(a.id, b.id);

  const ROOT_WORKLOAD = { Deployment: 1, StatefulSet: 1, DaemonSet: 1, CronJob: 1 };
  // Kinds that are leaves in a group (only ever referenced) and get folded
  // into one "N ConfigMaps" aggregate when a group has many of them.
  const FOLDABLE_KINDS = { ConfigMap: 1, Secret: 1, ServiceAccount: 1, Role: 1, RoleBinding: 1, NetworkPolicy: 1, PersistentVolumeClaim: 1,
    PersistentVolume: 1, ClusterRole: 1, ClusterRoleBinding: 1, Node: 1, ResourceQuota: 1, LimitRange: 1, Job: 1, ReplicaSet: 1,
    Service: 1, Ingress: 1, HorizontalPodAutoscaler: 1, StorageClass: 1 };

  // Progressive-disclosure thresholds. Deliberately small: the point is that
  // a first look at a big cluster is a readable summary, and detail is one
  // click away rather than drawn all at once.
  const LIMITS = {
    autoExpandResources: 160, // a cluster this small opens fully expanded
    podFanout: 5,             // more Pods than this under one owner fold into "N Pods"
    kindFanout: 16,           // more leaves of one kind than this in a group fold
    nodeGroupPods: 36,        // a worker node with more Pods folds them per namespace
    autoExpandGroups: 30,     // workload/node views expand every group up to this many
    hubFanout: 12,            // dependency view: more same-kind neighbours fold
    hubEdges: 8,              // a card with more lines than this of one category summarizes them
  };

  // ---------------------------------------------------------------------
  // 1. Store — the canonical graph
  // ---------------------------------------------------------------------

  function normNode(n) {
    const node = Object.assign({}, n);
    node.ns = node.ns || "";
    node.scope = node.scope || (node.ns ? "Namespaced" : "Cluster");
    node.cat = catOf(node.kind);
    if (node.resolved === undefined) node.resolved = true;
    return node;
  }
  function normEdge(e) {
    const edge = Object.assign({}, e);
    edge.category = edge.category || TYPE_CATEGORY[edge.type] || "configuration";
    edge.basis = edge.basis || (edge.inferred ? "inferred" : TYPE_BASIS[edge.type] || "reference");
    edge.key = edge.from + "|" + edge.type + "|" + edge.to;
    return edge;
  }

  function createStore() {
    const s = {
      revision: 0, meta: {}, nodes: new Map(), edges: new Map(),
      out: new Map(), in: new Map(), byNs: new Map(), version: 0,
    };
    function index() {
      s.out = new Map(); s.in = new Map(); s.byNs = new Map();
      for (const n of s.nodes.values()) {
        if (!s.byNs.has(n.ns)) s.byNs.set(n.ns, []);
        s.byNs.get(n.ns).push(n.id);
      }
      for (const e of s.edges.values()) {
        if (!s.out.has(e.from)) s.out.set(e.from, []);
        if (!s.in.has(e.to)) s.in.set(e.to, []);
        s.out.get(e.from).push(e);
        s.in.get(e.to).push(e);
      }
      s.version++;
    }
    // load replaces the store with a full snapshot (GraphDTO).
    s.load = function (dto) {
      s.meta = dto.meta || {};
      s.revision = s.meta.revision || 0;
      s.nodes = new Map();
      s.edges = new Map();
      (dto.nodes || []).forEach(n => { const x = normNode(n); s.nodes.set(x.id, x); });
      (dto.edges || []).forEach(e => { const x = normEdge(e); s.edges.set(x.key, x); });
      index();
      return { full: true, changedIds: null };
    };
    // apply accepts either a full snapshot or a /api/graph?since= response,
    // and reports which resource IDs changed.
    s.apply = function (payload) {
      if (!payload) return { full: false, changedIds: new Set() };
      if (payload.nodes) return s.load(payload);
      if (payload.meta) s.meta = payload.meta;
      if (payload.revision) s.revision = payload.revision;
      const changed = new Set();
      const d = payload.delta;
      if (!d || payload.unchanged) return { full: false, changedIds: changed };
      (d.removeNodes || []).forEach(id => { s.nodes.delete(id); changed.add(id); });
      (d.upsertNodes || []).forEach(n => { const x = normNode(n); s.nodes.set(x.id, x); changed.add(x.id); });
      (d.removeEdges || []).forEach(e => { const x = normEdge(e); s.edges.delete(x.key); changed.add(x.from); changed.add(x.to); });
      (d.addEdges || []).forEach(e => { const x = normEdge(e); s.edges.set(x.key, x); changed.add(x.from); changed.add(x.to); });
      index();
      return { full: false, changedIds: changed };
    };
    s.node = id => s.nodes.get(id);
    s.outEdges = id => s.out.get(id) || [];
    s.inEdges = id => s.in.get(id) || [];
    // ownerOf: the controlled-by target (first by id for the rare
    // multi-owner object, so the choice is stable).
    s.ownerOf = function (id) {
      const outs = s.outEdges(id).filter(e => e.type === "controlled-by");
      if (!outs.length) return null;
      outs.sort((a, b) => cmpStr(a.to, b.to));
      return outs[0].to;
    };
    // rootOf climbs ownership to the top-level controller.
    s.rootOf = function (id) {
      let cur = id; const seen = new Set();
      for (;;) {
        seen.add(cur);
        const o = s.ownerOf(cur);
        if (!o || seen.has(o) || !s.nodes.has(o)) return cur;
        cur = o;
      }
    };
    s.nodeOfPod = function (id) {
      const e = s.outEdges(id).find(x => x.type === "runs-on");
      return e ? e.to : null;
    };
    s.namespaces = function () {
      const set = new Set(s.meta.namespaceList || []);
      for (const n of s.nodes.values()) if (n.ns) set.add(n.ns);
      return Array.from(set).sort(cmpStr);
    };
    return s;
  }

  // ---------------------------------------------------------------------
  // Search (local index; same syntax as the engine's /api/graph/search)
  // ---------------------------------------------------------------------

  function search(store, q, limit) {
    limit = limit || 200;
    let kind = "", ns = "", labelK = "", labelV = null, hasLabel = false;
    const words = [];
    String(q || "").trim().split(/\s+/).filter(Boolean).forEach(t => {
      const low = t.toLowerCase();
      if (low.startsWith("kind:")) kind = low.slice(5);
      else if (low.startsWith("ns:")) ns = t.slice(3);
      else if (low.startsWith("label:")) { hasLabel = true; const kv = t.slice(6); const i = kv.indexOf("="); if (i >= 0) { labelK = kv.slice(0, i); labelV = kv.slice(i + 1); } else labelK = kv; }
      else words.push(low);
    });
    if (!kind && !ns && !hasLabel && !words.length) return { total: 0, results: [] };
    const hits = [];
    const pool = ns ? (store.byNs.get(ns) || []).map(id => store.nodes.get(id)) : store.nodes.values();
    for (const n of pool) {
      if (kind && n.kind.toLowerCase() !== kind) continue;
      if (ns && n.ns !== ns) continue;
      if (hasLabel) { const v = n.labels && n.labels[labelK]; if (v === undefined || (labelV !== null && v !== labelV)) continue; }
      let score = 0, ok = true;
      const name = n.name.toLowerCase();
      for (const w of words) {
        if (name === w) score += 0;
        else if (name.startsWith(w)) score += 1;
        else if (name.includes(w)) score += 2;
        else if (n.kind.toLowerCase() === w) score += 3;
        else if (n.ns.toLowerCase().includes(w)) score += 4;
        else { ok = false; break; }
      }
      if (ok) hits.push({ n, score });
    }
    hits.sort((a, b) => a.score - b.score || cmpStr(a.n.name, b.n.name) || cmpStr(a.n.id, b.n.id));
    return { total: hits.length, results: hits.slice(0, limit).map(h => h.n) };
  }

  // ---------------------------------------------------------------------
  // 2. Projections
  // ---------------------------------------------------------------------

  const VIEWS = ["namespace", "node", "workload", "dependency"];
  const DEFAULT_RELATIONS = {
    namespace: ["ownership", "networking", "configuration", "storage", "autoscaling"],
    node: ["ownership"],
    workload: ["ownership", "networking", "configuration", "storage", "autoscaling"],
    dependency: CATEGORIES.slice(),
  };

  function createViewState() {
    const relations = {};
    VIEWS.forEach(v => { relations[v] = new Set(DEFAULT_RELATIONS[v]); });
    return {
      view: "namespace",
      relations,
      // expanded[view]: Map groupKey -> bool, the user's overrides of the
      // view's default (which depends on cluster size).
      expanded: { namespace: new Map(), node: new Map(), workload: new Map(), dependency: new Map() },
      unfolded: new Set(),  // aggregate keys the user opened
      hiddenKinds: new Set(),
      namespaces: null,     // null = all; else a Set of namespaces to show
      focus: null,          // dependency-view subject
      direction: "both",    // dependency view: in | out | both
      depth: 2,
    };
  }

  // project builds a view of the store. Output:
  //   groups: [{ key, type, label, sub, collapsed, summary, ns }]
  //   vnodes: Map vid -> { vid, group, type: resource|aggregate|groupcard }
  //   vedges: [{ key, from, to, category, types, count, canonical, cross }]
  //   member: Map canonical id -> { group, vid } (where each resource went)
  //   stats:  { total, rendered, hidden }
  function project(store, st) {
    const view = st.view;
    const rel = st.relations[view] || new Set(CATEGORIES);
    const ctx = { store, st, view, rel, groups: new Map(), member: new Map(), vnodes: new Map(), depEdges: null };
    const nsAllowed = n => !st.namespaces || !n.ns || st.namespaces.has(n.ns);
    const visibleNode = n => !st.hiddenKinds.has(n.kind) && nsAllowed(n);

    if (view === "namespace") projectNamespace(ctx, visibleNode);
    else if (view === "node") projectNode(ctx, visibleNode);
    else if (view === "workload") projectWorkload(ctx, visibleNode);
    else projectDependency(ctx, visibleNode);

    // Each canonical resource is represented by the resource card itself,
    // the aggregate it is folded into, or its collapsed group's card. Every
    // canonical edge becomes a drawn edge between representatives; edges
    // landing on the same pair (and category) are bundled — never dropped,
    // only counted, and the bundle keeps every canonical edge it stands for.
    const rep = id => {
      const m = ctx.member.get(id);
      if (!m) return null;
      const g = ctx.groups.get(m.group);
      if (g && g.collapsed) return "group:" + g.key;
      return m.vid;
    };
    const bundles = new Map();
    for (const e of store.edges.values()) {
      if (!rel.has(e.category)) continue;
      if (ctx.depEdges && !ctx.depEdges.has(e.key)) continue;
      const a = rep(e.from), b = rep(e.to);
      if (!a || !b || a === b) continue;
      const k = a + "→" + b + "|" + e.category;
      let bd = bundles.get(k);
      if (!bd) { bd = { key: k, from: a, to: b, category: e.category, types: new Set(), count: 0, canonical: [], cross: false }; bundles.set(k, bd); }
      bd.types.add(e.type); bd.count++; bd.canonical.push(e);
    }

    for (const g of ctx.groups.values()) {
      if (g.collapsed) {
        const vid = "group:" + g.key;
        ctx.vnodes.set(vid, { vid, type: "groupcard", group: g.key, groupRef: g });
      }
    }
    for (const [vid, v] of Array.from(ctx.vnodes)) {
      if (v.type !== "groupcard" && ctx.groups.get(v.group).collapsed) ctx.vnodes.delete(vid);
    }

    const vedges = Array.from(bundles.values()).sort((x, y) => cmpStr(x.key, y.key));
    vedges.forEach(e => {
      e.cross = ctx.vnodes.get(e.from).group !== ctx.vnodes.get(e.to).group;
      e.inferred = e.canonical.some(c => c.basis === "inferred");
    });

    // Hub summarization. A card with many lines of one category (a
    // NetworkPolicy selecting every Pod in its namespace, a ConfigMap every
    // workload mounts) would tie a whole namespace into one knot. Those lines
    // are summarized on the hub's card ("→ 48 Pods") instead of drawn; the
    // relationships themselves are untouched — selecting the hub highlights
    // every target, and the inspector and dependency view list them all.
    // Ownership is never summarized: it is the skeleton of the layout.
    const summarized = [];
    if (view !== "dependency") {
      const fan = new Map();
      vedges.forEach(e => {
        // Ownership is the layout's skeleton, and lines touching a collapsed
        // group's card are already a bundled summary.
        if (e.category === "ownership" || e.from.startsWith("group:") || e.to.startsWith("group:")) return;
        [[e.from, "out", e.to], [e.to, "in", e.from]].forEach(([v, d, other]) => {
          const k = v + "\u0000" + d + "\u0000" + e.category;
          if (!fan.has(k)) fan.set(k, { v, d, cat: e.category, list: [] });
          fan.get(k).list.push({ e, other });
        });
      });
      const hubbed = new Set();
      Array.from(fan.values()).sort((a, b) => b.list.length - a.list.length || cmpStr(a.v, b.v)).forEach(h => {
        const live = h.list.filter(x => !hubbed.has(x.e));
        if (live.length <= LIMITS.hubEdges) return;
        live.forEach(x => hubbed.add(x.e));
        const hv = ctx.vnodes.get(h.v);
        if (!hv) return;
        const kinds = new Set(live.map(x => { const o = ctx.vnodes.get(x.other); return o ? (o.node ? o.node.kind : o.kind || "group") : "?"; }));
        const n = live.reduce((a, x) => a + x.e.count, 0);
        const what = kinds.size === 1 ? pluralKind(Array.from(kinds)[0]) : "resources";
        hv.hubs = (hv.hubs || []).concat([{ dir: h.d, category: h.cat, count: n, text: (h.d === "out" ? "→ " : "← ") + n + " " + what }]);
        if (hv.type === "resource") hv.badges = (hv.badges || []).concat([{ kind: "hub", text: (h.d === "out" ? "→ " : "← ") + n + " " + what }]);
      });
      for (let i = vedges.length - 1; i >= 0; i--) if (hubbed.has(vedges[i])) { summarized.push(vedges[i]); vedges.splice(i, 1); }
    }

    let rendered = 0;
    for (const v of ctx.vnodes.values()) if (v.type === "resource") rendered++;
    return {
      view, groups: Array.from(ctx.groups.values()), vnodes: ctx.vnodes, vedges, summarized,
      member: ctx.member, focus: ctx.focus || null,
      stats: { total: store.nodes.size, rendered, hidden: store.nodes.size - rendered },
    };
  }

  function addGroup(ctx, key, props) {
    const g = Object.assign({ key, members: [], summary: null, collapsed: false }, props);
    ctx.groups.set(key, g);
    return g;
  }
  function isExpanded(ctx, key, dflt) {
    const m = ctx.st.expanded[ctx.view];
    return m.has(key) ? m.get(key) : dflt;
  }
  function summarize(store, ids) {
    const s = { total: ids.length, byCat: {}, byKind: {}, podsOk: 0, podsBad: 0, broken: 0 };
    ids.forEach(id => {
      const n = store.node(id); if (!n) return;
      s.byCat[n.cat] = (s.byCat[n.cat] || 0) + 1;
      s.byKind[n.kind] = (s.byKind[n.kind] || 0) + 1;
      if (n.kind === "Pod") { if (n.status === "crit") s.podsBad++; else s.podsOk++; }
      if (n.resolved === false && !n.unverified) s.broken++;
    });
    return s;
  }
  function addResource(ctx, g, n) {
    ctx.vnodes.set(n.id, { vid: n.id, type: "resource", group: g.key, node: n, badges: badgesFor(ctx, n) });
    ctx.member.set(n.id, { group: g.key, vid: n.id });
  }
  function addAggregate(ctx, g, aggKey, list, label, kind) {
    const bad = list.filter(n => n.status === "crit").length;
    ctx.vnodes.set(aggKey, { vid: aggKey, type: "aggregate", group: g.key, kind, label, count: list.length, bad, members: list.map(n => n.id), cat: catOf(kind) });
    list.forEach(n => ctx.member.set(n.id, { group: g.key, vid: aggKey }));
  }

  // placeMembers puts canonical resources into group g, folding large
  // fan-outs into aggregates: Pods under one owner beyond podFanout, and
  // leaf kinds beyond kindFanout. An aggregate's key is stable
  // ("agg:<group>:<what>") so a user's "unfold" survives refreshes.
  function placeMembers(ctx, g, ids, opts) {
    opts = opts || {};
    const store = ctx.store;
    const nodes = ids.map(id => store.node(id)).filter(Boolean).sort(cmpNode);
    g.summary = summarize(store, ids);
    g.members = nodes.map(n => n.id);
    const placed = new Set();
    const fold = (aggKey, list, label, kind) => {
      if (ctx.st.unfoldAll || ctx.st.unfolded.has(aggKey) || (opts.keep && list.some(n => n.id === opts.keep))) return;
      addAggregate(ctx, g, aggKey, list, label, kind);
      list.forEach(n => placed.add(n.id));
    };
    if (!opts.noPodFold) {
      const byOwner = new Map();
      nodes.forEach(n => {
        if (n.kind !== "Pod") return;
        const o = store.ownerOf(n.id) || "";
        if (!byOwner.has(o)) byOwner.set(o, []);
        byOwner.get(o).push(n);
      });
      for (const [owner, pods] of byOwner) {
        if (pods.length <= LIMITS.podFanout) continue;
        const on = store.node(owner);
        fold("agg:" + g.key + ":pods:" + owner, pods, pods.length + " Pods" + (on ? " · " + on.name : ""), "Pod");
      }
    }
    const byKind = new Map();
    nodes.forEach(n => {
      if (placed.has(n.id) || !FOLDABLE_KINDS[n.kind]) return;
      if (!byKind.has(n.kind)) byKind.set(n.kind, []);
      byKind.get(n.kind).push(n);
    });
    const kindFanout = opts.kindFanout || LIMITS.kindFanout;
    for (const [kind, list] of byKind) {
      if (list.length <= kindFanout) continue;
      fold("agg:" + g.key + ":kind:" + kind, list, list.length + " " + pluralKind(kind), kind);
    }
    nodes.forEach(n => { if (!placed.has(n.id)) addResource(ctx, g, n); });
  }
  function pluralKind(k) {
    if (k.endsWith("y")) return k.slice(0, -1) + "ies";
    if (k.endsWith("s")) return k + "es";
    return k + "s";
  }
  // badgesFor: facts a card shows as a badge instead of a second copy of
  // the resource — a Pod's worker node in the namespace and workload views,
  // its namespace in the node and dependency views. A resource is drawn
  // once, in exactly one group.
  function badgesFor(ctx, n) {
    const b = [];
    if (n.kind === "Pod" && (ctx.view === "namespace" || ctx.view === "workload") && !ctx.rel.has("scheduling")) {
      const node = ctx.store.nodeOfPod(n.id);
      if (node) b.push({ kind: "node", text: node.split("/").pop() });
    }
    if ((ctx.view === "node" || ctx.view === "dependency") && n.ns) b.push({ kind: "ns", text: n.ns });
    return b;
  }

  function projectNamespace(ctx, visibleNode) {
    const { store, st } = ctx;
    const nsList = store.namespaces().filter(ns => !st.namespaces || st.namespaces.has(ns));
    const small = store.nodes.size <= LIMITS.autoExpandResources;
    nsList.forEach(ns => {
      const ids = (store.byNs.get(ns) || []).filter(id => visibleNode(store.node(id)));
      const g = addGroup(ctx, "ns:" + ns, { type: "namespace", label: ns, ns });
      g.collapsed = !isExpanded(ctx, g.key, small);
      placeMembers(ctx, g, ids);
    });
    // Cluster-scoped infrastructure is never forced into a namespace. With
    // a namespace filter active, only infrastructure connected to a shown
    // namespace is kept.
    let infra = (store.byNs.get("") || []).filter(id => visibleNode(store.node(id)));
    if (st.namespaces) {
      infra = infra.filter(id => store.outEdges(id).concat(store.inEdges(id)).some(e => {
        const o = store.node(e.from === id ? e.to : e.from); return o && o.ns && st.namespaces.has(o.ns);
      }));
    }
    if (infra.length) {
      const g = addGroup(ctx, "infra", { type: "infra", label: "cluster infrastructure", sub: "cluster-scoped" });
      g.collapsed = !isExpanded(ctx, g.key, small);
      placeMembers(ctx, g, infra);
    }
  }

  function projectNode(ctx, visibleNode) {
    const { store } = ctx;
    const byNode = new Map();
    for (const n of store.nodes.values()) {
      if (n.kind !== "Pod" || !visibleNode(n)) continue;
      const node = store.nodeOfPod(n.id) || "";
      if (!byNode.has(node)) byNode.set(node, []);
      byNode.get(node).push(n.id);
    }
    for (const n of store.nodes.values()) if (n.kind === "Node" && !byNode.has(n.id)) byNode.set(n.id, []);
    const keys = Array.from(byNode.keys()).sort((a, b) => (a === "") - (b === "") || cmpStr(a, b));
    const expandDefault = keys.length <= LIMITS.autoExpandGroups;
    keys.forEach(nodeId => {
      const nn = store.node(nodeId);
      const g = addGroup(ctx, "node:" + (nodeId || "unscheduled"), {
        type: "node", label: nodeId ? nodeId.split("/").pop() : "unscheduled", nodeRef: nn || null,
        sub: nn && nn.status === "crit" ? "NotReady" : nn && nn.status === "warn" ? "status unknown" : "",
      });
      g.collapsed = !isExpanded(ctx, g.key, expandDefault);
      const pods = byNode.get(nodeId);
      if (pods.length > LIMITS.nodeGroupPods) {
        // A busy node folds its Pods per namespace.
        g.summary = summarize(store, pods);
        g.members = pods.slice();
        const byNs = new Map();
        pods.forEach(id => { const ns = store.node(id).ns; if (!byNs.has(ns)) byNs.set(ns, []); byNs.get(ns).push(store.node(id)); });
        Array.from(byNs).sort((a, b) => cmpStr(a[0], b[0])).forEach(([ns, list]) => {
          const aggKey = "agg:" + g.key + ":ns:" + ns;
          if (ctx.st.unfolded.has(aggKey) || list.length === 1) list.sort(cmpNode).forEach(n => addResource(ctx, g, n));
          else addAggregate(ctx, g, aggKey, list, list.length + " Pods · " + ns, "Pod");
        });
      } else {
        placeMembers(ctx, g, pods, { noPodFold: true, kindFanout: 1e9 });
      }
    });
  }

  function projectWorkload(ctx, visibleNode) {
    const { store } = ctx;
    // Each visible resource goes to exactly one group: the workload that owns
    // it or is its only user; resources used by several workloads go to
    // their namespace's "shared" group; cluster-scoped ones to infrastructure.
    const owner = new Map();
    const nodes = Array.from(store.nodes.values()).filter(visibleNode).sort(cmpNode);
    nodes.forEach(n => {
      if (!n.ns) return;
      if (ROOT_WORKLOAD[n.kind] || n.kind === "Job" || n.kind === "Pod" || n.kind === "ReplicaSet") {
        const r = store.rootOf(n.id);
        const rn = store.node(r);
        if (rn && rn.ns === n.ns) owner.set(n.id, r);
      }
    });
    const users = new Map();
    const use = (dep, root) => { if (!users.has(dep)) users.set(dep, new Set()); users.get(dep).add(root); };
    for (const [id, root] of owner) {
      store.outEdges(id).forEach(e => { if (e.type !== "controlled-by" && e.type !== "runs-on") use(e.to, root); });
      store.inEdges(id).forEach(e => { if (e.type !== "controlled-by") use(e.from, root); });
    }
    // Ingresses follow the Service they route to; claims follow to their PV.
    nodes.forEach(n => {
      if (n.kind !== "Ingress") return;
      store.outEdges(n.id).forEach(e => (users.get(e.to) || new Set()).forEach(r => use(n.id, r)));
    });
    const groupOf = new Map();
    nodes.forEach(n => {
      if (owner.has(n.id)) { groupOf.set(n.id, "wl:" + owner.get(n.id)); return; }
      if (!n.ns) { groupOf.set(n.id, "infra"); return; }
      const u = users.get(n.id);
      if (u && u.size === 1) groupOf.set(n.id, "wl:" + Array.from(u)[0]);
      else groupOf.set(n.id, "shared:" + n.ns);
    });
    const byGroup = new Map();
    for (const [id, key] of groupOf) { if (!byGroup.has(key)) byGroup.set(key, []); byGroup.get(key).push(id); }
    const rankOf = k => (k === "infra" ? 2 : k.startsWith("shared:") ? 1 : 0);
    const nsOf = k => (k === "infra" ? "\uffff" : k.startsWith("wl:") ? (store.node(k.slice(3)) || {}).ns || "" : k.slice(k.indexOf(":") + 1));
    const keys = Array.from(byGroup.keys()).sort((a, b) => cmpStr(nsOf(a), nsOf(b)) || rankOf(a) - rankOf(b) || cmpStr(a, b));
    const expandDefault = keys.length <= LIMITS.autoExpandGroups;
    keys.forEach(key => {
      let props;
      if (key === "infra") props = { type: "infra", label: "cluster infrastructure", sub: "cluster-scoped" };
      else if (key.startsWith("shared:")) props = { type: "shared", label: "shared · " + key.slice(7), sub: "not owned by one workload", ns: key.slice(7) };
      else { const w = store.node(key.slice(3)); props = { type: "workload", label: w.name, sub: w.kind + " · " + w.ns, ns: w.ns, workload: w }; }
      const g = addGroup(ctx, key, props);
      g.collapsed = !isExpanded(ctx, key, expandDefault);
      placeMembers(ctx, g, byGroup.get(key));
    });
  }

  function projectDependency(ctx, visibleNode) {
    const { store, st } = ctx;
    ctx.depEdges = new Set();
    const focus = st.focus && store.node(st.focus) ? st.focus : null;
    ctx.focus = focus;
    if (!focus) return;
    const rel = ctx.rel;
    // The focus resource's ownership chain is treated as one unit: a
    // Deployment *is* its ReplicaSets and Pods, so what those Pods mount,
    // run on and are selected by are the Deployment's dependencies too, and
    // a Pod's are understood in the context of the workload that owns it.
    const unit = new Set([focus]);
    const down = [focus];
    while (down.length) {
      const id = down.pop();
      store.inEdges(id).forEach(e => { if (e.type === "controlled-by" && !unit.has(e.from)) { unit.add(e.from); down.push(e.from); } });
    }
    for (let cur = store.ownerOf(focus), guard = 0; cur && guard < 10; cur = store.ownerOf(cur), guard++) unit.add(cur);
    const ownership = rel.has("ownership");
    unit.forEach(id => {
      store.outEdges(id).concat(store.inEdges(id)).forEach(e => { if (e.type === "controlled-by" && unit.has(e.from) && unit.has(e.to) && ownership) ctx.depEdges.add(e.key); });
    });
    const dist = new Map();
    unit.forEach(id => dist.set(id, 0));
    // Outgoing closure (what it depends on) and incoming closure (what
    // depends on it) are walked separately: following both directions from
    // every node would pull in the whole cluster through any shared Node.
    const walk = dir => {
      let frontier = Array.from(unit).sort(cmpStr);
      for (let d = 0; d < st.depth && frontier.length; d++) {
        const next = [];
        frontier.forEach(id => {
          (dir === "out" ? store.outEdges(id) : store.inEdges(id)).forEach(e => {
            if (!rel.has(e.category) || e.type === "controlled-by") return;
            const other = dir === "out" ? e.to : e.from;
            const on = store.node(other);
            if (!on || !visibleNode(on)) return;
            ctx.depEdges.add(e.key);
            if (!dist.has(other)) { dist.set(other, d + 1); next.push(other); }
          });
        });
        frontier = next.sort(cmpStr);
      }
    };
    if (st.direction !== "in") walk("out");
    if (st.direction !== "out") walk("in");
    const g = addGroup(ctx, "dep", { type: "dependency", label: "dependencies of " + store.node(focus).name, frameless: true });
    placeMembers(ctx, g, Array.from(dist.keys()), { kindFanout: LIMITS.hubFanout, keep: focus });
    const fv = ctx.vnodes.get(focus);
    if (fv) fv.focus = true;
  }

  // ---------------------------------------------------------------------
  // 3. Layout
  // ---------------------------------------------------------------------

  const L = {
    // Card text is 12px JetBrains Mono (0.6em advance ≈ 7.2px per char).
    cardH: 44, cardHBadge: 58, minW: 140, maxW: 252, charW: 7.25,
    rankGap: 72, rowGap: 12, gridGapX: 36, gridGapY: 12,
    pad: 18, header: 34, groupGapX: 52, groupGapY: 64, infraGap: 92,
    collapsedW: 248, collapsedH: 112,
  };

  const shorten = (s, max) => (s.length > max ? s.slice(0, max - 1) + "…" : s);
  function sizeOf(v) {
    if (v.type === "groupcard") return { w: L.collapsedW, h: L.collapsedH };
    if (v.type === "aggregate") return { w: Math.min(L.maxW, Math.max(L.minW + 20, shorten(v.label, 30).length * L.charW + 48)), h: L.cardH };
    const name = shorten(v.node.name, 30);
    const w = Math.min(L.maxW, Math.max(L.minW, name.length * L.charW + 48));
    return { w, h: v.badges && v.badges.length ? L.cardHBadge : L.cardH };
  }

  // flowEnds orients an edge left-to-right: ownership flows from owner to
  // owned (the reverse of controlled-by's from/to); every other relation
  // flows from the resource that declares it to its target.
  function flowEnds(e) {
    return e.category === "ownership" ? [e.to, e.from] : [e.from, e.to];
  }

  // layoutLayered: deterministic layered layout of a group's connected
  // vnodes (ids must already be in a deterministic order). Returns local
  // coordinates and, per edge, waypoints through dummy nodes so an edge
  // spanning several columns passes between cards rather than through them.
  function layoutLayered(ids, sizes, edges) {
    const idx = new Map(ids.map((id, i) => [id, i]));
    const flow = [];
    edges.forEach(e => {
      const [u, v] = flowEnds(e);
      if (!idx.has(u) || !idx.has(v) || u === v) return;
      flow.push({ e, u, v });
    });
    flow.sort((a, b) => idx.get(a.u) - idx.get(b.u) || idx.get(a.v) - idx.get(b.v) || cmpStr(a.e.key, b.e.key));

    // Break cycles (only possible with malformed data) by reversing DFS
    // back edges.
    const outF = new Map(ids.map(id => [id, []]));
    flow.forEach(f => outF.get(f.u).push(f));
    const color = new Map();
    for (const start of ids) {
      if (color.get(start)) continue;
      const stack = [[start, 0]]; color.set(start, 1);
      while (stack.length) {
        const top = stack[stack.length - 1];
        const fs = outF.get(top[0]);
        if (top[1] >= fs.length) { color.set(top[0], 2); stack.pop(); continue; }
        const f = fs[top[1]++];
        const c = color.get(f.v);
        if (c === 1) f.rev = true;
        else if (!c) { color.set(f.v, 1); stack.push([f.v, 0]); }
      }
    }
    const succ = new Map(ids.map(id => [id, []])), pred = new Map(ids.map(id => [id, []]));
    flow.forEach(f => {
      if (f.rev) { const t = f.u; f.u = f.v; f.v = t; }
      succ.get(f.u).push(f.v); pred.get(f.v).push(f.u);
    });

    // Longest-path layering (Kahn, deterministic queue).
    const indeg = new Map(ids.map(id => [id, pred.get(id).length]));
    const rank = new Map(ids.map(id => [id, 0]));
    const queue = ids.filter(id => indeg.get(id) === 0);
    const topo = [];
    while (queue.length) {
      const id = queue.shift(); topo.push(id);
      succ.get(id).forEach(w => {
        rank.set(w, Math.max(rank.get(w), rank.get(id) + 1));
        indeg.set(w, indeg.get(w) - 1);
        if (indeg.get(w) === 0) queue.push(w);
      });
    }
    // Pull sources right, next to what they point at (a Service sits in the
    // column beside its Pods, not far left with the Deployments).
    for (let i = topo.length - 1; i >= 0; i--) {
      const id = topo[i];
      if (pred.get(id).length || !succ.get(id).length) continue;
      rank.set(id, Math.min(...succ.get(id).map(w => rank.get(w))) - 1);
    }
    let minR = Infinity; rank.forEach(r => { minR = Math.min(minR, r); });
    rank.forEach((r, id) => rank.set(id, r - minR));
    const maxR = Math.max(0, ...rank.values());

    // Dummy nodes for long edges.
    const layers = Array.from({ length: maxR + 1 }, () => []);
    const all = new Map();
    ids.forEach(id => all.set(id, { id, rank: rank.get(id), w: sizes.get(id).w, h: sizes.get(id).h, dummy: false }));
    const lsucc = new Map(), lpred = new Map();
    const link = (a, b) => {
      if (!lsucc.has(a)) lsucc.set(a, []); lsucc.get(a).push(b);
      if (!lpred.has(b)) lpred.set(b, []); lpred.get(b).push(a);
    };
    const chains = [];
    flow.forEach((f, fi) => {
      const ru = rank.get(f.u), rv = rank.get(f.v);
      const chain = [f.u];
      let prev = f.u;
      for (let r = ru + 1; r < rv; r++) {
        const d = "~d" + fi + ":" + r;
        all.set(d, { id: d, rank: r, w: 0, h: 4, dummy: true });
        link(prev, d); chain.push(d); prev = d;
      }
      link(prev, f.v); chain.push(f.v);
      chains.push({ f, chain });
    });
    all.forEach(n => layers[n.rank].push(n.id));

    // Initial order: DFS from roots in sorted order keeps subtrees together.
    const pos = new Map(), seen = new Set(), counters = layers.map(() => 0);
    const visit = id => {
      if (seen.has(id)) return; seen.add(id);
      pos.set(id, counters[all.get(id).rank]++);
      (lsucc.get(id) || []).forEach(visit);
    };
    ids.filter(id => !(lpred.get(id) || []).length).forEach(visit);
    all.forEach((n, id) => visit(id));
    layers.forEach(l => l.sort((a, b) => pos.get(a) - pos.get(b)));
    const reindex = () => layers.forEach(l => l.forEach((id, i) => pos.set(id, i)));
    reindex();

    // Crossing minimization: barycenter sweeps, keeping the best ordering.
    const crossings = () => {
      let c = 0;
      for (let r = 0; r < layers.length - 1; r++) {
        const es = [];
        layers[r].forEach(a => (lsucc.get(a) || []).forEach(b => es.push([pos.get(a), pos.get(b)])));
        if (es.length > 1500) continue; // quadratic count; huge layers skip scoring
        for (let i = 0; i < es.length; i++) for (let j = i + 1; j < es.length; j++) {
          if ((es[i][0] - es[j][0]) * (es[i][1] - es[j][1]) < 0) c++;
        }
      }
      return c;
    };
    let best = layers.map(l => l.slice()), bestC = crossings();
    for (let it = 0; it < 8 && bestC > 0; it++) {
      const down = it % 2 === 0;
      const order = [...Array(layers.length).keys()];
      (down ? order.slice(1) : order.reverse().slice(1)).forEach(r => {
        const nb = down ? lpred : lsucc;
        const bary = new Map();
        layers[r].forEach(id => {
          const ns = nb.get(id) || [];
          bary.set(id, ns.length ? ns.reduce((s, x) => s + pos.get(x), 0) / ns.length : pos.get(id));
        });
        layers[r].sort((a, b) => bary.get(a) - bary.get(b) || pos.get(a) - pos.get(b));
        layers[r].forEach((id, i) => pos.set(id, i));
      });
      const c = crossings();
      if (c < bestC) { bestC = c; best = layers.map(l => l.slice()); }
    }
    best.forEach((l, r) => { layers[r] = l; });
    reindex();

    // Vertical placement: pull each node toward the mean of its neighbours,
    // then restore order and minimum spacing by isotonic regression (pool
    // adjacent violators) — the least-squares fit that keeps the
    // crossing-minimized order and never lets two cards touch.
    const y = new Map();
    layers.forEach(l => { let acc = 0; l.forEach(id => { const n = all.get(id); y.set(id, acc + n.h / 2); acc += n.h + L.rowGap; }); });
    const place = (r, nbs) => {
      const l = layers[r]; if (!l.length) return;
      const desired = l.map(id => {
        const ns = [];
        nbs.forEach(m => (m.get(id) || []).forEach(x => ns.push(x)));
        return ns.length ? ns.reduce((s, x) => s + y.get(x), 0) / ns.length : y.get(id);
      });
      const off = [0];
      for (let i = 1; i < l.length; i++) {
        const gap = all.get(l[i - 1]).dummy || all.get(l[i]).dummy ? 6 : L.rowGap;
        off.push(off[i - 1] + all.get(l[i - 1]).h / 2 + gap + all.get(l[i]).h / 2);
      }
      const blocks = [];
      desired.forEach((d, i) => {
        blocks.push({ sum: d - off[i], n: 1 });
        while (blocks.length > 1 && blocks[blocks.length - 2].sum / blocks[blocks.length - 2].n > blocks[blocks.length - 1].sum / blocks[blocks.length - 1].n) {
          const b = blocks.pop(); blocks[blocks.length - 1].sum += b.sum; blocks[blocks.length - 1].n += b.n;
        }
      });
      let i = 0;
      blocks.forEach(b => { const v = b.sum / b.n; for (let k = 0; k < b.n; k++, i++) y.set(l[i], v + off[i]); });
    };
    for (let it = 0; it < 6; it++) {
      if (it % 2 === 0) for (let r = 1; r < layers.length; r++) place(r, [lpred]);
      else for (let r = layers.length - 2; r >= 0; r--) place(r, [lsucc]);
    }
    for (let r = 0; r < layers.length; r++) place(r, [lpred, lsucc]);

    // Horizontal: one column per rank, as wide as its widest card.
    const colW = layers.map(l => Math.max(0, ...l.map(id => all.get(id).w)));
    const colX = []; let acc = 0;
    colW.forEach((w, r) => { colX.push(acc + w / 2); acc += w + (r < colW.length - 1 ? L.rankGap : 0); });
    let minY = Infinity, maxY = -Infinity;
    all.forEach((n, id) => { minY = Math.min(minY, y.get(id) - n.h / 2); maxY = Math.max(maxY, y.get(id) + n.h / 2); });
    if (!isFinite(minY)) { minY = 0; maxY = 0; }
    const res = { pos: new Map(), routes: new Map(), width: acc, height: maxY - minY };
    const tmp = new Map();
    all.forEach((n, id) => {
      const cx = colX[n.rank], cy = y.get(id) - minY;
      tmp.set(id, { x: cx, y: cy, w: n.w, h: n.h });
      if (!n.dummy) res.pos.set(id, { x: cx, y: cy, w: n.w, h: n.h, chL: cx - colW[n.rank] / 2 - L.rankGap / 2, chR: cx + colW[n.rank] / 2 + L.rankGap / 2 });
    });
    chains.forEach(({ f, chain }) => {
      const pts = chain.map(id => tmp.get(id));
      const a = pts[0], b = pts[pts.length - 1];
      const path = [{ x: a.x + a.w / 2, y: a.y }];
      for (let i = 1; i < pts.length - 1; i++) path.push({ x: pts[i].x, y: pts[i].y });
      path.push({ x: b.x - b.w / 2, y: b.y });
      // The path runs from chain start (f.u, left) to chain end (f.v,
      // right); flip it if the edge's semantic target is the left end, so
      // the arrowhead always lands on e.to.
      res.routes.set(f.e.key, f.v === f.e.to ? path : path.slice().reverse());
    });
    return res;
  }

  // layoutGrid: unconnected cards, column-major (so kinds stay together),
  // with a card-free channel between columns.
  function layoutGrid(ids, sizes, targetH) {
    const n = ids.length;
    if (!n) return { pos: new Map(), width: 0, height: 0 };
    const avgW = ids.reduce((s, id) => s + sizes.get(id).w, 0) / n;
    let rows;
    if (targetH > 0) rows = Math.max(1, Math.floor((targetH + L.gridGapY) / (L.cardH + L.gridGapY)));
    else rows = Math.max(1, Math.round(Math.sqrt(n * (avgW + L.gridGapX) / (1.7 * (L.cardH + L.gridGapY)))));
    rows = Math.min(rows, n);
    const cols = Math.ceil(n / rows);
    const pos = new Map();
    let x = 0, height = 0;
    for (let c = 0; c < cols; c++) {
      const col = ids.slice(c * rows, (c + 1) * rows);
      const w = Math.max(...col.map(id => sizes.get(id).w));
      let yy = 0;
      col.forEach(id => {
        const s = sizes.get(id);
        pos.set(id, { x: x + w / 2, y: yy + s.h / 2, w: s.w, h: s.h, chL: x - L.gridGapX / 2, chR: x + w + L.gridGapX / 2 });
        yy += s.h + L.gridGapY;
      });
      height = Math.max(height, yy - L.gridGapY);
      x += w + L.gridGapX;
    }
    return { pos, width: x - L.gridGapX, height };
  }

  // layoutGroup lays out one expanded group's interior. Each connected
  // component (typically one application: its controller chain, Pods,
  // Service, configuration) gets its own layered layout; cards with no line
  // inside the group form a grid. These blocks are then shelf-packed into a
  // compact rectangle. Every block keeps a card-free channel beside each
  // column and a gutter between block rows, which is how cross-group edges
  // leave the group without crossing a card (see routeCross).
  const COMP_GAP_X = 56, COMP_GAP_Y = 44;
  function layoutGroup(vids, vnodes, intraEdges) {
    const sizes = new Map(vids.map(id => [id, sizeOf(vnodes.get(id))]));
    const order = new Map(vids.map((id, i) => [id, i]));
    const parent = new Map(vids.map(id => [id, id]));
    const find = x => { while (parent.get(x) !== x) { parent.set(x, parent.get(parent.get(x))); x = parent.get(x); } return x; };
    intraEdges.forEach(e => {
      if (!parent.has(e.from) || !parent.has(e.to)) return;
      const a = find(e.from), b = find(e.to);
      if (a !== b) { if (order.get(a) < order.get(b)) parent.set(b, a); else parent.set(a, b); }
    });
    const comps = new Map(), deg = new Map();
    intraEdges.forEach(e => { deg.set(e.from, 1); deg.set(e.to, 1); });
    const iso = [];
    vids.forEach(id => {
      if (!deg.has(id)) { iso.push(id); return; }
      const r = find(id);
      if (!comps.has(r)) comps.set(r, []);
      comps.get(r).push(id);
    });
    const blocks = [];
    Array.from(comps.values()).sort((a, b) => order.get(a[0]) - order.get(b[0])).forEach(ids => {
      const set = new Set(ids);
      blocks.push(layoutLayered(ids, sizes, intraEdges.filter(e => set.has(e.from) && set.has(e.to))));
    });
    if (iso.length) blocks.push(Object.assign({ routes: new Map() }, layoutGrid(iso, sizes, 0)));

    // Shelf-pack blocks.
    const maxW = shelfWidth(blocks.map(b => ({ w: b.width, h: b.height })), COMP_GAP_X, COMP_GAP_Y, 1.9);
    const rows = [];
    let x = 0, row = null;
    blocks.forEach(b => {
      if (!row || (x > 0 && x + b.width > maxW)) {
        const y0 = row ? row.y1 + COMP_GAP_Y : 0;
        row = { y0, y1: y0 }; rows.push(row); x = 0;
      }
      b.ox = x; b.oy = row.y0; b.row = rows.length - 1;
      row.y1 = Math.max(row.y1, row.y0 + b.height);
      x += b.width + COMP_GAP_X;
    });
    const pos = new Map(), routes = new Map();
    let width = 0;
    blocks.forEach(b => {
      width = Math.max(width, b.ox + b.width);
      const r = rows[b.row];
      b.pos.forEach((p, id) => pos.set(id, { x: p.x + b.ox, y: p.y + b.oy, w: p.w, h: p.h, chL: p.chL + b.ox, chR: p.chR + b.ox,
        row: b.row, rows: rows.length, rowTop: r.y0, rowBottom: r.y1 }));
      b.routes.forEach((pts, key) => routes.set(key, pts.map(q => ({ x: q.x + b.ox, y: q.y + b.oy }))));
    });
    return { pos, routes, width, height: rows.length ? rows[rows.length - 1].y1 : 0 };
  }

  // shelfWidth picks the shelf (row) width whose packing comes closest to
  // the target aspect ratio — wide, short blocks then share rows instead of
  // stacking into a tall column. Deterministic: candidates are tried in a
  // fixed order and ties keep the first.
  function shelfWidth(items, gapX, gapY, aspect) {
    if (!items.length) return 0;
    const maxW = Math.max(...items.map(b => b.w));
    const total = items.reduce((a, b) => a + b.w + gapX, 0);
    const cands = new Set([maxW]);
    let acc = 0;
    items.forEach(b => { acc += b.w + gapX; if (acc - gapX >= maxW) cands.add(acc - gapX); });
    const steps = 24;
    for (let i = 1; i <= steps; i++) cands.add(maxW + (total - maxW) * i / steps);
    let best = maxW, bestScore = Infinity;
    Array.from(cands).sort((a, b) => a - b).forEach(rw => {
      let x = 0, y = 0, rowH = 0, width = 0;
      items.forEach(b => {
        if (x > 0 && x + b.w > rw + 0.5) { y += rowH + gapY; x = 0; rowH = 0; }
        x += b.w + gapX; rowH = Math.max(rowH, b.h); width = Math.max(width, x - gapX);
      });
      const height = y + rowH;
      const score = Math.abs(Math.log((width / Math.max(1, height)) / aspect));
      if (score < bestScore - 1e-9) { bestScore = score; best = rw; }
    });
    return best;
  }

  // Layout cache: a group's interior depends only on its own vnodes and
  // intra-group edges, so it is memoized by exactly that content. A delta
  // touching one namespace re-lays out that namespace alone; every other
  // group keeps exactly the geometry it had.
  const groupCache = new Map();
  const GROUP_CACHE_MAX = 600;
  const cacheStats = { hits: 0, misses: 0 };
  function groupSignature(vids, vnodes, intra) {
    const parts = vids.map(id => {
      const v = vnodes.get(id);
      return id + ":" + (v.type === "resource" ? v.node.name.length + "," + (v.badges || []).length : v.label);
    });
    return parts.join(";") + "#" + intra.map(e => e.key).join(";");
  }

  function computeLayout(P) {
    const byGroup = new Map(P.groups.map(g => [g.key, []]));
    P.vnodes.forEach(v => { if (byGroup.has(v.group)) byGroup.get(v.group).push(v.vid); });
    const intraBy = new Map(P.groups.map(g => [g.key, []]));
    P.vedges.forEach(e => { if (!e.cross) intraBy.get(P.vnodes.get(e.from).group).push(e); });

    const boxes = [];
    P.groups.forEach(g => {
      if (g.collapsed) { boxes.push({ g, w: L.collapsedW, h: L.collapsedH, inner: null, frame: 1 }); return; }
      const vids = byGroup.get(g.key).sort((a, b) => {
        const va = P.vnodes.get(a), vb = P.vnodes.get(b);
        const ka = va.node || { kind: va.kind, name: va.label, id: a };
        const kb = vb.node || { kind: vb.kind, name: vb.label, id: b };
        return cmpNode(ka, kb);
      });
      const intra = intraBy.get(g.key);
      const sig = groupSignature(vids, P.vnodes, intra);
      let inner = groupCache.get(sig);
      if (inner) cacheStats.hits++;
      else {
        cacheStats.misses++;
        inner = layoutGroup(vids, P.vnodes, intra);
        groupCache.set(sig, inner);
        if (groupCache.size > GROUP_CACHE_MAX) groupCache.delete(groupCache.keys().next().value);
      }
      const frame = g.frameless ? 0 : 1;
      const w = frame ? Math.max(inner.width + 2 * L.pad, 220) : inner.width;
      const h = frame ? Math.max(inner.height + L.pad * 2 + L.header, 84) : inner.height;
      boxes.push({ g, w, h, inner, frame });
    });

    // Shelf packing. In the namespace view, cluster infrastructure is set
    // apart as a column on the right, so the many namespace→infrastructure
    // edges share one short highway instead of crisscrossing the canvas.
    const sideInfra = P.view === "namespace";
    const main = boxes.filter(b => !(sideInfra && b.g.type === "infra"));
    const side = boxes.filter(b => sideInfra && b.g.type === "infra");
    // Aim for roughly the shape of a wide screen, leaving room for the
    // infrastructure column when there is one.
    const rowW = shelfWidth(main.map(b => ({ w: b.w, h: b.h })), L.groupGapX, L.groupGapY, side.length ? 1.45 : 1.75);
    const rows = [];
    let x = 0, row = null;
    main.forEach(b => {
      if (!row || (x > 0 && x + b.w > rowW)) {
        const y0 = row ? row.y1 + L.groupGapY : 0;
        row = { index: rows.length, y0, y1: y0, spans: [] };
        rows.push(row); x = 0;
      }
      b.x0 = x; b.y0 = row.y0; b.x1 = x + b.w; b.y1 = row.y0 + b.h; b.row = row.index;
      row.spans.push([b.x0, b.x1]);
      row.y1 = Math.max(row.y1, b.y1);
      x += b.w + L.groupGapX;
    });
    const mainRight = main.length ? Math.max(...main.map(b => b.x1)) : 0;
    let sy = 0;
    const sx = mainRight + (main.length ? L.infraGap : 0);
    side.forEach(b => { b.x0 = sx; b.y0 = sy; b.x1 = sx + b.w; b.y1 = sy + b.h; b.row = -1; sy = b.y1 + L.groupGapY; });

    const groups = [], pos = new Map(), routes = new Map();
    boxes.forEach(b => {
      groups.push({ key: b.g.key, g: b.g, x0: b.x0, y0: b.y0, x1: b.x1, y1: b.y1, row: b.row, collapsed: b.g.collapsed, frameless: !b.frame });
      if (b.g.collapsed) {
        pos.set("group:" + b.g.key, { x: (b.x0 + b.x1) / 2, y: (b.y0 + b.y1) / 2, w: b.w, h: b.h, group: b.g.key, card: true });
        return;
      }
      const ox = b.x0 + L.pad * b.frame, oy = b.y0 + (L.pad + L.header) * b.frame;
      b.inner.pos.forEach((p, id) => pos.set(id, { x: p.x + ox, y: p.y + oy, w: p.w, h: p.h, chL: p.chL + ox, chR: p.chR + ox, group: b.g.key,
        row: p.row, rows: p.rows, rowTop: p.rowTop + oy, rowBottom: p.rowBottom + oy, hwyL: b.x0 + L.pad / 2, hwyR: b.x1 - L.pad / 2 }));
      b.inner.routes.forEach((pts, key) => routes.set(key, pts.map(p => ({ x: p.x + ox, y: p.y + oy }))));
    });

    const gByKey = new Map(groups.map(g => [g.key, g]));
    const rctx = { rows, gByKey, pos, highwayL: -L.groupGapX / 2, highwayR: mainRight + (side.length ? L.infraGap / 2 : L.groupGapX / 2) };
    // Intra-group routes are keyed by canonical edge key inside the
    // layered pass; re-key them by drawn-edge key.
    const drawn = new Map();
    // Port slots: the cross edges meeting one card are spread along its edge
    // (ordered by where the other end lies) instead of converging on a
    // single point.
    const slots = new Map();
    P.vedges.forEach(e => {
      if (!e.cross) return;
      [[e.from, e.to], [e.to, e.from]].forEach(([me, other]) => {
        if (!slots.has(me)) slots.set(me, []);
        slots.get(me).push({ e, ox: (pos.get(other) || { x: 0 }).x });
      });
    });
    const slotOf = new Map();
    slots.forEach((list, vid) => {
      list.sort((a, b) => a.ox - b.ox || cmpStr(a.e.key, b.e.key));
      const p = pos.get(vid); if (!p) return;
      const span = p.card ? Math.max(0, p.w - 48) : Math.max(0, p.h - 12);
      const step = list.length > 1 ? Math.min(p.card ? 16 : 6, span / (list.length - 1)) : 0;
      list.forEach((it, i) => slotOf.set(it.e.key + "@" + vid, (i - (list.length - 1) / 2) * step));
    });
    let lane = 0;
    P.vedges.forEach(e => {
      if (!e.cross) {
        const r = routes.get(e.key);
        if (r) drawn.set(e.key, r);
        else {
          const a = pos.get(e.from), b = pos.get(e.to);
          if (a && b) drawn.set(e.key, [{ x: a.x, y: a.y }, { x: b.x, y: b.y }]);
        }
        return;
      }
      const pts = routeCross(rctx, e.from, e.to, ((lane++ % 9) - 4) * 4, slotOf.get(e.key + "@" + e.from) || 0, slotOf.get(e.key + "@" + e.to) || 0);
      if (pts) drawn.set(e.key, pts);
    });

    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    groups.forEach(g => { minX = Math.min(minX, g.x0); minY = Math.min(minY, g.y0); maxX = Math.max(maxX, g.x1); maxY = Math.max(maxY, g.y1); });
    if (!isFinite(minX)) { minX = minY = 0; maxX = maxY = 1; }
    return { groups, pos, routes: drawn, bounds: { x0: minX - 48, y0: minY - 64, x1: maxX + 48, y1: maxY + 48 } };
  }

  // routeCross draws an edge between cards in two different groups using
  // only empty space: from the card sideways into its column's channel,
  // along the channel out of the group into the gutter between rows, along
  // gutters (and, between distant rows, along a margin highway no group
  // occupies), then back in through the target's channel. Every segment is
  // axis-aligned; `lane` offsets parallel edges so they read as a bundle.
  function routeCross(ctx, fromVid, toVid, lane, slotA, slotB) {
    const a = ctx.pos.get(fromVid), b = ctx.pos.get(toVid);
    if (!a || !b) return null;
    const ga = ctx.gByKey.get(a.group), gb = ctx.gByKey.get(b.group);
    if (!ga || !gb) return null;
    const above = g => (g.row >= 0 ? ctx.rows[g.row].y0 : g.y0) - L.groupGapY / 2 + lane;
    const below = g => (g.row >= 0 ? ctx.rows[g.row].y1 : g.y1) + L.groupGapY / 2 + lane;
    let dirA = "up", dirB = "up";
    if (ga.row >= 0 && gb.row >= 0 && ga.row !== gb.row) {
      if (gb.row > ga.row) { dirA = "down"; dirB = "up"; } else { dirA = "up"; dirB = "down"; }
    }
    const yA = dirA === "up" ? above(ga) : below(ga);
    const yB = dirB === "up" ? above(gb) : below(gb);
    const port = (p, dir, otherX, slot) => {
      if (p.card) return [{ x: p.x + slot, y: dir === "up" ? p.y - p.h / 2 : p.y + p.h / 2 }];
      const right = otherX >= p.x;
      const ch = (right ? p.chR : p.chL) + lane * 0.5;
      const y = p.y + slot;
      const pts = [{ x: right ? p.x + p.w / 2 : p.x - p.w / 2, y }, { x: ch, y }];
      // Inside the group: if other blocks sit between this card's block row
      // and the group edge, run along the gutter between block rows to the
      // group's inner margin, then out.
      const blocked = dir === "up" ? p.row > 0 : p.row < p.rows - 1;
      if (blocked) {
        const gy = (dir === "up" ? p.rowTop - COMP_GAP_Y / 2 : p.rowBottom + COMP_GAP_Y / 2) + lane * 0.4;
        const hx = (right ? p.hwyR : p.hwyL) + lane * 0.3;
        pts.push({ x: ch, y: gy }, { x: hx, y: gy });
      }
      return pts;
    };
    const pa = port(a, dirA, b.x, slotA || 0), pb = port(b, dirB, a.x, slotB || 0);
    const xa = pa[pa.length - 1].x, xb = pb[pb.length - 1].x;
    const pts = pa.slice();
    pts.push({ x: xa, y: yA });
    if (Math.abs(yA - yB) > 0.5) {
      const hx = verticalLane(ctx, ga, gb, xa, xb) + lane;
      pts.push({ x: hx, y: yA });
      pts.push({ x: hx, y: yB });
    }
    pts.push({ x: xb, y: yB });
    pb.slice().reverse().forEach(p => pts.push(p));
    return simplify(pts);
  }

  // verticalLane picks the x at which an edge travels between two gutters
  // that are rows apart: the cheapest vertical channel that no group in any
  // row in between occupies — usually the gap between two groups right
  // beside the edge's ends — falling back to the margin highways. To or from
  // the side column it is always the highway between the rows and it.
  function verticalLane(ctx, ga, gb, xa, xb) {
    if (ga.row < 0 || gb.row < 0) return ctx.highwayR;
    const lo = Math.min(ga.row, gb.row), hi = Math.max(ga.row, gb.row);
    const between = ctx.rows.slice(lo + 1, hi);
    const half = L.groupGapX / 2;
    const cands = [ctx.highwayL, ctx.highwayR];
    for (let r = lo; r <= hi; r++) {
      ctx.rows[r].spans.forEach(([x0, x1]) => { cands.push(x0 - half, x1 + half); });
    }
    const free = x => between.every(row => row.spans.every(([x0, x1]) => x < x0 - 10 || x > x1 + 10));
    let best = null, bestCost = Infinity;
    cands.forEach(x => {
      if (!free(x)) return;
      const cost = Math.abs(xa - x) + Math.abs(xb - x);
      if (cost < bestCost - 0.01 || (Math.abs(cost - bestCost) <= 0.01 && x < best)) { best = x; bestCost = cost; }
    });
    return best === null ? ctx.highwayL : best;
  }

  function simplify(pts) {
    const out = [];
    pts.forEach(p => {
      const last = out[out.length - 1];
      if (last && Math.abs(last.x - p.x) < 0.01 && Math.abs(last.y - p.y) < 0.01) return;
      if (out.length >= 2) {
        const q = out[out.length - 2], r = out[out.length - 1];
        const col = (Math.abs(q.x - r.x) < 0.01 && Math.abs(r.x - p.x) < 0.01) || (Math.abs(q.y - r.y) < 0.01 && Math.abs(r.y - p.y) < 0.01);
        if (col) { out[out.length - 1] = p; return; }
      }
      out.push(p);
    });
    return out;
  }

  // ---------------------------------------------------------------------
  // Exploration helpers
  // ---------------------------------------------------------------------

  // trace returns every canonical resource within `depth` hops of id,
  // walking outgoing and incoming edges separately (see projectDependency).
  function trace(store, id, depth, categories) {
    const dist = new Map([[id, 0]]);
    const edges = new Set();
    ["out", "in"].forEach(dir => {
      let frontier = [id];
      for (let d = 0; d < depth && frontier.length; d++) {
        const next = [];
        frontier.forEach(cur => (dir === "out" ? store.outEdges(cur) : store.inEdges(cur)).forEach(e => {
          if (categories && !categories.has(e.category)) return;
          const o = dir === "out" ? e.to : e.from;
          edges.add(e.key);
          if (!dist.has(o)) { dist.set(o, d + 1); next.push(o); }
        }));
        frontier = next;
      }
    });
    return { dist, edges };
  }

  // reveal makes a resource individually visible in the current view:
  // expands its group and unfolds the aggregate it sits in. Returns true if
  // the view state changed (the caller re-projects).
  function reveal(st, P, id) {
    const m = P.member.get(id);
    if (!m) return false;
    let changed = false;
    const g = P.groups.find(x => x.key === m.group);
    if (g && g.collapsed) { st.expanded[st.view].set(g.key, true); changed = true; }
    if (m.vid !== id && m.vid.startsWith("agg:")) { st.unfolded.add(m.vid); changed = true; }
    return changed;
  }

  // Decorative namespace palette (resource kinds use CSS tokens instead).
  const NS_PALETTE_DARK = ["#a6e22e", "#6ea8fe", "#d980fb", "#e3b341", "#34d1bf", "#f0883e", "#db61a2", "#7ee787", "#a371f7", "#4ec9b0"];
  const NS_PALETTE_LIGHT = ["#4f8a1c", "#2f6fe0", "#a23fd1", "#a06400", "#0a8f7c", "#b85c17", "#c23f82", "#1f8f4e", "#7a4fd1", "#0f8f82"];
  function hashColor(key, palette) {
    let h = 0; for (let i = 0; i < key.length; i++) h = (h * 31 + key.charCodeAt(i)) >>> 0;
    return palette[h % palette.length];
  }

  return {
    CAT, catOf, CATEGORIES, CATEGORY_LABEL, TYPE_CATEGORY, BASIS_LABEL, REVERSE, KIND_ORDER, LIMITS, VIEWS, DEFAULT_RELATIONS, LAYOUT: L,
    createStore, createViewState, search, project, computeLayout, trace, reveal, shorten, sizeOf,
    NS_PALETTE_DARK, NS_PALETTE_LIGHT, hashColor, cmpNode,
    _internal: { layoutLayered, layoutGrid, layoutGroup, routeCross, groupCache, cacheStats, flowEnds },
  };
});
