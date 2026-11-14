// app-shell.js — the Mantis application around the topology: navigation,
// dashboard, resource tables, resource details, events, search, access and
// settings. It reads the same canonical graph the topology does (via
// graph-engine.js's store — no second graph model) plus the engine's events
// and capacity endpoints. Everything it shows comes from the backend; when
// something is not available it says so instead of filling in numbers.
//
// The Topology page mounts graph-view.js (MantisApp.mount) unchanged, inside
// the shell, using only its public API.

(function () {
  "use strict";
  const G = window.MantisGraph;
  const $ = id => document.getElementById(id);
  const esc = s => String(s == null ? "" : s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  const fmt = n => Number(n || 0).toLocaleString("en-US");
  const enc = encodeURIComponent;

  // ------------------------------------------------------------------ icons
  const IC = {
    dashboard: '<rect x="3" y="3" width="7" height="9" rx="1"/><rect x="14" y="3" width="7" height="5" rx="1"/><rect x="14" y="12" width="7" height="9" rx="1"/><rect x="3" y="16" width="7" height="5" rx="1"/>',
    topology: '<circle cx="5" cy="6" r="2.5"/><circle cx="19" cy="6" r="2.5"/><circle cx="12" cy="18" r="2.5"/><path d="M7.2 7.4 10.4 16M16.8 7.4 13.6 16M7.5 6h9"/>',
    workloads: '<path d="M12 2 3 7v10l9 5 9-5V7z"/><path d="M3 7l9 5 9-5M12 12v10"/>',
    network: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>',
    storage: '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>',
    config: '<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6"/>',
    cluster: '<rect x="2" y="3" width="20" height="7" rx="1.5"/><rect x="2" y="14" width="20" height="7" rx="1.5"/><path d="M6 6.5h.01M6 17.5h.01"/>',
    security: '<path d="M12 2 4 5v6c0 5 3.4 9.4 8 11 4.6-1.6 8-6 8-11V5z"/><path d="m9 12 2 2 4-4"/>',
    admin: '<circle cx="12" cy="8" r="4"/><path d="M4 21c0-4 3.6-7 8-7s8 3 8 7"/>',
    settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>',
    events: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
    chevron: '<path d="m6 9 6 6 6-6"/>',
    menu: '<path d="M3 6h18M3 12h18M3 18h18"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>',
    refresh: '<path d="M21 12a9 9 0 1 1-2.6-6.4L21 8M21 3v5h-5"/>',
    collapse: '<path d="M15 18l-6-6 6-6"/>',
    expand: '<path d="m9 18 6-6-6-6"/>',
    logout: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9"/>',
    external: '<path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6M15 3h6v6M10 14 21 3"/>',
    report: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M12 17v-6M9.5 14.5 12 17l2.5-2.5"/>',
    lock: '<rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',
  };
  const icon = (name, extra) => '<svg class="m-ico' + (extra ? " " + extra : "") + '" viewBox="0 0 24 24" aria-hidden="true">' + IC[name] + '</svg>';

  // ------------------------------------------------------------------ kinds
  // Only kinds Mantis actually collects are listed. Gateway API is not read
  // yet, so it has no page.
  const KINDS = [
    { g: "workloads", slug: "pods", kind: "Pod", label: "Pods" },
    { g: "workloads", slug: "deployments", kind: "Deployment", label: "Deployments" },
    { g: "workloads", slug: "replicasets", kind: "ReplicaSet", label: "ReplicaSets" },
    { g: "workloads", slug: "statefulsets", kind: "StatefulSet", label: "StatefulSets" },
    { g: "workloads", slug: "daemonsets", kind: "DaemonSet", label: "DaemonSets" },
    { g: "workloads", slug: "jobs", kind: "Job", label: "Jobs" },
    { g: "workloads", slug: "cronjobs", kind: "CronJob", label: "CronJobs" },
    { g: "workloads", slug: "hpas", kind: "HorizontalPodAutoscaler", label: "HPAs", long: "HorizontalPodAutoscalers" },
    { g: "network", slug: "services", kind: "Service", label: "Services" },
    { g: "network", slug: "ingresses", kind: "Ingress", label: "Ingresses" },
    { g: "network", slug: "networkpolicies", kind: "NetworkPolicy", label: "NetworkPolicies" },
    { g: "storage", slug: "persistentvolumeclaims", kind: "PersistentVolumeClaim", label: "PersistentVolumeClaims" },
    { g: "storage", slug: "persistentvolumes", kind: "PersistentVolume", label: "PersistentVolumes" },
    { g: "storage", slug: "storageclasses", kind: "StorageClass", label: "StorageClasses" },
    { g: "config", slug: "configmaps", kind: "ConfigMap", label: "ConfigMaps" },
    { g: "config", slug: "secrets", kind: "Secret", label: "Secrets" },
    { g: "config", slug: "serviceaccounts", kind: "ServiceAccount", label: "ServiceAccounts" },
    { g: "config", slug: "resourcequotas", kind: "ResourceQuota", label: "ResourceQuotas" },
    { g: "config", slug: "limitranges", kind: "LimitRange", label: "LimitRanges" },
    { g: "cluster", slug: "nodes", kind: "Node", label: "Nodes" },
    { g: "security", slug: "roles", kind: "Role", label: "Roles" },
    { g: "security", slug: "clusterroles", kind: "ClusterRole", label: "ClusterRoles" },
    { g: "security", slug: "rolebindings", kind: "RoleBinding", label: "RoleBindings" },
    { g: "security", slug: "clusterrolebindings", kind: "ClusterRoleBinding", label: "ClusterRoleBindings" },
  ];
  const KIND_BY_SLUG = new Map(KINDS.map(k => [k.slug, k]));
  const SLUG_BY_KIND = new Map(KINDS.map(k => [k.kind, k.slug]));
  const GROUPS = [
    { id: "workloads", label: "Workloads", icon: "workloads" },
    { id: "network", label: "Networking", icon: "network" },
    { id: "storage", label: "Storage", icon: "storage" },
    { id: "config", label: "Configuration", icon: "config" },
    { id: "cluster", label: "Cluster", icon: "cluster", extra: [{ href: "#/namespaces", label: "Namespaces", key: "namespaces" }, { href: "#/events", label: "Events", key: "events" }] },
    { id: "security", label: "RBAC", icon: "security" },
  ];
  // Kinds where system-managed objects are hidden unless the engine runs
  // with MANTIS_SHOW_ALL=true — the table says so, rather than looking empty.
  const FILTERED_KINDS = { Secret: 1, ServiceAccount: 1, ConfigMap: 1, Role: 1, RoleBinding: 1, ClusterRole: 1, ClusterRoleBinding: 1 };
  const WORKLOAD_KINDS = ["Deployment", "StatefulSet", "DaemonSet", "CronJob", "Job"];

  // ------------------------------------------------------------------ state
  const store = G.createStore();
  const S = {
    loaded: false, error: null, lastSync: null, syncing: false, offline: false,
    ns: "", events: null, capacity: null, theme: "light", refreshMs: 10000,
    table: {}, // per-kind table state: q, status, sort, dir, page
  };
  try {
    S.theme = localStorage.getItem("mantis-theme") || (window.matchMedia && matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
    S.refreshMs = parseInt(localStorage.getItem("mantis-refresh") || "10000", 10);
    S.ns = localStorage.getItem("mantis-ns") || "";
    if (localStorage.getItem("mantis-rail") === "1") $("shell").classList.add("rail");
  } catch (e) { /* storage unavailable: defaults */ }
  document.documentElement.setAttribute("data-theme", S.theme === "dark" ? "dark" : "light");

  // ------------------------------------------------------------------ data
  function api(url) {
    return fetch(url, { cache: "no-store" }).then(r => {
      if (r.status === 401) { location.href = "/login"; throw new Error("session expired"); }
      return r.ok ? r.json() : r.text().then(t => { throw new Error(t || "HTTP " + r.status); });
    });
  }
  function syncGraph() {
    S.syncing = true; renderSync();
    return api("/api/graph" + (S.loaded ? "?since=" + store.revision : "")).then(p => {
      if (p.delta && S.loaded) recordChanges(p.delta);
      const res = store.apply(p);
      const changed = res.full || (res.changedIds && res.changedIds.size > 0);
      S.loaded = true; S.error = null; S.offline = false; S.lastSync = new Date();
      return changed;
    }).catch(err => {
      S.error = String(err.message || err); S.offline = S.loaded;
      return false;
    }).finally(() => { S.syncing = false; renderSync(); });
  }
  // Recent changes: every graph delta the engine sends is real — a resource
  // added, removed or updated between two syncs — so the dashboard can show
  // what changed while this page has been open (the engine keeps no history
  // before that, so neither does this list).
  const changeLog = [];
  function recordChanges(d) {
    const at = new Date();
    (d.upsertNodes || []).forEach(n => changeLog.unshift({ at, id: n.id, kind: n.kind, name: n.name, ns: n.ns, what: store.node(n.id) ? "updated" : "added" }));
    (d.removeNodes || []).forEach(id => { const n = store.node(id); changeLog.unshift({ at, id, kind: n ? n.kind : id.split("/")[1], name: n ? n.name : id.split("/").pop(), ns: n ? n.ns : id.split("/")[0], what: "removed" }); });
    if (changeLog.length > 200) changeLog.length = 200;
  }
  function loadEvents() { return api("/api/events?limit=500").then(e => { S.events = e; }).catch(err => { S.events = { error: String(err.message || err), events: [] }; }); }
  function loadCapacity() { return api("/api/cluster/capacity").then(c => { S.capacity = c; }).catch(err => { S.capacity = { error: String(err.message || err) }; }); }
  let timer = null;
  function schedule() {
    if (timer) clearInterval(timer);
    if (S.refreshMs > 0) timer = setInterval(tick, S.refreshMs);
  }
  function tick() {
    syncGraph().then(changed => {
      if (topo) topo.load(false).catch(() => {});
      const r = route();
      if (r.page === "dashboard" || r.page === "events") Promise.all([loadEvents(), r.page === "dashboard" ? loadCapacity() : null]).then(() => rerender());
      else if (changed) rerender();
      renderNavCounts();
    });
  }

  // ------------------------------------------------------------------ derived facts
  const attrMap = n => {
    const m = { _free: [] };
    (n.attributes || []).forEach(a => { const i = a.indexOf(": "); if (i < 0) m._free.push(a); else m[a.slice(0, i)] = a.slice(i + 2); });
    return m;
  };
  const visibleNodes = kind => {
    const out = [];
    store.nodes.forEach(n => { if (n.kind === kind && n.resolved !== false && (!S.ns || !n.ns || n.ns === S.ns)) out.push(n); });
    return out;
  };
  const allOf = kind => { const out = []; store.nodes.forEach(n => { if (n.kind === kind && n.resolved !== false) out.push(n); }); return out; };
  function ratio(s) { const m = /(\d+)\s*\/\s*(\d+)/.exec(s || ""); return m ? [parseInt(m[1], 10), parseInt(m[2], 10)] : null; }
  // statusOf: one consistent status vocabulary for every kind.
  function statusOf(n) {
    if (n.resolved === false) return n.unverified ? { t: "Unverified", c: "" } : { t: "Not found", c: "crit" };
    const a = attrMap(n);
    switch (n.kind) {
      case "Pod": {
        const head = (a._free[0] || "").split(" · ")[0] || "Unknown";
        if (head === "Succeeded" || head === "Completed") return { t: "Completed", c: "" };
        if (n.status === "ok") return { t: head, c: "ok" };
        if (head === "Pending" || head === "ContainerCreating" || head === "PodInitializing") return { t: head, c: "warn" };
        return { t: head, c: "crit" };
      }
      case "Node": {
        const s = a.status || "Unknown";
        return { t: s, c: s === "Ready" ? "ok" : s === "NotReady" ? "crit" : "" };
      }
      case "Deployment": case "StatefulSet": case "ReplicaSet": case "DaemonSet": {
        const r = ratio(a.replicas || a.scheduled);
        if (!r) return { t: "—", c: "" };
        if (r[1] === 0) return { t: "Scaled to 0", c: "" };
        if (r[0] >= r[1]) return { t: "Available", c: "ok" };
        if (r[0] === 0) return { t: "Unavailable", c: "crit" };
        return { t: "Degraded", c: "warn" };
      }
      case "PersistentVolumeClaim": case "PersistentVolume": {
        const ph = a._free.find(x => /^(Bound|Pending|Lost|Available|Released|Failed)$/.test(x)) || "";
        return ph ? { t: ph, c: ph === "Bound" ? "ok" : ph === "Pending" || ph === "Released" || ph === "Available" ? "warn" : "crit" } : { t: "—", c: "" };
      }
      case "Job": {
        const s = a.status || a._free[0] || "";
        return s ? { t: s, c: /fail/i.test(s) ? "crit" : /complete|succeed/i.test(s) ? "ok" : "info" } : { t: "—", c: "" };
      }
      default: return null;
    }
  }
  const badge = s => s ? '<span class="m-badge ' + s.c + '">' + esc(s.t) + '</span>' : '<span class="m-muted">—</span>';
  function age(iso) {
    if (!iso) return "—";
    const s = Math.max(0, (Date.now() - Date.parse(iso)) / 1000);
    if (s < 60) return Math.round(s) + "s"; if (s < 3600) return Math.round(s / 60) + "m";
    if (s < 86400) return Math.round(s / 3600) + "h"; return Math.round(s / 86400) + "d";
  }
  const relCount = (id, types, dir) => (dir === "in" ? store.inEdges(id) : store.outEdges(id)).filter(e => types.includes(e.type)).length;
  const relTargets = (id, types, dir) => (dir === "in" ? store.inEdges(id) : store.outEdges(id)).filter(e => types.includes(e.type)).map(e => (dir === "in" ? e.from : e.to));
  const short = id => id.split("/").pop();
  const objHref = id => "#/o/" + enc(id);

  // Per-kind columns. Each column: [header, value(n) -> html, sortKey(n)].
  const NAME = ["Name", n => '<a href="' + objHref(n.id) + '">' + esc(n.name) + '</a>', n => n.name];
  const NS = ["Namespace", n => esc(n.ns || "—"), n => n.ns];
  const STATUS = ["Status", n => badge(statusOf(n)), n => (statusOf(n) || {}).t || ""];
  const AGE = ["Age", n => esc(age(n.created)), n => -(Date.parse(n.created || 0) || 0)];
  const A = (header, key, mono) => [header, n => { const v = attrMap(n)[key]; return v ? (mono ? '<span class="m-mono">' + esc(v) + '</span>' : esc(v)) : '<span class="m-muted">—</span>'; }, n => attrMap(n)[key] || ""];
  const LINKS = (header, types, dir) => [header, n => { const t = relTargets(n.id, types, dir); return t.length ? t.slice(0, 3).map(id => '<span class="m-pill">' + esc(short(id)) + '</span>').join("") + (t.length > 3 ? ' <span class="m-muted">+' + (t.length - 3) + '</span>' : "") : '<span class="m-muted">—</span>'; }, n => relCount(n.id, types, dir)];
  const USED = (header, types) => [header, n => { const c = relCount(n.id, types, "in"); return c ? fmt(c) : '<span class="m-muted">0</span>'; }, n => relCount(n.id, types, "in"), "num"];
  const COLUMNS = {
    Pod: [NAME, NS, STATUS, ["Ready", n => esc(((attrMap(n)._free[0] || "").split(" · ")[1]) || "—"), n => (attrMap(n)._free[0] || "")],
      ["Restarts", n => { const m = /restarts (\d+)/.exec(attrMap(n)._free[0] || ""); return m ? '<span style="color:var(--m-warn-ink)">' + m[1] + '</span>' : "0"; }, n => { const m = /restarts (\d+)/.exec(attrMap(n)._free[0] || ""); return m ? +m[1] : 0; }, "num"],
      ["Node", n => { const x = store.nodeOfPod(n.id); return x ? '<a class="m-link" href="' + objHref(x) + '">' + esc(short(x)) + '</a>' : '<span class="m-muted">—</span>'; }, n => store.nodeOfPod(n.id) || ""],
      A("QoS", "qos"), AGE],
    Deployment: [NAME, NS, STATUS, A("Ready", "replicas"), A("Image", "image", true), A("Strategy", "strategy"), AGE],
    StatefulSet: [NAME, NS, STATUS, A("Ready", "replicas"), A("Image", "image", true), AGE],
    ReplicaSet: [NAME, NS, STATUS, A("Ready", "replicas"), ["Owner", n => { const o = store.ownerOf(n.id); return o ? '<a class="m-link" href="' + objHref(o) + '">' + esc(short(o)) + '</a>' : "—"; }, n => store.ownerOf(n.id) || ""], AGE],
    DaemonSet: [NAME, NS, STATUS, A("Scheduled", "scheduled"), A("Image", "image", true), AGE],
    Job: [NAME, NS, STATUS, ["Details", n => esc((n.attributes || []).slice(0, 2).join(" · ") || "—"), n => ""], AGE],
    CronJob: [NAME, NS, A("Schedule", "schedule", true), ["Details", n => esc((n.attributes || []).filter(a => !a.startsWith("schedule")).slice(0, 2).join(" · ") || "—"), n => ""], AGE],
    HorizontalPodAutoscaler: [NAME, NS, A("Target", "target", true), A("Replicas", "replicas"), A("Current", "current"), A("Metrics", "metrics"), AGE],
    Service: [NAME, NS, A("Type", "type"), A("Cluster IP", "clusterIP", true), A("Ports", "ports", true), LINKS("Selects", ["selects"], "out"), AGE],
    Ingress: [NAME, NS, LINKS("Routes to", ["routes-to"], "out"), AGE],
    NetworkPolicy: [NAME, NS, A("Policy types", "policy types"), A("Ingress rules", "ingress rules"), A("Egress rules", "egress rules"), ["Pods selected", n => fmt(relCount(n.id, ["applies-to"], "out")), n => relCount(n.id, ["applies-to"], "out"), "num"], AGE],
    PersistentVolumeClaim: [NAME, NS, STATUS, ["Capacity", n => esc(attrMap(n)._free[1] || "—"), n => attrMap(n)._free[1] || ""], ["Access", n => esc(attrMap(n)._free[2] || "—"), n => attrMap(n)._free[2] || ""],
      ["StorageClass", n => esc(((attrMap(n)._free.find(x => x.startsWith("sc:")) || "").slice(3)) || "—"), n => ""], LINKS("Volume", ["bound-to"], "out"), AGE],
    PersistentVolume: [NAME, STATUS, ["Capacity", n => esc(attrMap(n)._free[0] || "—"), n => attrMap(n)._free[0] || ""], ["Reclaim", n => esc(attrMap(n)._free[1] || "—"), n => ""], LINKS("Claim", ["bound-to"], "in"), LINKS("StorageClass", ["storage-class"], "out"), AGE],
    StorageClass: [NAME, A("Provisioner", "provisioner", true), A("Reclaim", "reclaim"), ["Default", n => attrMap(n).default === "true" ? '<span class="m-badge info">default</span>' : "", n => attrMap(n).default || ""], USED("Used by", ["storage-class"]), AGE],
    ConfigMap: [NAME, NS, USED("Used by", ["references", "mounts"]), AGE],
    Secret: [NAME, NS, USED("Used by", ["references", "mounts"]), AGE],
    ServiceAccount: [NAME, NS, USED("Pods", ["uses-service-account"]), USED("Bindings", ["grants-to"]), AGE],
    ResourceQuota: [NAME, NS, ["Details", n => esc((n.attributes || []).slice(0, 2).join(" · ") || "—"), n => ""], AGE],
    LimitRange: [NAME, NS, ["Details", n => esc((n.attributes || []).slice(0, 2).join(" · ") || "—"), n => ""], AGE],
    Node: [NAME, STATUS, A("Roles", "roles"), A("Version", "version", true), A("OS", "os"), A("Allocatable", "allocatable"),
      ["Pods", n => fmt(relCount(n.id, ["runs-on"], "in")), n => relCount(n.id, ["runs-on"], "in"), "num"], AGE],
    Role: [NAME, NS, A("Rules", "rules"), A("Verbs", "verbs"), USED("Bindings", ["binds-role"]), AGE],
    ClusterRole: [NAME, A("Rules", "rules"), A("Verbs", "verbs"), USED("Bindings", ["binds-role"]), AGE],
    RoleBinding: [NAME, NS, A("Role", "role", true), A("Subjects", "subjects"), LINKS("ServiceAccounts", ["grants-to"], "out"), AGE],
    ClusterRoleBinding: [NAME, A("Role", "role", true), A("Subjects", "subjects"), LINKS("ServiceAccounts", ["grants-to"], "out"), AGE],
  };

  // ------------------------------------------------------------------ router
  function route() {
    const h = (location.hash || "#/dashboard").slice(1);
    const [path, qs] = h.split("?");
    const parts = path.split("/").filter(Boolean);
    const q = new URLSearchParams(qs || "");
    if (!parts.length || parts[0] === "dashboard") return { page: "dashboard" };
    if (parts[0] === "topology") return { page: "topology", q };
    if (parts[0] === "r" && KIND_BY_SLUG.has(parts[1])) return { page: "list", kind: KIND_BY_SLUG.get(parts[1]) };
    if (parts[0] === "o") return { page: "object", id: decodeURIComponent(parts.slice(1).join("/")), tab: q.get("tab") || "overview" };
    if (parts[0] === "namespaces") return { page: "namespaces" };
    if (parts[0] === "events") return { page: "events" };
    if (parts[0] === "access") return { page: "access" };
    if (parts[0] === "settings") return { page: "settings" };
    return { page: "notfound" };
  }
  window.addEventListener("hashchange", () => { $("mContent").scrollTop = 0; closeDrawer(); render(); });

  // ------------------------------------------------------------------ sidebar
  function buildSidebar() {
    let collapsed = {};
    try { collapsed = JSON.parse(localStorage.getItem("mantis-nav") || "{}"); } catch (e) { /* default */ }
    const link = (href, ic, label, key) => '<a href="' + href + '" data-key="' + key + '">' + (ic ? icon(ic) : "") + '<span class="m-label">' + esc(label) + '</span></a>';
    let h = '<div class="m-nav-sec">Overview</div>' + link("#/dashboard", "dashboard", "Dashboard", "dashboard") + link("#/topology", "topology", "Topology", "topology");
    h += '<div class="m-nav-sec">Kubernetes resources</div>';
    GROUPS.forEach(g => {
      const items = KINDS.filter(k => k.g === g.id);
      if (g.id === "security") h += '<div class="m-nav-sec">Security &amp; access</div>';
      h += '<button class="m-nav-grp" aria-expanded="' + (collapsed[g.id] ? "false" : "true") + '" data-grp="' + g.id + '" title="' + esc(g.label) + '">' + icon(g.icon) + '<span class="m-label">' + esc(g.label) + '</span>' + icon("chevron", "m-chev") + '</button><div class="m-sub">' +
        items.map(k => '<a href="#/r/' + k.slug + '" data-key="r/' + k.slug + '"' + (k.long ? ' title="' + esc(k.long) + '"' : "") + '><span class="m-label">' + esc(k.label) + '</span><span class="m-cnt" data-cnt="' + k.kind + '"></span></a>').join("") +
        (g.extra || []).map(x => '<a href="' + x.href + '" data-key="' + x.key + '"><span class="m-label">' + esc(x.label) + '</span><span class="m-cnt" data-cnt="' + x.key + '"></span></a>').join("") + '</div>';
    });
    h += '<div class="m-nav-sec">Administration</div>' + link("#/access", "admin", "Access & authentication", "access") + link("#/settings", "settings", "Settings", "settings");
    $("mNav").innerHTML = h;
    $("mNav").querySelectorAll("[data-grp]").forEach(b => b.addEventListener("click", () => {
      const shell = $("shell");
      if (shell.classList.contains("rail") || (innerWidth <= 1100 && innerWidth > 760 && !shell.classList.contains("wide"))) {
        const first = b.nextElementSibling.querySelector("a"); if (first) location.hash = first.getAttribute("href"); return;
      }
      const open = b.getAttribute("aria-expanded") !== "false";
      b.setAttribute("aria-expanded", String(!open));
      collapsed[b.getAttribute("data-grp")] = open;
      try { localStorage.setItem("mantis-nav", JSON.stringify(collapsed)); } catch (e) { /* ignore */ }
    }));
  }
  function markActiveNav(r) {
    let key = r.page;
    if (r.page === "list") key = "r/" + r.kind.slug;
    if (r.page === "object") { const n = store.node(r.id); key = n && SLUG_BY_KIND.has(n.kind) ? "r/" + SLUG_BY_KIND.get(n.kind) : ""; }
    $("mNav").querySelectorAll("a").forEach(a => {
      const on = a.getAttribute("data-key") === key;
      a.classList.toggle("active", on);
      if (on) a.setAttribute("aria-current", "page"); else a.removeAttribute("aria-current");
      if (on) { const grp = a.closest(".m-sub"); if (grp && grp.previousElementSibling.getAttribute("aria-expanded") === "false") grp.previousElementSibling.setAttribute("aria-expanded", "true"); }
    });
  }
  function renderNavCounts() {
    const counts = {};
    store.nodes.forEach(n => { if (n.resolved !== false && (!S.ns || !n.ns || n.ns === S.ns)) counts[n.kind] = (counts[n.kind] || 0) + 1; });
    counts.namespaces = store.namespaces().length;
    document.querySelectorAll("[data-cnt]").forEach(el => { const c = counts[el.getAttribute("data-cnt")]; el.textContent = c ? fmt(c) : ""; });
  }
  const closeDrawer = () => $("shell").classList.remove("drawer");

  // ------------------------------------------------------------------ topbar
  function renderTopbar(r) {
    const meta = store.meta || {};
    $("mCluster").innerHTML = '<span class="m-ctx-k">Cluster</span><b title="' + esc(meta.server || "") + '">' + esc(meta.context || "…") + '</b>';
    const sel = $("mNs");
    const nss = store.namespaces();
    const cur = S.ns && !nss.includes(S.ns) ? "" : S.ns;
    if (sel.dataset.sig !== nss.join(",") + "|" + cur) {
      sel.innerHTML = '<option value="">All namespaces</option>' + nss.map(ns => '<option value="' + esc(ns) + '"' + (ns === cur ? " selected" : "") + '>' + esc(ns) + '</option>').join("");
      sel.dataset.sig = nss.join(",") + "|" + cur;
    }
    // The namespace filter applies where it means something.
    sel.disabled = ["settings", "access"].includes(r.page);
    const titles = { dashboard: "Dashboard", topology: "Topology", namespaces: "Namespaces", events: "Events", access: "Access & authentication", settings: "Settings", notfound: "Not found" };
    $("mTitle").innerHTML = r.page === "list" ? esc(r.kind.label) : r.page === "object" ? esc((store.node(r.id) || {}).kind || "Resource") : esc(titles[r.page] || "");
    $("mThemeBtn").innerHTML = icon(S.theme === "dark" ? "sun" : "moon");
    $("mThemeBtn").setAttribute("aria-label", S.theme === "dark" ? "Switch to light theme" : "Switch to dark theme");
    $("mThemeBtn").title = $("mThemeBtn").getAttribute("aria-label");
  }
  function renderSync() {
    const b = $("mSync");
    b.classList.toggle("syncing", S.syncing);
    b.classList.toggle("offline", !!S.offline || (!S.loaded && !!S.error));
    const label = S.offline ? "Connection lost" : S.syncing && !S.loaded ? "Connecting…" : S.lastSync ? "Synced " + ago(S.lastSync) : "Connecting…";
    b.innerHTML = '<span class="m-dot"></span><span>' + esc(label) + '</span>';
    b.title = (S.error ? "Last error: " + S.error + "\n" : "") + "Revision " + (store.revision || "—") + " · refreshes every " + (S.refreshMs ? S.refreshMs / 1000 + "s" : "— (manual)") + ". Click to refresh now.";
  }
  function ago(d) { const s = Math.round((Date.now() - d.getTime()) / 1000); return s < 5 ? "just now" : s < 60 ? s + "s ago" : Math.round(s / 60) + "m ago"; }
  setInterval(renderSync, 5000);

  // global search
  let gMatches = [], gActive = -1;
  function renderGlobalSearch() {
    const q = $("mSearch").value.trim(), box = $("mSearchResults");
    if (!q) { box.classList.remove("open"); return; }
    const res = G.search(store, q, 400);
    const groups = new Map();
    res.results.filter(n => n.resolved !== false).forEach(n => { if (!groups.has(n.kind)) groups.set(n.kind, []); groups.get(n.kind).push(n); });
    gMatches = [];
    let h = "";
    Array.from(groups).sort((a, b) => G.cmpNode({ kind: a[0], name: "", id: "" }, { kind: b[0], name: "", id: "" })).forEach(([kind, list]) => {
      const slug = SLUG_BY_KIND.get(kind);
      h += '<div class="m-gr-h"><span>' + esc(kind) + ' · ' + list.length + '</span>' + (slug ? '<a class="m-link" href="#/r/' + slug + '" data-close>all ' + esc(kind) + 's</a>' : "") + '</div>';
      list.slice(0, 6).forEach(n => {
        gMatches.push(n);
        h += '<div class="m-gr" role="option" data-i="' + (gMatches.length - 1) + '"><span class="m-badge" style="padding:0 6px;background:none"></span><span class="m-gr-name">' + esc(n.name) + '</span><span class="m-gr-ns">' + esc(n.ns || "cluster-scoped") + '</span></div>';
      });
    });
    if (!gMatches.length) h = '<div class="m-gr-empty">No resource matches “' + esc(q) + '”. Try <code>kind:Pod</code>, <code>ns:payments</code> or <code>label:app=web</code>.</div>';
    box.innerHTML = h; box.classList.add("open");
    gActive = gMatches.length ? 0 : -1; markGActive();
    box.querySelectorAll(".m-gr").forEach(el => el.addEventListener("mousedown", ev => { ev.preventDefault(); openResult(+el.getAttribute("data-i")); }));
    box.querySelectorAll("[data-close]").forEach(el => el.addEventListener("mousedown", () => setTimeout(() => box.classList.remove("open"), 0)));
  }
  function markGActive() { $("mSearchResults").querySelectorAll(".m-gr").forEach(el => el.classList.toggle("active", +el.getAttribute("data-i") === gActive)); }
  function openResult(i) { const n = gMatches[i]; if (!n) return; $("mSearchResults").classList.remove("open"); $("mSearch").blur(); location.hash = objHref(n.id); }

  // ------------------------------------------------------------------ pages
  let topo = null;
  function render() {
    const r = route();
    renderTopbar(r); markActiveNav(r);
    const content = $("mContent"), topoEl = $("topoPage");
    const isTopo = r.page === "topology";
    content.hidden = isTopo; topoEl.hidden = !isTopo;
    if (isTopo) { showTopology(r); return; }
    if (!S.loaded) { content.innerHTML = S.error ? errorPage() : loadingPage(); return; }
    const pages = { dashboard: pageDashboard, list: pageList, object: pageObject, namespaces: pageNamespaces, events: pageEvents, access: pageAccess, settings: pageSettings };
    (pages[r.page] || pageNotFound)(r, content);
  }
  // rerender redraws the current page after new data without losing the
  // user's place (table filters live in S.table, scroll is preserved).
  function rerender() {
    const r = route();
    if (r.page === "topology") { renderTopbar(r); return; }
    const c = $("mContent"), top = c.scrollTop;
    const focus = document.activeElement && document.activeElement.id;
    render(); c.scrollTop = top;
    if (focus && $(focus) && $(focus).tagName === "INPUT") { const el = $(focus); el.focus(); el.setSelectionRange(el.value.length, el.value.length); }
  }
  const loadingPage = () => '<div class="m-page"><div class="m-grid m-g-4">' + Array(4).fill('<div class="m-card m-stat"><div class="m-skel" style="width:40%"></div><div class="m-skel" style="width:60%;height:24px;margin-top:10px"></div></div>').join("") +
    '</div><div class="m-card" style="margin-top:14px;padding:18px"><div class="m-skel"></div><div class="m-skel" style="margin-top:10px;width:80%"></div><div class="m-skel" style="margin-top:10px;width:60%"></div></div><p class="m-muted" style="margin-top:14px">Reading resources and relationships from your cluster…</p></div>';
  const errorPage = () => '<div class="m-page"><div class="m-card m-empty"><b>Cannot read the cluster</b>' + esc(S.error) + '<p>Is the Mantis engine running, and can it reach the Kubernetes API?</p><button class="m-btn primary" onclick="location.reload()">Retry</button></div></div>';
  function pageNotFound(r, el) { el.innerHTML = '<div class="m-page"><div class="m-card m-empty"><b>Page not found</b>Nothing lives at this address. <a class="m-link" href="#/dashboard">Go to the dashboard</a></div></div>'; }

  // ---------------------------------------------------------------- dashboard
  function pageDashboard(r, el) {
    if (!S.events) loadEvents().then(() => route().page === "dashboard" && rerender());
    if (!S.capacity) loadCapacity().then(() => route().page === "dashboard" && rerender());
    const meta = store.meta || {};
    const inNs = n => !S.ns || !n.ns || n.ns === S.ns;
    const pods = allOf("Pod").filter(inNs), nodes = allOf("Node");
    const podState = { Running: 0, Pending: 0, Failed: 0, Completed: 0 };
    pods.forEach(p => { const s = statusOf(p); if (s.t === "Completed") podState.Completed++; else if (s.c === "ok") podState.Running++; else if (s.c === "warn") podState.Pending++; else podState.Failed++; });
    const wl = WORKLOAD_KINDS.reduce((a, k) => a + allOf(k).filter(inNs).length, 0);
    const unavailable = ["Deployment", "StatefulSet", "DaemonSet"].flatMap(k => allOf(k).filter(inNs)).filter(n => { const s = statusOf(n); return s && (s.c === "crit" || s.c === "warn"); });
    const readyNodes = nodes.filter(n => statusOf(n).c === "ok").length;
    const broken = []; store.nodes.forEach(n => { if (n.resolved === false && !n.unverified && inNs(n)) broken.push(n); });
    const ev = S.events || {}, warnings = (ev.events || []).filter(e => e.type === "Warning" && (!S.ns || e.ns === S.ns));
    const nsCount = store.namespaces().length;
    const scope = S.ns ? 'namespace <b class="m-mono">' + esc(S.ns) + '</b>' : "the whole cluster";
    const stat = (k, v, s, href, cls) => '<div class="m-card m-stat">' + (href ? '<a href="' + href + '">' : "") + '<div class="m-stat-k">' + k + '</div><div class="m-stat-v"' + (cls ? ' style="color:var(--m-' + cls + ')"' : "") + '>' + v + '</div><div class="m-stat-s">' + s + '</div>' + (href ? "</a>" : "") + '</div>';

    let h = '<div class="m-page"><div class="m-page-h"><div><h1>Cluster overview</h1><p>What is happening in ' + scope + ' right now — every number below is read from the Kubernetes API.</p></div><div class="m-grow"></div><button class="m-btn" data-report>' + icon("report") + 'Download report</button><a class="m-btn primary" href="#/topology">' + icon("topology") + 'Open topology</a></div>';

    // A. cluster overview
    h += '<div class="m-card" style="margin-bottom:14px"><div class="m-card-b" style="display:flex;flex-wrap:wrap;gap:10px 28px;align-items:center">' +
      [["Cluster", '<span class="m-mono">' + esc(meta.context || "—") + '</span>'], ["Kubernetes", '<span class="m-mono">' + esc(meta.version || "—") + '</span>'],
        ["Connection", S.offline ? '<span class="m-badge crit">lost</span>' : '<span class="m-badge ok">connected</span>'],
        ["Last sync", esc(S.lastSync ? S.lastSync.toLocaleTimeString() : "—")], ["Nodes", fmt(nodes.length)], ["Namespaces", fmt(nsCount)], ["Workloads", fmt(wl)], ["Pods", fmt(pods.length)],
        ...(meta.nodeAutoscaler ? [["Node autoscaler", esc(meta.nodeAutoscaler)]] : [])]
        .map(([k, v]) => '<div><div class="m-muted" style="font-size:11.5px">' + k + '</div><div style="margin-top:3px;font-weight:500">' + v + '</div></div>').join("") +
      (meta.partial ? '<div class="m-note warn" style="flex-basis:100%">Mantis is still reading the cluster for the first time; counts are growing.</div>' : "") +
      ((meta.skipped || []).length ? '<div class="m-note warn" style="flex-basis:100%">Not visible to Mantis (RBAC forbids listing): ' + (meta.skipped || []).map(s => '<code>' + esc(s) + '</code>').join(", ") + '. Related counts may be incomplete.</div>' : "") +
      '</div></div>';

    // B. health
    h += '<div class="m-grid m-g-4" style="margin-bottom:14px">' +
      stat(icon("workloads") + "Pods running", fmt(podState.Running) + '<span class="m-muted" style="font-size:14px;font-weight:400"> / ' + fmt(pods.length) + '</span>',
        (podState.Pending ? '<span style="color:var(--m-warn-ink)">' + podState.Pending + " pending</span> · " : "") + (podState.Failed ? '<span style="color:var(--m-crit-ink)">' + podState.Failed + " failing</span>" : "none failing") + (podState.Completed ? " · " + podState.Completed + " completed" : ""), "#/r/pods") +
      stat(icon("workloads") + "Workloads unavailable", fmt(unavailable.length), unavailable.length ? "Deployments, StatefulSets or DaemonSets below desired replicas" : "every workload has its desired replicas", "#/r/deployments", unavailable.length ? "crit" : "") +
      stat(icon("cluster") + "Nodes ready", fmt(readyNodes) + '<span class="m-muted" style="font-size:14px;font-weight:400"> / ' + fmt(nodes.length) + '</span>', nodes.length - readyNodes ? '<span style="color:var(--m-crit-ink)">' + (nodes.length - readyNodes) + " not ready</span>" : "all nodes Ready", "#/r/nodes", nodes.length - readyNodes ? "crit" : "") +
      stat(icon("events") + "Warning events", ev.forbidden ? "—" : fmt(warnings.length), ev.forbidden ? "RBAC does not allow listing events" : (broken.length ? '<span style="color:var(--m-crit-ink)">' + broken.length + " broken reference" + (broken.length > 1 ? "s" : "") + "</span> in the graph" : "no broken references"), "#/events", warnings.length ? "warn" : "") +
      '</div>';

    // C. distribution + D. capacity
    const DIST_CATS = [["workload", "Workloads"], ["pod", "Pods"], ["network", "Networking"], ["config", "Config"], ["storage", "Storage"], ["security", "RBAC"]];
    const nsDist = store.namespaces().map(ns => {
      const by = {}; let total = 0;
      (store.byNs.get(ns) || []).forEach(id => { const n = store.node(id); if (!n || n.resolved === false) return; const c = DIST_CATS.some(d => d[0] === n.cat) ? n.cat : null; if (c) { by[c] = (by[c] || 0) + 1; total++; } });
      return { ns, by, total };
    }).filter(x => x.total).sort((a, b) => b.total - a.total).slice(0, 8);
    const maxDist = Math.max(1, ...nsDist.map(x => x.total));
    const podsByNode = nodes.map(n => [n, relCount(n.id, ["runs-on"], "in")]).sort((a, b) => b[1] - a[1]).slice(0, 8);
    const maxPn = Math.max(1, ...podsByNode.map(x => x[1]));
    const seg = (n, c) => n ? '<span style="width:' + (100 * n / Math.max(1, pods.length)) + '%;background:var(--m-' + c + ')" title="' + n + '"></span>' : "";
    h += '<div class="m-grid m-g-3" style="margin-bottom:14px">' +
      '<div class="m-card"><div class="m-card-h"><h2>Pod status</h2><span class="m-card-sub">' + fmt(pods.length) + ' pods</span></div><div class="m-card-b">' +
        (pods.length ? '<div class="m-stack">' + seg(podState.Running, "ok") + seg(podState.Pending, "warn") + seg(podState.Failed, "crit") + seg(podState.Completed, "unknown") + '</div>' +
        '<div class="m-legend"><span><i style="background:var(--m-ok)"></i>Running ' + podState.Running + '</span><span><i style="background:var(--m-warn)"></i>Pending ' + podState.Pending + '</span><span><i style="background:var(--m-crit)"></i>Failing ' + podState.Failed + '</span><span><i style="background:var(--m-unknown)"></i>Completed ' + podState.Completed + '</span></div>'
          : '<div class="m-empty">No pods.</div>') + '</div></div>' +
      '<div class="m-card"><div class="m-card-h"><h2>Resources by namespace</h2><a href="#/namespaces">All namespaces</a></div><div class="m-card-b"><div class="m-dist">' +
        (nsDist.length ? nsDist.map(x => '<div class="m-dist-row"><a class="m-bl" href="#/dashboard" data-ns="' + esc(x.ns) + '" title="Scope the dashboard to ' + esc(x.ns) + '">' + esc(x.ns) + '</a><div class="m-stack" style="width:' + Math.max(4, 100 * x.total / maxDist) + '%" title="' + esc(DIST_CATS.map(([c, l]) => l + " " + (x.by[c] || 0)).join(" · ")) + '">' +
          DIST_CATS.map(([c]) => x.by[c] ? '<span style="width:' + (100 * x.by[c] / x.total) + '%;background:var(--cat-' + c + ')"></span>' : "").join("") + '</div><span class="num">' + x.total + '</span></div>').join("") : '<div class="m-empty">No namespaced resources.</div>') +
        '</div><div class="m-legend">' + DIST_CATS.map(([c, l]) => '<span><i style="background:var(--cat-' + c + ')"></i>' + l + '</span>').join("") + '</div></div></div>' +
      '<div class="m-card"><div class="m-card-h"><h2>Pods per node</h2><a href="#/topology?view=node">Node view</a></div><div class="m-card-b"><div class="m-bars">' +
        (podsByNode.length ? podsByNode.map(([n, c]) => '<div class="m-bar-row"><a class="m-bl" href="' + objHref(n.id) + '">' + esc(n.name) + '</a><div class="m-bar"><span style="width:' + (100 * c / maxPn) + '%;background:var(--m-' + (statusOf(n).c === "ok" ? "info" : "crit") + ')"></span></div><span class="num">' + c + '</span></div>').join("") : '<div class="m-empty">No nodes.</div>') + '</div></div></div>' +
      '</div>';

    h += '<div class="m-grid m-g-3">' + capacityCard() +
      '<div class="m-card m-span-2"><div class="m-card-h"><h2>Recent warnings</h2><span class="m-card-sub">Kubernetes Events</span><a href="#/events">All events</a></div>' + eventsList(warnings.slice(0, 7), "No warning events" + (S.ns ? " in this namespace" : "") + ". Kubernetes keeps events for about an hour.") + '</div>' +
      '</div>';

    // E. needs attention
    const failingPods = pods.filter(p => statusOf(p).c === "crit").slice(0, 6);
    const attn = unavailable.slice(0, 6).map(n => ({ n, why: (attrMap(n).replicas || attrMap(n).scheduled || "") }))
      .concat(failingPods.map(n => ({ n, why: (attrMap(n)._free[0] || "") })))
      .concat(broken.slice(0, 6).map(n => ({ n, why: "referenced by " + store.inEdges(n.id).map(e => short(e.from)).slice(0, 2).join(", ") + " but missing" })));
    h += '<div class="m-grid m-g-2" style="margin-top:14px"><div class="m-card"><div class="m-card-h"><h2>Needs attention</h2><span class="m-card-sub">unavailable, failing or broken</span></div>' +
      (attn.length ? '<ul class="m-list">' + attn.slice(0, 12).map(({ n, why }) => '<li>' + badge(statusOf(n)) + '<div class="m-li-main"><div class="m-li-t"><a class="m-link m-mono" href="' + objHref(n.id) + '">' + esc(n.kind + "/" + n.name) + '</a></div><div class="m-li-s">' + esc(why) + '</div></div><span class="m-li-r">' + esc(n.ns || "cluster") + '</span></li>').join("") + '</ul>'
        : '<div class="m-empty"><b>Nothing needs attention</b>Every workload is available, every pod is healthy and every reference resolves.</div>') + '</div>';
    const chg = changeLog.filter(c => !S.ns || c.ns === S.ns).slice(0, 12);
    h += '<div class="m-card"><div class="m-card-h"><h2>Recent changes</h2><span class="m-card-sub">since this page opened · from graph revisions</span></div>' +
      (chg.length ? '<ul class="m-list">' + chg.map(c => '<li><span class="m-badge m-chg ' + (c.what === "added" ? "ok" : c.what === "removed" ? "crit" : "info") + '">' + c.what + '</span><div class="m-li-main"><div class="m-li-t">' +
        (c.what === "removed" ? '<span class="m-mono">' + esc(c.kind + "/" + c.name) + '</span>' : '<a class="m-link m-mono" href="' + objHref(c.id) + '">' + esc(c.kind + "/" + c.name) + '</a>') + '</div><div class="m-li-s">' + esc(c.ns || "cluster-scoped") + '</div></div><span class="m-li-r">' + esc(age(c.at.toISOString())) + ' ago</span></li>').join("") + '</ul>'
        : '<div class="m-empty"><b>No changes yet</b>Mantis checks for changes every ' + (S.refreshMs ? S.refreshMs / 1000 + " seconds" : "time you refresh") + '; anything added, removed or updated in the cluster appears here.</div>') + '</div></div>';

    h += '</div>';
    el.innerHTML = h;
    el.querySelectorAll("[data-ns]").forEach(a => a.addEventListener("click", () => setNs(a.getAttribute("data-ns"))));
  }
  function capacityCard() {
    const c = S.capacity;
    let b;
    const cpu = m => (m / 1000).toFixed(m % 1000 ? 1 : 0) + " cores";
    const mem = x => { const g = x / 1073741824; return g >= 1 ? g.toFixed(1) + " GiB" : (x / 1048576).toFixed(0) + " MiB"; };
    if (!c) b = '<div class="m-skel"></div><div class="m-skel" style="margin-top:10px;width:70%"></div>';
    else if (c.error) b = '<div class="m-note crit">Could not read node capacity: ' + esc(c.error) + '</div>';
    else {
      const gauge = (label, used, total, f) => '<div class="m-gauge"><div style="font-size:12.5px;font-weight:500">' + label + '</div><div class="m-bar"><span style="width:' + Math.min(100, 100 * used / Math.max(1, total)) + '%;background:var(--m-' + (used / total > .85 ? "crit" : used / total > .7 ? "warn" : "accent") + ')"></span></div><div class="m-gauge-l"><span>' + f(used) + ' used</span><span>' + Math.round(100 * used / Math.max(1, total)) + '% of ' + f(total) + '</span></div></div>';
      b = c.metrics
        ? gauge("CPU", c.cpuUsageMilli, c.cpuAllocatableMilli, cpu) + gauge("Memory", c.memUsageBytes, c.memAllocatableBytes, mem)
        : '<div class="m-kv"><div class="k">Allocatable CPU</div><div class="v">' + cpu(c.cpuAllocatableMilli) + '</div><div class="k">Allocatable memory</div><div class="v">' + mem(c.memAllocatableBytes) + '</div><div class="k">Pod capacity</div><div class="v">' + fmt(c.podCapacity) + '</div></div>' +
          '<div class="m-note neutral" style="margin-top:10px">Live usage is not shown: ' + esc(c.metricsReason || "metrics unavailable") + '. Install <code>metrics-server</code> to see CPU and memory utilization. Mantis does not estimate usage.</div>';
      if (c.metrics) b += '<div class="m-muted" style="font-size:12px;margin-top:10px">Usage from metrics.k8s.io · allocatable from node status · pod capacity ' + fmt(c.podCapacity) + '</div>';
    }
    return '<div class="m-card"><div class="m-card-h"><h2>Cluster capacity</h2><span class="m-card-sub">' + (c && !c.error ? c.readyNodes + "/" + c.nodes + " nodes ready" : "") + '</span></div><div class="m-card-b">' + b + '</div></div>';
  }
  function eventsList(list, emptyText) {
    if (!S.events) return '<div class="m-card-b"><div class="m-skel"></div><div class="m-skel" style="margin-top:10px;width:70%"></div></div>';
    if (S.events.error) return '<div class="m-card-b"><div class="m-note crit">Could not read events: ' + esc(S.events.error) + '</div></div>';
    if (S.events.forbidden) return '<div class="m-card-b"><div class="m-note warn">Mantis is not allowed to list events. Grant <code>list</code> on <code>events</code> to its ServiceAccount (the Helm chart does this).</div></div>';
    if (!list.length) return '<div class="m-empty">' + esc(emptyText) + '</div>';
    return '<ul class="m-list">' + list.map(e => {
      const known = store.node(e.objectId);
      const obj = known ? '<a class="m-link m-mono" href="' + objHref(e.objectId) + '">' + esc(e.kind + "/" + e.name) + '</a>' : '<span class="m-mono">' + esc(e.kind + "/" + e.name) + '</span>';
      return '<li><span class="m-badge ' + (e.type === "Warning" ? "warn" : "info") + '">' + esc(e.reason || e.type) + '</span><div class="m-li-main"><div class="m-li-t">' + obj + (e.count > 1 ? ' <span class="m-muted">×' + e.count + '</span>' : "") + '</div><div class="m-li-s">' + esc(e.message) + '</div></div><span class="m-li-r" title="' + esc(e.last) + '">' + esc(age(e.last)) + ' ago</span></li>';
    }).join("") + '</ul>';
  }

  // ---------------------------------------------------------------- resource list
  function pageList(r, el) {
    const k = r.kind;
    const cols = COLUMNS[k.kind] || [NAME, NS, AGE];
    const ts = S.table[k.kind] || (S.table[k.kind] = { q: "", status: "", sort: 0, dir: 1, page: 0 });
    const all = visibleNodes(k.kind);
    const statuses = Array.from(new Set(all.map(n => (statusOf(n) || {}).t).filter(Boolean))).sort();
    const q = ts.q.trim().toLowerCase();
    let rows = all.filter(n => (!q || n.name.toLowerCase().includes(q) || (n.ns || "").includes(q) || Object.entries(n.labels || {}).some(([a, b]) => (a + "=" + b).toLowerCase().includes(q))) &&
      (!ts.status || (statusOf(n) || {}).t === ts.status));
    const key = (cols[ts.sort] || cols[0])[2];
    rows.sort((a, b) => { const x = key(a), y = key(b); return (x < y ? -1 : x > y ? 1 : 0) * ts.dir || (a.name < b.name ? -1 : 1); });
    const PAGE = 100, pages = Math.max(1, Math.ceil(rows.length / PAGE));
    ts.page = Math.min(ts.page, pages - 1);
    const view = rows.slice(ts.page * PAGE, (ts.page + 1) * PAGE);
    const scoped = !!(all.length && all[0].ns !== undefined && all.some(n => n.ns));

    let h = '<div class="m-page"><div class="m-page-h"><div><div class="m-crumbs"><a href="#/dashboard">Dashboard</a> › ' + esc(GROUPS.find(g => g.id === k.g).label) + '</div><h1>' + esc(k.label) + '</h1>' +
      '<p>' + (S.ns && scoped ? 'In namespace <b class="m-mono">' + esc(S.ns) + '</b>' : scoped ? "Across all namespaces" : "Cluster-scoped") + '</p></div><div class="m-grow"></div>' +
      '<a class="m-btn" href="#/topology">' + icon("topology") + 'See in topology</a></div>';
    if (FILTERED_KINDS[k.kind]) h += '<div class="m-note neutral" style="margin-bottom:12px">System-managed ' + esc(k.label) + ' (for example service-account tokens, Helm release data, <code>default</code> ServiceAccounts, <code>system:</code> RBAC, and ClusterRoles nothing visible uses) are hidden. Run the engine with <code>MANTIS_SHOW_ALL=true</code> to include them.</div>';
    h += '<div class="m-card"><div class="m-toolbar"><input class="m-input" id="tQ" type="search" placeholder="Filter by name, namespace or label…" value="' + esc(ts.q) + '" style="width:min(320px,100%)" aria-label="Filter ' + esc(k.label) + '">' +
      (statuses.length > 1 ? '<select class="m-input" id="tStatus" aria-label="Filter by status"><option value="">All statuses</option>' + statuses.map(s => '<option' + (s === ts.status ? " selected" : "") + '>' + esc(s) + '</option>').join("") + '</select>' : "") +
      '<button class="m-btn" id="tRefresh" title="Refresh now">' + icon("refresh") + 'Refresh</button>' +
      '<span class="m-count">' + (rows.length === all.length ? fmt(all.length) + " " + esc(k.label) : fmt(rows.length) + " of " + fmt(all.length)) + '</span></div>';
    if (!all.length) {
      h += '<div class="m-empty"><b>No ' + esc(k.label) + (S.ns && scoped ? ' in ' + esc(S.ns) : "") + '</b>' +
        ((store.meta.skipped || []).includes(k.slug) ? "Mantis is not allowed to list " + esc(k.label) + " — RBAC forbids it." : S.ns && scoped ? '<a class="m-link" id="tAllNs">Show all namespaces</a>' : "There are none in this cluster.") + '</div>';
    } else if (!rows.length) {
      h += '<div class="m-empty"><b>No matches</b>Nothing matches the current filter.</div>';
    } else {
      h += '<div class="m-table-wrap"><table class="m-table"><thead><tr>' + cols.map((c, i) => {
        const sorted = i === ts.sort;
        return '<th' + (sorted ? ' aria-sort="' + (ts.dir > 0 ? "ascending" : "descending") + '"' : "") + '><button data-sort="' + i + '">' + esc(c[0]) + '<span class="m-sort">' + (sorted ? (ts.dir > 0 ? "▲" : "▼") : "↕") + '</span></button></th>';
      }).join("") + '</tr></thead><tbody>' +
        view.map(n => '<tr tabindex="0" data-href="' + objHref(n.id) + '">' + cols.map((c, i) => '<td class="' + (i === 0 ? "name" : c[3] || "") + '">' + c[1](n) + '</td>').join("") + '</tr>').join("") + '</tbody></table></div>';
      if (pages > 1) h += '<div class="m-pager"><span>Showing ' + fmt(ts.page * PAGE + 1) + '–' + fmt(Math.min(rows.length, (ts.page + 1) * PAGE)) + ' of ' + fmt(rows.length) + '</span><span class="m-grow"></span>' +
        '<button class="m-btn" id="tPrev"' + (ts.page === 0 ? " disabled" : "") + '>Previous</button><button class="m-btn" id="tNext"' + (ts.page >= pages - 1 ? " disabled" : "") + '>Next</button></div>';
    }
    h += '</div></div>';
    el.innerHTML = h;
    const qEl = $("tQ");
    qEl.addEventListener("input", () => { ts.q = qEl.value; ts.page = 0; rerender(); });
    if ($("tStatus")) $("tStatus").addEventListener("change", ev => { ts.status = ev.target.value; ts.page = 0; rerender(); });
    $("tRefresh").addEventListener("click", () => tick());
    if ($("tAllNs")) $("tAllNs").addEventListener("click", () => setNs(""));
    if ($("tPrev")) $("tPrev").addEventListener("click", () => { ts.page--; rerender(); });
    if ($("tNext")) $("tNext").addEventListener("click", () => { ts.page++; rerender(); });
    el.querySelectorAll("[data-sort]").forEach(b => b.addEventListener("click", () => { const i = +b.getAttribute("data-sort"); if (ts.sort === i) ts.dir = -ts.dir; else { ts.sort = i; ts.dir = 1; } rerender(); }));
    el.querySelectorAll("tr[data-href]").forEach(tr => {
      tr.addEventListener("click", ev => { if (!ev.target.closest("a")) location.hash = tr.getAttribute("data-href"); });
      tr.addEventListener("keydown", ev => { if (ev.key === "Enter") location.hash = tr.getAttribute("data-href"); });
    });
  }

  // ---------------------------------------------------------------- resource detail
  const CONDITION_KINDS = { Pod: 1, Node: 1, Deployment: 1, StatefulSet: 1, ReplicaSet: 1, DaemonSet: 1, Job: 1, PersistentVolumeClaim: 1, HorizontalPodAutoscaler: 1 };
  const objCache = new Map(); // id -> {json, yaml, at}
  function fetchObject(n, format) {
    const p = new URLSearchParams({ kind: n.kind, name: n.name }); if (n.ns) p.set("ns", n.ns);
    if (format) p.set("format", format);
    return fetch("/api/resource?" + p.toString(), { cache: "no-store" }).then(async r => ({ ok: r.ok, status: r.status, text: await r.text() }));
  }
  function pageObject(r, el) {
    const n = store.node(r.id);
    if (!n) { el.innerHTML = '<div class="m-page"><div class="m-card m-empty"><b>Resource not found</b><span class="m-mono">' + esc(r.id) + '</span> is not in the current graph — it may have been deleted, or it is hidden as system-managed.<p><a class="m-link" href="#/dashboard">Back to the dashboard</a></p></div></div>'; return; }
    const isSecret = n.kind === "Secret", missing = n.resolved === false;
    const evs = S.events ? (S.events.events || []).filter(e => e.objectId === n.id) : null;
    if (!S.events && !missing) loadEvents().then(() => route().page === "object" && rerender());
    const tabs = [["overview", "Overview"]];
    if (!missing) tabs.push(["yaml", "YAML"]);
    if (!missing) tabs.push(["events", "Events", evs ? evs.length : null]);
    if (CONDITION_KINDS[n.kind] && !missing) tabs.push(["conditions", "Conditions"]);
    tabs.push(["related", "Related resources", store.outEdges(n.id).length + store.inEdges(n.id).length]);
    const tab = tabs.some(t => t[0] === r.tab) ? r.tab : "overview";
    const slug = SLUG_BY_KIND.get(n.kind);

    let h = '<div class="m-page"><div class="m-crumbs"><a href="#/dashboard">Dashboard</a> › ' + (slug ? '<a href="#/r/' + slug + '">' + esc(KIND_BY_SLUG.get(slug).label) + '</a>' : esc(n.kind)) + (n.ns ? ' › <span class="m-mono">' + esc(n.ns) + '</span>' : "") + '</div>' +
      '<div class="m-detail-h"><div><span class="m-kindtag">' + esc(n.kind) + '</span> ' + badge(statusOf(n) || (missing ? null : { t: "Exists", c: "" })) + '<h1>' + esc(n.name) + '</h1></div>' +
      '<div class="m-detail-actions"><a class="m-btn" href="#/topology?focus=' + enc(n.id) + '">' + icon("topology") + 'Dependencies</a><a class="m-btn" href="#/topology?select=' + enc(n.id) + '">Show in topology</a></div></div>' +
      '<div class="m-tabs" role="tablist">' + tabs.map(([id, label, cnt]) => '<button role="tab" aria-selected="' + (id === tab) + '" data-tab="' + id + '">' + esc(label) + (cnt != null ? '<span class="m-tab-n">' + cnt + '</span>' : "") + '</button>').join("") + '</div><div id="oBody"></div></div>';
    el.innerHTML = h;
    el.querySelectorAll("[data-tab]").forEach(b => b.addEventListener("click", () => { history.replaceState(null, "", objHref(n.id) + "?tab=" + b.getAttribute("data-tab")); render(); }));
    const body = $("oBody");

    if (tab === "overview") {
      const a = attrMap(n);
      const kv = [["Kind", esc(n.kind) + (n.apiVersion ? ' <span class="m-muted">' + esc(n.apiVersion) + '</span>' : "")], ["Name", esc(n.name)], ["Namespace", n.ns ? '<a class="m-link" data-setns="' + esc(n.ns) + '">' + esc(n.ns) + '</a>' : "— (cluster-scoped)"]];
      if (n.uid) kv.push(["UID", esc(n.uid)]);
      if (n.created) kv.push(["Created", esc(new Date(n.created).toLocaleString()) + ' <span class="m-muted">(' + esc(age(n.created)) + ' ago)</span>']);
      if (n.kind === "Pod") { const x = store.nodeOfPod(n.id); if (x) kv.push(["Node", '<a class="m-link" href="' + objHref(x) + '">' + esc(short(x)) + '</a>']); }
      const chain = []; let cur = store.ownerOf(n.id); const seen = new Set();
      while (cur && !seen.has(cur)) { seen.add(cur); chain.push(cur); cur = store.ownerOf(cur); }
      if (chain.length) kv.push(["Owners", chain.map(id => '<a class="m-link" href="' + objHref(id) + '">' + esc(id.split("/").slice(1).join("/")) + '</a>').join(" → ")]);
      let o = '<div class="m-grid m-g-2"><div class="m-card"><div class="m-card-h"><h2>Metadata</h2></div><div class="m-card-b"><div class="m-kv">' + kv.map(([k, v]) => '<div class="k">' + k + '</div><div class="v">' + v + '</div>').join("") + '</div></div></div>';
      const details = (n.attributes || []).map(x => { const i = x.indexOf(": "); return i < 0 ? ["Summary", x] : [x.slice(0, i), x.slice(i + 2)]; });
      o += '<div class="m-card"><div class="m-card-h"><h2>Specification</h2><span class="m-card-sub">key fields</span></div><div class="m-card-b">' +
        (missing ? '<div class="m-note ' + (n.unverified ? "neutral" : "crit") + '">' + (n.unverified ? "Referenced by another resource, but Mantis could not check whether it exists — its kind is not one Mantis lists, or RBAC forbids listing it." : "Referenced by another resource, but it does not exist in the cluster.") + '</div>'
          : details.length ? '<div class="m-kv">' + details.map(([k, v]) => '<div class="k">' + esc(k) + '</div><div class="v">' + esc(v) + '</div>').join("") + '</div>' : '<div class="m-muted">No additional fields recorded for this kind.</div>') + '</div></div></div>';
      o += '<div class="m-grid m-g-2" style="margin-top:14px"><div class="m-card"><div class="m-card-h"><h2>Labels</h2></div><div class="m-card-b">' +
        (n.labels && Object.keys(n.labels).length ? Object.keys(n.labels).sort().map(k => '<span class="m-pill" title="' + esc(k + "=" + n.labels[k]) + '">' + esc(k) + '=' + esc(n.labels[k]) + '</span>').join("") : '<span class="m-muted">No labels</span>') + '</div></div>' +
        '<div class="m-card"><div class="m-card-h"><h2>Annotations</h2></div><div class="m-card-b" id="oAnno">' + (isSecret ? '<span class="m-muted">Not shown for Secrets — annotations can carry Secret data.</span>' : missing ? '<span class="m-muted">—</span>' : '<div class="m-skel"></div>') + '</div></div></div>';
      if (isSecret) o += '<div class="m-note neutral" style="margin-top:14px">Secret values are never read into Mantis or shown in the UI — only that the Secret exists and what uses it.</div>';
      body.innerHTML = o;
      body.querySelectorAll("[data-setns]").forEach(x => x.addEventListener("click", () => { setNs(x.getAttribute("data-setns")); location.hash = "#/namespaces"; }));
      if (!isSecret && !missing) withJSON(n, obj => {
        const an = (obj.metadata && obj.metadata.annotations) || {};
        const ks = Object.keys(an).sort();
        const box = $("oAnno"); if (!box) return;
        box.innerHTML = ks.length ? '<div class="m-kv">' + ks.map(k => '<div class="k" style="overflow-wrap:anywhere">' + esc(k) + '</div><div class="v">' + esc(an[k].length > 400 ? an[k].slice(0, 400) + "…" : an[k]) + '</div>').join("") + '</div>' : '<span class="m-muted">No annotations</span>';
      }, err => { const box = $("oAnno"); if (box) box.innerHTML = '<span class="m-muted">' + esc(err) + '</span>'; });
    } else if (tab === "yaml") {
      if (isSecret) { body.innerHTML = '<div class="m-note neutral">Secret contents are hidden for security. Mantis never displays Secret data through the UI.</div>'; return; }
      body.innerHTML = '<div class="m-card"><div class="m-card-b"><div class="m-skel"></div></div></div>';
      fetchObject(n).then(res => {
        if (route().id !== n.id) return;
        body.innerHTML = res.ok ? '<div style="display:flex;justify-content:flex-end;margin-bottom:8px"><button class="m-btn" id="oCopy">Copy YAML</button></div><pre class="m-code">' + esc(res.text) + '</pre>'
          : '<div class="m-note ' + (res.status === 403 ? "warn" : "crit") + '">' + esc(res.text || "HTTP " + res.status) + '</div>';
        if ($("oCopy")) $("oCopy").addEventListener("click", () => navigator.clipboard && navigator.clipboard.writeText(res.text).then(() => { $("oCopy").textContent = "Copied"; }));
      });
    } else if (tab === "events") {
      body.innerHTML = '<div class="m-card">' + eventsList(evs || [], "No events for this resource. Kubernetes keeps events for about an hour.") + '</div>';
    } else if (tab === "conditions") {
      body.innerHTML = '<div class="m-card"><div class="m-card-b"><div class="m-skel"></div></div></div>';
      withJSON(n, obj => {
        const cs = (obj.status && obj.status.conditions) || [];
        body.innerHTML = '<div class="m-card">' + (cs.length ? '<div class="m-table-wrap"><table class="m-table"><thead><tr><th><button>Type</button></th><th><button>Status</button></th><th><button>Reason</button></th><th><button>Message</button></th><th><button>Last transition</button></th></tr></thead><tbody>' +
          cs.map(c => {
            const good = (c.status === "True") !== /pressure|unavailable|fail|disk|network/i.test(c.type || "");
            return '<tr style="cursor:default"><td class="m-mono">' + esc(c.type) + '</td><td><span class="m-badge ' + (c.status === "Unknown" ? "" : good ? "ok" : "warn") + '">' + esc(c.status) + '</span></td><td>' + esc(c.reason || "—") + '</td><td style="white-space:normal">' + esc(c.message || "—") + '</td><td>' + esc(c.lastTransitionTime ? age(c.lastTransitionTime) + " ago" : "—") + '</td></tr>';
          }).join("") + '</tbody></table></div>' : '<div class="m-empty">This resource reports no conditions.</div>') + '</div>';
      }, err => { body.innerHTML = '<div class="m-note crit">' + esc(err) + '</div>'; });
    } else {
      const cats = new Map();
      const add = (e, dir) => { const other = dir === "out" ? e.to : e.from; if (!cats.has(e.category)) cats.set(e.category, []); cats.get(e.category).push({ e, dir, other }); };
      store.outEdges(n.id).forEach(e => add(e, "out")); store.inEdges(n.id).forEach(e => add(e, "in"));
      let o = '<div class="m-card">';
      if (!cats.size) o += '<div class="m-empty">No relationships discovered for this resource.</div>';
      G.CATEGORIES.forEach(c => {
        if (!cats.has(c)) return;
        o += '<div class="m-rel-cat"><i style="border-color:var(--rel-' + c + ')"></i>' + esc(G.CATEGORY_LABEL[c]) + '</div><ul class="m-list">' + cats.get(c).map(({ e, dir, other }) => {
          const on = store.node(other);
          const verb = dir === "out" ? e.type : (G.REVERSE[e.type] || e.type);
          const st = on && on.resolved === false ? '<span class="m-badge ' + (on.unverified ? "" : "crit") + '">' + (on.unverified ? "unverified" : "not found") + '</span>' : "";
          return '<li><span class="m-muted" style="min-width:150px">' + esc(verb) + ' ' + (dir === "out" ? "→" : "←") + '</span><div class="m-li-main"><a class="m-link m-mono" href="' + objHref(other) + '">' + esc(on ? on.kind + "/" + on.name : other) + '</a> ' + st +
            '<div class="m-li-s">' + esc(G.BASIS_LABEL[e.basis] || e.basis) + (e.evidence ? " · " + esc(e.evidence) : "") + '</div></div><span class="m-li-r">' + esc(on ? on.ns || "cluster" : "") + '</span></li>';
        }).join("") + '</ul>';
      });
      body.innerHTML = o + '</div><p class="m-muted" style="font-size:12px;margin-top:10px">Relationships are what the Kubernetes API declares (ownership, spec references, label selectors, EndpointSlice records). Mantis does not observe network traffic.</p>';
    }
  }
  function withJSON(n, ok, fail) {
    const c = objCache.get(n.id);
    if (c && Date.now() - c.at < 15000) { ok(c.json); return; }
    fetchObject(n, "json").then(res => {
      if (!res.ok) { fail(res.text || "HTTP " + res.status); return; }
      try { const json = JSON.parse(res.text); objCache.set(n.id, { json, at: Date.now() }); ok(json); } catch (e) { fail(String(e)); }
    }).catch(e => fail(String(e)));
  }

  // ---------------------------------------------------------------- namespaces & events
  function pageNamespaces(r, el) {
    const rows = store.namespaces().map(ns => {
      const ids = store.byNs.get(ns) || [];
      let pods = 0, bad = 0, wl = 0, broken = 0;
      ids.forEach(id => { const n = store.node(id); if (n.resolved === false) { if (!n.unverified) broken++; return; } if (n.kind === "Pod") { pods++; if (statusOf(n).c === "crit") bad++; } if (WORKLOAD_KINDS.includes(n.kind)) wl++; });
      return { ns, total: ids.length, pods, bad, wl, broken };
    });
    el.innerHTML = '<div class="m-page"><div class="m-page-h"><div><div class="m-crumbs"><a href="#/dashboard">Dashboard</a> › Cluster</div><h1>Namespaces</h1><p>' + rows.length + ' namespaces. Selecting one scopes every page to it.</p></div></div>' +
      '<div class="m-card"><div class="m-table-wrap"><table class="m-table"><thead><tr><th><button>Name</button></th><th><button>Health</button></th><th><button>Workloads</button></th><th><button>Pods</button></th><th><button>Resources</button></th><th><button>Broken refs</button></th><th></th></tr></thead><tbody>' +
      rows.map(x => '<tr tabindex="0" data-ns="' + esc(x.ns) + '"><td class="name"><a>' + esc(x.ns) + '</a>' + (x.ns === S.ns ? ' <span class="m-badge info">selected</span>' : "") + '</td><td>' + (x.bad ? '<span class="m-badge crit">' + x.bad + ' failing</span>' : x.pods ? '<span class="m-badge ok">healthy</span>' : '<span class="m-muted">—</span>') +
        '</td><td class="num">' + x.wl + '</td><td class="num">' + x.pods + '</td><td class="num">' + x.total + '</td><td class="num">' + (x.broken ? '<span style="color:var(--m-crit-ink)">' + x.broken + '</span>' : "0") + '</td><td><a class="m-link" href="#/topology?ns=' + enc(x.ns) + '">Topology</a></td></tr>').join("") +
      '</tbody></table></div></div></div>';
    el.querySelectorAll("tr[data-ns]").forEach(tr => {
      const go = ev => { if (ev.target.closest("a.m-link")) return; setNs(tr.getAttribute("data-ns")); location.hash = "#/dashboard"; };
      tr.addEventListener("click", go); tr.addEventListener("keydown", ev => { if (ev.key === "Enter") go(ev); });
    });
  }
  function pageEvents(r, el) {
    if (!S.events) { loadEvents().then(() => route().page === "events" && rerender()); }
    const ts = S.table.__events || (S.table.__events = { type: "", q: "" });
    let list = S.events ? (S.events.events || []) : [];
    list = list.filter(e => (!S.ns || e.ns === S.ns) && (!ts.type || e.type === ts.type) && (!ts.q || (e.reason + " " + e.message + " " + e.name).toLowerCase().includes(ts.q.toLowerCase())));
    el.innerHTML = '<div class="m-page"><div class="m-page-h"><div><div class="m-crumbs"><a href="#/dashboard">Dashboard</a> › Cluster</div><h1>Events</h1><p>Most recent Kubernetes Events' + (S.ns ? ' in <b class="m-mono">' + esc(S.ns) + '</b>' : "") + (S.events && S.events.total ? " · " + fmt(S.events.total) + " in the cluster" : "") + '.</p></div></div>' +
      '<div class="m-card"><div class="m-toolbar"><input class="m-input" id="eQ" type="search" placeholder="Filter by reason, message or object…" value="' + esc(ts.q) + '" style="width:min(320px,100%)" aria-label="Filter events">' +
      '<select class="m-input" id="eType" aria-label="Event type"><option value="">All types</option><option' + (ts.type === "Warning" ? " selected" : "") + '>Warning</option><option' + (ts.type === "Normal" ? " selected" : "") + '>Normal</option></select>' +
      '<span class="m-count">' + fmt(list.length) + ' shown</span></div>' + eventsList(list.slice(0, 300), "No events match.") + '</div></div>';
    $("eQ").addEventListener("input", ev => { ts.q = ev.target.value; rerender(); });
    $("eType").addEventListener("change", ev => { ts.type = ev.target.value; rerender(); });
  }

  // ---------------------------------------------------------------- administration
  function pageAccess(r, el) {
    const meta = store.meta || {};
    el.innerHTML = '<div class="m-page"><div class="m-page-h"><div><div class="m-crumbs">Administration</div><h1>Access &amp; authentication</h1><p>How people sign in to Mantis, and what Mantis itself can read in the cluster.</p></div></div>' +
      '<div class="m-grid m-g-2"><div class="m-card"><div class="m-card-h"><h2>Mantis sign-in</h2><span class="m-badge warn">Public Preview</span></div><div class="m-card-b">' +
      '<div class="m-kv"><div class="k">Method</div><div class="v">Built-in preview gate</div><div class="k">Accounts</div><div class="v">One shared account (<code>admin</code>)</div><div class="k">Sessions</div><div class="v">Server-side, in memory; cleared on sign-out or restart</div><div class="k">Enforced by</div><div class="v">mantis-web, on every page and API request</div></div>' +
      '<div class="m-note warn" style="margin-top:12px">This preview gate is not production authentication. User accounts, roles (Admin, Maintainer, Viewer), OIDC and audit logs are on the roadmap and are not available yet — so there are no controls for them here.</div>' +
      '<div style="margin-top:12px"><button class="m-btn" id="aOut">' + icon("logout") + 'Sign out of this session</button></div></div></div>' +
      '<div class="m-card"><div class="m-card-h"><h2>Kubernetes access</h2><span class="m-badge ok">Read-only</span></div><div class="m-card-b">' +
      '<div class="m-kv"><div class="k">Identity</div><div class="v">The engine\'s ServiceAccount (or your kubeconfig when run locally)</div><div class="k">Permissions</div><div class="v">get / list only — no create, update, patch or delete anywhere</div><div class="k">Cluster</div><div class="v">' + esc(meta.context || "—") + '</div><div class="k">API server</div><div class="v">' + esc(meta.server || "—") + '</div></div>' +
      ((meta.skipped || []).length ? '<div class="m-note warn" style="margin-top:12px">RBAC does not allow Mantis to list: ' + meta.skipped.map(s => '<code>' + esc(s) + '</code>').join(", ") + '. Those resources are missing from every page, and references to them show as “unverified”.</div>'
        : '<div class="m-note neutral" style="margin-top:12px">Mantis can list every kind it understands in this cluster.</div>') +
      '<p class="m-muted" style="font-size:12px;margin-bottom:0">Everyone signed in to Mantis sees what this one identity can read. Per-user, RBAC-aware access will arrive with real authentication.</p></div></div></div></div>';
    $("aOut").addEventListener("click", signOut);
  }
  function pageSettings(r, el) {
    const meta = store.meta || {};
    const opt = (v, l, cur) => '<option value="' + v + '"' + (String(cur) === String(v) ? " selected" : "") + '>' + l + '</option>';
    el.innerHTML = '<div class="m-page"><div class="m-page-h"><div><div class="m-crumbs">Administration</div><h1>Settings</h1><p>Preferences for this browser.</p></div></div>' +
      '<div class="m-grid m-g-2"><div class="m-card"><div class="m-card-h"><h2>Preferences</h2></div><div class="m-card-b"><div class="m-kv">' +
      '<div class="k"><label for="sTheme">Theme</label></div><div class="v"><select class="m-input" id="sTheme">' + opt("light", "Light", S.theme) + opt("dark", "Dark", S.theme) + '</select></div>' +
      '<div class="k"><label for="sRefresh">Refresh interval</label></div><div class="v"><select class="m-input" id="sRefresh">' + opt(5000, "5 seconds", S.refreshMs) + opt(10000, "10 seconds", S.refreshMs) + opt(30000, "30 seconds", S.refreshMs) + opt(60000, "1 minute", S.refreshMs) + opt(0, "Manual", S.refreshMs) + '</select></div>' +
      '</div><p class="m-muted" style="font-size:12px;margin-bottom:0">Refreshing is cheap: after the first load only what changed is transferred.</p></div></div>' +
      '<div class="m-card"><div class="m-card-h"><h2>About this connection</h2></div><div class="m-card-b"><div class="m-kv">' +
      [["Cluster", meta.context], ["Kubernetes", meta.version], ["API server", meta.server], ["Cluster ID", meta.clusterId], ["Graph revision", store.revision], ["Resources", fmt(store.nodes.size)], ["Relationships", fmt(store.edges.size)]]
        .map(([k, v]) => '<div class="k">' + k + '</div><div class="v">' + esc(v == null || v === "" ? "—" : v) + '</div>').join("") + '</div></div></div></div></div>';
    $("sTheme").addEventListener("change", ev => setTheme(ev.target.value));
    $("sRefresh").addEventListener("change", ev => { S.refreshMs = parseInt(ev.target.value, 10); try { localStorage.setItem("mantis-refresh", S.refreshMs); } catch (e) { /* ignore */ } schedule(); renderSync(); });
  }

  // ---------------------------------------------------------------- topology
  // The topology is the existing renderer, mounted once and kept alive while
  // other pages are shown, so its camera, expansions and selection persist.
  function showTopology(r) {
    if (!topo) {
      topo = MantisApp.mount({
        source: {
          load: since => api("/api/graph" + (since ? "?since=" + since : "")),
          resource: n => fetchObject(n),
        },
        errorHint: "Is your cluster running and can the engine reach it?",
      });
      topo.firstLoad().then(() => applyTopoParams(r)).catch(() => {});
      applyTopoNs();
      return;
    }
    applyTopoNs();
    if (topo.isLoaded()) applyTopoParams(r);
    topo.render();
  }
  function applyTopoParams(r) {
    const q = r.q || new URLSearchParams();
    if (q.get("ns") && q.get("ns") !== S.ns) { setNs(q.get("ns")); }
    if (q.get("view")) topo.setView(q.get("view"));
    if (q.get("focus")) topo.focusDependencies(q.get("focus"));
    else if (q.get("select")) topo.jumpTo(q.get("select"));
    if ([...q.keys()].length) history.replaceState(null, "", "#/topology");
  }
  function applyTopoNs() {
    if (!topo) return;
    topo.state.namespaces = S.ns ? new Set([S.ns]) : null;
  }

  // ---------------------------------------------------------------- global actions
  function setNs(ns) {
    S.ns = ns || "";
    try { localStorage.setItem("mantis-ns", S.ns); } catch (e) { /* ignore */ }
    applyTopoNs(); if (topo && route().page === "topology") topo.render({ fit: true });
    renderNavCounts(); rerender();
  }
  function setTheme(t) {
    S.theme = t === "dark" ? "dark" : "light";
    document.documentElement.setAttribute("data-theme", S.theme);
    try { localStorage.setItem("mantis-theme", S.theme); sessionStorage.setItem("mantis-theme", S.theme); } catch (e) { /* ignore */ }
    if (topo && topo.isLoaded()) topo.render();
    rerender();
  }
  function signOut() { fetch("/logout", { method: "POST" }).finally(() => { location.href = "/login"; }); }

  // ---------------------------------------------------------------- reports
  // Infrastructure Report: the browser lays out the topology with the same
  // engine the Topology page uses (report-layout.js), then mantis-report
  // validates it against the engine's snapshot, builds the PDF and encrypts
  // it (AES-256). Progress events stream back; closing the dialog cancels.
  const REPORT_STAGES = [
    ["prepare", "Preparing cluster data"], ["topology", "Building topology"], ["inventory", "Generating inventory"],
    ["render", "Rendering PDF"], ["encrypt", "Applying encryption"], ["ready", "Report ready"],
  ];
  let reportAbort = null, reportURL = null;
  function openReportDialog() {
    closeReportDialog();
    const wrap = document.createElement("div");
    wrap.className = "m-modal-wrap"; wrap.id = "reportModal";
    const cluster = esc((store.meta || {}).context || "this cluster");
    wrap.innerHTML = '<div class="m-modal" role="dialog" aria-modal="true" aria-labelledby="rpTitle">' +
      '<div class="m-modal-h"><h2 id="rpTitle">' + icon("report") + 'Infrastructure report</h2><button class="m-iconbtn" data-close aria-label="Close">✕</button></div>' +
      '<div class="m-modal-b" id="rpBody">' +
        '<div class="m-field"><div class="m-field-l">Scope</div><label class="m-check"><input type="radio" checked disabled> Entire cluster <span class="m-muted">(' + cluster + ')</span></label></div>' +
        '<div class="m-field"><div class="m-field-l">Contents</div>' +
          [["topology", "Complete cluster topology"], ["inventory", "Resource inventory"], ["health", "Cluster health summary"], ["findings", "Findings and anomalies"]]
            .map(([k, l]) => '<label class="m-check"><input type="checkbox" data-c="' + k + '" checked> ' + l + '</label>').join("") +
          '<div class="m-muted" style="font-size:12px;margin-top:2px">A cluster overview and report metadata are always included.</div></div>' +
        '<div class="m-field"><div class="m-field-l">' + icon("lock") + 'Password protection <span class="m-badge ok" style="margin-left:6px">AES-256</span></div>' +
          '<input class="m-input" id="rpPw" type="password" autocomplete="new-password" placeholder="PDF password (8+ characters)" aria-label="PDF password" style="width:100%">' +
          '<input class="m-input" id="rpPw2" type="password" autocomplete="new-password" placeholder="Confirm password" aria-label="Confirm PDF password" style="width:100%;margin-top:8px">' +
          '<div class="m-muted" style="font-size:12px;margin-top:6px">Needed to open the PDF. Mantis never stores or logs it, and it is separate from your Mantis sign-in — keep it somewhere safe, it cannot be recovered.</div>' +
          '<div class="m-note crit" id="rpErr" hidden style="margin-top:8px"></div></div>' +
      '</div>' +
      '<div class="m-modal-f" id="rpFoot"><button class="m-btn" data-close>Cancel</button><button class="m-btn primary" id="rpGo">' + icon("report") + 'Generate report</button></div></div>';
    document.body.appendChild(wrap);
    wrap.querySelectorAll("[data-close]").forEach(b => b.addEventListener("click", closeReportDialog));
    wrap.addEventListener("click", ev => { if (ev.target === wrap) closeReportDialog(); });
    wrap.addEventListener("keydown", ev => { if (ev.key === "Escape") closeReportDialog(); });
    $("rpGo").addEventListener("click", startReport);
    [$("rpPw"), $("rpPw2")].forEach(i => i.addEventListener("keydown", ev => { if (ev.key === "Enter") startReport(); }));
    setTimeout(() => $("rpPw").focus(), 30);
  }
  function closeReportDialog() {
    if (reportAbort) { reportAbort.abort(); reportAbort = null; }
    if (reportURL) { URL.revokeObjectURL(reportURL); reportURL = null; }
    const m = $("reportModal"); if (m) m.remove();
  }
  function reportError(msg) { const e = $("rpErr"); if (e) { e.textContent = msg; e.hidden = false; } }
  function renderStages(state, detail) {
    const body = $("rpBody"); if (!body) return;
    body.innerHTML = '<ol class="m-steps">' + REPORT_STAGES.map(([k, l]) => {
      const st = state[k] || "pending";
      const mark = st === "done" ? "✓" : st === "active" ? '<span class="m-spin" aria-hidden="true"></span>' : st === "error" ? "!" : "";
      return '<li class="m-step ' + st + '"><span class="m-step-i">' + mark + '</span>' + esc(l) + (st === "active" && detail ? ' <span class="m-muted">' + esc(detail) + '</span>' : "") + '</li>';
    }).join("") + '</ol><div class="m-note crit" id="rpErr" hidden style="margin-top:10px"></div><div id="rpDone"></div>';
  }
  async function startReport() {
    const pw = $("rpPw").value, pw2 = $("rpPw2").value;
    const contents = {};
    document.querySelectorAll("#reportModal [data-c]").forEach(c => { contents[c.getAttribute("data-c")] = c.checked; });
    if (pw.length < 8) { reportError("The password must be at least 8 characters."); return; }
    if (pw !== pw2) { reportError("The two passwords do not match."); return; }
    const state = { prepare: "active" };
    renderStages(state);
    $("rpFoot").innerHTML = '<button class="m-btn" data-close>Cancel</button>';
    $("rpFoot").querySelector("[data-close]").addEventListener("click", closeReportDialog);
    reportAbort = new AbortController();
    const signal = reportAbort.signal;
    const fail = msg => {
      Object.keys(state).forEach(k => { if (state[k] === "active") state[k] = "error"; });
      renderStages(state); reportError(msg);
      $("rpFoot").innerHTML = '<button class="m-btn" data-close>Close</button><button class="m-btn primary" id="rpRetry">Try again</button>';
      $("rpFoot").querySelector("[data-close]").addEventListener("click", closeReportDialog);
      $("rpRetry").addEventListener("click", () => { closeReportDialog(); openReportDialog(); });
    };
    try {
      for (let attempt = 0; attempt < 3; attempt++) {
        await syncGraph();
        state.prepare = "done"; state.topology = "active"; renderStages(state, contents.topology ? "laying out the graph" : "");
        await new Promise(r => setTimeout(r, 30)); // let the progress paint before layout work
        const layout = contents.topology ? MantisReportLayout.build(store) : { revision: store.revision, pages: [] };
        const resp = await fetch("/api/report", {
          method: "POST", signal, headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ revision: layout.revision, password: pw, contents, pages: layout.pages }),
        });
        if (resp.status === 409) { state.topology = undefined; state.prepare = "active"; renderStages(state, "the cluster changed — refreshing"); continue; }
        if (resp.status === 401) { location.href = "/login"; return; }
        if (!resp.ok) { let m = "HTTP " + resp.status; try { m = (await resp.json()).error || m; } catch (e) { /* not JSON */ } fail(m); return; }
        await readReportStream(resp, state);
        return;
      }
      fail("The cluster kept changing while the report was prepared. Try again.");
    } catch (e) {
      if (e && e.name === "AbortError") return;
      fail("The report could not be generated: " + (e && e.message || e));
    }
  }
  async function readReportStream(resp, state) {
    const order = REPORT_STAGES.map(s => s[0]);
    const advance = k => { const i = order.indexOf(k); order.slice(0, i).forEach(x => { state[x] = "done"; }); state[k] = "active"; renderStages(state); };
    const stageMap = { "building-topology": "topology", "rendering-topology": "topology", "generating-inventory": "inventory", "rendering-pdf": "render", "encrypting": "encrypt" };
    const reader = resp.body.getReader(), dec = new TextDecoder();
    let buf = "";
    for (;;) {
      const { value, done } = await reader.read();
      if (value) buf += dec.decode(value, { stream: true });
      let nl;
      while ((nl = buf.indexOf("\n")) >= 0) {
        const line = buf.slice(0, nl).trim(); buf = buf.slice(nl + 1);
        if (!line) continue;
        const ev = JSON.parse(line);
        if (ev.stage === "error") throw new Error(ev.error || "report failed");
        if (ev.stage === "ready") { finishReport(ev, state); return; }
        if (stageMap[ev.stage]) advance(stageMap[ev.stage]);
      }
      if (done) throw new Error("the connection closed before the report was complete");
    }
  }
  function finishReport(ev, state) {
    REPORT_STAGES.forEach(([k]) => { state[k] = "done"; });
    renderStages(state);
    const bin = atob(ev.pdf), bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    reportURL = URL.createObjectURL(new Blob([bytes], { type: "application/pdf" }));
    const a = document.createElement("a"); a.href = reportURL; a.download = ev.filename; document.body.appendChild(a); a.click(); a.remove();
    $("rpDone").innerHTML = '<div class="m-note neutral" style="margin-top:12px">Your download has started: <b>' + esc(ev.filename) + '</b> (' + (ev.bytes / 1048576).toFixed(1) + ' MB). It opens with the password you set.</div>';
    $("rpFoot").innerHTML = '<a class="m-btn" href="' + reportURL + '" download="' + esc(ev.filename) + '">Download again</a><button class="m-btn primary" data-close>Done</button>';
    $("rpFoot").querySelector("[data-close]").addEventListener("click", closeReportDialog);
    reportAbort = null;
  }
  document.addEventListener("click", ev => { if (ev.target.closest("[data-report]")) { ev.preventDefault(); openReportDialog(); } });

  // ---------------------------------------------------------------- wiring
  buildSidebar();
  $("mNs").addEventListener("change", ev => setNs(ev.target.value));
  $("mThemeBtn").addEventListener("click", () => setTheme(S.theme === "dark" ? "light" : "dark"));
  $("mSync").addEventListener("click", () => tick());
  $("mRail").addEventListener("click", () => {
    const shell = $("shell");
    const narrow = innerWidth <= 1100;
    if (narrow) shell.classList.toggle("wide"); else shell.classList.toggle("rail");
    try { localStorage.setItem("mantis-rail", shell.classList.contains("rail") ? "1" : "0"); } catch (e) { /* ignore */ }
    $("mRail").innerHTML = icon(shell.classList.contains("rail") || (narrow && !shell.classList.contains("wide")) ? "expand" : "collapse") + '<span class="m-label">Collapse sidebar</span>';
  });
  $("mRail").innerHTML = icon($("shell").classList.contains("rail") ? "expand" : "collapse") + '<span class="m-label">Collapse sidebar</span>';
  $("mHamburger").addEventListener("click", () => $("shell").classList.toggle("drawer"));
  $("mUserBtn").addEventListener("click", ev => { ev.stopPropagation(); const m = $("mUserMenu"); const open = !m.classList.contains("open"); m.classList.toggle("open", open); $("mUserBtn").setAttribute("aria-expanded", String(open)); });
  document.addEventListener("click", ev => {
    if (!ev.target.closest(".m-menu-wrap")) { $("mUserMenu").classList.remove("open"); $("mUserBtn").setAttribute("aria-expanded", "false"); }
    if (!ev.target.closest(".m-gsearch")) $("mSearchResults").classList.remove("open");
    if ($("shell").classList.contains("drawer") && !ev.target.closest(".m-sidebar") && !ev.target.closest("#mHamburger")) closeDrawer();
  });
  $("mSignOut").addEventListener("click", signOut);
  const gs = $("mSearch");
  gs.addEventListener("input", renderGlobalSearch);
  gs.addEventListener("focus", () => { if (gs.value.trim()) renderGlobalSearch(); });
  gs.addEventListener("keydown", ev => {
    if (ev.key === "ArrowDown") { ev.preventDefault(); if (gMatches.length) { gActive = (gActive + 1) % gMatches.length; markGActive(); } }
    else if (ev.key === "ArrowUp") { ev.preventDefault(); if (gMatches.length) { gActive = (gActive - 1 + gMatches.length) % gMatches.length; markGActive(); } }
    else if (ev.key === "Enter") { if (gActive >= 0) openResult(gActive); }
    else if (ev.key === "Escape") { gs.value = ""; $("mSearchResults").classList.remove("open"); gs.blur(); }
  });
  document.addEventListener("keydown", ev => {
    if ((ev.ctrlKey || ev.metaKey) && ev.key.toLowerCase() === "k") { ev.preventDefault(); gs.focus(); gs.select(); }
    else if (ev.key === "/" && !/INPUT|TEXTAREA|SELECT/.test(document.activeElement.tagName)) { ev.preventDefault(); gs.focus(); }
  });

  render();
  syncGraph().then(() => { renderNavCounts(); render(); });
  schedule();
})();
