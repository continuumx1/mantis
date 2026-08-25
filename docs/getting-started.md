# Getting Started

## Prerequisites

- A Kubernetes cluster — any distribution (minikube, kind, kubeadm, RKE2, EKS, GKE, AKS, …)
- `kubectl` configured against it
- Helm 3, if installing via the chart (recommended)

## Install with Helm

No clone needed — the chart is published as an OCI artifact:

```bash
helm install mantis oci://registry-1.docker.io/cx1tech/mantis \
  --version 0.3.0 --namespace mantis --create-namespace
```

This installs both services, the ServiceAccount, and the read-only
ClusterRole/ClusterRoleBinding `mantis-engine` needs. Nothing is exposed
outside the cluster by default — see [Deployment](deployment.md#exposing-it)
for Ingress/LoadBalancer/port-forward options.

Quick local look at what you just installed:

```bash
kubectl port-forward -n mantis svc/mantis-web 8081:8080
```

Then open [http://localhost:8081](http://localhost:8081).

## Or pull the images directly

```bash
docker pull cx1tech/mantis:0.1.0-preview.3-engine
docker pull cx1tech/mantis:0.1.0-preview.3-web
```

Both are multi-arch (`linux/amd64` + `linux/arm64`), built on a distroless
nonroot base. See [Deployment](deployment.md) for running them without Helm.

## Run it locally, against your own kubeconfig

Useful for evaluating Mantis before deploying it anywhere:

```bash
git clone https://github.com/continuumx1/mantis.git
cd mantis

# Terminal 1 — backend, reads the cluster via your kubeconfig
MANTIS_ENGINE_ADDR=":8080" go run ./cmd/mantis-engine

# Terminal 2 — frontend, serves the UI and proxies /api to the engine
MANTIS_WEB_ADDR=":8081" MANTIS_ENGINE_URL="http://localhost:8080" go run ./cmd/mantis-web
```

Requires Go 1.26+. The ports above are just an example — use whatever's
free on your machine.

## First login

Every Mantis deployment right now sits behind a Public Preview login gate —
sign in with **`admin` / `admin`**, printed on the login screen itself. This
is a temporary demo gate, not real authentication — read
[Public Preview notes](security.md#public-preview-notes) before pointing a
deployment at anything beyond your own machine or team.

## Explore your topology

Once signed in you land on the live graph of whatever cluster you pointed
Mantis at. Click any resource for its details and relationships, drag to
rearrange, scroll to zoom, `Ctrl F` to search. The full walkthrough of every
control is in the [User Guide](user-guide.md).
