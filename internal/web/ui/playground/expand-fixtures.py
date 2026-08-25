#!/usr/bin/env python3
"""Expands the Playground's 3 scenarios from one namespace/one node each to a
richer, more representative small-cluster shape (multiple namespaces sharing
several physical nodes) — hand-authored, not captured, per the decision to
inject data directly rather than provision more real cluster infrastructure.
Every new object's shape (fields, relation directions, attribute strings) is
modeled directly on Mantis's own real captured examples already in data/, so
it renders identically to genuine engine output; only the *source* is
synthetic, not the schema.

Run once against the existing (real, captured) data/ directories — it loads
each scenario's current graph.json, adds new nodes/edges/resource YAML, and
rewrites graph.json in place.
"""
import json
from pathlib import Path

ROOT = Path("/Users/gokul/knw/internal/web/ui/playground/data")

def load(scenario):
    return json.loads((ROOT / scenario / "graph.json").read_text())

def save(scenario, d):
    d["meta"]["nodeCount"] = len(d["nodes"])
    d["meta"]["edgeCount"] = len(d["edges"])
    (ROOT / scenario / "graph.json").write_text(json.dumps(d, indent=2) + "\n")

def write_yaml(scenario, node_id, text):
    fn = node_id.replace("/", "__") + ".yaml"
    (ROOT / scenario / "resources" / fn).write_text(text)

def node_dto(id_, kind, name, ns, status=None, resolved=True, attrs=None):
    d = {"id": id_, "kind": kind, "name": name, "ns": ns, "resolved": resolved}
    if status: d["status"] = status
    if attrs: d["attributes"] = attrs
    return d

def edge(from_, to, type_):
    return {"from": from_, "to": to, "type": type_}

def rid(ns, kind, name):
    return f"{ns}/{kind}/{name}"

# ---------------------------------------------------------------------------
# Physical Node objects — same shape as sanitize-fixtures.py already produced
# for the one real node in each scenario, just cloned under new names so pods
# can spread across a believably-sized small cluster instead of a single box.
# ---------------------------------------------------------------------------

def kubeadm_node_yaml(name, ip):
    return f"""apiVersion: v1
kind: Node
metadata:
  annotations:
    node.alpha.kubernetes.io/ttl: "0"
    volumes.kubernetes.io/controller-managed-attach-detach: "true"
  creationTimestamp: "2026-08-16T04:43:51Z"
  labels:
    kubernetes.io/arch: amd64
    kubernetes.io/hostname: {name}
    kubernetes.io/os: linux
    node-role.kubernetes.io/worker: ""
  name: {name}
  resourceVersion: "17992"
  uid: {name}-uid
spec: {{}}
status:
  addresses:
  - address: {ip}
    type: InternalIP
  - address: {name}
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
    message: kubelet is posting ready status
    reason: KubeletReady
    status: "True"
    type: Ready
  daemonEndpoints:
    kubeletEndpoint:
      Port: 10250
  nodeInfo:
    architecture: amd64
    containerRuntimeVersion: containerd://1.7.13
    kernelVersion: 5.15.0-91-generic
    kubeProxyVersion: v1.29.4
    kubeletVersion: v1.29.4
    operatingSystem: linux
    osImage: Ubuntu 22.04.4 LTS
"""

def gke_node_yaml(name, ip):
    return f"""apiVersion: v1
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
    kubernetes.io/hostname: {name}
    kubernetes.io/os: linux
    node.kubernetes.io/instance-type: e2-highcpu-8
    topology.kubernetes.io/region: us-central1
    topology.kubernetes.io/zone: us-central1-a
  name: {name}
  resourceVersion: "17992"
  uid: {name}-uid
spec:
  providerID: gce://mantis-demo-project/us-central1-a/{name}
status:
  addresses:
  - address: {ip}
    type: InternalIP
  - address: {name}
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
    message: kubelet is posting ready status
    reason: KubeletReady
    status: "True"
    type: Ready
  daemonEndpoints:
    kubeletEndpoint:
      Port: 10250
  nodeInfo:
    architecture: amd64
    containerRuntimeVersion: containerd://1.7.13
    kernelVersion: 6.1.85+
    kubeProxyVersion: v1.29.5-gke.1091000
    kubeletVersion: v1.29.5-gke.1091000
    operatingSystem: linux
    osImage: Container-Optimized OS from Google
"""

