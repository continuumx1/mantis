// playground-api.js — lets the real Mantis app run with no server.
//
// playground.html is index.html plus this file, loaded before the app. It
// answers the app's /api/* requests from playground/api.json: responses
// recorded from mantis-engine and mantis-report for a synthetic sample
// cluster (see playground/capture.js). The app code is unchanged.
(function () {
  "use strict";
  const realFetch = window.fetch.bind(window);
  const DATA_URL = "playground/api.json";
  const REPORT_URL = "playground/sample-report.pdf";
  let data = null;
  const ready = realFetch(DATA_URL).then(r => r.json()).then(d => {
    data = d;
    rebaseEvents(d);
    return d;
  });

  // Recorded event times are shifted so the newest one is a few minutes
  // old whenever the Playground is opened, as it would be on a live cluster.
  function rebaseEvents(d) {
    const evs = (d.events && d.events.events) || [];
    const times = evs.map(e => Date.parse(e.last)).filter(t => !isNaN(t));
    if (!times.length) return;
    const shift = Date.now() - 4 * 60 * 1000 - Math.max.apply(null, times);
    evs.forEach(e => { const t = Date.parse(e.last); if (!isNaN(t)) e.last = new Date(t + shift).toISOString(); });
  }

  const json = (body, status) => new Response(JSON.stringify(body), { status: status || 200, headers: { "Content-Type": "application/json" } });
  const text = (body, status, type) => new Response(body, { status: status || 200, headers: { "Content-Type": type || "text/plain" } });
  const sleep = ms => new Promise(r => setTimeout(r, ms));

  async function report(init) {
    let req = {};
    try { req = JSON.parse((init && init.body) || "{}"); } catch (e) { /* empty */ }
    const pw = data.report.password;
    if (req.password !== pw) {
      return json({ error: "In the Playground, the report is a real one generated in advance from this sample cluster, protected with the password “" + pw + "”. Enter that password to download it. In your own cluster you choose the password." }, 400);
    }
    const pdf = new Uint8Array(await (await realFetch(REPORT_URL)).arrayBuffer());
    let bin = "";
    for (let i = 0; i < pdf.length; i += 0x8000) bin += String.fromCharCode.apply(null, pdf.subarray(i, i + 0x8000));
    const signal = init && init.signal;
    const enc = new TextEncoder();
    const stages = ["building-topology", "rendering-topology", "generating-inventory", "rendering-pdf", "encrypting"];
    const body = new ReadableStream({
      async start(ctl) {
        for (const stage of stages) {
          if (signal && signal.aborted) { ctl.error(new DOMException("aborted", "AbortError")); return; }
          ctl.enqueue(enc.encode(JSON.stringify({ stage }) + "\n"));
          await sleep(350);
        }
        ctl.enqueue(enc.encode(JSON.stringify({ stage: "ready", filename: data.report.filename, bytes: pdf.length, pdf: btoa(bin) }) + "\n"));
        ctl.close();
      },
    });
    return new Response(body, { status: 200, headers: { "Content-Type": "application/x-ndjson" } });
  }

  async function answer(url, init) {
    await ready;
    const p = url.pathname, q = url.searchParams;
    if (p === "/api/graph") {
      const since = q.get("since");
      if (since) return json({ meta: data.graph.meta, since: Number(since), revision: data.graph.meta.revision || 1, unchanged: true });
      return json(data.graph);
    }
    if (p === "/api/events") return json(data.events);
    if (p === "/api/cluster/capacity") return json(data.capacity);
    if (p === "/api/session") return json({ username: "playground", role: "Administrator" });
    if (p === "/api/report") return report(init);
    if (p === "/api/resource") {
      const key = [q.get("kind"), q.get("ns") || "", q.get("name"), q.get("format") || ""].join("|");
      const r = data.resources[key];
      return r ? text(r.body, r.status, r.type) : text("not found in the Playground's sample data", 404);
    }
    return json({ error: "not available in the Playground" }, 404);
  }

  window.fetch = function (input, init) {
    const url = new URL(typeof input === "string" ? input : input.url, location.href);
    if (url.origin === location.origin && (url.pathname.startsWith("/api/") || url.pathname === "/logout")) {
      if (url.pathname === "/logout") return Promise.resolve(json({ ok: true }));
      return answer(url, init);
    }
    return realFetch(input, init);
  };
})();
