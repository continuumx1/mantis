# User Guide

## Signing in

Mantis asks for the administrator account that was configured when it was
installed. With the Helm chart, a random password is generated unless you
set one; the install notes print the command to read it:

```sh
kubectl get secret mantis-admin -n <namespace> \
  -o jsonpath='{.data.password}' | base64 -d; echo
```

A session lasts 12 hours, or until you sign out. After 10 failed sign-ins
from one address, that address is refused for 10 minutes. See
[Security](security.md#signing-in) for how the account is stored and how to
change it.

## Finding your way around

The sidebar holds every page; it collapses to an icon rail on narrower
screens and to a drawer on phones. The top bar has:

- **Namespace selector.** It applies to every page, the Topology included.
- **Search.** `Ctrl K` or `/` from anywhere. Filters such as `kind:Pod`,
  `ns:shop` and `label:app=web` narrow the results; arrow keys and Enter
  open one. The same keys work on Windows, Linux and macOS.
- **Sync status.** "Synced 4s ago", or "Connection lost" with how old the
  data on screen is, while it keeps retrying.
- **Theme** (light or dark) and your **account menu** (Access, Settings,
  Download report, Sign out).

## Dashboard

The cluster at a glance: resource and namespace counts, health across
workloads and Pods, Pod status, workloads by namespace, Pods per node,
capacity (usage only when metrics-server is installed, otherwise
allocatable capacity), recent warning events, and a **Needs attention**
list of what is failing or degraded.

## Resources

Each kind in the sidebar opens a table you can filter, sort, page through
and filter by status. Open a row (click or Enter) for its details page:

- **Overview**: status, owner, labels, and the facts that matter for that
  kind (replicas, images, ports, selectors, capacity, …).
- **YAML**: the live manifest, fetched when you open the tab, with
  `managedFields` removed. Secrets never show values or YAML.
- **Events** and **Conditions**.
- **Related resources**: everything it is connected to, with the kind of
  relationship and whether Mantis verified it.

## Topology

The graph of how your resources relate. It is laid out deterministically:
the same cluster always produces the same picture, nothing drifts, and no
two cards or lines overlap.

- **Views.** *Namespaces* (the default), *Nodes* (which Pods run where),
  *Workloads* (each application with what it uses), and *Dependencies*
  (one resource's neighbourhood, 1–3 hops out and in).
- **Overview first.** On a larger cluster each namespace starts as a
  summary card with its resource count, Pod health and broken references.
  Click one to expand it; **Expand all** and **Overview** switch between the
  two. Large sets fold into stacks such as "48 Pods"; click to open one.
- **Relationships.** Lines are typed (ownership, networking, configuration,
  storage, security, autoscaling, scheduling) and can be filtered. A solid
  line was verified against the cluster; a dashed red line points at
  something that does not exist. Hover a line to see the relationship and
  the field it was read from. The legend (bottom-left) explains every style.
- **Select a resource** to highlight its neighbourhood and open the
  inspector; `Esc` clears it. `Ctrl F` finds a resource in the graph.
- **Zoom.** Scroll or pinch, drag empty space to pan, or use the zoom bar:
  `+`, `−`, a slider and *Fit*. Drag the bar wherever it suits you; Mantis
  remembers where you put it.
- The status bar counts what is drawn, collapsed, folded or filtered out;
  each count is a button that undoes that hiding.

## Infrastructure reports

**Download report** (dashboard, account menu, or *Report* on the Topology
toolbar) produces a password-protected PDF: cover, contents, overview,
health, topology diagrams, full inventory, findings and report metadata. You
choose the password (8 characters or more); the PDF is encrypted with
AES-256 and Mantis keeps no copy. The dialog shows each stage as it runs and
can be cancelled. See [Infrastructure Reports](reports.md).

## Staying current

The engine watches the cluster, so changes reach it within about a second.
The browser fetches only what changed since its last update, every
10 seconds by default; change the interval (5 s, 10 s, 30 s, 1 min, or
Manual) in **Settings**. Updates never close what you have open: an
inspector, a details page or a YAML tab stays where it is.

## Access & Settings

**Access & authentication** describes how sign-in works and exactly what
Mantis can read in the cluster, including any kinds RBAC does not allow it
to list (those are missing everywhere and references to them show as
"unverified"). **Settings** holds the theme and refresh interval.