def eks_node_yaml(name, ip):
    return f"""apiVersion: v1
kind: Node
metadata:
  annotations:
    node.alpha.kubernetes.io/ttl: "0"
    volumes.kubernetes.io/controller-managed-attach-detach: "true"
  creationTimestamp: "2026-08-16T04:43:51Z"
  labels:
    beta.kubernetes.io/instance-type: m5.large
    eks.amazonaws.com/nodegroup: payments-ng
    kubernetes.io/arch: amd64
    kubernetes.io/hostname: {name}
    kubernetes.io/os: linux
    node.kubernetes.io/instance-type: m5.large
    topology.kubernetes.io/region: us-west-2
    topology.kubernetes.io/zone: us-west-2a
  name: {name}
  resourceVersion: "17992"
  uid: {name}-uid
spec:
  providerID: aws:///us-west-2a/i-0{abs(hash(name)) % 10**16:016x}
status:
  addresses:
  - address: {ip}
    type: InternalIP
  - address: {name}
    type: InternalDNS
  - address: {name}
    type: Hostname
  allocatable:
    cpu: "2"
    ephemeral-storage: 83873772Ki
    memory: 7907668Ki
    pods: "29"
  capacity:
    cpu: "2"
    ephemeral-storage: 104845292Ki
    memory: 8175316Ki
    pods: "29"
  conditions:
  - lastHeartbeatTime: "2026-08-16T18:22:00Z"
    lastTransitionTime: "2026-08-16T04:43:51Z"
    message: kubelet is posting ready status
    reason: KubeletReady
    status: "True"
    type: Ready
  daemonEndpoints:
    kubeletEndpoint:
      Port: 10250
  nodeInfo:
    architecture: amd64
    containerRuntimeVersion: containerd://1.7.7
    kernelVersion: 5.10.234-225.921.amzn2.x86_64
    kubeProxyVersion: v1.29.6-eks-a5df82a
    kubeletVersion: v1.29.6-eks-a5df82a
    operatingSystem: linux
    osImage: Amazon Linux 2
"""

def add_node(d, scenario, id_, name, yaml_fn, ip, roles_attr=None, version_attr="v1.29.4"):
    attrs = ["status: Ready"]
    if roles_attr: attrs.append(f"roles: {roles_attr}")
    attrs += [f"version: {version_attr}", "os: linux/amd64", "allocatable: cpu 8 · mem 8126756Ki", "pod capacity: 110"]
    d["nodes"].append(node_dto(id_, "Node", name, "", attrs=attrs))
    write_yaml(scenario, id_, yaml_fn(name, ip))

# ---------------------------------------------------------------------------
# App bundles — Deployment + ReplicaSet + N Pods + Service (+ConfigMap
# +Secret), modeled field-for-field on shop-frontend/storefront's real
# capture. `node_names` assigns each pod's runs-on edge + spec.nodeName,
# round-robin, so replicas spread across the scenario's shared node pool.
# ---------------------------------------------------------------------------

def container_block(indent, cm_name, secret_name, image, name, port, requests_cpu, requests_mem, limits_cpu, limits_mem):
    """Renders one container list item ("- envFrom: ...\\n  image: ...\\n...")
    at the given left indent. Building this as one piece — rather than
    splicing an optional envFrom fragment into a fixed template — is what
    keeps envFrom's presence/absence from ever disagreeing with where the
    following sibling keys (image, name, ...) end up lining up."""
    pad = " " * indent
    lines = []
    env_from = []
    if cm_name: env_from.append(f"{pad}    - configMapRef:\n{pad}        name: {cm_name}")
    if secret_name: env_from.append(f"{pad}    - secretRef:\n{pad}        name: {secret_name}")
    if env_from:
        lines.append(f"{pad}- envFrom:")
        lines.extend(env_from)
    else:
        lines.append(f"{pad}- envFrom: []")
    lines.append(f"{pad}  image: {image}")
    lines.append(f"{pad}  imagePullPolicy: IfNotPresent")
    lines.append(f"{pad}  name: {name}")
    lines.append(f"{pad}  ports:")
    lines.append(f"{pad}  - containerPort: {port}")
    lines.append(f"{pad}    protocol: TCP")
    lines.append(f"{pad}  resources:")
    lines.append(f"{pad}    limits:")
    lines.append(f"{pad}      cpu: {limits_cpu}")
    lines.append(f"{pad}      memory: {limits_mem}")
    lines.append(f"{pad}    requests:")
    lines.append(f"{pad}      cpu: {requests_cpu}")
    lines.append(f"{pad}      memory: {requests_mem}")
    return "\n".join(lines)

