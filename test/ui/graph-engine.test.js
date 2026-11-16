// Tests for internal/web/ui/graph-engine.js — the DOM-free projection and
// layout core. Run with:  node --test test/ui/
// No dependencies: Node's built-in test runner and assert only.
"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const G = require("../../internal/web/ui/graph-engine.js");

// Real graphs captured from mantis-engine (the old Playground scenarios).
const FIXTURES = path.join(__dirname, "fixtures");

// A small synthetic cluster in the engine's wire format: several
// namespaces, Deployments with ReplicaSets and Pods across worker nodes,
// Services, ConfigMaps, a namespace-wide NetworkPolicy (a "hub"), shared
// storage and a cross-namespace RBAC grant.
function synth(nsCount, appsPerNs, podsPerApp) {
  const nodes = [], edges = [];
  const N = (id, kind, name, ns, extra) => nodes.push(Object.assign({ id, kind, name, ns, resolved: true }, extra || {}));
  const E = (from, type, to, category, basis) => edges.push({ from, to, type, category, basis: basis || "reference" });
  for (let w = 1; w <= 4; w++) N("/Node/w" + w, "Node", "w" + w, "", { status: "ok" });
  N("/StorageClass/fast", "StorageClass", "fast", "");
  N("/ServiceAccount-placeholder", "Namespace", "x", "", {}); nodes.pop();
  for (let i = 0; i < nsCount; i++) {
    const ns = "ns" + i;
    N(ns + "/NetworkPolicy/deny", "NetworkPolicy", "deny", ns);
    N(ns + "/ServiceAccount/app", "ServiceAccount", "app", ns);
    for (let a = 0; a < appsPerNs; a++) {
      const app = "app" + a, d = ns + "/Deployment/" + app, rs = ns + "/ReplicaSet/" + app + "-x", svc = ns + "/Service/" + app, cm = ns + "/ConfigMap/" + app;
      N(d, "Deployment", app, ns); N(rs, "ReplicaSet", app + "-x", ns); N(svc, "Service", app, ns); N(cm, "ConfigMap", app, ns);
      E(rs, "controlled-by", d, "ownership", "ownership");
      for (let p = 0; p < podsPerApp; p++) {
        const pod = ns + "/Pod/" + app + "-" + p;
        N(pod, "Pod", app + "-" + p, ns, { status: p === 0 && a === 0 ? "crit" : "ok" });
        E(pod, "controlled-by", rs, "ownership", "ownership");
        E(pod, "runs-on", "/Node/w" + ((a + p) % 4 + 1), "scheduling");
        E(pod, "references", cm, "configuration");
        E(pod, "uses-service-account", ns + "/ServiceAccount/app", "configuration");
        E(svc, "selects", pod, "networking", "selector");
        E(ns + "/NetworkPolicy/deny", "applies-to", pod, "networking", "selector");
      }
      if (a === 0) {
        const pvc = ns + "/PersistentVolumeClaim/data", pv = "/PersistentVolume/pv-" + ns;
        N(pvc, "PersistentVolumeClaim", "data", ns); N(pv, "PersistentVolume", "pv-" + ns, "");
        E(ns + "/Pod/" + app + "-0", "claims", pvc, "storage");
        E(pvc, "bound-to", pv, "storage"); E(pv, "storage-class", "/StorageClass/fast", "storage");
      }
    }
    if (i > 0) { N(ns + "/RoleBinding/mon", "RoleBinding", "mon", ns); E(ns + "/RoleBinding/mon", "grants-to", "ns0/ServiceAccount/app", "security"); }
  }
  N("ns0/ConfigMap/missing", "ConfigMap", "missing", "ns0", { resolved: false });
  E("ns0/Pod/app0-0", "mounts", "ns0/ConfigMap/missing", "configuration");
  return { meta: { context: "test", revision: 1, namespaceList: Array.from({ length: nsCount }, (_, i) => "ns" + i) }, nodes, edges };
}

