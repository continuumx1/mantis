// graph-view.js — the Mantis graph UI, shared by index.html (live engine)
// and playground.html (static fixtures). Everything page-specific — where the
// graph comes from, login/sign-out, the playground's scenarios and tour — is
// passed in through MantisApp.mount(options); everything else lives here, so
// the two pages cannot drift apart.
//
// It renders what graph-engine.js computes: projection + deterministic layout.
// It never moves a card on its own. Only the camera pans and zooms, and only
// cards and edges inside (or near) the viewport are in the DOM at all.

(function (global) {
  "use strict";
  const G = global.MantisGraph;
  const SVGNS = "http://www.w3.org/2000/svg";
  const el = (t, a) => { const e = document.createElementNS(SVGNS, t); if (a) for (const k in a) e.setAttribute(k, a[k]); return e; };
  const esc = s => String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  const css = v => getComputedStyle(document.documentElement).getPropertyValue(v).trim();
  const fmt = n => Number(n).toLocaleString("en-US");
  const $ = id => document.getElementById(id);
  const VIEW_LABEL = { namespace: "Namespaces", node: "Nodes", workload: "Workloads", dependency: "Dependencies" };

  function mount(opts) {
    const source = opts.source;
    const svg = $("canvas");
    svg.innerHTML = "";
    const defs = el("defs");
    const gView = el("g", { id: "viewport" });
    const layers = {};
    ["groups", "edges", "labels", "nodes"].forEach(k => { layers[k] = el("g", { class: "layer-" + k }); gView.appendChild(layers[k]); });
    svg.appendChild(defs); svg.appendChild(gView);
    G.CATEGORIES.concat(["ghost"]).forEach(c => {
      const m = el("marker", { id: "arr-" + c, viewBox: "0 0 10 10", refX: "9", refY: "5", markerWidth: "6.5", markerHeight: "6.5", orient: "auto-start-reverse" });
      m.appendChild(el("path", { d: "M0,1 L9,5 L0,9 z", style: "fill:var(" + (c === "ghost" ? "--crit" : "--rel-" + c) + ")" }));
      defs.appendChild(m);
    });

    const panel = $("panel"), panelContent = $("panelContent"), overlay = $("overlay"), toast = $("toast");
    const store = G.createStore();
    const st = G.createViewState();
    let P = null, Lay = null;
    let selected = null, curTab = "summary", yamlToken = 0, everLoaded = false;
    let lastChange = null, prevView = "namespace";
    const W = () => svg.clientWidth || 1000, H = () => svg.clientHeight || 640;

    // ------------------------------------------------------------ chrome
    buildViewbar();
    buildLegend();
    const statusbar = document.createElement("div"); statusbar.className = "statusbar"; statusbar.id = "statusbar";
    svg.parentNode.appendChild(statusbar);

    function buildViewbar() {
      let bar = $("viewbar");
      if (!bar) { bar = document.createElement("div"); bar.id = "viewbar"; bar.className = "viewbar"; svg.parentNode.appendChild(bar); }
      bar.innerHTML =
        '<button class="btn back-btn" id="vbBack" title="Back to the previous view">← Back</button>' +
        '<div class="seg" id="vbViews" role="tablist">' + G.VIEWS.map(v => '<button role="tab" data-view="' + v + '">' + VIEW_LABEL[v] + '</button>').join("") + '</div>' +
        '<div class="dep-ctl" id="vbDep">' +
          '<select id="vbDir" title="Direction"><option value="both">in + out</option><option value="out">depends on</option><option value="in">used by</option></select>' +
          '<select id="vbDepth" title="Depth"><option value="1">1 hop</option><option value="2" selected>2 hops</option><option value="3">3 hops</option></select>' +
        '</div>' +
        '<div class="filters-wrap"><button class="btn" id="vbFilters" aria-haspopup="true">Filters ▾</button><div class="filters-pop" id="vbPop"></div></div>';
      bar.querySelectorAll("[data-view]").forEach(b => b.addEventListener("click", () => setView(b.getAttribute("data-view"))));
      $("vbBack").addEventListener("click", () => setView(prevView === "dependency" ? "namespace" : prevView));
      $("vbDir").addEventListener("change", ev => { st.direction = ev.target.value; render(); });
      $("vbDepth").addEventListener("change", ev => { st.depth = parseInt(ev.target.value, 10) || 2; render(); });
      $("vbFilters").addEventListener("click", ev => { ev.stopPropagation(); const p = $("vbPop"); if (!p.classList.contains("open")) renderFilters(); p.classList.toggle("open"); });
      document.addEventListener("click", ev => { if (!ev.target.closest(".filters-wrap")) $("vbPop").classList.remove("open"); });
    }
    function syncViewbar() {
      document.querySelectorAll("#vbViews [data-view]").forEach(b => { const on = b.getAttribute("data-view") === st.view; b.classList.toggle("active", on); b.setAttribute("aria-selected", String(on)); });
      $("vbDep").classList.toggle("show", st.view === "dependency");
      $("vbBack").classList.toggle("show", st.view === "dependency");
    }
    function renderFilters() {
      const pop = $("vbPop");
      const rel = st.relations[st.view];
      const kinds = new Map();
      store.nodes.forEach(n => kinds.set(n.kind, (kinds.get(n.kind) || 0) + 1));
      const nsCounts = new Map();
      store.nodes.forEach(n => { if (n.ns) nsCounts.set(n.ns, (nsCounts.get(n.ns) || 0) + 1); });
      const lineStyle = c => ({ configuration: "dashed", storage: "dashed", scheduling: "dotted", security: "dotted", autoscaling: "dashed" }[c] || "solid");
      pop.innerHTML =
        '<h5>Relationships <span style="text-transform:none;letter-spacing:0">(' + esc(VIEW_LABEL[st.view]) + ' view)</span></h5><div class="chips">' +
        G.CATEGORIES.map(c => '<span class="chip' + (rel.has(c) ? " on" : "") + '" data-cat="' + c + '"><span class="ln" style="border-color:var(--rel-' + c + ');border-top-style:' + lineStyle(c) + '"></span>' + G.CATEGORY_LABEL[c] + '</span>').join("") + '</div>' +
        '<h5>Namespaces <button id="nsAll">' + (st.namespaces ? "show all" : "") + '</button></h5><div class="chk-list">' +
        store.namespaces().map(ns => '<label><input type="checkbox" data-ns="' + esc(ns) + '"' + (!st.namespaces || st.namespaces.has(ns) ? " checked" : "") + '>' + esc(ns) + '<span class="cnt">' + (nsCounts.get(ns) || 0) + '</span></label>').join("") + '</div>' +
        '<h5>Resource kinds <button id="kindsAll">' + (st.hiddenKinds.size ? "show all" : "") + '</button></h5><div class="chk-list">' +
        Array.from(kinds).sort((a, b) => G.cmpNode({ kind: a[0], name: "", id: "" }, { kind: b[0], name: "", id: "" })).map(([k, c]) => '<label><input type="checkbox" data-kind="' + esc(k) + '"' + (st.hiddenKinds.has(k) ? "" : " checked") + '>' + esc(k) + '<span class="cnt">' + c + '</span></label>').join("") + '</div>';
      pop.querySelectorAll("[data-cat]").forEach(ch => ch.addEventListener("click", () => {
        const c = ch.getAttribute("data-cat"); if (rel.has(c)) rel.delete(c); else rel.add(c); renderFilters(); render();
      }));
      pop.querySelectorAll("[data-ns]").forEach(cb => cb.addEventListener("change", () => {
        const all = store.namespaces();
        const set = st.namespaces ? new Set(st.namespaces) : new Set(all);
        if (cb.checked) set.add(cb.getAttribute("data-ns")); else set.delete(cb.getAttribute("data-ns"));
        st.namespaces = set.size === all.length ? null : set; renderFilters(); render();
      }));
      pop.querySelectorAll("[data-kind]").forEach(cb => cb.addEventListener("change", () => {
        const k = cb.getAttribute("data-kind"); if (cb.checked) st.hiddenKinds.delete(k); else st.hiddenKinds.add(k); renderFilters(); render();
      }));
      const nsAll = $("nsAll"); if (nsAll) nsAll.addEventListener("click", () => { st.namespaces = null; renderFilters(); render(); });
      const kAll = $("kindsAll"); if (kAll) kAll.addEventListener("click", () => { st.hiddenKinds.clear(); renderFilters(); render(); });
    }
    function buildLegend() {
      const body = document.querySelector("#legend .legend-body");
      if (!body) return;
      const row = (sw, label) => '<div class="row">' + sw + esc(label) + '</div>';
      const ln = (c, cls) => '<span class="ln ' + (cls || "") + '" style="border-color:var(--rel-' + c + ')"></span>';
      body.innerHTML = '<h4>Resource</h4>' +
        [["workload", "Workload"], ["pod", "Pod"], ["network", "Service / Ingress / Policy"], ["config", "ConfigMap / Secret"], ["storage", "Storage"], ["node", "Node"], ["security", "Identity / RBAC"], ["autoscale", "Autoscaler"]]
          .map(([c, l]) => row('<span class="sw" style="background:var(--cat-' + c + ')"></span>', l)).join("") +
        '<div class="divider"></div><h4>Relationship</h4>' +
        row(ln("ownership"), "ownership (ownerReference)") + row(ln("networking"), "networking (selector / endpoints / routes)") +
        row(ln("configuration", "dash"), "configuration (env, volumes, identity)") + row(ln("storage", "dash"), "storage (claims, binds, class)") +
        row(ln("scheduling", "dot"), "scheduling (runs on)") + row(ln("security", "dot"), "RBAC (binds role, grants to)") +
        row('<span class="ln dash" style="border-color:var(--crit)"></span>', "reference to something not found") +
        '<div class="divider"></div><p class="hint" style="font-size:11px;margin:0">Lines show what the Kubernetes API declares — ownership, references, selector matches and endpoint records. Mantis does not observe network traffic.</p>';
    }

    // ------------------------------------------------------------ data
    function showOverlay(title, bodyHTML, isErr) {
      overlay.style.display = "flex"; overlay.className = "overlay" + (isErr ? " err" : "");
      overlay.innerHTML = '<div class="card"><h3>' + esc(title) + '</h3>' + bodyHTML + '</div>';
    }
    const hideOverlay = () => { overlay.style.display = "none"; };
    const showToast = msg => { $("toastMsg").textContent = msg; toast.classList.add("show"); };
    const hideToast = () => toast.classList.remove("show");

    function applyPayload(payload, first) {
      const res = store.apply(payload);
      const meta = store.meta || {};
      const setText = (id, v) => { const e = $(id); if (e) e.textContent = v; };
      setText("clusterName", meta.context || "unknown");
      setText("nCount", fmt(store.nodes.size));
      setText("eCount", fmt(store.edges.size));
      setText("nsCount", meta.namespaces != null ? meta.namespaces : store.namespaces().length);
      const chip = $("autoscalerChip");
      if (chip) chip.innerHTML = meta.nodeAutoscaler ? '<span class="autoscale-badge">autoscaler <b>' + esc(meta.nodeAutoscaler) + '</b></span>' : "";
      if (!res.full && res.changedIds && !res.changedIds.size) return false; // nothing to redraw
      if (!res.full && res.changedIds) lastChange = { n: res.changedIds.size, at: Date.now() };
      if (selected && !store.node(selected) && !String(selected).startsWith("agg:")) selected = null;
      render({ fit: first });
      return true;
    }
    function load(first) {
      const since = first ? 0 : store.revision;
      return source.load(since).then(payload => { everLoaded = true; hideOverlay(); hideToast(); applyPayload(payload, first); return payload; });
    }
    // Deep links: #view=namespace&expand=ns:catalog,ns:shop|all&select=<id>
    // &focus=<id>&dir=in|out|both&depth=2 — so a specific picture of the
    // cluster can be shared, bookmarked, or captured in a screenshot.
    function applyHash() {
      const h = new URLSearchParams((location.hash || "").replace(/^#/, ""));
      if (![...h.keys()].length) return false;
      const v = h.get("view"); if (v && G.VIEWS.includes(v)) st.view = v;
      if (h.get("dir")) st.direction = h.get("dir");
      if (h.get("depth")) st.depth = parseInt(h.get("depth"), 10) || 2;
      if (h.get("focus")) st.focus = h.get("focus");
      if (h.get("unfold") === "all") st.unfoldAll = true;
      const ex = h.get("expand");
      if (ex) {
        if (ex === "all") { P = G.project(store, st); P.groups.forEach(g => st.expanded[st.view].set(g.key, true)); }
        else ex.split(",").forEach(k => st.expanded[st.view].set(k, true));
      }
      render({ fit: true });
      const sel = h.get("select");
      if (sel && store.node(sel)) {
        showPanel(true);
        if (G.reveal(st, P, sel)) render({ fit: true }); else fit(false);
        select(sel);
        if (h.get("zoom")) { const p = Lay.pos.get((P.member.get(sel) || {}).vid); if (p) { const pw = panel.offsetWidth + 28; view.k = 1; view.x = (W() - pw) / 2 - p.x; view.y = H() / 2 - p.y; applyView(); cull(true); } }
      }
      return true;
    }
    function firstLoad() {
      showOverlay(opts.loadingTitle || "Connecting to your cluster…", "<p>" + esc(opts.loadingText || "Reading resources and relationships.") + "</p>", false);
      return load(true).then(p => { applyHash(); return p; }).catch(err => {
        showOverlay(opts.errorTitle || "Cannot read the cluster", '<p>' + esc(String(err && err.message || err)) + '</p>' + (opts.errorHint ? '<p>' + esc(opts.errorHint) + '</p>' : ""), true);
        throw err;
      });
    }

    // ------------------------------------------------------------ render
    function render(o) {
      o = o || {};
      syncViewbar();
      if (st.view === "dependency" && !st.focus) {
        P = G.project(store, st); Lay = G.computeLayout(P);
        clearScene();
        statusbar.innerHTML = "Select a resource, then <b>Dependencies</b>, to see what it depends on and what depends on it.";
        return;
      }
      const t0 = performance.now();
      P = G.project(store, st);
      Lay = G.computeLayout(P);
      const t1 = performance.now();
      buildItems();
      if (o.fit) fit(false);
      cull(true);
      applySelection();
      updateStatus(t1 - t0);
      if (opts.onRender) opts.onRender();
    }
    function updateStatus(ms) {
      const s = P.stats;
      const drawn = P.vedges.length, canon = P.vedges.reduce((a, e) => a + e.count, 0);
      const folded = P.groups.filter(g => g.collapsed).length;
      let h = (s.rendered === 0 && folded
        ? 'Overview · <b>' + fmt(s.total) + '</b> resources in <b>' + folded + '</b> groups — click one to open it'
        : '<b>' + fmt(s.rendered) + '</b> of <b>' + fmt(s.total) + '</b> resources drawn' + (s.hidden ? ' · ' + fmt(s.hidden) + ' summarized or filtered' : '')) +
        ' · <b>' + fmt(drawn) + '</b> lines for ' + fmt(canon) + ' relationships' +
        (store.revision ? ' · rev ' + store.revision : '') + ' · layout ' + Math.round(ms) + ' ms';
      if (lastChange && Date.now() - lastChange.at < 8000) h += ' · <span class="chg">' + lastChange.n + ' changed</span>';
      if (store.meta && store.meta.partial) h += ' · <span class="chg">still reading the cluster…</span>';
      statusbar.innerHTML = h;
    }

    // Items are plain descriptors with a bounding box; DOM elements are only
    // created for items inside the (padded) viewport.
    let items = [], itemById = new Map(), domById = new Map();
    function clearScene() {
      items = []; itemById = new Map(); domById = new Map();
      Object.values(layers).forEach(l => { l.innerHTML = ""; });
    }
    function buildItems() {
      clearScene();
      const palette = document.documentElement.getAttribute("data-theme") === "light" ? G.NS_PALETTE_LIGHT : G.NS_PALETTE_DARK;
      const groupColor = g => (g.type === "infra" ? css("--ns-cluster") : g.type === "node" ? css("--cat-node") : G.hashColor(g.ns || g.label, palette));
      Lay.groups.forEach(gb => {
        if (gb.collapsed || gb.frameless) return;
        items.push({ id: "G:" + gb.key, layer: "groups", x0: gb.x0, y0: gb.y0, x1: gb.x1, y1: gb.y1, make: () => makeGroupFrame(gb, groupColor(gb.g)) });
      });
      P.vedges.forEach(e => {
        const pts = Lay.routes.get(e.key); if (!pts) return;
        let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
        pts.forEach(p => { x0 = Math.min(x0, p.x); y0 = Math.min(y0, p.y); x1 = Math.max(x1, p.x); y1 = Math.max(y1, p.y); });
        items.push({ id: "E:" + e.key, layer: "edges", x0, y0, x1, y1, e, pts, make: () => makeEdge(e, pts) });
      });
      P.vnodes.forEach(v => {
        const p = Lay.pos.get(v.vid); if (!p) return;
        const color = v.type === "groupcard" ? groupColor(v.groupRef) : null;
        items.push({ id: "N:" + v.vid, layer: "nodes", x0: p.x - p.w / 2, y0: p.y - p.h / 2, x1: p.x + p.w / 2, y1: p.y + p.h / 2, v, p, make: () => makeNode(v, p, color) });
      });
      items.forEach(it => itemById.set(it.id, it));
    }

    function makeGroupFrame(gb, color) {
      const g = el("g", { class: "grp " + gb.g.type, style: "--grp:" + color });
      g.appendChild(el("rect", { class: "grp-frame", x: gb.x0, y: gb.y0, width: gb.x1 - gb.x0, height: gb.y1 - gb.y0, rx: 12 }));
      const t = el("text", { class: "grp-title", x: gb.x0 + 14, y: gb.y0 + 20 });
      const kind = { namespace: "namespace · ", infra: "", node: "node · ", workload: "", shared: "", dependency: "" }[gb.g.type] || "";
      const room = Math.max(10, Math.floor((gb.x1 - gb.x0 - 60) / 6.7));
      const main = G.shorten(kind + gb.g.label, room);
      t.appendChild(document.createTextNode(main));
      if (gb.g.sub && main.length + gb.g.sub.length + 5 <= room) { const s = el("tspan", { class: "grp-sub" }); s.textContent = "  ·  " + gb.g.sub; t.appendChild(s); }
      const sum = gb.g.summary;
      if (sum && sum.podsBad) { const s = el("tspan", { style: "fill:var(--crit)" }); s.textContent = "  ·  " + sum.podsBad + " pod" + (sum.podsBad > 1 ? "s" : "") + " failing"; t.appendChild(s); }
      g.appendChild(t);
      const btn = el("g", { class: "grp-btn", "data-collapse": gb.key, transform: "translate(" + (gb.x1 - 26) + "," + (gb.y0 + 8) + ")" });
      btn.appendChild(el("rect", { width: 18, height: 18, rx: 5 }));
      const bt = el("text", { x: 9, y: 9.5 }); bt.textContent = "–"; btn.appendChild(bt);
      const title = el("title"); title.textContent = "Collapse " + gb.g.label; btn.appendChild(title);
      g.appendChild(btn);
      return g;
    }

    function roundedPath(pts, r) {
      if (pts.length < 2) return "";
      let d = "M" + pts[0].x + "," + pts[0].y;
      for (let i = 1; i < pts.length - 1; i++) {
        const a = pts[i - 1], b = pts[i], c = pts[i + 1];
        const l1 = Math.hypot(b.x - a.x, b.y - a.y), l2 = Math.hypot(c.x - b.x, c.y - b.y);
        const rr = Math.min(r, l1 / 2, l2 / 2);
        const p1 = { x: b.x + (a.x - b.x) * rr / (l1 || 1), y: b.y + (a.y - b.y) * rr / (l1 || 1) };
        const p2 = { x: b.x + (c.x - b.x) * rr / (l2 || 1), y: b.y + (c.y - b.y) * rr / (l2 || 1) };
        d += " L" + p1.x + "," + p1.y + " Q" + b.x + "," + b.y + " " + p2.x + "," + p2.y;
      }
      const z = pts[pts.length - 1];
      return d + " L" + z.x + "," + z.y;
    }
    function smoothPath(pts) {
      let d = "M" + pts[0].x + "," + pts[0].y;
      for (let i = 1; i < pts.length; i++) {
        const a = pts[i - 1], b = pts[i], dx = (b.x - a.x) / 2;
        d += " C" + (a.x + dx) + "," + a.y + " " + (b.x - dx) + "," + b.y + " " + b.x + "," + b.y;
      }
      return d;
    }
    function makeEdge(e, pts) {
      const g = el("g", { class: "ve", "data-e": e.key });
      const target = P.vnodes.get(e.to);
      const ghost = target && target.type === "resource" && target.node.resolved === false && !target.node.unverified;
      const d = e.cross ? roundedPath(pts, 10) : smoothPath(pts);
      const cls = "vedge c-" + e.category + (e.cross ? " cross" : "") + (ghost ? " to-ghost" : "") + (e.inferred ? " inferred" : "");
      const path = el("path", { class: cls, d, "marker-end": "url(#arr-" + (ghost ? "ghost" : e.category) + ")" });
      g.appendChild(path);
      const hit = el("path", { class: "ehit", d });
      const t = el("title");
      const types = Array.from(e.types).join(", ");
      const bases = Array.from(new Set(e.canonical.map(c => c.basis))).map(b => G.BASIS_LABEL[b] || b).join("; ");
      t.textContent = (e.count > 1 ? e.count + " × " : "") + types + "\n" + labelFor(e.from) + " → " + labelFor(e.to) + "\nbasis: " + bases;
      hit.appendChild(t);
      g.appendChild(hit);
      if (e.count > 1) {
        const mid = pts[Math.floor(pts.length / 2)], prev = pts[Math.max(0, Math.floor(pts.length / 2) - 1)];
        const c = el("text", { class: "ecount", x: (mid.x + prev.x) / 2 + 4, y: (mid.y + prev.y) / 2 - 4 }); c.textContent = "×" + e.count;
        g.appendChild(c);
      }
      g._path = path;
      return g;
    }
    function labelFor(vid) {
      const v = P.vnodes.get(vid);
      if (!v) return vid;
      if (v.type === "resource") return v.node.kind + "/" + v.node.name;
      if (v.type === "aggregate") return v.label;
      return v.groupRef.label;
    }
    function makeNode(v, p, color) {
      if (v.type === "groupcard") return makeGroupCard(v, p, color);
      const w = p.w, h = p.h;
      const g = el("g", { class: "rnode", "data-vid": v.vid, transform: "translate(" + p.x + "," + p.y + ")" });
      if (v.type === "aggregate") {
        g.classList.add("agg");
        g.appendChild(el("rect", { class: "bg-back", x: -w / 2 + 6, y: -h / 2 - 5, width: w - 12, height: h, rx: 8 }));
        g.appendChild(el("rect", { class: "bg", x: -w / 2, y: -h / 2, width: w, height: h, rx: 9 }));
        g.appendChild(el("rect", { x: -w / 2, y: -h / 2, width: 4, height: h, rx: 2, style: "fill:var(--cat-" + v.cat + ")" }));
        const k = el("text", { class: "kind", x: -w / 2 + 12, y: -3, style: "fill:var(--cat-" + v.cat + ")" }); k.textContent = "GROUP · click to expand";
        const n = el("text", { class: "name", x: -w / 2 + 12, y: 12 }); n.textContent = G.shorten(v.label, 30);
        g.appendChild(k); g.appendChild(n);
        if (v.bad) { const b = el("text", { class: "flag", x: w / 2 - 8, y: -h / 2 + 12, "text-anchor": "end", style: "fill:var(--crit)" }); b.textContent = v.bad + " failing"; g.appendChild(b); }
        const t = el("title"); t.textContent = v.label + " — click to show each one"; g.appendChild(t);
        return g;
      }
      const n = v.node;
      const ghost = n.resolved === false && !n.unverified, unver = n.resolved === false && n.unverified;
      if (ghost) g.classList.add("ghost");
      if (unver) g.classList.add("unverified");
      if (n.status) g.classList.add("st-" + n.status);
      if (v.focus) g.classList.add("focus");
      g.appendChild(el("rect", { class: "bg", x: -w / 2, y: -h / 2, width: w, height: h, rx: 9 }));
      g.appendChild(el("rect", { x: -w / 2, y: -h / 2, width: 4, height: h, rx: 2, style: "fill:var(--cat-" + n.cat + ")" }));
      const top = -h / 2;
      const k = el("text", { class: "kind", x: -w / 2 + 12, y: top + 15, style: "fill:var(--cat-" + n.cat + ")" }); k.textContent = n.kind;
      const nm = el("text", { class: "name", x: -w / 2 + 12, y: top + 30 }); nm.textContent = G.shorten(n.name, 30);
      g.appendChild(k); g.appendChild(nm);
      if (ghost || unver) { const f = el("text", { class: "flag", x: w / 2 - 8, y: top + 13, "text-anchor": "end" }); f.textContent = ghost ? "NOT FOUND" : "UNVERIFIED"; g.appendChild(f); }
      else if (n.status) g.appendChild(el("circle", { class: "dot", cx: w / 2 - 10, cy: top + 10, r: 3.5 }));
      if (v.badges && v.badges.length) {
        const b = el("text", { class: "badge-t", x: -w / 2 + 12, y: top + 44 });
        const maxChars = Math.max(8, Math.floor((w - 24) / 5.9));
        b.textContent = G.shorten(v.badges.map(x => (x.kind === "node" ? "on " : x.kind === "ns" ? "ns " : "") + x.text).join("  ·  "), maxChars);
        g.appendChild(b);
      }
      const t = el("title");
      t.textContent = n.kind + "/" + n.name + (n.ns ? "  (namespace " + n.ns + ")" : "  (cluster-scoped)") +
        (ghost ? "\nReferenced, but not found in the cluster." : unver ? "\nReferenced, but Mantis could not check whether it exists (not listed, or listing forbidden by RBAC)." : "");
      g.appendChild(t);
      return g;
    }
    function makeGroupCard(v, p, color) {
      const gr = v.groupRef, w = p.w, h = p.h, s = gr.summary || { total: 0, byCat: {} };
      const g = el("g", { class: "gcard", "data-vid": v.vid, "data-expand": gr.key, transform: "translate(" + p.x + "," + p.y + ")", style: "--grp:" + color });
      g.appendChild(el("rect", { class: "gc-bg", x: -w / 2, y: -h / 2, width: w, height: h, rx: 12 }));
      const kind = el("text", { class: "gc-kind", x: -w / 2 + 14, y: -h / 2 + 20 });
      kind.textContent = { namespace: "namespace", infra: "cluster-scoped", node: "worker node", workload: (gr.workload ? gr.workload.kind : "workload"), shared: "shared" }[gr.type] || gr.type;
      const title = el("text", { class: "gc-title", x: -w / 2 + 14, y: -h / 2 + 40 }); title.textContent = G.shorten(gr.label, 26);
      const l1 = el("text", { class: "gc-line", x: -w / 2 + 14, y: -h / 2 + 60 });
      const pods = (s.podsOk || 0) + (s.podsBad || 0);
      l1.textContent = fmt(s.total) + " resources" + (pods ? " · " + fmt(pods) + " pods" : "");
      g.appendChild(kind); g.appendChild(title); g.appendChild(l1);
      const l2 = el("text", { class: "gc-line", x: -w / 2 + 14, y: -h / 2 + 76 });
      if (s.podsBad) { const b = el("tspan", { class: "gc-bad" }); b.textContent = s.podsBad + " failing"; l2.appendChild(b); }
      if (s.broken) { const b = el("tspan", { class: "gc-bad" }); b.textContent = (s.podsBad ? " · " : "") + s.broken + " broken ref" + (s.broken > 1 ? "s" : ""); l2.appendChild(b); }
      if (gr.sub && gr.type !== "namespace") { const b = el("tspan"); b.textContent = (s.podsBad || s.broken ? " · " : "") + gr.sub; l2.appendChild(b); }
      g.appendChild(l2);
      // Composition bar: share of each resource category.
      const cats = ["workload", "pod", "network", "config", "storage", "security", "autoscale", "node", "other"];
      let x = -w / 2 + 14; const bw = w - 28, tot = Math.max(1, s.total);
      cats.forEach(c => { const n = s.byCat[c] || 0; if (!n) return; const ww = bw * n / tot; g.appendChild(el("rect", { x, y: h / 2 - 20, width: Math.max(1, ww - 1), height: 5, rx: 1.5, style: "fill:var(--cat-" + c + ")" })); x += ww; });
      const plus = el("text", { class: "gc-plus", x: w / 2 - 22, y: -h / 2 + 24 }); plus.textContent = "+"; g.appendChild(plus);
      const t = el("title"); t.textContent = gr.label + " — click to expand"; g.appendChild(t);
      return g;
    }

    // ------------------------------------------------------------ camera
    const view = { x: 0, y: 0, k: 1 };
    let cullTimer = null;
    const applyView = () => {
      gView.setAttribute("transform", "translate(" + view.x + "," + view.y + ") scale(" + view.k + ")");
      svg.classList.toggle("lod-mid", view.k < 0.55 && view.k >= 0.3);
      svg.classList.toggle("lod-far", view.k < 0.3);
      if (!cullTimer) cullTimer = setTimeout(() => { cullTimer = null; cull(false); }, 60);
    };
    function cull(force) {
      const pad = 0.6;
      const vw = W() / view.k, vh = H() / view.k;
      const x0 = -view.x / view.k - vw * pad, y0 = -view.y / view.k - vh * pad, x1 = x0 + vw * (1 + 2 * pad), y1 = y0 + vh * (1 + 2 * pad);
      const want = new Set();
      items.forEach(it => { if (it.x1 >= x0 && it.x0 <= x1 && it.y1 >= y0 && it.y0 <= y1) want.add(it.id); });
      for (const [id, dom] of domById) if (!want.has(id)) { dom.remove(); domById.delete(id); }
      let added = false;
      want.forEach(id => {
        if (domById.has(id)) return;
        const it = itemById.get(id), dom = it.make();
        dom._item = it;
        layers[it.layer].appendChild(dom); domById.set(id, dom); added = true;
      });
      if (added || force) applySelection();
    }
    function fit(animate) {
      if (!Lay) return;
      const b = Lay.bounds;
      const pw = panel.classList.contains("collapsed") ? 0 : panel.offsetWidth + 28;
      const availW = Math.max(200, W() - pw - 40), availH = Math.max(200, H() - 110);
      const k = Math.max(0.08, Math.min(1.25, availW / (b.x1 - b.x0), availH / (b.y1 - b.y0)));
      const tx = 20 + (availW - (b.x1 - b.x0) * k) / 2 - b.x0 * k, ty = 64 + (availH - (b.y1 - b.y0) * k) / 2 - b.y0 * k;
      if (animate) animateView(tx, ty, k); else { view.x = tx; view.y = ty; view.k = k; applyView(); }
    }
    let animId = null;
    function animateView(tx, ty, tk, duration) {
      if (animId) cancelAnimationFrame(animId);
      const sx = view.x, sy = view.y, sk = view.k, start = performance.now();
      duration = duration || 420;
      const reduce = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
      if (reduce) { view.x = tx; view.y = ty; view.k = tk; applyView(); return; }
      const step = now => {
        const t = Math.min(1, (now - start) / duration), e = 1 - Math.pow(1 - t, 3);
        view.x = sx + (tx - sx) * e; view.y = sy + (ty - sy) * e; view.k = sk + (tk - sk) * e; applyView();
        animId = t < 1 ? requestAnimationFrame(step) : null;
      };
      animId = requestAnimationFrame(step);
    }
    function centerOn(vid) {
      const p = Lay && Lay.pos.get(vid); if (!p) return;
      const tk = Math.max(view.k, 0.85);
      const pw = panel.classList.contains("collapsed") ? 0 : panel.offsetWidth + 28;
      animateView((W() - pw) / 2 - p.x * tk, H() / 2 - p.y * tk, tk);
    }
    svg.addEventListener("wheel", ev => {
      ev.preventDefault();
      const r = svg.getBoundingClientRect(), mx = ev.clientX - r.left, my = ev.clientY - r.top, f = Math.exp(-ev.deltaY * 0.0016);
      const nk = Math.min(2.6, Math.max(0.06, view.k * f));
      view.x = mx - (mx - view.x) * (nk / view.k); view.y = my - (my - view.y) * (nk / view.k); view.k = nk; applyView();
    }, { passive: false });
    let drag = null;
    svg.addEventListener("pointerdown", ev => {
      const r = svg.getBoundingClientRect();
      drag = { sx: ev.clientX - r.left - view.x, sy: ev.clientY - r.top - view.y, moved: false, target: ev.target };
      svg.classList.add("panning"); svg.setPointerCapture(ev.pointerId);
    });
    svg.addEventListener("pointermove", ev => {
      if (!drag) return;
      const r = svg.getBoundingClientRect(), nx = ev.clientX - r.left - drag.sx, ny = ev.clientY - r.top - drag.sy;
      if (!drag.moved && Math.hypot(nx - view.x, ny - view.y) < 3) return;
      drag.moved = true; view.x = nx; view.y = ny; applyView();
    });
    svg.addEventListener("pointerup", ev => {
      svg.classList.remove("panning");
      const d = drag; drag = null;
      if (!d || d.moved) return;
      onClick(d.target, ev);
    });
    svg.addEventListener("dblclick", ev => {
      const n = ev.target.closest(".rnode");
      if (n && P.vnodes.get(n.getAttribute("data-vid")).type === "resource") focusDependencies(n.getAttribute("data-vid"));
    });
    window.addEventListener("resize", () => cull(false));

    function onClick(target) {
      const collapse = target.closest("[data-collapse]");
      if (collapse) { setExpanded(collapse.getAttribute("data-collapse"), false); return; }
      const card = target.closest(".gcard");
      if (card) { setExpanded(card.getAttribute("data-expand"), true); return; }
      const n = target.closest(".rnode");
      if (n) {
        const vid = n.getAttribute("data-vid"), v = P.vnodes.get(vid);
        if (v && v.type === "aggregate") { st.unfolded.add(vid); render(); return; }
        select(vid);
        return;
      }
      select(null);
    }
    function setExpanded(key, on) {
      st.expanded[st.view].set(key, on);
      render();
      // Keep the expanded/collapsed group in view.
      const gb = Lay.groups.find(g => g.key === key);
      if (gb && on) {
        const k = Math.min(1, Math.max(view.k, Math.min(W() / (gb.x1 - gb.x0 + 120), H() / (gb.y1 - gb.y0 + 160))));
        animateView(W() / 2 - ((gb.x0 + gb.x1) / 2) * k, H() / 2 - ((gb.y0 + gb.y1) / 2) * k, k);
      }
    }

    // ------------------------------------------------------------ selection
    function applySelection() {
      if (!P) return;
      const nodesDom = layers.nodes.children, edgesDom = layers.edges.children;
      if (!selected || !store.node(selected)) {
        for (const g of nodesDom) g.classList.remove("selected", "dim", "hl");
        for (const g of edgesDom) { g._path.classList.remove("dim", "hl"); }
        layers.labels.innerHTML = "";
        return;
      }
      // In the dependency view everything drawn is already the selection's
      // neighbourhood: nothing is dimmed there.
      const tr = st.view === "dependency" && P.focus
        ? { dist: new Map(Array.from(P.member.keys()).map(id => [id, 1])), edges: new Set(P.vedges.flatMap(e => e.canonical.map(c => c.key))) }
        : G.trace(store, selected, 1, st.relations[st.view]);
      const hlVids = new Set();
      tr.dist.forEach((d, id) => { const m = P.member.get(id); if (m) { const g = P.groups.find(x => x.key === m.group); hlVids.add(g && g.collapsed ? "group:" + g.key : m.vid); } });
      const selVid = (P.member.get(selected) || {}).vid;
      for (const g of nodesDom) {
        const vid = g.getAttribute("data-vid");
        g.classList.toggle("selected", vid === selected);
        g.classList.toggle("hl", hlVids.has(vid) && vid !== selected);
        g.classList.toggle("dim", !hlVids.has(vid));
      }
      layers.labels.innerHTML = "";
      for (const g of edgesDom) {
        const e = g._item.e;
        const on = e.canonical.some(c => tr.edges.has(c.key));
        g._path.classList.toggle("hl", on); g._path.classList.toggle("dim", !on);
        if (on && (e.from === selVid || e.to === selVid)) {
          const pts = g._item.pts, i = Math.floor(pts.length / 2), a = pts[Math.max(0, i - 1)], b = pts[i];
          const t = el("text", { class: "elabel", x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 - 5, "text-anchor": "middle" });
          t.textContent = Array.from(e.types).join(", ") + (e.count > 1 ? " ×" + e.count : "");
          layers.labels.appendChild(t);
        }
      }
    }
    function select(id, o) {
      o = o || {};
      if (id && !store.node(id)) id = null;
      const isNew = id !== selected;
      if (isNew && !o.tab) curTab = "summary";
      if (o.tab) curTab = o.tab;
      selected = id;
      applySelection();
      if (!id) { showPanel(false); renderEmpty(); return; }
      showPanel(true);
      renderNode(store.node(id));
      if (o.center) centerOn((P.member.get(id) || {}).vid || id);
    }
    // jumpTo: make a resource visible in the current view (or, if the view
    // cannot show it, in the namespace view), then select and fly to it.
    function jumpTo(id, o) {
      if (!store.node(id)) return;
      if (st.view === "dependency" && !(P.member && P.member.has(id))) { st.focus = id; }
      if (!P.member.has(id) && st.view !== "dependency") { prevView = st.view; st.view = "namespace"; render(); }
      if (G.reveal(st, P, id)) render();
      select(id, Object.assign({ center: true }, o || {}));
    }
    function focusDependencies(id) {
      if (!store.node(id)) return;
      if (st.view !== "dependency") prevView = st.view;
      st.view = "dependency"; st.focus = id;
      showPanel(true); // fit around the inspector that select() opens
      render({ fit: true });
      select(id);
    }
    function setView(v) {
      if (v === st.view) return;
      if (v === "dependency" && !selected && !st.focus) { st.view = v; render(); return; }
      if (v === "dependency" && selected) st.focus = selected;
      if (st.view !== "dependency") prevView = st.view;
      st.view = v;
      render({ fit: true });
      if (selected && P.member.has(selected)) select(selected, { center: true });
    }

    // ------------------------------------------------------------ panel
    const showPanel = show => panel.classList.toggle("collapsed", !show);
    const TABS = { summary: "Summary", relationships: "Relationships", yaml: "YAML" };
    const statusBadge = n => n.status === "ok" ? '<span class="badge ok">healthy</span>' : n.status === "crit" ? '<span class="badge crit">needs attention</span>' :
      n.status === "warn" ? '<span class="badge warn">status unknown</span>' : n.resolved === false ? (n.unverified ? '<span class="badge warn">unverified</span>' : '<span class="badge crit">not found</span>') : "";
    function renderEmpty() {
      const meta = store.meta || {};
      panelContent.innerHTML = '<div><div class="p-kind" style="color:var(--brand)">Cluster graph</div><div class="p-name">' + esc(meta.context || "") + '</div></div>' +
        '<p class="hint">Namespaces are groups; <b>click</b> one to open it, <b>click</b> a "N Pods" stack to unfold it. <b>Click</b> a resource for its details, <b>double-click</b> it (or use <b>Dependencies</b>) to see only what it depends on and what depends on it. <b>Drag</b> to pan, <b>scroll</b> to zoom.</p>';
    }
    function renderNode(n) {
      const head = '<div><div class="p-kind" style="color:var(--cat-' + n.cat + ')">' + esc(n.kind) + '</div><div class="p-name">' + esc(n.name) + '</div>' +
        '<div class="p-ns">' + (n.ns ? "namespace · " + esc(n.ns) : "cluster-scoped") + '</div></div>' + statusBadge(n) +
        '<div class="panel-actions"><button class="btn" id="pDeps">Dependencies</button><button class="btn" id="pIn">Used by</button><button class="btn" id="pOut">Depends on</button></div>';
      const tabs = '<div class="tabs">' + Object.keys(TABS).map(t => '<div class="tab' + (t === curTab ? " active" : "") + '" data-tab="' + t + '">' + TABS[t] + '</div>').join("") + '</div>';
      panelContent.innerHTML = head + tabs + '<div id="tabbody"></div>';
      panelContent.querySelectorAll(".tab").forEach(t => t.addEventListener("click", () => { curTab = t.getAttribute("data-tab"); renderBody(n); }));
      $("pDeps").addEventListener("click", () => { st.direction = "both"; $("vbDir").value = "both"; focusDependencies(n.id); });
      $("pIn").addEventListener("click", () => { st.direction = "in"; $("vbDir").value = "in"; focusDependencies(n.id); });
      $("pOut").addEventListener("click", () => { st.direction = "out"; $("vbDir").value = "out"; focusDependencies(n.id); });
      renderBody(n);
    }
    function renderBody(n) {
      panelContent.querySelectorAll(".tab").forEach(t => t.classList.toggle("active", t.getAttribute("data-tab") === curTab));
      const body = $("tabbody"); if (!body) return;
      if (curTab === "yaml") { renderYaml(n, body); return; }
      body.innerHTML = curTab === "summary" ? summaryHTML(n) : relHTML(n);
      body.querySelectorAll("[data-goto]").forEach(r => r.addEventListener("click", () => jumpTo(r.getAttribute("data-goto"))));
    }
    const drow = (k, v) => '<div class="drow"><span class="dk">' + esc(k) + '</span><span class="dv">' + v + '</span></div>';
    function link(id) { const o = store.node(id); return o ? '<span class="linkish" data-goto="' + esc(id) + '">' + esc(o.kind + "/" + o.name) + '</span>' : esc(id); }
    function summaryHTML(n) {
      let h = "";
      if (n.resolved === false) {
        h += n.unverified
          ? '<div class="notice">Referenced by another resource, but Mantis could not check whether it exists — its kind is not one Mantis lists, or listing it was forbidden by RBAC. It is shown as unverified rather than missing.</div>'
          : '<div class="dstat">not found in cluster</div><p class="hint">This resource is referenced by another object, but it does not exist. It is flagged rather than hidden.</p>';
      }
      const rows = [];
      rows.push(drow("kind", esc(n.kind) + (n.apiVersion ? ' <span class="tag">' + esc(n.apiVersion) + '</span>' : "")));
      rows.push(drow("scope", n.ns ? "namespace " + esc(n.ns) : "cluster-scoped"));
      if (n.uid) rows.push(drow("uid", esc(n.uid)));
      if (n.created) rows.push(drow("created", esc(n.created)));
      if (n.kind === "Pod") { const node = store.nodeOfPod(n.id); if (node) rows.push(drow("node", link(node))); }
      const chain = []; let cur = store.ownerOf(n.id); const seen = new Set();
      while (cur && !seen.has(cur)) { seen.add(cur); chain.push(cur); cur = store.ownerOf(cur); }
      if (chain.length) rows.push(drow("owned by", chain.map(link).join(" → ")));
      h += '<div class="sec-h">Identity</div><div class="detail">' + rows.join("") + '</div>';
      if (n.labels && Object.keys(n.labels).length) {
        h += '<div class="sec-h">Labels</div><div class="labels">' + Object.keys(n.labels).sort().map(k => '<span class="lbl">' + esc(k) + "=" + esc(n.labels[k]) + '</span>').join("") + '</div>';
      }
      const attrs = n.attributes || [];
      if (attrs.length) {
        const parts = attrs.map(a => { const i = a.indexOf(": "); return i < 0 ? { k: null, v: a } : { k: a.slice(0, i), v: a.slice(i + 2) }; });
        h += parts.filter(p => p.k === null).map(p => '<div class="dstat">' + esc(p.v) + '</div>').join("");
        const kv = parts.filter(p => p.k !== null);
        if (kv.length) h += '<div class="sec-h">Details</div><div class="detail">' + kv.map(p => drow(p.k, esc(p.v))).join("") + '</div>';
      }
      if (n.kind === "Secret") h += '<p class="hint">Secret values are never read into Mantis\'s graph or shown here — only that the Secret exists and what references it.</p>';
      return h;
    }
    function relHTML(n) {
      const byCat = new Map();
      const add = (cat, html) => { if (!byCat.has(cat)) byCat.set(cat, []); byCat.get(cat).push(html); };
      const row = (e, dir) => {
        const otherId = dir === "out" ? e.to : e.from, o = store.node(otherId);
        if (!o) return "";
        const verb = dir === "out" ? e.type : (G.REVERSE[e.type] || e.type);
        const cross = o.ns && n.ns && o.ns !== n.ns;
        const basis = e.basis !== "reference" && e.basis !== "ownership" ? '<span class="tag ' + esc(e.basis) + '" title="' + esc(G.BASIS_LABEL[e.basis] || e.basis) + '">' + esc(e.basis) + '</span>' : "";
        return '<div class="rel" data-goto="' + esc(o.id) + '" title="' + esc((e.evidence ? "from " + e.evidence + " — " : "") + (G.BASIS_LABEL[e.basis] || "")) + '">' +
          '<span class="cat-sw" style="border-color:var(--rel-' + e.category + ')"></span><span class="verb">' + esc(verb) + '</span><span class="arrow">' + (dir === "out" ? "→" : "←") + '</span>' +
          '<span class="rel-target"><span class="rk" style="color:var(--cat-' + o.cat + ')">' + esc(o.kind) + '/</span>' + esc(o.name) + '</span>' + basis +
          (cross ? '<span class="cross-tag">' + esc(o.ns) + '</span>' : o.resolved === false ? '<span class="cross-tag" style="color:var(--crit)">' + (o.unverified ? "unverified" : "missing") + '</span>' : "") + '</div>';
      };
      store.outEdges(n.id).forEach(e => add(e.category, row(e, "out")));
      store.inEdges(n.id).forEach(e => add(e.category, row(e, "in")));
      if (!byCat.size) return '<p class="hint">No relationships discovered.</p>';
      let h = "";
      G.CATEGORIES.forEach(c => { if (byCat.has(c)) h += '<div class="sec-h">' + esc(G.CATEGORY_LABEL[c]) + '</div>' + byCat.get(c).join(""); });
      return h + '<p class="hint" style="font-size:11px">Hover a line for the API field it was read from. <span class="tag selector">selector</span> = label-selector match; <span class="tag endpoint">endpoint</span> = EndpointSlice record. Neither is observed traffic.</p>';
    }
    function renderYaml(n, body) {
      if (n.resolved === false) { body.innerHTML = '<p class="hint">This resource was referenced but not found in the cluster, so there is no manifest to show.</p>'; return; }
      body.innerHTML = '<p class="hint">Loading manifest…</p>';
      const token = ++yamlToken;
      source.resource(n).then(res => {
        if (token !== yamlToken) return;
        if (res.ok) {
          body.innerHTML = '<div class="yaml-wrap"><div class="yaml-tools"><button class="copy-btn" id="expandYaml" title="Open a large reader">⤢ Expand</button><button class="copy-btn" id="copyYaml">Copy</button></div><pre class="yaml">' + esc(res.text) + '</pre></div>';
          wireCopy($("copyYaml"), res.text);
          $("expandYaml").addEventListener("click", () => openYamlModal(res.text, n));
        } else if (res.status === 403) body.innerHTML = '<div class="notice">' + esc(res.text) + '</div>';
        else body.innerHTML = '<div class="notice">Could not load manifest — ' + esc(res.text || ("HTTP " + res.status)) + '</div>';
      }).catch(e => { if (token === yamlToken) body.innerHTML = '<div class="notice">Could not load manifest — ' + esc(String(e)) + '</div>'; });
    }
    async function doCopy(btn, text) {
      try {
        if (navigator.clipboard && navigator.clipboard.writeText) await navigator.clipboard.writeText(text);
        else { const ta = document.createElement("textarea"); ta.value = text; document.body.appendChild(ta); ta.select(); document.execCommand("copy"); ta.remove(); }
        btn.textContent = "Copied ✓"; btn.classList.add("done");
        setTimeout(() => { btn.textContent = "Copy"; btn.classList.remove("done"); }, 1600);
      } catch (e) { btn.textContent = "Copy failed"; setTimeout(() => { btn.textContent = "Copy"; }, 1600); }
    }
    const wireCopy = (btn, text) => { if (btn) btn.addEventListener("click", () => doCopy(btn, text)); };

    // YAML reader modal.
    const ym = { overlay: $("ymodalOverlay"), pre: $("ymodalPre"), title: $("ymodalTitle"), copy: $("ymodalCopy"), close: $("ymodalClose"),
      search: $("ymodalSearch"), count: $("ymodalMatchCount"), prev: $("ymodalPrev"), next: $("ymodalNext"), raw: "", matches: [], idx: -1 };
    function renderYamlSearch() {
      const q = ym.search.value;
      if (!q) { ym.pre.innerHTML = esc(ym.raw); ym.count.textContent = ""; ym.matches = []; ym.idx = -1; return; }
      const low = ym.raw.toLowerCase(), ql = q.toLowerCase(), offs = [];
      for (let i = low.indexOf(ql); i !== -1; i = low.indexOf(ql, i + q.length)) offs.push(i);
      ym.matches = offs;
      if (!offs.length) { ym.pre.innerHTML = esc(ym.raw); ym.count.textContent = "0/0"; ym.idx = -1; return; }
      if (ym.idx < 0 || ym.idx >= offs.length) ym.idx = 0;
      let html = "", last = 0;
      offs.forEach((o, i) => { html += esc(ym.raw.slice(last, o)) + '<mark' + (i === ym.idx ? ' class="cur"' : "") + '>' + esc(ym.raw.slice(o, o + q.length)) + '</mark>'; last = o + q.length; });
      ym.pre.innerHTML = html + esc(ym.raw.slice(last));
      ym.count.textContent = (ym.idx + 1) + "/" + offs.length;
      const cur = ym.pre.querySelector("mark.cur"); if (cur) cur.scrollIntoView({ block: "center" });
    }
    const ymStep = d => { if (!ym.matches.length) return; ym.idx = (ym.idx + d + ym.matches.length) % ym.matches.length; renderYamlSearch(); };
    ym.search.addEventListener("input", () => { ym.idx = 0; renderYamlSearch(); });
    ym.search.addEventListener("keydown", ev => {
      if (ev.key === "Enter") { ev.preventDefault(); ymStep(ev.shiftKey ? -1 : 1); }
      else if (ev.key === "Escape") { if (ym.search.value) { ev.stopPropagation(); ym.search.value = ""; renderYamlSearch(); } else ym.search.blur(); }
    });
    ym.prev.addEventListener("click", () => ymStep(-1));
    ym.next.addEventListener("click", () => ymStep(1));
    function openYamlModal(text, n) {
      ym.raw = text; ym.search.value = ""; ym.matches = []; ym.idx = -1; ym.pre.innerHTML = esc(text); ym.count.textContent = "";
      ym.title.innerHTML = '<b>' + esc(n.kind) + ' · </b>' + esc(n.name) + (n.ns ? ' <b>· ' + esc(n.ns) + '</b>' : "");
      ym.copy.textContent = "Copy"; ym.copy.classList.remove("done"); ym.copy.onclick = () => doCopy(ym.copy, text);
      ym.overlay.classList.add("open");
    }
    const closeYamlModal = () => { ym.overlay.classList.remove("open"); ym.pre.textContent = ""; ym.raw = ""; };
    ym.close.addEventListener("click", closeYamlModal);
    ym.overlay.addEventListener("click", ev => { if (ev.target === ym.overlay) closeYamlModal(); });

    // Panel close + resize.
    $("panelCollapse").addEventListener("click", () => select(null));
    let resizing = null;
    const resizer = $("panelResizer");
    resizer.addEventListener("pointerdown", ev => { resizing = { x: ev.clientX, w: panel.offsetWidth }; panel.classList.add("resizing"); resizer.setPointerCapture(ev.pointerId); ev.preventDefault(); });
    resizer.addEventListener("pointermove", ev => { if (resizing) panel.style.width = Math.max(300, Math.min(window.innerWidth - 80, resizing.w + resizing.x - ev.clientX)) + "px"; });
    const endResize = () => { if (resizing) { resizing = null; panel.classList.remove("resizing"); } };
    resizer.addEventListener("pointerup", endResize); resizer.addEventListener("pointercancel", endResize);

    // ------------------------------------------------------------ search
    const searchInput = $("search"), searchResults = $("searchResults");
    let sMatches = [], sActive = -1, sTotal = 0;
    function renderSearch() {
      if (!sMatches.length) {
        searchResults.innerHTML = searchInput.value.trim() ? '<div class="search-empty">No matching resource. Try kind:Pod, ns:name or label:app=web.</div>' : "";
        searchResults.classList.toggle("open", !!searchInput.value.trim()); return;
      }
      searchResults.innerHTML = sMatches.map((n, i) => '<div class="sresult' + (i === sActive ? " active" : "") + '" data-id="' + esc(n.id) + '"><span class="sw" style="background:var(--cat-' + n.cat + ')"></span><span class="sr-name">' + esc(n.name) + '</span><span class="sr-kind">' + esc(n.kind) + '</span><span class="sr-ns">' + (n.ns ? esc(n.ns) : "cluster") + '</span></div>').join("") +
        (sTotal > sMatches.length ? '<div class="search-empty">+' + (sTotal - sMatches.length) + ' more — refine your search</div>' : "");
      searchResults.classList.add("open");
      searchResults.querySelectorAll(".sresult").forEach(r => r.addEventListener("click", () => pick(r.getAttribute("data-id"))));
    }
    const closeSearch = () => { searchResults.classList.remove("open"); sActive = -1; };
    function pick(id) { jumpTo(id); closeSearch(); }
    searchInput.addEventListener("input", () => { const r = G.search(store, searchInput.value, 200); sMatches = r.results; sTotal = r.total; sActive = sMatches.length ? 0 : -1; renderSearch(); });
    searchInput.addEventListener("keydown", ev => {
      if (ev.key === "ArrowDown") { ev.preventDefault(); if (sMatches.length) { sActive = (sActive + 1) % sMatches.length; renderSearch(); } }
      else if (ev.key === "ArrowUp") { ev.preventDefault(); if (sMatches.length) { sActive = (sActive - 1 + sMatches.length) % sMatches.length; renderSearch(); } }
      else if (ev.key === "Enter") { if (sActive >= 0 && sMatches[sActive]) pick(sMatches[sActive].id); }
      else if (ev.key === "Escape") { ev.stopPropagation(); searchInput.value = ""; sMatches = []; sTotal = 0; closeSearch(); searchInput.blur(); }
    });
    document.addEventListener("click", ev => { if (!ev.target.closest(".search-wrap")) closeSearch(); });
    document.addEventListener("keydown", ev => {
      // Only while the graph is on screen: inside the application shell the
      // Topology page can be hidden behind other pages without unmounting.
      if (!svg.getClientRects().length) return;
      if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === "f") {
        ev.preventDefault();
        const t = ym.overlay.classList.contains("open") ? ym.search : searchInput; t.focus(); t.select(); return;
      }
      if (ev.key !== "Escape") return;
      if (ym.overlay.classList.contains("open")) { closeYamlModal(); return; }
      if (searchResults.classList.contains("open")) { closeSearch(); searchInput.blur(); return; }
      if ($("vbPop").classList.contains("open")) { $("vbPop").classList.remove("open"); return; }
      if (selected) select(null);
    });

    // ------------------------------------------------------------ controls
    const on = (id, f) => { const e = $(id); if (e) e.addEventListener("click", f); };
    on("reset", () => fit(true));
    on("fit", () => fit(true));
    on("expandAll", () => { (P ? P.groups : []).forEach(g => st.expanded[st.view].set(g.key, true)); render({ fit: true }); });
    on("collapseAll", () => { (P ? P.groups : []).forEach(g => st.expanded[st.view].set(g.key, false)); st.unfolded.clear(); render({ fit: true }); });
    const legend = $("legend"), legendToggle = $("legendToggle");
    if (legendToggle) legendToggle.addEventListener("click", () => {
      const c = legend.classList.toggle("collapsed"); const l = c ? "Legend" : "Close legend"; legendToggle.title = l; legendToggle.setAttribute("aria-label", l);
    });
    const themeToggle = $("themeToggle");
    function applyTheme(theme) {
      if (theme === "light") document.documentElement.setAttribute("data-theme", "light"); else document.documentElement.removeAttribute("data-theme");
      if (!themeToggle) return;
      const isLight = theme === "light", label = isLight ? "Switch to dark theme" : "Switch to light theme";
      themeToggle.setAttribute("aria-pressed", String(isLight)); themeToggle.title = label; themeToggle.setAttribute("aria-label", label);
    }
    applyTheme(document.documentElement.getAttribute("data-theme") === "light" ? "light" : "dark");
    if (themeToggle) themeToggle.addEventListener("click", () => {
      const next = document.documentElement.getAttribute("data-theme") === "light" ? "dark" : "light";
      try { sessionStorage.setItem("mantis-theme", next); } catch (e) { /* theme just won't persist */ }
      applyTheme(next);
      if (P) { buildItems(); cull(true); } // namespace colors are resolved at draw time
    });

    renderEmpty();

    return {
      store, state: st,
      firstLoad, load, applyPayload, render, select, jumpTo, focusDependencies, setView, fit, centerOn,
      setTab(t) { curTab = t; if (selected) renderBody(store.node(selected)); },
      isLoaded: () => everLoaded,
      reset() { selected = null; st.focus = null; st.unfolded.clear(); G.VIEWS.forEach(v => st.expanded[v].clear()); everLoaded = false; showPanel(false); },
      nodeElement: id => layers.nodes.querySelector('[data-vid="' + (window.CSS && CSS.escape ? CSS.escape(id) : id) + '"]'),
      showToast, hideToast, showOverlay, hideOverlay,
      projection: () => P, layout: () => Lay,
    };
  }

  global.MantisApp = { mount };
})(window);