def add_simple_app(d, scenario, ns, app, image, replicas, node_names, port=80,
                    cm_name=None, cm_data=None, secret_name=None, secret_data=None,
                    cluster_ip="10.100.0.10", svc_type="ClusterIP", headless=False,
                    requests_cpu="50m", requests_mem="64Mi", limits_cpu="250m", limits_mem="128Mi",
                    rs_hash="7d8f9c5b6"):
    dep_id = rid(ns, "Deployment", app)
    rs_name = f"{app}-{rs_hash}"
    rs_id = rid(ns, "ReplicaSet", rs_name)
    svc_id = rid(ns, "Service", f"{app}-svc")

    dep_attrs = [f"replicas: {replicas}/{replicas} ready", "strategy: RollingUpdate", f"image: {image}",
                 f"requests: cpu {requests_cpu} · mem {requests_mem}", f"limits: cpu {limits_cpu} · mem {limits_mem}", "probes: none"]
    d["nodes"].append(node_dto(dep_id, "Deployment", app, ns, attrs=dep_attrs))
    d["nodes"].append(node_dto(rs_id, "ReplicaSet", rs_name, ns, attrs=dep_attrs[:1]+dep_attrs[2:]))
    d["edges"].append(edge(rs_id, dep_id, "controlled-by"))

    dep_container = container_block(6, cm_name, secret_name, image, app, port, requests_cpu, requests_mem, limits_cpu, limits_mem)

    dep_yaml = f"""apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    deployment.kubernetes.io/revision: "1"
  creationTimestamp: "2026-08-16T18:20:58Z"
  labels:
    app: {app}
  name: {app}
  namespace: {ns}
  resourceVersion: "17850"
  uid: {app}-deploy-uid
spec:
  replicas: {replicas}
  selector:
    matchLabels:
      app: {app}
  strategy:
    rollingUpdate:
      maxSurge: 25%
      maxUnavailable: 25%
    type: RollingUpdate
  template:
    metadata:
      labels:
        app: {app}
    spec:
      containers:
{dep_container}
status:
  availableReplicas: {replicas}
  readyReplicas: {replicas}
  replicas: {replicas}
  updatedReplicas: {replicas}
"""
    write_yaml(scenario, dep_id, dep_yaml)

    rs_yaml = f"""apiVersion: apps/v1
kind: ReplicaSet
metadata:
  annotations:
    deployment.kubernetes.io/desired-replicas: "{replicas}"
    deployment.kubernetes.io/revision: "1"
  creationTimestamp: "2026-08-16T18:20:58Z"
  labels:
    app: {app}
    pod-template-hash: {rs_hash}
  name: {rs_name}
  namespace: {ns}
  ownerReferences:
  - apiVersion: apps/v1
    blockOwnerDeletion: true
    controller: true
    kind: Deployment
    name: {app}
    uid: {app}-deploy-uid
  resourceVersion: "17849"
  uid: {rs_name}-uid
spec:
  replicas: {replicas}
  selector:
    matchLabels:
      app: {app}
      pod-template-hash: {rs_hash}
status:
  availableReplicas: {replicas}
  readyReplicas: {replicas}
  replicas: {replicas}
"""
    write_yaml(scenario, rs_id, rs_yaml)

    pod_attrs = ["Running · 1/1", f"image: {image}", "qos: Burstable",
                 f"requests: cpu {requests_cpu} · mem {requests_mem}", f"limits: cpu {limits_cpu} · mem {limits_mem}", "probes: none"]
    pod_ids = []
    for i in range(replicas):
        suffix = f"{rs_hash[:5]}{i:02d}"
        pod_name = f"{app}-{rs_hash}-{suffix}"
        pod_id = rid(ns, "Pod", pod_name)
        pod_ids.append(pod_id)
        node_name = node_names[i % len(node_names)]
        node_id = f"/Node/{node_name}"

        d["nodes"].append(node_dto(pod_id, "Pod", pod_name, ns, status="ok", attrs=pod_attrs))
        d["edges"].append(edge(pod_id, rs_id, "controlled-by"))
        d["edges"].append(edge(pod_id, node_id, "runs-on"))
        if cm_name: d["edges"].append(edge(pod_id, rid(ns, "ConfigMap", cm_name), "references"))
        if secret_name: d["edges"].append(edge(pod_id, rid(ns, "Secret", secret_name), "references"))

        pod_yaml = f"""apiVersion: v1
kind: Pod
metadata:
  creationTimestamp: "2026-08-16T18:20:58Z"
  generateName: {app}-{rs_hash}-
  labels:
    app: {app}
    pod-template-hash: {rs_hash}
  name: {pod_name}
  namespace: {ns}
  ownerReferences:
  - apiVersion: apps/v1
    blockOwnerDeletion: true
    controller: true
    kind: ReplicaSet
    name: {rs_name}
    uid: {rs_name}-uid
  resourceVersion: "17836"
  uid: {pod_name}-uid
spec:
  containers:
{container_block(2, cm_name, secret_name, image, app, port, requests_cpu, requests_mem, limits_cpu, limits_mem)}
  nodeName: {node_name}
  restartPolicy: Always
  schedulerName: default-scheduler
  serviceAccount: default
  serviceAccountName: default
status:
  phase: Running
  podIP: 10.244.{abs(hash(pod_name))%250}.{i+10}
  qosClass: Burstable
  startTime: "2026-08-16T18:20:58Z"
"""
        write_yaml(scenario, pod_id, pod_yaml)

    endpoints = ", ".join(f"10.244.{abs(hash(p))%250}.{i+10}:{port}" for i, p in enumerate(pod_ids))
    svc_attrs = [f"type: {svc_type}",
                 "clusterIP: None (headless)" if headless else f"clusterIP: {cluster_ip}",
                 f"ports: {port}→{port}/TCP", f"endpoints: {endpoints}"]
    d["nodes"].append(node_dto(svc_id, "Service", f"{app}-svc", ns, attrs=svc_attrs))
    for pid in pod_ids:
        d["edges"].append(edge(svc_id, pid, "selects"))
        d["edges"].append(edge(svc_id, pid, "serves"))

    svc_yaml = f"""apiVersion: v1
kind: Service
metadata:
  creationTimestamp: "2026-08-16T18:20:58Z"
  name: {app}-svc
  namespace: {ns}
  resourceVersion: "17786"
  uid: {app}-svc-uid
spec:
  clusterIP: {"None" if headless else cluster_ip}
  ports:
  - port: {port}
    protocol: TCP
    targetPort: {port}
  selector:
    app: {app}
  sessionAffinity: None
  type: {svc_type}
status:
  loadBalancer: {{}}
"""
    write_yaml(scenario, svc_id, svc_yaml)

    if cm_name and cm_data:
        cm_id = rid(ns, "ConfigMap", cm_name)
        d["nodes"].append(node_dto(cm_id, "ConfigMap", cm_name, ns))
        data_lines = "\n".join(f"  {k}: {v}" for k, v in cm_data.items())
        write_yaml(scenario, cm_id, f"apiVersion: v1\ndata:\n{data_lines}\nkind: ConfigMap\nmetadata:\n  name: {cm_name}\n  namespace: {ns}\n  resourceVersion: \"17781\"\n  uid: {cm_name}-uid\n")

    if secret_name and secret_data:
        # No write_yaml call here, deliberately: Secret contents are never
        # captured anywhere else in the Playground (cmd/playground-capture
        # skips them, the frontend shows a "hidden" notice without ever
        # fetching) — a synthetic Secret must follow the same rule, not just
        # the real captured ones, or the security story this scenario exists
        # to demonstrate would be self-contradicting.
        secret_id = rid(ns, "Secret", secret_name)
        d["nodes"].append(node_dto(secret_id, "Secret", secret_name, ns))