function storeOf(dto) { const s = G.createStore(); s.load(JSON.parse(JSON.stringify(dto))); return s; }
function expandAll(store, st) { const P = G.project(store, st); P.groups.forEach(g => st.expanded[st.view].set(g.key, true)); return G.project(store, st); }

function boxes(L) {
  return Array.from(L.pos.entries()).map(([id, p]) => ({ id, x0: p.x - p.w / 2, y0: p.y - p.h / 2, x1: p.x + p.w / 2, y1: p.y + p.h / 2 }));
}
function overlapping(list) {
  const out = []; const s = list.slice().sort((a, b) => a.x0 - b.x0);
  for (let i = 0; i < s.length; i++) for (let j = i + 1; j < s.length && s[j].x0 < s[i].x1; j++) {
    if (s[i].y0 < s[j].y1 && s[j].y0 < s[i].y1) out.push([s[i].id, s[j].id]);
  }
  return out;
}
function segHitsBox(a, b, r) {
  // Axis-aligned segment against a box shrunk by 1px (touching is fine).
  const x0 = r.x0 + 1, x1 = r.x1 - 1, y0 = r.y0 + 1, y1 = r.y1 - 1;
  if (Math.abs(a.x - b.x) < 0.01) { const lo = Math.min(a.y, b.y), hi = Math.max(a.y, b.y); return a.x > x0 && a.x < x1 && hi > y0 && lo < y1; }
  if (Math.abs(a.y - b.y) < 0.01) { const lo = Math.min(a.x, b.x), hi = Math.max(a.x, b.x); return a.y > y0 && a.y < y1 && hi > x0 && lo < x1; }
  return false;
}

const VIEWS = ["namespace", "node", "workload"];

test("store: load, indexes and deltas", () => {
  const s = storeOf(synth(2, 2, 2));
  assert.ok(s.nodes.size > 0);
  const pod = "ns0/Pod/app0-0";
  assert.equal(s.nodeOfPod(pod), "/Node/w1");
  assert.equal(s.rootOf(pod), "ns0/Deployment/app0");
  const before = s.nodes.size;
  const res = s.apply({ revision: 2, meta: { revision: 2 }, delta: {
    upsertNodes: [{ id: "ns0/Pod/app0-9", kind: "Pod", name: "app0-9", ns: "ns0", resolved: true }],
    removeNodes: ["ns1/Pod/app1-1"],
    addEdges: [{ from: "ns0/Pod/app0-9", to: "/Node/w3", type: "runs-on" }],
    removeEdges: [{ from: pod, to: "/Node/w1", type: "runs-on" }],
  } });
  assert.equal(s.revision, 2);
  assert.equal(s.nodes.size, before);
  assert.ok(res.changedIds.has("ns0/Pod/app0-9") && res.changedIds.has("ns1/Pod/app1-1"));
  assert.equal(s.nodeOfPod(pod), null, "removed runs-on edge must be gone");
  assert.equal(s.edges.get("ns0/Pod/app0-9|runs-on|/Node/w3").category, "scheduling", "category falls back from type");
  assert.equal(s.apply({ revision: 2, unchanged: true }).changedIds.size, 0);
});

test("projection: every visible resource is drawn in exactly one group", () => {
  const s = storeOf(synth(4, 3, 3));
  for (const view of VIEWS) {
    const st = G.createViewState(); st.view = view;
    const P = expandAll(s, st);
    const seen = new Map();
    P.member.forEach((m, id) => { assert.ok(!seen.has(id), view + ": " + id + " in two groups"); seen.set(id, m.group); });
    P.vnodes.forEach(v => { if (v.type === "resource") assert.equal(seen.get(v.vid), v.group); });
  }
});

