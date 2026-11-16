// Frontend engine benchmark over graph dumps from cmd/mantis-dev:
//   go run ./cmd/mantis-dev -resources 10000 -dump /tmp/g10k.json
//   node test/ui/bench.js /tmp/g100.json /tmp/g1k.json /tmp/g10k.json
// Reports projection + layout time per view, overview and fully expanded,
// and verifies no overlapping cards and no cross-group line through a card.
"use strict";
const fs = require("node:fs");
const G = require("../../internal/web/ui/graph-engine.js");

function boxes(L) { return Array.from(L.pos.entries()).map(([id, p]) => ({ id, x0: p.x - p.w / 2, y0: p.y - p.h / 2, x1: p.x + p.w / 2, y1: p.y + p.h / 2 })); }
function overlaps(list) {
  let n = 0; const s = list.slice().sort((a, b) => a.x0 - b.x0);
  for (let i = 0; i < s.length; i++) for (let j = i + 1; j < s.length && s[j].x0 < s[i].x1; j++) if (s[i].y0 < s[j].y1 && s[j].y0 < s[i].y1) n++;
  return n;
}
function crossings(P, L) {
  // Grid-bucket the cards so checking every routed segment stays fast.
  const B = 400, grid = new Map();
  boxes(L).forEach(b => { for (let gx = Math.floor(b.x0 / B); gx <= Math.floor(b.x1 / B); gx++) for (let gy = Math.floor(b.y0 / B); gy <= Math.floor(b.y1 / B); gy++) { const k = gx + "," + gy; if (!grid.has(k)) grid.set(k, []); grid.get(k).push(b); } });
  let hits = 0;
  P.vedges.filter(e => e.cross).forEach(e => {
    const pts = L.routes.get(e.key) || [];
    for (let i = 1; i < pts.length; i++) {
      const a = pts[i - 1], b = pts[i];
      const cand = new Set();
      for (let gx = Math.floor(Math.min(a.x, b.x) / B); gx <= Math.floor(Math.max(a.x, b.x) / B); gx++) for (let gy = Math.floor(Math.min(a.y, b.y) / B); gy <= Math.floor(Math.max(a.y, b.y) / B); gy++) (grid.get(gx + "," + gy) || []).forEach(c => cand.add(c));
      cand.forEach(c => {
        if (c.id === e.from || c.id === e.to) return;
        const x0 = c.x0 + 1, x1 = c.x1 - 1, y0 = c.y0 + 1, y1 = c.y1 - 1;
        if (Math.abs(a.x - b.x) < .01) { if (a.x > x0 && a.x < x1 && Math.max(a.y, b.y) > y0 && Math.min(a.y, b.y) < y1) hits++; }
        else if (Math.abs(a.y - b.y) < .01) { if (a.y > y0 && a.y < y1 && Math.max(a.x, b.x) > x0 && Math.min(a.x, b.x) < x1) hits++; }
      });
    }
  });
  return hits;
}
for (const f of process.argv.slice(2)) {
  const dto = JSON.parse(fs.readFileSync(f, "utf8"));
  const s = G.createStore();
  let t = performance.now(); s.load(dto); const loadMs = performance.now() - t;
  console.log(`\n${f}: ${dto.nodes.length} resources, ${dto.edges.length} relationships (store load ${loadMs.toFixed(0)} ms)`);
  t = performance.now(); for (let i = 0; i < 20; i++) G.search(s, "api"); console.log(`  search "api": ${((performance.now() - t) / 20).toFixed(2)} ms`);
  for (const view of ["namespace", "node", "workload"]) for (const mode of ["overview", "expanded"]) {
    G._internal.groupCache.clear();
    const st = G.createViewState(); st.view = view;
    if (mode === "expanded") G.project(s, st).groups.forEach(g => st.expanded[view].set(g.key, true));
    t = performance.now(); const P = G.project(s, st); const pMs = performance.now() - t;
    t = performance.now(); const L = G.computeLayout(P); const lMs = performance.now() - t;
    t = performance.now(); G.computeLayout(G.project(s, st)); const cached = performance.now() - t;
    console.log(`  ${view.padEnd(9)} ${mode.padEnd(8)} drawn ${String(P.stats.rendered).padStart(5)} cards ${String(P.vedges.length).padStart(5)} lines | project ${pMs.toFixed(0).padStart(4)} ms layout ${lMs.toFixed(0).padStart(4)} ms (cached re-layout ${cached.toFixed(0)} ms) | overlaps ${overlaps(boxes(L))} line-through-card ${crossings(P, L)}`);
  }
  const focus = Array.from(s.nodes.values()).find(n => n.kind === "Deployment");
  const st = G.createViewState(); st.view = "dependency"; st.focus = focus.id;
  t = performance.now(); const P = G.project(s, st); G.computeLayout(P);
  console.log(`  dependency (${focus.id}): ${P.stats.rendered} cards in ${(performance.now() - t).toFixed(1)} ms`);
}