def expand_web_app():
    d = load("web-app")
    nodes = ["k8s-worker-01", "k8s-worker-02", "k8s-worker-03", "k8s-worker-04", "k8s-worker-05"]
    for i, (name, ip) in enumerate([("k8s-worker-02", "10.0.1.12"), ("k8s-worker-03", "10.0.1.13"),
                                     ("k8s-worker-04", "10.0.1.14"), ("k8s-worker-05", "10.0.1.15")]):
        add_node(d, "web-app", f"/Node/{name}", name, kubeadm_node_yaml, ip, roles_attr="worker")

    add_simple_app(d, "web-app", "checkout", "checkout-api", "node:20-alpine", 2, ["k8s-worker-02", "k8s-worker-04"],
                    cm_name="checkout-config", cm_data={"PAYMENT_PROVIDER": "stripe", "CURRENCY": "USD"},
                    cluster_ip="10.100.0.21", rs_hash="6c9b7d4a1")
    add_simple_app(d, "web-app", "catalog", "catalog-api", "python:3.12-slim", 2, ["k8s-worker-03", "k8s-worker-05"],
                    cm_name="catalog-config", cm_data={"CACHE_TTL": "300", "PAGE_SIZE": "24"},
                    cluster_ip="10.100.0.22", rs_hash="5f8a3e2c9")
    add_simple_app(d, "web-app", "ingress-nginx", "ingress-nginx-controller", "registry.k8s.io/ingress-nginx/controller:v1.10.1", 2,
                    ["k8s-worker-01", "k8s-worker-04"], cluster_ip="10.100.0.23", svc_type="LoadBalancer", rs_hash="4d7c1f8b3")
    add_simple_app(d, "web-app", "monitoring", "prometheus-server", "prom/prometheus:v2.53.0", 1, ["k8s-worker-05"],
                    cm_name="prometheus-config", cm_data={"scrape_interval": "15s", "retention": "15d"},
                    cluster_ip="10.100.0.24", rs_hash="3a6b9d2e7")

    d["meta"]["namespaceList"] = sorted(set(d["meta"]["namespaceList"]) | {"checkout", "catalog", "ingress-nginx", "monitoring"})
    d["meta"]["namespaces"] = len(d["meta"]["namespaceList"])
    save("web-app", d)
    print("web-app:", len(d["nodes"]), "nodes,", len(d["edges"]), "edges,", d["meta"]["namespaces"], "namespaces")


