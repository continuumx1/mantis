// capture.js — records the Playground's data from a running mantis-dev
// serving a synthetic cluster (dev-only; nothing here ships in a request
// path). Every response is exactly what the engine and the report service
// returned, so the Playground runs the real app against real output.
//
//   go run ./cmd/mantis-dev -resources 110 -seed 8 -addr 127.0.0.1:8090
//   node internal/web/ui/playground/capture.js http://127.0.0.1:8090
//
// Writes, next to this file:
//   api.json           graph, events, capacity, and every resource's YAML/JSON
//   sample-report.pdf  a report of the same cluster, encrypted with the
//                      Playground password below
"use strict";
const fs = require("fs");
const path = require("path");
const G = require("../graph-engine.js");
const R = require("../report-layout.js");

const PASSWORD = "mantis-playground";
const base = (process.argv[2] || "http://127.0.0.1:8090").replace(/\/$/, "");
const out = __dirname;

async function get(url) {
  const r = await fetch(base + url);
  return { status: r.status, type: r.headers.get("content-type") || "text/plain", body: await r.text() };
}
async function getJSON(url) {
  const r = await get(url);
  if (r.status !== 200) throw new Error(url + " -> " + r.status);
  return JSON.parse(r.body);
}

(async () => {
  const graph = await getJSON("/api/graph");
  const events = await getJSON("/api/events?limit=500");
  const capacity = await getJSON("/api/cluster/capacity");

  // Every resource the UI can open, in both formats it asks for.
  const resources = {};
  for (const n of graph.nodes) {
    if (n.resolved === false) continue;
    for (const format of ["", "json"]) {
      const p = new URLSearchParams({ kind: n.kind, name: n.name });
      if (n.ns) p.set("ns", n.ns);
      if (format) p.set("format", format);
      resources[[n.kind, n.ns || "", n.name, format].join("|")] = await get("/api/resource?" + p.toString());
    }
  }

  // A real report of the same cluster, laid out exactly as the browser does.
  const store = G.createStore();
  store.load(JSON.parse(JSON.stringify(graph)));
  const layout = R.build(store);
  const resp = await fetch(base + "/api/report", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ revision: layout.revision, password: PASSWORD, contents: { topology: true, inventory: true, health: true, findings: true }, pages: layout.pages }),
  });
  const ready = (await resp.text()).trim().split("\n").map(l => JSON.parse(l)).find(e => e.stage === "ready");
  if (!ready) throw new Error("report generation failed");
  fs.writeFileSync(path.join(out, "sample-report.pdf"), Buffer.from(ready.pdf, "base64"));

  const data = {
    capturedAt: new Date().toISOString(),
    graph,
    events,
    capacity,
    resources,
    report: { password: PASSWORD, filename: ready.filename, bytes: ready.bytes },
  };
  fs.writeFileSync(path.join(out, "api.json"), JSON.stringify(data));
  console.log(`captured ${graph.nodes.length} resources, ${(events.events || []).length} events, ${Object.keys(resources).length} manifests, report ${ready.bytes} bytes`);
})().catch(e => { console.error(e.message || e); process.exit(1); });
