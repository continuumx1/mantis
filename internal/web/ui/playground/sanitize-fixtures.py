#!/usr/bin/env python3
"""One-off sanitizer: re-flavors the captured playground fixtures so each
scenario reads as a different real-world Kubernetes distribution (kubeadm,
GKE, EKS) instead of all three revealing the same local minikube dev cluster
they were actually captured from. Run once after (re)capturing fixtures with
cmd/playground-capture; not part of the normal build.
"""
import json
import re
from pathlib import Path

ROOT = Path("/Users/gokul/knw/internal/web/ui/playground/data")

# ---------- scenario: web-app -> kubeadm ----------
WEB_APP = ROOT / "web-app"
OLD_NODE_ID = "/Node/minikube"
NEW_NODE_ID = "/Node/k8s-worker-01"
NEW_NODE_NAME = "k8s-worker-01"

def sanitize_web_app():
    gp = WEB_APP / "graph.json"
    d = json.loads(gp.read_text())
    d["meta"]["context"] = "kubeadm · web application (sample data)"
    d["meta"]["version"] = "v1.29.4"
    for n in d["nodes"]:
        if n["id"] == OLD_NODE_ID:
            n["id"] = NEW_NODE_ID
            n["name"] = NEW_NODE_NAME
            n["attributes"] = [
                "status: Ready",
                "roles: worker",
                "version: v1.29.4",
                "os: linux/amd64",
                "allocatable: cpu 8 · mem 8126756Ki",
                "pod capacity: 110",
            ]
    for e in d["edges"]:
        if e["from"] == OLD_NODE_ID: e["from"] = NEW_NODE_ID
        if e["to"] == OLD_NODE_ID: e["to"] = NEW_NODE_ID
    gp.write_text(json.dumps(d, indent=2) + "\n")

    node_yaml = WEB_APP / "resources" / "__Node__minikube.yaml"
    node_yaml.write_text(f"""apiVersion: v1
kind: Node
metadata:
  annotations:
    node.alpha.kubernetes.io/ttl: "0"
    volumes.kubernetes.io/controller-managed-attach-detach: "true"
  creationTimestamp: "2026-08-16T04:43:51Z"
  labels:
    kubernetes.io/arch: amd64
    kubernetes.io/hostname: {NEW_NODE_NAME}
    kubernetes.io/os: linux
    node-role.kubernetes.io/worker: ""
  name: {NEW_NODE_NAME}
  resourceVersion: "17992"
  uid: dcfaf36c-ce1c-4171-b481-90d859da81db
spec: {{}}
status:
  addresses:
  - address: 10.0.1.11
    type: InternalIP
  - address: {NEW_NODE_NAME}
    type: Hostname
  allocatable:
    cpu: "8"
    ephemeral-storage: 234492896Ki
    memory: 8126756Ki
    pods: "110"
  capacity:
    cpu: "8"
    ephemeral-storage: 234492896Ki
    memory: 8126756Ki
    pods: "110"
  conditions:
  - lastHeartbeatTime: "2026-08-16T18:22:00Z"
    lastTransitionTime: "2026-08-16T04:43:51Z"
    message: kubelet has sufficient memory available
    reason: KubeletHasSufficientMemory
    status: "False"
    type: MemoryPressure
  - lastHeartbeatTime: "2026-08-16T18:22:00Z"
    lastTransitionTime: "2026-08-16T04:43:51Z"
    message: kubelet has no disk pressure
    reason: KubeletHasNoDiskPressure
    status: "False"
    type: DiskPressure
  - lastHeartbeatTime: "2026-08-16T18:22:00Z"
    lastTransitionTime: "2026-08-16T04:43:51Z"
    message: kubelet has sufficient PID available
    reason: KubeletHasSufficientPID
    status: "False"
    type: PIDPressure
  - lastHeartbeatTime: "2026-08-16T18:22:00Z"
    lastTransitionTime: "2026-08-16T04:43:58Z"
    message: kubelet is posting ready status
    reason: KubeletReady
    status: "True"
    type: Ready
  daemonEndpoints:
    kubeletEndpoint:
      Port: 10250
  nodeInfo:
    architecture: amd64
    bootID: fcb0db6b-5fbc-4f26-9939-1e354b8a4aa4
    containerRuntimeVersion: containerd://1.7.13
    kernelVersion: 5.15.0-91-generic
    kubeProxyVersion: v1.29.4
    kubeletVersion: v1.29.4
    machineID: e366bd4b77b9d6be2d67552f69964f40
    operatingSystem: linux
    osImage: Ubuntu 22.04.4 LTS
    systemUUID: e366bd4b77b9d6be2d67552f69964f40
""")
    node_yaml.rename(WEB_APP / "resources" / "__Node__k8s-worker-01.yaml")

    for pod_file in (WEB_APP / "resources").glob("shop-frontend__Pod__*.yaml"):
        text = pod_file.read_text()
        pod_file.write_text(text.replace("nodeName: minikube", f"nodeName: {NEW_NODE_NAME}"))