def add_stateful_app(d, scenario, ns, app, image, replicas, node_names, port,
                      cluster_ip_placeholder_unused=None, storage_size="1Gi", storage_class="standard", rs_hash_unused=None):
    sts_id = rid(ns, "StatefulSet", app)
    svc_id = rid(ns, "Service", f"{app}-headless")
    sts_attrs = [f"replicas: {replicas}/{replicas} ready", f"image: {image}",
                 "requests: cpu 100m · mem 128Mi", "limits: cpu 500m · mem 512Mi", "probes: none"]
    d["nodes"].append(node_dto(sts_id, "StatefulSet", app, ns, attrs=sts_attrs))
    write_yaml(scenario, sts_id, f"""apiVersion: apps/v1
kind: StatefulSet
metadata:
  creationTimestamp: "2026-08-16T18:21:00Z"
  name: {app}
  namespace: {ns}
  resourceVersion: "17900"
  uid: {app}-sts-uid
spec:
  replicas: {replicas}
  selector:
    matchLabels:
      app: {app}
  serviceName: {app}-headless
  template:
    metadata:
      labels:
        app: {app}
    spec:
      containers:
      - image: {image}
        name: {app}
        ports:
        - containerPort: {port}
        resources:
          limits:
            cpu: 500m
            memory: 512Mi
          requests:
            cpu: 100m
            memory: 128Mi
status:
  availableReplicas: {replicas}
  readyReplicas: {replicas}
  replicas: {replicas}
""")

    pod_ids = []
    for i in range(replicas):
        pod_name = f"{app}-{i}"
        pod_id = rid(ns, "Pod", pod_name)
        pod_ids.append(pod_id)
        node_name = node_names[i % len(node_names)]
        node_id = f"/Node/{node_name}"
        pvc_name = f"data-{app}-{i}"
        pvc_id = rid(ns, "PersistentVolumeClaim", pvc_name)
        pv_name = f"pvc-{app}-{i}-vol"
        pv_id = f"/PersistentVolume/{pv_name}"

        pod_attrs = ["Running · 1/1", f"image: {image}", "qos: Burstable",
                     "requests: cpu 100m · mem 128Mi", "limits: cpu 500m · mem 512Mi", "probes: none"]
        d["nodes"].append(node_dto(pod_id, "Pod", pod_name, ns, status="ok", attrs=pod_attrs))
        d["edges"].append(edge(pod_id, sts_id, "controlled-by"))
        d["edges"].append(edge(pod_id, node_id, "runs-on"))
        d["edges"].append(edge(pod_id, pvc_id, "claims"))

        d["nodes"].append(node_dto(pvc_id, "PersistentVolumeClaim", pvc_name, ns,
                                    attrs=["Bound", storage_size, "RWO", f"sc:{storage_class}"]))
        d["nodes"].append(node_dto(pv_id, "PersistentVolume", pv_name, "", attrs=[storage_size, "Delete", "Bound"]))
        d["edges"].append(edge(pvc_id, pv_id, "bound-to"))

        write_yaml(scenario, pod_id, f"""apiVersion: v1
kind: Pod
metadata:
  creationTimestamp: "2026-08-16T18:21:00Z"
  labels:
    app: {app}
  name: {pod_name}
  namespace: {ns}
  ownerReferences:
  - apiVersion: apps/v1
    blockOwnerDeletion: true
    controller: true
    kind: StatefulSet
    name: {app}
    uid: {app}-sts-uid
  resourceVersion: "17910"
  uid: {pod_name}-uid
spec:
  containers:
  - image: {image}
    name: {app}
    ports:
    - containerPort: {port}
    resources:
      limits:
        cpu: 500m
        memory: 512Mi
      requests:
        cpu: 100m
        memory: 128Mi
    volumeMounts:
    - mountPath: /data
      name: data
  nodeName: {node_name}
  restartPolicy: Always
  serviceAccount: default
  volumes:
  - name: data
    persistentVolumeClaim:
      claimName: {pvc_name}
status:
  phase: Running
  podIP: 10.244.{abs(hash(pod_name))%250}.{i+20}
  qosClass: Burstable
  startTime: "2026-08-16T18:21:00Z"
""")
        write_yaml(scenario, pvc_id, f"""apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  creationTimestamp: "2026-08-16T18:21:00Z"
  labels:
    app: {app}
  name: {pvc_name}
  namespace: {ns}
  resourceVersion: "17911"
  uid: {pvc_name}-uid
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: {storage_size}
  storageClassName: {storage_class}
  volumeName: {pv_name}
status:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: {storage_size}
  phase: Bound
""")
        write_yaml(scenario, pv_id, f"""apiVersion: v1
kind: PersistentVolume
metadata:
  annotations:
    pv.kubernetes.io/provisioned-by: pd.csi.storage.gke.io
  creationTimestamp: "2026-08-16T18:21:00Z"
  name: {pv_name}
  resourceVersion: "17912"
  uid: {pv_name}-vol-uid
spec:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: {storage_size}
  claimRef:
    kind: PersistentVolumeClaim
    name: {pvc_name}
    namespace: {ns}
  csi:
    driver: pd.csi.storage.gke.io
    fsType: ext4
    volumeHandle: projects/mantis-demo-project/zones/us-central1-a/disks/{pv_name}
  persistentVolumeReclaimPolicy: Delete
  storageClassName: {storage_class}
status:
  phase: Bound
""")

    endpoints = ", ".join(f"10.244.{abs(hash(p))%250}.{i+20}:{port}" for i, p in enumerate(pod_ids))
    d["nodes"].append(node_dto(svc_id, "Service", f"{app}-headless", ns,
                                attrs=["type: ClusterIP", "clusterIP: None (headless)", f"ports: {port}→{port}/TCP", f"endpoints: {endpoints}"]))
    for pid in pod_ids:
        d["edges"].append(edge(svc_id, pid, "selects"))
        d["edges"].append(edge(svc_id, pid, "serves"))
    write_yaml(scenario, svc_id, f"""apiVersion: v1
kind: Service
metadata:
  creationTimestamp: "2026-08-16T18:21:00Z"
  name: {app}-headless
  namespace: {ns}
  resourceVersion: "17913"
  uid: {app}-headless-uid
spec:
  clusterIP: None
  ports:
  - port: {port}
    protocol: TCP
    targetPort: {port}
  selector:
    app: {app}
status:
  loadBalancer: {{}}
""")