test("projection: cluster-scoped resources never sit in a namespace group", () => {
  const s = storeOf(synth(3, 2, 2));
  const st = G.createViewState();
  const P = expandAll(s, st);
  s.nodes.forEach(n => {
    if (n.ns) return;
    const m = P.member.get(n.id); assert.ok(m, n.id + " not projected");
    assert.equal(m.group, "infra", n.id + " must be in cluster infrastructure");
  });
});

test("projection: a Pod is drawn once — node is a badge in the namespace view, namespace a badge in the node view", () => {
  const s = storeOf(synth(2, 2, 2));
  const pod = "ns1/Pod/app1-1";
  const st = G.createViewState();
  let P = expandAll(s, st);
  P.vnodes.forEach(v => { if (v.type === "resource") assert.notEqual(v.vid === pod && v.group !== "ns:ns1", true); });
  assert.deepEqual(P.vnodes.get(pod).badges.find(b => b.kind === "node"), { kind: "node", text: "w3" });
  st.view = "node"; P = expandAll(s, st);
  assert.equal(P.member.get(pod).group, "node:/Node/w3");
  assert.ok(P.vnodes.get(pod).badges.some(b => b.kind === "ns" && b.text === "ns1"));
});

test("projection: bundling and filtering never alter the canonical graph", () => {
  const s = storeOf(synth(3, 3, 3));
  const nodesBefore = s.nodes.size, edgesBefore = s.edges.size;
  const st = G.createViewState(); st.view = "namespace";
  const P = G.project(s, st); // overview: everything bundled into group cards
  const drawnCanonical = P.vedges.reduce((a, e) => a + e.count, 0) + P.summarized.reduce((a, e) => a + e.count, 0);
  const rep = id => { const m = P.member.get(id); if (!m) return null; const g = P.groups.find(x => x.key === m.group); return g.collapsed ? "group:" + g.key : m.vid; };
  const expected = Array.from(s.edges.values()).filter(e => st.relations.namespace.has(e.category))
    .filter(e => { const a = rep(e.from), b = rep(e.to); return a && b && a !== b; }).length;
  assert.equal(drawnCanonical, expected, "every enabled canonical edge between two different cards is represented exactly once");
  // And at the overview level (all groups collapsed) the same holds for bundles between group cards.
  P.groups.forEach(g => st.expanded.namespace.set(g.key, false));
  const O = G.project(s, st);
  assert.ok(O.vedges.every(e => e.from.startsWith("group:") && e.to.startsWith("group:")));
  const crossCanonical = Array.from(s.edges.values()).filter(e => st.relations.namespace.has(e.category))
    .filter(e => { const a = O.member.get(e.from), b = O.member.get(e.to); return a && b && a.group !== b.group; }).length;
  assert.equal(O.vedges.reduce((a, e) => a + e.count, 0), crossCanonical, "overview bundles account for every cross-group relationship");
  st.hiddenKinds.add("Pod"); st.namespaces = new Set(["ns1"]);
  G.project(s, st);
  assert.equal(s.nodes.size, nodesBefore); assert.equal(s.edges.size, edgesBefore);
});

test("projection: large fan-outs fold into aggregates and hubs summarize instead of drawing", () => {
  const s = storeOf(synth(1, 3, 8));
  const st = G.createViewState();
  const P = expandAll(s, st);
  const agg = Array.from(P.vnodes.values()).filter(v => v.type === "aggregate" && v.kind === "Pod");
  assert.equal(agg.length, 3, "8 Pods under one owner fold");
  // Unfolding one restores its Pods individually.
  st.unfolded.add(agg[0].vid);
  const P2 = G.project(s, st);
  assert.equal(Array.from(P2.vnodes.values()).filter(v => v.type === "resource" && v.node.kind === "Pod").length, 8);
  // The NetworkPolicy selecting all 24 Pods is a hub: summarized on its card.
  st.unfolded = new Set(agg.map(a => a.vid));
  const P3 = G.project(s, st);
  const np = P3.vnodes.get("ns0/NetworkPolicy/deny");
  assert.ok(np.hubs && np.hubs.some(h => h.count === 24), "hub summary on the NetworkPolicy");
  assert.ok(P3.summarized.length > 0);
  assert.ok(!P3.vedges.some(e => e.from === "ns0/NetworkPolicy/deny"), "hub edges are not drawn individually");
});