# ---------- scenario: stateful-db -> GKE ----------
STATEFUL_DB = ROOT / "stateful-db"
GKE_NODE_NAME = "gke-demo-cluster-default-pool-3f9a1c2b-x7k4"

def sanitize_stateful_db():
    gp = STATEFUL_DB / "graph.json"
    d = json.loads(gp.read_text())
    d["meta"]["context"] = "GKE · stateful database (sample data)"
    d["meta"]["version"] = "v1.29.5-gke.1091000"
    for n in d["nodes"]:
        if n["id"] == OLD_NODE_ID:
            n["id"] = f"/Node/{GKE_NODE_NAME}"
            n["name"] = GKE_NODE_NAME
            n["attributes"] = [
                "status: Ready",
                "version: v1.29.5-gke.1091000",
                "os: linux/amd64",
                "allocatable: cpu 8 · mem 8126756Ki",
                "pod capacity: 110",
            ]
    for e in d["edges"]:
        if e["from"] == OLD_NODE_ID: e["from"] = f"/Node/{GKE_NODE_NAME}"
        if e["to"] == OLD_NODE_ID: e["to"] = f"/Node/{GKE_NODE_NAME}"
    gp.write_text(json.dumps(d, indent=2) + "\n")

    node_yaml = STATEFUL_DB / "resources" / "__Node__minikube.yaml"
    node_yaml.write_text(f"""apiVersion: v1
kind: Node
metadata:
  annotations:
    node.alpha.kubernetes.io/ttl: "0"
    volumes.kubernetes.io/controller-managed-attach-detach: "true"
  creationTimestamp: "2026-08-16T04:43:51Z"
  labels:
    cloud.google.com/gke-nodepool: default-pool
    cloud.google.com/gke-os-distribution: cos
    kubernetes.io/arch: amd64
    kubernetes.io/hostname: {GKE_NODE_NAME}
    kubernetes.io/os: linux
    node.kubernetes.io/instance-type: e2-highcpu-8
    topology.kubernetes.io/region: us-central1
    topology.kubernetes.io/zone: us-central1-a
  name: {GKE_NODE_NAME}
  resourceVersion: "17992"
  uid: dcfaf36c-ce1c-4171-b481-90d859da81db
spec:
  providerID: gce://mantis-demo-project/us-central1-a/{GKE_NODE_NAME}
status:
  addresses:
  - address: 10.128.0.14
    type: InternalIP
  - address: {GKE_NODE_NAME}
    type: Hostname
  allocatable:
    cpu: "8"
    ephemeral-storage: 234492896Ki
    memory: 8126756Ki
    pods: "110"
  capacity:
    cpu: "8"
    ephemeral-storage: 234492896Ki
    memory: 8126756Ki
    pods: "110"
  conditions:
  - lastHeartbeatTime: "2026-08-16T18:22:00Z"
    lastTransitionTime: "2026-08-16T04:43:51Z"
    message: kubelet has sufficient memory available
    reason: KubeletHasSufficientMemory
    status: "False"
    type: MemoryPressure
  - lastHeartbeatTime: "2026-08-16T18:22:00Z"
    lastTransitionTime: "2026-08-16T04:43:51Z"
    message: kubelet has no disk pressure
    reason: KubeletHasNoDiskPressure
    status: "False"
    type: DiskPressure
  - lastHeartbeatTime: "2026-08-16T18:22:00Z"
    lastTransitionTime: "2026-08-16T04:43:51Z"
    message: kubelet has sufficient PID available
    reason: KubeletHasSufficientPID
    status: "False"
    type: PIDPressure
  - lastHeartbeatTime: "2026-08-16T18:22:00Z"
    lastTransitionTime: "2026-08-16T04:43:58Z"
    message: kubelet is posting ready status
    reason: KubeletReady
    status: "True"
    type: Ready
  daemonEndpoints:
    kubeletEndpoint:
      Port: 10250
  nodeInfo:
    architecture: amd64
    bootID: fcb0db6b-5fbc-4f26-9939-1e354b8a4aa4
    containerRuntimeVersion: containerd://1.7.13
    kernelVersion: 6.1.85+
    kubeProxyVersion: v1.29.5-gke.1091000
    kubeletVersion: v1.29.5-gke.1091000
    machineID: e366bd4b77b9d6be2d67552f69964f40
    operatingSystem: linux
    osImage: Container-Optimized OS from Google
    systemUUID: e366bd4b77b9d6be2d67552f69964f40
""")
    node_yaml.rename(STATEFUL_DB / "resources" / f"__Node__{GKE_NODE_NAME}.yaml")

    res = STATEFUL_DB / "resources"
    for pod_file in res.glob("data-platform__Pod__*.yaml"):
        text = pod_file.read_text()
        pod_file.write_text(text.replace("nodeName: minikube", f"nodeName: {GKE_NODE_NAME}"))

    for pvc_file in res.glob("data-platform__PersistentVolumeClaim__*.yaml"):
        text = pvc_file.read_text()
        text = text.replace(
            "volume.beta.kubernetes.io/storage-provisioner: k8s.io/minikube-hostpath",
            "volume.beta.kubernetes.io/storage-provisioner: pd.csi.storage.gke.io",
        ).replace(
            "volume.kubernetes.io/storage-provisioner: k8s.io/minikube-hostpath",
            "volume.kubernetes.io/storage-provisioner: pd.csi.storage.gke.io",
        )
        pvc_file.write_text(text)

    for pv_file in res.glob("__PersistentVolume__*.yaml"):
        text = pv_file.read_text()
        # Pull the PV's own name (== the volumeHandle disk name) back out of the
        # file so the swapped-in csi block still lines up with metadata.name.
        m = re.search(r"^  name: (pvc-[a-f0-9-]+)$", text, re.MULTILINE)
        pv_name = m.group(1) if m else "pvc-unknown"
        text = re.sub(
            r"  annotations:\n(?:.*\n)*?  creationTimestamp:",
            "  annotations:\n    pv.kubernetes.io/provisioned-by: pd.csi.storage.gke.io\n  creationTimestamp:",
            text,
        )
        text = re.sub(
            r"  hostPath:\n    path: .*\n    type: \"\"\n",
            "  csi:\n    driver: pd.csi.storage.gke.io\n    fsType: ext4\n"
            f"    volumeHandle: projects/mantis-demo-project/zones/us-central1-a/disks/{pv_name}\n",
            text,
        )
        pv_file.write_text(text)

# ---------- scenario: broken-relationships -> EKS ----------
BROKEN = ROOT / "broken-relationships"

def sanitize_broken():
    gp = BROKEN / "graph.json"
    d = json.loads(gp.read_text())
    d["meta"]["context"] = "EKS · broken relationships (sample data)"
    d["meta"]["version"] = "v1.29.6-eks-a5df82a"
    gp.write_text(json.dumps(d, indent=2) + "\n")

    ing = BROKEN / "resources" / "payments-api__Ingress__payments-ingress.yaml"
    text = ing.read_text()
    if "alb.ingress.kubernetes.io" not in text:
        text = text.replace(
            "metadata:\n",
            "metadata:\n  annotations:\n    alb.ingress.kubernetes.io/scheme: internet-facing\n"
            "    alb.ingress.kubernetes.io/target-type: ip\n    kubernetes.io/ingress.class: alb\n",
            1,
        )
        ing.write_text(text)

sanitize_web_app()
sanitize_stateful_db()
sanitize_broken()
print("done")