def expand_stateful_db():
    d = load("stateful-db")
    existing_node = "gke-demo-cluster-default-pool-3f9a1c2b-x7k4"
    new_nodes = [
        ("gke-demo-cluster-default-pool-3f9a1c2b-p2m8", "10.128.0.15"),
        ("gke-demo-cluster-default-pool-3f9a1c2b-q9r3", "10.128.0.16"),
        ("gke-demo-cluster-default-pool-3f9a1c2b-t4w1", "10.128.0.17"),
        ("gke-demo-cluster-default-pool-3f9a1c2b-v6y5", "10.128.0.18"),
    ]
    for name, ip in new_nodes:
        add_node(d, "stateful-db", f"/Node/{name}", name, gke_node_yaml, ip, version_attr="v1.29.5-gke.1091000")

    pool = [existing_node] + [n for n, _ in new_nodes]

    add_simple_app(d, "stateful-db", "cache", "redis", "redis:7.2-alpine", 2, [pool[2], pool[3]],
                    port=6379, cm_name="redis-config", cm_data={"maxmemory": "256mb", "maxmemory-policy": "allkeys-lru"},
                    cluster_ip="10.100.1.10", rs_hash="8e2c5a9f1")
    add_stateful_app(d, "stateful-db", "search", "opensearch", "opensearchproject/opensearch:2.15.0", 2,
                      [pool[1], pool[4]], port=9200, storage_size="2Gi")
    add_simple_app(d, "stateful-db", "analytics", "analytics-worker", "python:3.12-slim", 2, [pool[0], pool[2]],
                    port=8080, cm_name="analytics-config", cm_data={"BATCH_SIZE": "500", "WORKER_THREADS": "4"},
                    secret_name="analytics-api-keys", secret_data={"placeholder": "true"},
                    cluster_ip="10.100.1.12", rs_hash="1b4f7c8d2")
    add_simple_app(d, "stateful-db", "logging", "fluentd", "fluent/fluentd:v1.17-debian", 1, [pool[4]],
                    port=24224, cm_name="fluentd-config", cm_data={"LOG_LEVEL": "info"},
                    cluster_ip="10.100.1.13", rs_hash="9c3e6b1a5")

    d["meta"]["namespaceList"] = sorted(set(d["meta"]["namespaceList"]) | {"cache", "search", "analytics", "logging"})
    d["meta"]["namespaces"] = len(d["meta"]["namespaceList"])
    save("stateful-db", d)
    print("stateful-db:", len(d["nodes"]), "nodes,", len(d["edges"]), "edges,", d["meta"]["namespaces"], "namespaces")