test("projection: dependency view walks the ownership unit, in and out", () => {
  const s = storeOf(synth(2, 2, 2));
  const st = G.createViewState(); st.view = "dependency"; st.focus = "ns0/Deployment/app0"; st.depth = 1;
  const P = G.project(s, st);
  for (const id of ["ns0/Deployment/app0", "ns0/ReplicaSet/app0-x", "ns0/Pod/app0-0", "ns0/ConfigMap/app0", "ns0/Service/app0", "/Node/w1"]) {
    assert.ok(P.member.has(id), id + " should be in the dependency neighbourhood");
  }
  assert.ok(!P.member.has("ns1/Deployment/app0"), "unrelated workload must not appear");
  st.direction = "out";
  assert.ok(!G.project(s, st).member.has("ns0/Service/app0"), "'depends on' excludes what selects it");
});

test("layout: no overlapping cards or groups, in every view and at overview and expanded levels", () => {
  const s = storeOf(synth(6, 4, 4));
  for (const view of VIEWS) for (const expand of [false, true]) {
    const st = G.createViewState(); st.view = view;
    const P = expand ? expandAll(s, st) : G.project(s, st);
    const L = G.computeLayout(P);
    assert.deepEqual(overlapping(boxes(L)), [], view + (expand ? " expanded" : " overview") + ": overlapping cards");
    assert.deepEqual(overlapping(L.groups.map(g => ({ id: g.key, x0: g.x0, y0: g.y0, x1: g.x1, y1: g.y1 }))), [], view + ": overlapping groups");
  }
});

test("layout: cross-group edges are axis-aligned and never pass through a card", () => {
  const s = storeOf(synth(5, 3, 3));
  const st = G.createViewState(); st.relations.namespace.add("security");
  const P = expandAll(s, st);
  const L = G.computeLayout(P);
  const cards = boxes(L);
  let checked = 0;
  P.vedges.filter(e => e.cross).forEach(e => {
    const pts = L.routes.get(e.key); assert.ok(pts, "route for " + e.key);
    for (let i = 1; i < pts.length; i++) {
      const a = pts[i - 1], b = pts[i];
      assert.ok(Math.abs(a.x - b.x) < 0.01 || Math.abs(a.y - b.y) < 0.01, "segment not axis-aligned in " + e.key);
      cards.forEach(c => { if (c.id !== e.from && c.id !== e.to) assert.ok(!segHitsBox(a, b, c), e.key + " crosses card " + c.id); });
      checked++;
    }
  });
  assert.ok(checked > 10, "expected cross-group routes to check");
});

test("layout: deterministic — same input, same picture", () => {
  const dto = synth(4, 3, 3);
  const run = () => { G._internal.groupCache.clear(); const s = storeOf(dto); const st = G.createViewState(); const L = G.computeLayout(expandAll(s, st)); return JSON.stringify(Array.from(L.pos.entries())) + JSON.stringify(Array.from(L.routes.entries())); };
  assert.equal(run(), run());
});

