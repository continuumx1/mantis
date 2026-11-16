# Mantis Playground

`playground.html` is the real Mantis app — `index.html` with the same
`app-shell.js`, `graph-view.js`, `graph-engine.js`, `report-layout.js` and
styles — running in the browser with no server. One extra script,
`playground-api.js`, answers the app's `/api/*` requests from recorded data,
so every page works: dashboard, resource tables and details (YAML included),
events, topology in every view, and Download report.

There is no live cluster behind it, by design: a public page connected to an
always-on cluster would be a real cluster exposed to the internet.

## The sample cluster

A synthetic cluster of 101 resources in five namespaces on three nodes
(`internal/synth`, seed 8): Deployments, a StatefulSet, a DaemonSet,
Services, Ingresses, NetworkPolicies, ConfigMaps, Secrets, storage and RBAC,
with one crash-looping Pod, its `BackOff` warning events, a broken
reference, and node metrics for the capacity panel. Every response in
`api.json` is what `mantis-engine` returned for it; `sample-report.pdf` is
what `mantis-report` generated.

Times are shifted when the page loads, so the newest event is a few minutes
old and the newest resource two days old, as on a live cluster. Reports are generated in advance, so the Playground's report opens with
the password **`mantis-playground`**; any other password gets a message
saying so.

## Files

| File | What it is |
|---|---|
| `../playground.html` | Generated from `../index.html` by `build.js` — do not edit |
| `../playground-api.js` | Serves `/api/*` from `api.json` (and the report from `sample-report.pdf`) |
| `api.json` | Recorded graph, events, capacity, and every resource's YAML and JSON |
| `sample-report.pdf` | The sample cluster's report, encrypted with `mantis-playground` |
| `capture.js` | Records `api.json` and `sample-report.pdf` from a running `mantis-dev` |
| `build.js` | Generates `playground.html`; `--check` fails if it is out of date |

## Updating it

After changing `index.html`, rebuild the page (the UI tests fail if you
forget):

```bash
node internal/web/ui/playground/build.js
```

After changing what the engine or report service returns, re-record:

```bash
go run ./cmd/mantis-dev -resources 110 -seed 8 -cluster-name sample-cluster -addr 127.0.0.1:8090
node internal/web/ui/playground/capture.js http://127.0.0.1:8090
```

## Publishing it on the website

Copy these from `internal/web/ui/` to the website's
`public/mantis-playground/`: `playground.html`, `playground-api.js`,
`app-shell.js`, `app-shell.css`, `graph-engine.js`, `graph-view.js`,
`graph-view.css`, `report-layout.js`, `mantis-mascot.webp`, `brand/`,
`fonts/`, and `playground/api.json` + `playground/sample-report.pdf`.
