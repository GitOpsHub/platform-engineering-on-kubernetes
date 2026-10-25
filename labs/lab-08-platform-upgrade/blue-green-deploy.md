# Runbook: Blue-Green Platform Upgrade

Blue is the Lab 01 cluster on v1.35.8, running Argo CD from Lab 02. Green is a new kind cluster on v1.36.4. Blue's Argo CD deploys the platform baseline to green. Traffic moves from blue to green at the ingress. Blue stays available until you decommission it.

Context names used throughout:
- Blue: `kind-platform-engineering`
- Green: `kind-platform-engineering-green`

Before you start, blue must be running with Argo CD installed (Labs 01 and 02), and `kind`, `helm`, `kubectl`,
`docker` and the `argocd` CLI must be on your PATH. See the [lab README](README.md#prerequisites) for versions.
Run every command from `labs/lab-08-platform-upgrade` unless a step says otherwise.

---

## Step 1: Create the green cluster

```bash
cd labs/lab-08-platform-upgrade
kind create cluster --config green-kind-cluster.yaml
kubectl --context kind-platform-engineering-green get nodes   # NotReady until Cilium is installed
```

Expected result: kind creates a cluster named `platform-engineering-green` with one control-plane and two
worker nodes, and the nodes show `NotReady`. It takes a few minutes the first time because the
`kindest/node:v1.36.4` image must be pulled.

`green-kind-cluster.yaml` disables the default CNI and kube-proxy, as Lab 01 does. Install Cilium the same way, with one override: the API server host is green's control-plane container name.

```bash
helm upgrade --install cilium cilium \
  --kube-context kind-platform-engineering-green \
  --repo https://helm.cilium.io --version 1.20.4 \
  --namespace kube-system \
  --values ../lab-01-cluster-setup/values/cilium.yaml \
  --set k8sServiceHost=platform-engineering-green-control-plane \
  --wait

kubectl --context kind-platform-engineering-green wait --for=condition=Ready nodes --all --timeout=300s
```

Expected result: `helm` prints `STATUS: deployed`, and `wait` prints `node/... condition met` for all three nodes.

Lab 01's `values/cilium.yaml` hard-codes `platform-engineering-control-plane`. The `--set` line overrides it for green. Without that override, Cilium on green cannot reach its own API server.

Check: `kubectl --context kind-platform-engineering-green get nodes` shows 3 nodes `Ready`.

## Step 2: Confirm both versions

```bash
kubectl --context kind-platform-engineering version -o json | grep gitVersion          # v1.35.8
kubectl --context kind-platform-engineering-green version -o json | grep gitVersion     # v1.36.4
```

Expected result: `"gitVersion": "v1.35.8"` for blue and `"gitVersion": "v1.36.4"` for green. (Each command
prints the client version line first; read the `serverVersion` line.) Both clusters are running at once and nothing has moved yet.

## Step 3: Register green with Argo CD in blue

Log in to blue's Argo CD (Lab 02). Use port `18080`, because Lab 01 owns host port 8080:

```bash
kubectl --context kind-platform-engineering -n argocd port-forward svc/argocd-server 18080:443 &
argocd login localhost:18080 --username admin --password <password> --insecure
```

Replace `<password>` with the initial admin password:
`kubectl --context kind-platform-engineering -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d; echo`.
Expected result: `'admin:login' logged in successfully`.

Register green using your normal kubeconfig, which has the host address:

```bash
argocd cluster add kind-platform-engineering-green --name platform-engineering-green --yes
```

Expected result: `Cluster 'https://127.0.0.1:<port>' added`, where `<port>` is the random port kind chose.

`cluster add` stores green's server URL as `https://127.0.0.1:<random port>`. Argo CD's controller runs inside blue's nodes, where that address does not work. Patch the stored URL to the Docker-network name, which blue's nodes can resolve:

```bash
# Find the cluster Secret for green (the name is stored base64-encoded in data.name).
# Replace <cluster-secret> below with the Secret whose decoded name is platform-engineering-green.
kubectl --context kind-platform-engineering -n argocd get secrets -l argocd.argoproj.io/secret-type=cluster
kubectl --context kind-platform-engineering -n argocd get secret <cluster-secret> -o jsonpath='{.data.name}' | base64 -d; echo

kubectl --context kind-platform-engineering -n argocd patch secret <cluster-secret> --type merge \
  -p '{"stringData":{"server":"https://platform-engineering-green-control-plane:6443"}}'

# Label it so the ApplicationSet in step 4 can select it
kubectl --context kind-platform-engineering -n argocd label secret <cluster-secret> platform.mycompany.io/lab=lab-08
```

The API server's serving certificate includes the control-plane node name as a SAN, which is why the name works over TLS. Lab 01's Cilium install relies on the same thing.

Check:

```bash
argocd cluster list   # platform-engineering-green should show a successful connection
```

Expected result: the `STATUS` column for `platform-engineering-green` reads `Successful` (it can read
`Unknown` for up to a minute until the controller refreshes) and the `SERVER` column shows
`https://platform-engineering-green-control-plane:6443`.

## Step 4: Install the Add-Ons on Green and Deploy the Baseline

Green needs the same ingress and PKI that Lab 01 gives blue: Traefik, cert-manager and the `platform-ca`
ClusterIssuer. Step 4a installs the two charts with the exact commands, versions and values files that
`labs/lab-01-cluster-setup/setup.sh` uses, pointed at green. Step 4b then has blue's Argo CD deploy Lab 01's
`platform-components.yaml` (namespaces, root CA, `platform-ca`, and the `hello` smoke app) to green from this
repository.

> These commands are taken from `setup.sh` and the repository files they reference. They have not been run
> against a green cluster as written here; check each expected result before you continue.

### 4a. Install Traefik and cert-manager on green

```bash
helm upgrade --install traefik traefik \
  --kube-context kind-platform-engineering-green \
  --repo https://traefik.github.io/charts --version 42.1.3 \
  --namespace traefik --create-namespace \
  --values ../lab-01-cluster-setup/values/traefik.yaml \
  --wait --timeout 10m

helm upgrade --install cert-manager cert-manager \
  --kube-context kind-platform-engineering-green \
  --repo https://charts.jetstack.io --version v1.21.3 \
  --namespace cert-manager --create-namespace \
  --values ../lab-01-cluster-setup/values/cert-manager.yaml \
  --wait --timeout 10m
```

Expected result: each `helm` command prints `STATUS: deployed`. Traefik listens on NodePort 30080, which
`green-kind-cluster.yaml` maps to host port 9080.

Check:

```bash
kubectl --context kind-platform-engineering-green -n traefik get svc traefik -o jsonpath='{.spec.ports[?(@.name=="web")].nodePort}{"\n"}'
kubectl --context kind-platform-engineering-green -n cert-manager get deploy cert-manager -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
```

Expected result: `30080`, then an image that ends in `:v1.21.3`.

Lab 01 also installs External Secrets, Sealed Secrets and metrics-server. Nothing later in this lab needs them
on green. To install them too, copy their commands from `install_components` in `setup.sh` and add
`--kube-context kind-platform-engineering-green` (or `--context` for `kubectl`).

### 4b. Deploy the platform baseline to green with an ApplicationSet

The ApplicationSet uses a cluster generator: every cluster whose Secret carries
`platform.mycompany.io/lab: lab-08` (you labelled green in step 3) gets an Application that syncs
`labs/lab-01-cluster-setup/platform-components.yaml` from this repository. Do not label blue's cluster
Secret: blue already has these objects from Lab 01's `setup.sh`, and step 5b changes blue's Ingress by hand.

The `repoURL` must be the repository Argo CD already reads for Lab 02. Print it:

```bash
kubectl --context kind-platform-engineering -n argocd get application platform-root -o jsonpath='{.spec.source.repoURL}{"\n"}'
```

Expected result: `https://github.com/GitOpsHub/platform-engineering-on-kubernetes`, or your fork's URL if you
ran Lab 02's `bootstrap.sh --repo-url`. If it differs, put that URL in `repoURL` below.

Create `applicationset.yaml` in `labs/lab-08-platform-upgrade` and apply it to blue:

```bash
cat > applicationset.yaml <<'EOF'
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: platform-addons
  namespace: argocd
spec:
  generators:
    - clusters:
        selector:
          matchLabels:
            platform.mycompany.io/lab: lab-08
  template:
    metadata:
      name: 'platform-addons-{{name}}'
    spec:
      project: default
      source:
        repoURL: https://github.com/GitOpsHub/platform-engineering-on-kubernetes   # from the command above
        targetRevision: main
        path: labs/lab-01-cluster-setup
        directory:
          include: platform-components.yaml   # the directory also holds kind/k3d configs, which are not Kubernetes objects
      destination:
        server: '{{server}}'
        namespace: default   # every object in the file sets its own namespace
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
EOF
kubectl --context kind-platform-engineering apply -f applicationset.yaml
argocd app list
```

Expected result: `applicationset.argoproj.io/platform-addons created`, and `argocd app list` shows one
application, `platform-addons-platform-engineering-green`.

Wait until it shows `Synced` and `Healthy`, then check the PKI on green:

```bash
argocd app wait platform-addons-platform-engineering-green --health --timeout 600
kubectl --context kind-platform-engineering-green wait --for=condition=Ready clusterissuer/platform-ca --timeout=120s
kubectl --context kind-platform-engineering-green -n platform-smoke-test rollout status deployment/hello --timeout=180s
```

Expected result: `Health Status: Healthy`, `clusterissuer.cert-manager.io/platform-ca condition met`, and
`deployment "hello" successfully rolled out`.

Confirm green serves the smoke app directly, bypassing the switch:

```bash
curl -s -H 'Host: hello.localtest.me' http://127.0.0.1:9080/hostname   # prints a green pod name
```

Expected result: a pod name from green, for example `hello-<hash>`. Green's Traefik is published on host
port 9080, so the Host header routes the request. If the request fails, check that step 4a's Traefik check
printed `30080` and that the `hello` rollout finished.

## Step 5: Switch traffic

### Mechanism

Blue's Traefik serves `hello.localtest.me` from the Ingress `hello` in namespace `platform-smoke-test`. Its backend is blue's Service `hello`. The switch changes that backend to `hello-green-proxy`, which is a blue-side Service with manual Endpoints pointing at green's Traefik NodePort (30080) on the shared `kind` Docker network. Green's Traefik then routes by Host header to green's own `hello`.

This is a real request path. Requests go through blue's Traefik to green's Traefik to green's pods. The simulation is in the edge: a production cutover moves DNS or a load-balancer weight, and blue's ingress would not be the front door. Here blue's Traefik is the only front door, so blue cannot be removed first.

### 5a. Create the blue-side proxy

Get green's node IP on the `kind` network:

```bash
GREEN_IP="$(docker inspect -f '{{ (index .NetworkSettings.Networks "kind").IPAddress }}' platform-engineering-green-control-plane)"
echo "${GREEN_IP}"
```

Expected result: an address on the `kind` network, for example `172.18.0.5`.

Create the proxy in blue. This is blue-only, so keep it out of the shared ApplicationSet:

```bash
cat <<EOF | kubectl --context kind-platform-engineering apply -f -
apiVersion: v1
kind: Service
metadata:
  name: hello-green-proxy
  namespace: platform-smoke-test
spec:
  ports:
    - name: http
      port: 80
---
apiVersion: v1
kind: Endpoints
metadata:
  name: hello-green-proxy
  namespace: platform-smoke-test
subsets:
  - addresses:
      - ip: ${GREEN_IP}
    ports:
      - name: http
        port: 30080
EOF
```

Check that Traefik can reach green through the proxy before switching. Port 80 on the proxy Service should return a green pod name:

```bash
kubectl --context kind-platform-engineering -n platform-smoke-test run proxy-check --rm -it --restart=Never \
  --image=curlimages/curl -- -s -H 'Host: hello.localtest.me' http://hello-green-proxy/hostname
```

The `curlimages/curl` image is not pinned here; pin it if you keep this check. Expected result: a green
pod name. If `proxy-check` does not return a pod name, do not switch.

### 5b. Flip the Ingress backend

Change `spec.rules[0].http.paths[0].backend.service.name` in blue's Ingress `hello` from `hello` to
`hello-green-proxy`. Lab 01's `setup.sh` created that Ingress with `kubectl apply`, and step 4b deliberately
leaves blue out of the ApplicationSet, so no Argo CD application owns it and a direct patch stays in place:

```bash
kubectl --context kind-platform-engineering -n platform-smoke-test patch ingress hello --type json \
  -p '[{"op":"replace","path":"/spec/rules/0/http/paths/0/backend/service/name","value":"hello-green-proxy"}]'
```

Expected result: `ingress.networking.k8s.io/hello patched`.

Check: `kubectl --context kind-platform-engineering -n platform-smoke-test get ingress hello -o jsonpath='{.spec.rules[0].http.paths[0].backend.service.name}'`
prints `hello-green-proxy`.

In production the Ingress would be managed by Argo CD, and you would make this one-line change in Git
instead: with `selfHeal: true`, Argo CD reverts a manual patch.

### 5c. Smoke test through the front door

```bash
curl -s http://hello.localtest.me:8080/hostname
kubectl --context kind-platform-engineering-green -n platform-smoke-test get pods -o name
```

Expected result: the `curl` output matches one of the pod names listed by the second command (a green pod).

### Rollback of the switch

Point blue's Ingress back at blue's own `hello` Service. The proxy Service can stay in place.

```bash
kubectl --context kind-platform-engineering -n platform-smoke-test patch ingress hello --type json \
  -p '[{"op":"replace","path":"/spec/rules/0/http/paths/0/backend/service/name","value":"hello"}]'
```

Expected result: `ingress.networking.k8s.io/hello patched`.
Check: `curl -s http://hello.localtest.me:8080/hostname` prints a blue pod name again.

## Step 6: Smoke tests and watch period

Run this checklist before you call the cutover done:

| Check | Command or source | Pass when |
|-------|-------------------|-----------|
| Green nodes Ready | `kubectl --context kind-platform-engineering-green get nodes` | 3 Ready |
| Green apps healthy | `argocd app list` | all green apps Synced and Healthy |
| Switched front door | `curl -s http://hello.localtest.me:8080/hostname` | a green pod name |
| Direct green path | `curl -s -H 'Host: hello.localtest.me' http://127.0.0.1:9080/hostname` | a green pod name |
| Certificates | `kubectl --context kind-platform-engineering-green get clusterissuer platform-ca` | Ready=True |
| No crash loops | `kubectl --context kind-platform-engineering-green get pods -A` | no CrashLoopBackOff in platform namespaces |

On a laptop, a 10-minute soak with `curl` in a loop is enough. In production, watch the metrics in [canary-upgrade.md](canary-upgrade.md#metrics-to-watch) for at least one full traffic cycle.

## Decision table

| Situation | Decision |
|-----------|----------|
| Green apps not Synced/Healthy after 15 minutes | Stay on blue. Fix green, then re-check |
| Direct smoke test (`:9080`) fails | Do not switch |
| Proxy check (5a) fails | Do not switch. Check Endpoints and the green IP |
| Switched traffic returns errors | Roll back the switch from 5b (rollback trigger below) |
| Green is healthy but blue has an unrelated open incident | Delay the switch. Do not cut over into an unclear state |
| Everything green for the soak period | Proceed to decommission blue (step 7) |

### Rollback trigger criteria

Roll back the switch ([rollback of the switch](#rollback-of-the-switch)) if any of these hold. The numbers are starting points; tune them for your traffic.

- HTTP 5xx rate through the front door is above 1% for 5 minutes, or is clearly higher than the blue baseline
- Any Pod in a platform namespace stays in `CrashLoopBackOff` for more than 5 minutes
- An Argo CD application on green is `Degraded` for more than 10 minutes
- cert-manager does not issue or renew a certificate on green within 10 minutes (`certmanager_certificate_ready_status`, see the canary runbook)

## Step 7: Decommission blue

> **Warning:** This step deletes the blue cluster. Do it only after the soak, and only in this order. The order matters because blue's Argo CD is the controller for green.

1. **Move Argo CD to green first.** Install Argo CD on green with the Lab 02 bootstrap, pointed at the green context. Re-point its applications at green's own cluster. If you delete blue first, green loses its controller and nothing reconciles it.
2. **Take a final Velero backup of blue** before you delete it: [rollback-procedures.md, section d](rollback-procedures.md#d-velero).
3. **Delete blue's cluster** with Lab 01's teardown:
   ```bash
   cd ../lab-01-cluster-setup && ./teardown.sh --provider kind
   ```
   Expected result: kind deletes the `platform-engineering` cluster. Check with `kind get clusters`, which then
   lists only `platform-engineering-green`.
4. Green is now the only cluster. Make it the blue for the next upgrade, and give the next cluster the green name.

If you are not ready to decommission, keep blue running. You can switch back at any time with the [rollback of the switch](#rollback-of-the-switch).
