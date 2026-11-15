# Getting Started

## Prerequisites

- A Kubernetes cluster — any distribution (minikube, kind, kubeadm, RKE2, EKS, GKE, AKS, …)
- `kubectl` configured against it
- Helm 3, if installing via the chart (recommended)

## Install with Helm

No clone needed — the chart is published as an OCI artifact:

```bash
helm install mantis oci://registry-1.docker.io/cx1tech/mantis \
  --version 1.0.0 --namespace mantis --create-namespace
```

This installs the three services (`mantis-web`, `mantis-engine`,
`mantis-report`), the ServiceAccount and read-only ClusterRole
`mantis-engine` needs, network policies, and a Secret with a generated
administrator password. To choose the password yourself, add
`--set auth.adminPassword='<at least 8 characters>'`. Nothing is exposed
outside the cluster by default — see [Deployment](deployment.md#exposing-it)
for Ingress/LoadBalancer/port-forward options.

Quick local look at what you just installed:

```bash
kubectl port-forward -n mantis svc/mantis-web 8081:8080
```

Then open [http://localhost:8081](http://localhost:8081).

## Or pull the images directly

```bash
docker pull cx1tech/mantis:1.0.0-engine
docker pull cx1tech/mantis:1.0.0-web
docker pull cx1tech/mantis:1.0.0-report
```

All three are multi-arch (`linux/amd64` + `linux/arm64`), built on a distroless
nonroot base. See [Deployment](deployment.md) for running them without Helm.

## Run it locally, against your own kubeconfig

Useful for evaluating Mantis before deploying it anywhere:

```bash
git clone https://github.com/continuumx1/mantis.git
cd mantis

# Terminal 1 — backend, reads the cluster via your kubeconfig
MANTIS_ENGINE_ADDR=":8080" go run ./cmd/mantis-engine

# Terminal 2 — reports, reads snapshots from the engine
MANTIS_REPORT_ADDR=":8082" MANTIS_ENGINE_URL="http://localhost:8080" go run ./cmd/mantis-report

# Terminal 3 — frontend, serves the UI and proxies /api to the engine
MANTIS_WEB_ADDR=":8081" MANTIS_ENGINE_URL="http://localhost:8080" \
  MANTIS_REPORT_URL="http://localhost:8082" \
  MANTIS_ADMIN_PASSWORD='choose-a-password' go run ./cmd/mantis-web
```

Requires Go 1.26+. The ports above are just an example — use whatever's
free on your machine.

## First login

Sign in as `admin`. With Helm, read the generated password with:

```bash
kubectl get secret mantis-mantis-admin -n mantis \
  -o jsonpath='{.data.password}' | base64 -d; echo
```

(The Secret is named `<release>-mantis-admin`; the install notes print the
exact command.) Run locally, it is the `MANTIS_ADMIN_PASSWORD` you set. See
[Security](security.md#signing-in) before exposing Mantis beyond your own
machine.

## Explore your topology

Once signed in you land on the dashboard of whatever cluster you pointed
Mantis at. Open **Topology** for the relationship graph, click any resource
for its details, and press `Ctrl K` to search everything. The full walkthrough of every
control is in the [User Guide](user-guide.md).
