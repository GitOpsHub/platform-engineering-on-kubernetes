# Lab 01: Cluster Setup - Local Kubernetes for Platform Engineering

## Objectives
- Create a reproducible, multi-node local Kubernetes cluster with **kind** or **k3d**
- Install a CNI (Cilium with kube-proxy replacement on kind), an ingress controller, and storage
- Install the platform baseline: cert-manager, External Secrets Operator, Sealed Secrets, metrics-server
- Create standard platform namespaces and an internal CA (`platform-ca` ClusterIssuer)
- Verify the cluster is ready for the GitOps bootstrap in [Lab 02](../lab-02-gitops-bootstrap/README.md)

## Prerequisites
| Tool | Version | Notes |
|------|---------|-------|
| Docker Desktop / Docker Engine | 25+ | Give Docker **8 GB RAM and 4 CPUs** or more |
| kubectl | 1.33+ | Within one minor version of the cluster (1.35) |
| Helm | 3.12+ (Helm 4 works too) | |
| kind | 0.33+ | For Option A |
| k3d | 5.8+ | For Option B |
| curl | any | Used by `verify.sh` |

Background reading: [Kubernetes Architecture](../../docs/01-kubernetes-fundamentals/kubernetes-architecture.md),
[CNI Overview](../../docs/03-networking-fundamentals/cni-overview.md),
[Ingress Controllers](../../docs/03-networking-fundamentals/ingress-controllers.md).

## Estimated Time: 30-45 minutes
(About 10 minutes of that is image pulls on the first run.)

---

## Files in this lab

| File | Purpose |
|------|---------|
| [`kind-cluster.yaml`](kind-cluster.yaml) | 1 control-plane + 2 workers, no default CNI, no kube-proxy, host ports 8080/8443 |
| [`k3d-cluster.yaml`](k3d-cluster.yaml) | 1 server + 2 agents, local registry on `localhost:5001`, host ports 8080/8443 |
| [`values/`](values/) | Helm values for Cilium, Traefik, cert-manager, ESO, metrics-server |
| [`platform-components.yaml`](platform-components.yaml) | Platform namespaces, internal CA, smoke-test app |
| [`setup.sh`](setup.sh) | Creates the cluster and installs everything (idempotent) |
| [`verify.sh`](verify.sh) | Pass/fail checks for every component |
| [`teardown.sh`](teardown.sh) | Deletes the cluster(s) |

### Pinned versions

| Component | Version | Installed by |
|-----------|---------|--------------|
| Kubernetes (kind) | v1.35.8 (`kindest/node`) | `kind-cluster.yaml` |
| Kubernetes (k3d) | v1.35.9 (`rancher/k3s:v1.35.9-k3s1`) | `k3d-cluster.yaml` |
| Cilium | 1.20.4 | Helm, kind only |
| Traefik | chart 42.1.3 (Traefik v3.7) | Helm |
| cert-manager | v1.21.3 | Helm |
| External Secrets Operator | 2.14.0 | Helm |
| Sealed Secrets | v0.41.1 | Release manifest |
| metrics-server | chart 3.14.0 (v0.9.0) | Helm |

All versions live at the top of `setup.sh`; change them there.

> **Why Traefik instead of ingress-nginx?** The community `ingress-nginx` project
> was retired in March 2026 and no longer receives releases or security fixes.
> Traefik implements the standard Ingress API (and Gateway API), so the
> manifests in later labs are portable to any conformant controller.

---

## Quick start

```bash
cd labs/lab-01-cluster-setup

./setup.sh                  # kind + Cilium   (Option A, default)
# or
./setup.sh --provider k3d   # k3d + flannel   (Option B)

./verify.sh                 # add --provider k3d if you used k3d
```

The rest of this README walks through what the script does so you can run each
step by hand and understand it.

---

## Option A: kind + Cilium (recommended, closest to a real cluster)

### Step 1 - Create the cluster

Key settings in [`kind-cluster.yaml`](kind-cluster.yaml):

```yaml
networking:
  disableDefaultCNI: true   # Cilium is installed next
  kubeProxyMode: "none"     # Cilium replaces kube-proxy
nodes:
  - role: control-plane
    extraPortMappings:
      - { containerPort: 30080, hostPort: 8080, listenAddress: "127.0.0.1" }  # HTTP
      - { containerPort: 30443, hostPort: 8443, listenAddress: "127.0.0.1" }  # HTTPS
  - role: worker
  - role: worker
```

```bash
kind create cluster --config kind-cluster.yaml
kubectl get nodes      # STATUS is NotReady - expected, there is no CNI yet
```

### Step 2 - Install Cilium with kube-proxy replacement

Because there is no kube-proxy, nothing implements the `kubernetes` Service
ClusterIP yet. Cilium must therefore talk to the API server **directly**, which
is why [`values/cilium.yaml`](values/cilium.yaml) sets:

```yaml
kubeProxyReplacement: true
k8sServiceHost: platform-engineering-control-plane   # control-plane container name
k8sServicePort: 6443
ipam:
  mode: kubernetes
```

The container name resolves on kind's Docker network on both Linux and macOS
Docker Desktop, so the same values work everywhere (don't use `127.0.0.1` or
the host-mapped API port here - that only works from your laptop, not from
inside the nodes).

```bash
helm upgrade --install cilium cilium \
  --repo https://helm.cilium.io --version 1.20.4 \
  --namespace kube-system -f values/cilium.yaml --wait

kubectl wait --for=condition=Ready nodes --all --timeout=300s
kubectl -n kube-system get pods -l k8s-app=cilium
```

Optional, if you have the [cilium CLI](https://github.com/cilium/cilium-cli/releases)
(`brew install cilium-cli` on macOS):

```bash
cilium status --wait
cilium hubble ui          # opens Hubble UI on http://localhost:12000
```

### Step 3 - Storage

kind already ships the Rancher **local-path-provisioner** as the default
`standard` StorageClass, so nothing needs installing:

```bash
kubectl get storageclass   # standard (default)   rancher.io/local-path
```

Continue with [Platform components](#platform-components-both-options).

---

## Option B: k3d (faster, lighter)

k3d runs k3s, which bundles flannel, kube-proxy, a NetworkPolicy controller and
a `local-path` StorageClass. To keep this path fast we **keep** those and only
disable Traefik, ServiceLB and metrics-server so the lab installs the same
versions as on kind.

```yaml
# k3d-cluster.yaml (excerpt)
apiVersion: k3d.io/v1alpha5
kind: Simple
image: rancher/k3s:v1.35.9-k3s1
ports:
  - port: 127.0.0.1:8080:30080     # host 8080 -> Traefik NodePort
    nodeFilters: [loadbalancer]
registries:
  create:
    name: platform-registry.localhost
    hostPort: "5001"
options:
  k3s:
    extraArgs:
      - arg: "--disable=traefik,servicelb,metrics-server"
        nodeFilters: [server:*]
```

```bash
k3d cluster create --config k3d-cluster.yaml
kubectl get nodes -o wide       # Ready within ~30s
```

Push images to the bundled registry and use them in-cluster:

```bash
docker tag nginx:alpine localhost:5001/nginx:alpine
docker push localhost:5001/nginx:alpine
# reference it in manifests as: platform-registry.localhost:5000/nginx:alpine
```

> Cilium on k3d is possible but needs extra node mounts (`/sys/fs/bpf`) and
> `--flannel-backend=none --disable-network-policy --disable-kube-proxy`. Use
> the kind path if you want to explore Cilium/Hubble.

---

## Platform components (both options)

Every chart is installed with `helm upgrade --install`, so re-running is safe.
`--repo` is used instead of `helm repo add` so no local repo state is needed.

### Step 4 - Traefik ingress controller

```bash
helm upgrade --install traefik traefik \
  --repo https://traefik.github.io/charts --version 42.1.3 \
  --namespace traefik --create-namespace -f values/traefik.yaml --wait
```

Traefik's Service is a `NodePort` on **30080/30443**, which the cluster config
maps to `127.0.0.1:8080/8443` on your laptop.

### Step 5 - cert-manager

```bash
helm upgrade --install cert-manager cert-manager \
  --repo https://charts.jetstack.io --version v1.21.3 \
  --namespace cert-manager --create-namespace -f values/cert-manager.yaml --wait
```

`crds.enabled: true` installs the CRDs with the chart (the old `installCRDs`
flag is deprecated). `crds.keep: true` stops `helm uninstall` from deleting
every Certificate in the cluster.

### Step 6 - External Secrets Operator

```bash
helm upgrade --install external-secrets external-secrets \
  --repo https://charts.external-secrets.io --version 2.14.0 \
  --namespace external-secrets --create-namespace -f values/external-secrets.yaml --wait
```

See [External Secrets](../../docs/06-secrets-management/external-secrets.md) for how
`SecretStore` / `ExternalSecret` are used.

### Step 7 - Sealed Secrets

```bash
kubectl apply --server-side \
  -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.41.1/controller.yaml
kubectl -n kube-system rollout status deployment/sealed-secrets-controller
```

### Step 8 - metrics-server

```bash
helm upgrade --install metrics-server metrics-server \
  --repo https://kubernetes-sigs.github.io/metrics-server --version 3.14.0 \
  --namespace kube-system -f values/metrics-server.yaml --wait
```

`--kubelet-insecure-tls` is needed because kind/k3d kubelets use self-signed
serving certificates. **Never** use it on a real cluster.

### Step 9 - Platform baseline

[`platform-components.yaml`](platform-components.yaml) creates:

- Namespaces `platform-system`, `platform-monitoring`, `platform-security`,
  `developer-platform`, `platform-smoke-test`, all labelled
  `platform.mycompany.io/managed: "true"` and with
  [Pod Security Admission](../../docs/12-security-compliance/pod-security.md) labels
- A self-signed root CA and a **`platform-ca` ClusterIssuer** for internal TLS
- A smoke-test app exposed at `http://hello.localtest.me:8080`
  (`*.localtest.me` is public DNS that resolves to `127.0.0.1`)

```bash
kubectl apply -f platform-components.yaml
kubectl get clusterissuer
kubectl -n platform-smoke-test get deploy,svc,ingress,certificate
```

> Argo CD namespace annotations and Flux labels are intentionally **not** set
> here - Lab 02 adopts these namespaces via GitOps instead.

---

## Verification

```bash
./verify.sh                 # or ./verify.sh --provider k3d
```

Expected output (abridged):

```
=== Lab 01 verification (context: kind-platform-engineering) ===
Cluster
  [PASS] 3 nodes registered
  [PASS] all nodes Ready
  ...
Platform baseline
  [PASS] ingress reachable on :8080
  [PASS] in-cluster DNS + Service routing

=== 24 passed, 0 failed ===
```

The count depends on the provider. The kind run (Cilium checks included) reports
**24 passed**. The k3d run skips the four Cilium/Hubble checks and reports **20 passed**
(`./verify.sh --provider k3d`).

Manual checks:

```bash
curl http://hello.localtest.me:8080/hostname   # prints a pod name
kubectl top nodes                              # metrics-server works
kubectl get nodes -L platform.mycompany.io/role
kubectl -n kube-system get ds kube-proxy       # NotFound on kind (Cilium KPR)
```

---

## Cleanup

```bash
./teardown.sh               # deletes whichever lab cluster exists
# or explicitly:
kind delete cluster --name platform-engineering
k3d cluster delete platform-engineering
```

Keep the cluster if you are continuing to Lab 02. Optionally reclaim disk
space afterwards with `docker image prune` (this affects all of Docker, not
just the lab).

---

## Troubleshooting

### Nodes stay `NotReady` (kind)
No CNI yet, or Cilium is failing. Check:
```bash
kubectl -n kube-system get pods -l k8s-app=cilium
kubectl -n kube-system logs ds/cilium -c cilium-agent | tail -50
```
If the agent logs `unable to contact k8s api-server`, the `k8sServiceHost`
value does not match the control-plane container name (`docker ps` shows it -
it is `<cluster-name>-control-plane`).

### `too many open files` / pods crash-looping on Linux
Multi-node kind clusters exhaust the default inotify limits:
```bash
sudo sysctl fs.inotify.max_user_watches=524288 fs.inotify.max_user_instances=512
```

### Pods stuck in `Pending`
```bash
kubectl get events -A --sort-by=.lastTimestamp | tail -20
```
Usual causes: Docker has too little memory/CPU (raise it in Docker Desktop >
Settings > Resources), or a PVC without a StorageClass.

### `curl http://hello.localtest.me:8080` fails
```bash
docker ps --format '{{.Names}}\t{{.Ports}}' | grep platform-engineering   # port mapping present?
kubectl -n traefik get svc traefik       # NodePorts 30080/30443?
kubectl -n traefik logs deploy/traefik
```
If `localtest.me` doesn't resolve (some corporate DNS blocks it), use
`curl -H 'Host: hello.localtest.me' http://127.0.0.1:8080/`.
Port 8080 already in use? Stop the other process, or change `hostPort` in the
cluster config and recreate the cluster.

### Hubble UI not accessible
```bash
kubectl -n kube-system port-forward svc/hubble-ui 12000:80
```

### k3d registry not reachable
```bash
curl http://localhost:5001/v2/_catalog
k3d registry list
```
In-cluster, use the name `platform-registry.localhost:5000`, not `localhost:5001`.

### cert-manager webhook errors on `kubectl apply`
`failed calling webhook "webhook.cert-manager.io"` right after install just
means the webhook isn't serving yet - wait 30 seconds and re-apply (setup.sh
uses `--wait` to avoid this).

---

## Next Steps

1. **Read**: [GitOps Principles](../../docs/05-gitops/gitops-principles.md)
2. **Lab 02**: [GitOps Bootstrap with Argo CD](../lab-02-gitops-bootstrap/README.md)
3. **Explore**: [Network Policies](../../docs/03-networking-fundamentals/network-policies.md) with Cilium and Hubble

---

## Resources

- [kind documentation](https://kind.sigs.k8s.io/)
- [k3d documentation](https://k3d.io/)
- [Cilium kube-proxy replacement](https://docs.cilium.io/en/stable/network/kubernetes/kubeproxy-free/)
- [Traefik Helm chart](https://github.com/traefik/traefik-helm-chart)
- [cert-manager documentation](https://cert-manager.io/docs/)
- [External Secrets documentation](https://external-secrets.io/)
- [Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets)
