# Runbook: Blue-Green Platform Upgrade

Blue is the Lab 01 cluster on v1.35.8, running Argo CD from Lab 02. Green is a new kind cluster on v1.36.4. Blue's Argo CD deploys to both. Traffic moves from blue to green at the ingress. Blue stays available until you decommission it.

Context names used throughout:
- Blue: `kind-platform-engineering`
- Green: `kind-platform-engineering-green`

---

## Step 1: Create the green cluster

```bash
cd labs/lab-08-platform-upgrade
kind create cluster --config green-kind-cluster.yaml
kubectl --context kind-platform-engineering-green get nodes   # NotReady until Cilium is installed
```

`green-kind-cluster.yaml` disables the default CNI and kube-proxy, as Lab 01 does. Install Cilium the same way, with one override: the API server host is green's control-plane container name.

```bash
helm upgrade --install cilium cilium \
  --kube-context kind-platform-engineering-green \
  --repo https://helm.cilium.io --version 1.20.2 \
  --namespace kube-system \
  --values ../lab-01-cluster-setup/values/cilium.yaml \
  --set k8sServiceHost=platform-engineering-green-control-plane \
  --wait

kubectl --context kind-platform-engineering-green wait --for=condition=Ready nodes --all --timeout=300s
```

Lab 01's `values/cilium.yaml` hard-codes `platform-engineering-control-plane`. The `--set` line overrides it for green. Without that override, Cilium on green cannot reach its own API server.

## Step 2: Confirm both versions

```bash
kubectl --context kind-platform-engineering version -o json | grep gitVersion          # v1.35.8
kubectl --context kind-platform-engineering-green version -o json | grep gitVersion     # v1.36.4
```

Both clusters should be running at once. Nothing has moved yet.

## Step 3: Register green with Argo CD in blue

Log in to blue's Argo CD (Lab 02). Use port `18080`, because Lab 01 owns host port 8080:

```bash
kubectl --context kind-platform-engineering -n argocd port-forward svc/argocd-server 18080:443 &
argocd login localhost:18080 --username admin --password <password> --insecure
```

Register green using your normal kubeconfig, which has the host address:

```bash
argocd cluster add kind-platform-engineering-green --name platform-engineering-green --yes
```

`cluster add` stores green's server URL as `https://127.0.0.1:<random port>`. Argo CD's controller runs inside blue's nodes, where that address does not work. Patch the stored URL to the Docker-network name, which blue's nodes can resolve:

```bash
# Find the cluster Secret for green (the name is stored base64-encoded in data.name)
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

## Step 4: Deploy the same ApplicationSet to both clusters

Use one ApplicationSet with a cluster generator. Any cluster whose Secret carries `platform.mycompany.io/lab: lab-08` gets the same applications. Label blue's cluster Secret too if you want blue managed by the same set.

```yaml
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
        repoURL: https://github.com/YOUR_ORG/platform-config.git
        targetRevision: main
        path: clusters/production/platform-core   # your addon path from Lab 02
      destination:
        server: '{{server}}'
        namespace: argocd
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
          - CreateNamespace=true
```

Put the Lab 01 smoke app (`platform-components.yaml`: namespace, `hello` Deployment, Service, Ingress) into the same repo so it lands on both clusters. Apply the ApplicationSet to blue:

```bash
# save the YAML above as applicationset.yaml, then:
kubectl --context kind-platform-engineering apply -f applicationset.yaml
argocd app list
```

Wait until every app shows `Synced` and `Healthy` for both clusters. Then confirm green serves the smoke app directly, bypassing the switch:

```bash
curl -s -H 'Host: hello.localtest.me' http://127.0.0.1:9080/hostname   # prints a green pod name
```

Green's Traefik is published on host port 9080, so the Host header routes the request.

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

The `curlimages/curl` image is not pinned here; pin it if you keep this check. If `proxy-check` does not return a pod name, do not switch.

### 5b. Flip the Ingress backend, in Git

Change `spec.rules[0].http.paths[0].backend.service.name` in the Ingress `hello` from `hello` to `hello-green-proxy`. Make the change in the blue overlay in Git and let Argo CD sync it. Do not use `kubectl edit`: with `selfHeal: true`, Argo CD reverts manual edits.

### 5c. Smoke test through the front door

```bash
curl -s http://hello.localtest.me:8080/hostname
kubectl --context kind-platform-engineering-green -n platform-smoke-test get pods -o name
```

The hostname returned should match a green pod name.

### Rollback of the switch

Revert the Git commit from 5b and let Argo CD sync. Blue's `hello` becomes the backend again within one sync. The proxy Service can stay in place.

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
| Switched traffic returns errors | Revert the Git commit from 5b (rollback trigger below) |
| Green is healthy but blue has an unrelated open incident | Delay the switch. Do not cut over into an unclear state |
| Everything green for the soak period | Proceed to decommission blue (step 7) |

### Rollback trigger criteria

Roll back the switch (revert the 5b commit) if any of these hold. The numbers are starting points; tune them for your traffic.

- HTTP 5xx rate through the front door is above 1% for 5 minutes, or is clearly higher than the blue baseline
- Any Pod in a platform namespace stays in `CrashLoopBackOff` for more than 5 minutes
- An Argo CD application on green is `Degraded` for more than 10 minutes
- cert-manager does not issue or renew a certificate on green within 10 minutes (`certmanager_certificate_ready_status`, see the canary runbook)

## Step 7: Decommission blue

Do this only after the soak, and only in this order. The order matters because blue's Argo CD is the controller for green.

1. **Move Argo CD to green first.** Install Argo CD on green with the Lab 02 bootstrap, pointed at the green context. Re-point its applications at green's own cluster. If you delete blue first, green loses its controller and nothing reconciles it.
2. **Take a final Velero backup of blue** before you delete it: [rollback-procedures.md, section d](rollback-procedures.md#d-velero).
3. **Delete blue's cluster** with Lab 01's teardown:
   ```bash
   cd ../lab-01-cluster-setup && ./teardown.sh --provider kind
   ```
4. Green is now the only cluster. Make it the blue for the next upgrade, and give the next cluster the green name.

If you are not ready to decommission, keep blue running. You can switch back at any time with the 5b rollback.