def expand_broken_relationships():
    d = load("broken-relationships")
    eks_nodes = [
        ("ip-192-168-14-87.us-west-2.compute.internal", "192.168.14.87"),
        ("ip-192-168-32-140.us-west-2.compute.internal", "192.168.32.140"),
        ("ip-192-168-51-203.us-west-2.compute.internal", "192.168.51.203"),
    ]
    for name, ip in eks_nodes:
        add_node(d, "broken-relationships", f"/Node/{name}", name, eks_node_yaml, ip, version_attr="v1.29.6-eks-a5df82a")

    # The 2 existing payments-api pods were captured genuinely Pending because
    # a *second*, separate real failure (the unbindable payments-cache PVC
    # mounted into them) blocked scheduling entirely — a pod requiring a PVC
    # that can never bind is genuinely unschedulable, so it never got a
    # nodeName. That's a real, valid failure mode, but it was masking the
    # *other* one (the missing ConfigMap) behind a different problem, and
    # meant nothing in this scenario ever ran on a real node.
    #
    # Decoupling them here: drop the cache volume/mount (so payments-cache
    # becomes what it more often is in practice — an orphaned PVC nobody's
    # using anymore, still stuck Pending, still worth showing, just not
    # blocking anything) and let the pods actually get scheduled. What's left
    # is the more common real-world shape of this failure: the pod schedules
    # fine, then CreateContainerConfigError once the kubelet can't find the
    # ConfigMap — which is what the rewritten status below reflects, so nothing
    # in the manifest contradicts itself.
    pod_specs = {
        "payments-api-cc7dfd779-khq8q": (eks_nodes[0][0], eks_nodes[0][1], "kube-api-access-nk7v5", "465f0101-676b-4e10-a885-f91dbf220e88"),
        "payments-api-cc7dfd779-pm58k": (eks_nodes[1][0], eks_nodes[1][1], "kube-api-access-8j3qz", "7a2c9e14-3f5b-4d6a-9c81-2e5f8a1b7d40"),
    }
    for pod_name, (node_name, node_ip, token_vol, uid) in pod_specs.items():
        pod_id = f"payments-api/Pod/{pod_name}"
        d["edges"].append(edge(pod_id, f"/Node/{node_name}", "runs-on"))
        # The claims edge is dropped along with the volume/mount below — it
        # would otherwise assert a relationship the rewritten manifest no
        # longer shows.
        d["edges"] = [e for e in d["edges"] if not (e["from"] == pod_id and e["type"] == "claims")]
        write_yaml("broken-relationships", pod_id, f"""apiVersion: v1
kind: Pod
metadata:
  creationTimestamp: "2026-08-16T18:21:53Z"
  generateName: payments-api-cc7dfd779-
  labels:
    app: payments-api
    pod-template-hash: cc7dfd779
  name: {pod_name}
  namespace: payments-api
  ownerReferences:
  - apiVersion: apps/v1
    blockOwnerDeletion: true
    controller: true
    kind: ReplicaSet
    name: payments-api-cc7dfd779
    uid: 4bd69626-e5a6-4a2b-82df-f3c429e05534
  resourceVersion: "17972"
  uid: {uid}
spec:
  containers:
  - envFrom:
    - configMapRef:
        name: payments-config-v2
    image: nginx:1.27-alpine
    imagePullPolicy: IfNotPresent
    name: payments-api
    ports:
    - containerPort: 80
      protocol: TCP
    resources: {{}}
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    volumeMounts:
    - mountPath: /var/run/secrets/kubernetes.io/serviceaccount
      name: {token_vol}
      readOnly: true
  dnsPolicy: ClusterFirst
  enableServiceLinks: true
  nodeName: {node_name}
  preemptionPolicy: PreemptLowerPriority
  priority: 0
  restartPolicy: Always
  schedulerName: default-scheduler
  securityContext: {{}}
  serviceAccount: default
  serviceAccountName: default
  terminationGracePeriodSeconds: 30
  volumes:
  - name: {token_vol}
    projected:
      defaultMode: 420
      sources:
      - serviceAccountToken:
          expirationSeconds: 3607
          path: token
status:
  conditions:
  - lastTransitionTime: "2026-08-16T18:21:53Z"
    status: "True"
    type: PodScheduled
  - lastTransitionTime: "2026-08-16T18:21:53Z"
    status: "True"
    type: Initialized
  - lastTransitionTime: "2026-08-16T18:21:53Z"
    message: 'containers with unready status: [payments-api]'
    reason: ContainersNotReady
    status: "False"
    type: Ready
  - lastTransitionTime: "2026-08-16T18:21:53Z"
    message: 'containers with unready status: [payments-api]'
    reason: ContainersNotReady
    status: "False"
    type: ContainersReady
  containerStatuses:
  - image: nginx:1.27-alpine
    name: payments-api
    ready: false
    restartCount: 0
    started: false
    state:
      waiting:
        message: 'configmap "payments-config-v2" not found'
        reason: CreateContainerConfigError
  hostIP: {node_ip}
  phase: Pending
  qosClass: BestEffort
  startTime: "2026-08-16T18:21:53Z"
""")
        # containerStatuses[0].state.waiting.reason is a "problem reason" in
        # engine/attributes.go's podHealthNote, so it becomes the headline in
        # place of the bare phase — match that here so the pre-set attrs the
        # UI shows without a re-fetch agree with what a real engine would
        # compute from the manifest above.
        for n in d["nodes"]:
            if n["id"] == pod_id and n.get("attributes"):
                n["attributes"][0] = "CreateContainerConfigError · 0/1"

    add_simple_app(d, "broken-relationships", "platform", "platform-gateway", "envoyproxy/envoy:v1.30-latest", 2,
                    [eks_nodes[1][0], eks_nodes[2][0]], cluster_ip="10.100.2.10", rs_hash="2f5a8c1e6")

    d["meta"]["namespaceList"] = sorted(set(d["meta"]["namespaceList"]) | {"platform"})
    d["meta"]["namespaces"] = len(d["meta"]["namespaceList"])
    save("broken-relationships", d)
    print("broken-relationships:", len(d["nodes"]), "nodes,", len(d["edges"]), "edges,", d["meta"]["namespaces"], "namespaces")


expand_web_app()
expand_stateful_db()
expand_broken_relationships()