test("layout: a change in one namespace does not move cards in another", () => {
  const dto = synth(4, 3, 3);
  const s = storeOf(dto);
  const st = G.createViewState();
  const L1 = G.computeLayout(expandAll(s, st));
  s.apply({ revision: 2, delta: { upsertNodes: [{ id: "ns3/Pod/app2-new", kind: "Pod", name: "app2-new", ns: "ns3", resolved: true }],
    addEdges: [{ from: "ns3/Pod/app2-new", to: "ns3/ReplicaSet/app2-x", type: "controlled-by" }] } });
  const L2 = G.computeLayout(G.project(s, st));
  // Cards keep their position relative to their group (the group itself may
  // only translate as a whole if a neighbour changed size).
  const rel = (L, id) => { const p = L.pos.get(id), g = L.groups.find(x => x.key === p.group); return [p.x - g.x0, p.y - g.y0]; };
  for (const id of ["ns0/Pod/app0-0", "ns1/Service/app1", "ns2/Deployment/app2"]) assert.deepEqual(rel(L2, id), rel(L1, id), id + " moved inside its group");
  // A status-only change moves nothing at all.
  s.apply({ revision: 3, delta: { upsertNodes: [Object.assign({}, s.node("ns1/Pod/app1-1"), { status: "crit" })] } });
  const L3 = G.computeLayout(G.project(s, st));
  assert.deepEqual(L3.pos.get("ns1/Pod/app1-1"), L2.pos.get("ns1/Pod/app1-1"));
});

test("expand/collapse and reveal", () => {
  const s = storeOf(synth(8, 4, 4)); // large enough to open as an overview
  const st = G.createViewState();
  let P = G.project(s, st);
  assert.ok(P.groups.every(g => g.collapsed), "large cluster opens collapsed");
  assert.equal(P.stats.rendered, 0);
  const target = "ns5/Pod/app3-2";
  assert.ok(G.reveal(st, P, target));
  P = G.project(s, st);
  assert.ok(!P.groups.find(g => g.key === "ns:ns5").collapsed);
  assert.equal(P.vnodes.get(target).type, "resource");
});

test("search: names, kind:, ns: and label: terms", () => {
  const s = storeOf(synth(2, 2, 2));
  s.node("ns1/Pod/app1-0").labels = { tier: "web" };
  assert.equal(G.search(s, "app1-0").total, 2);
  assert.equal(G.search(s, "kind:pod ns:ns1").total, 4);
  assert.equal(G.search(s, "label:tier=web").results[0].id, "ns1/Pod/app1-0");
  assert.equal(G.search(s, "").total, 0);
});

test("real captured fixtures: every scenario lays out without overlap", () => {
  for (const sc of fs.readdirSync(FIXTURES).filter(f => f.endsWith(".json"))) {
    const dto = JSON.parse(fs.readFileSync(path.join(FIXTURES, sc), "utf8"));
    const s = storeOf(dto);
    for (const view of VIEWS) {
      const st = G.createViewState(); st.view = view;
      const L = G.computeLayout(expandAll(s, st));
      assert.deepEqual(overlapping(boxes(L)), [], sc + "/" + view);
    }
  }
});

test("report layout: small clusters get one diagram, large ones an overview plus every namespace, nothing dropped", () => {
  const R = require("../../internal/web/ui/report-layout.js");
  const small = storeOf(synth(2, 2, 2));
  const one = R.build(small);
  assert.equal(one.partitioned, false);
  assert.equal(one.pages.length, 1);
  assert.equal(one.pages[0].nodes.filter(n => n.type === "resource").length, small.nodes.size, "every resource drawn on the single diagram");

  const big = storeOf(synth(12, 8, 6));
  const many = R.build(big);
  assert.equal(many.partitioned, true);
  assert.equal(many.pages[0].kind, "overview");
  const covered = new Set();
  many.pages.slice(1).forEach(p => p.nodes.forEach(n => {
    if (n.type === "resource") covered.add(n.id);
    if (n.type === "aggregate") n.members.forEach(m => covered.add(m));
  }));
  big.nodes.forEach(n => { if (n.ns) assert.ok(covered.has(n.id), n.id + " missing from every namespace diagram"); });
  many.pages.forEach(p => {
    p.edges.forEach(e => e.keys.forEach(k => assert.ok(big.edges.has(k), "edge key " + k + " not canonical")));
    assert.ok(p.bounds[2] > p.bounds[0] && p.bounds[3] > p.bounds[1]);
  });
});
