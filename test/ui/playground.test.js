// The Playground is the real app plus playground-api.js answering /api/*
// from recorded data. These tests keep the page in sync with index.html and
// check the shim answers the way the engine and report service do.
"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const path = require("path");
const vm = require("vm");

const UI = path.join(__dirname, "../../internal/web/ui");
const { build } = require(path.join(UI, "playground/build.js"));
const data = JSON.parse(fs.readFileSync(path.join(UI, "playground/api.json"), "utf8"));

test("playground.html is generated from the current index.html", () => {
  const index = fs.readFileSync(path.join(UI, "index.html"), "utf8");
  const page = fs.readFileSync(path.join(UI, "playground.html"), "utf8");
  assert.equal(page, build(index), "run: node internal/web/ui/playground/build.js");
  for (const src of [...page.matchAll(/(?:src|href)="([^":#]+)"/g)].map(m => m[1])) {
    assert.ok(fs.existsSync(path.join(UI, src)), "playground.html references missing " + src);
  }
});

test("recorded data covers every resource the app can open", () => {
  assert.ok(data.graph.nodes.length >= 95 && data.graph.nodes.length <= 110, "sample cluster of about 100 resources");
  for (const n of data.graph.nodes) {
    if (n.resolved === false) continue;
    for (const format of ["", "json"]) {
      const r = data.resources[[n.kind, n.ns || "", n.name, format].join("|")];
      assert.ok(r, "no recorded manifest for " + n.id + " " + format);
      assert.equal(r.status, n.kind === "Secret" ? 403 : 200, n.id);
    }
  }
  assert.ok(data.capacity.metrics && data.capacity.cpuUsageMilli > 0, "node metrics recorded");
  assert.ok((data.events.events || []).some(e => e.type === "Warning"), "a warning event to show");
});

// Runs playground-api.js against a stand-in window whose real fetch serves
// files from internal/web/ui.
function shim() {
  const realFetch = async url => {
    const file = path.join(UI, String(url));
    return new Response(fs.readFileSync(file));
  };
  const window = { fetch: realFetch };
  const location = { href: "https://mantis.example/mantis-playground/playground.html", origin: "https://mantis.example" };
  vm.runInNewContext(fs.readFileSync(path.join(UI, "playground-api.js"), "utf8"), {
    window, location, Response, URL, TextEncoder, ReadableStream, DOMException, setTimeout, btoa, Date, JSON, Math, Promise,
  });
  return window.fetch;
}

test("the shim answers the app's API like the engine does", async () => {
  const f = shim();
  const g = await (await f("/api/graph")).json();
  assert.equal(g.nodes.length, data.graph.nodes.length);
  const d = await (await f("/api/graph?since=" + (g.meta.revision || 1))).json();
  assert.equal(d.unchanged, true);
  const dep = data.graph.nodes.find(n => n.kind === "Deployment");
  const y = await f(`/api/resource?kind=Deployment&name=${dep.name}&ns=${dep.ns}`);
  assert.equal(y.status, 200);
  assert.match(await y.text(), /kind: Deployment/);
  const sec = data.graph.nodes.find(n => n.kind === "Secret");
  assert.equal((await f(`/api/resource?kind=Secret&name=${sec.name}&ns=${sec.ns}`)).status, 403);
  assert.equal((await (await f("/api/session")).json()).username, "playground");
  assert.equal((await f("/api/unknown")).status, 404);
  const events = (await (await f("/api/events?limit=500")).json()).events;
  const newest = Math.max(...events.map(e => Date.parse(e.last)));
  assert.ok(Date.now() - newest < 10 * 60 * 1000, "event times rebased to recent");
  const created = g.nodes.map(n => Date.parse(n.created)).filter(t => !isNaN(t));
  const age = Date.now() - Math.max(...created);
  assert.ok(age > 86400e3 && age < 3 * 86400e3, "creation times rebased to days old");
  assert.ok(!/2026-01-01/.test(await (await f(`/api/resource?kind=Deployment&name=${dep.name}&ns=${dep.ns}`)).text()), "manifest timestamps rebased too");
});

test("the shim's report: the Playground password downloads the recorded PDF", async () => {
  const f = shim();
  const wrong = await f("/api/report", { method: "POST", body: JSON.stringify({ password: "something-else" }) });
  assert.equal(wrong.status, 400);
  assert.match((await wrong.json()).error, /mantis-playground/);
  const ok = await f("/api/report", { method: "POST", body: JSON.stringify({ password: data.report.password }) });
  const lines = (await ok.text()).trim().split("\n").map(l => JSON.parse(l));
  assert.deepEqual(lines.map(l => l.stage), ["building-topology", "rendering-topology", "generating-inventory", "rendering-pdf", "encrypting", "ready"]);
  const pdf = Buffer.from(lines.at(-1).pdf, "base64");
  assert.ok(pdf.subarray(0, 5).toString() === "%PDF-" && pdf.includes("/Encrypt"), "an encrypted PDF");
});
