# Runbook: Canary Upgrades

Two kinds of canary appear in this lab:

1. **Add-on canary**: upgrade one platform add-on (cert-manager here) on green first, while blue keeps the old version. Promote only when it is healthy.
2. **Kubernetes minor canary**: the blue-green cutover in [blue-green-deploy.md](blue-green-deploy.md) is the minor-level canary. This runbook covers the ordering rules and the checks you run before any minor upgrade.

---

## Part 1: Canary add-on rollout

Use this for any single add-on: Traefik, cert-manager, External Secrets, metrics-server. Run the `kubectl` and `argocd` commands from any directory; run `./upgrade-automation.sh` from
`labs/lab-08-platform-upgrade`. Complete steps 1 to 4 of [blue-green-deploy.md](blue-green-deploy.md) first,
so green is registered with Argo CD. This example uses cert-manager, moving from v1.21.3 (the version Lab 01 installs) to the next patch release you choose.

### Principle

Only one place runs the new version at a time. Green is the canary cluster because it takes no production traffic until the cutover. Blue keeps the old add-on until green is proven.

### Steps

1. **Pick the target version** and check its release notes and upgrade notes. For cert-manager, check the CRD changes first.
   Expected result: you have a target version `<new-version>` and have read the CRD changes.
   Check: confirm the version exists with
   `helm show chart cert-manager --repo https://charts.jetstack.io --version <new-version>`.
2. **Confirm both clusters run the old version.** Lab 01's `setup.sh` installed cert-manager v1.21.3 on blue,
   and step 4a of [blue-green-deploy.md](blue-green-deploy.md) installed the same version on green. Both are
   Helm releases named `cert-manager` in namespace `cert-manager`.
   ```bash
   for ctx in kind-platform-engineering kind-platform-engineering-green; do
     kubectl --context "${ctx}" -n cert-manager get deploy cert-manager -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
   done
   ```
   Expected result: two image names, each ending in `:v1.21.3`.
3. **Check that the shared ApplicationSet does not own cert-manager.** The `platform-addons` ApplicationSet
   from step 4b syncs only `platform-components.yaml`, so it never renders the cert-manager chart. The canary
   therefore needs no change to the ApplicationSet's cluster selector: excluding green from that selector would
   remove green's whole platform baseline, not just cert-manager. Instead, the canary is a separate
   Application for this one add-on, and its `destination` names green only.
   ```bash
   kubectl --context kind-platform-engineering -n argocd get applicationset platform-addons \
     -o jsonpath='{.spec.template.spec.source.directory.include}{"\n"}'
   ```
   Expected result: `platform-components.yaml`. If your ApplicationSet also deploys cert-manager (for example
   because you pointed it at your own add-on directory), remove cert-manager from that directory for green
   before you continue, or the two Applications will fight over the same objects.
4. **Create, apply and sync the canary Application.** `releaseName: cert-manager` makes Argo CD render the same
   object names as the existing Helm release on green, so the canary upgrades green's cert-manager in place
   instead of installing a second copy. The values match Lab 01's `values/cert-manager.yaml`. Replace
   `<new-version>` with the version from step 1 before you run the block.
   ```bash
   cat > cert-manager-canary.yaml <<'EOF'
   apiVersion: argoproj.io/v1alpha1
   kind: Application
   metadata:
     name: cert-manager-canary
     namespace: argocd
   spec:
     project: default
     source:
       repoURL: https://charts.jetstack.io
       chart: cert-manager
       targetRevision: <new-version>   # the canary version from step 1
       helm:
         releaseName: cert-manager     # same object names as the Helm release on green
         valuesObject:                 # same values as labs/lab-01-cluster-setup/values/cert-manager.yaml
           crds:
             enabled: true
             keep: true
           prometheus:
             enabled: true
           webhook:
             timeoutSeconds: 10
     destination:
       name: platform-engineering-green   # the cluster name from blue-green-deploy.md step 3; blue is never targeted
       namespace: cert-manager
     syncPolicy:
       syncOptions:
         - CreateNamespace=true
         - ServerSideApply=true
   EOF
   kubectl --context kind-platform-engineering apply -f cert-manager-canary.yaml
   argocd app sync cert-manager-canary
   argocd app wait cert-manager-canary --health --timeout 600
   ```
   Expected result: `application.argoproj.io/cert-manager-canary created`, then `argocd app wait` returns with
   `Health Status: Healthy`.
   Check: run the step 2 loop again. Blue still ends in `:v1.21.3`; green now ends in `:<new-version>`.
5. **Watch the metrics** (below) for one full soak period. Run the cert-manager canary check: issue a test Certificate from the `platform-ca` ClusterIssuer and confirm it becomes Ready.
   ```bash
   cat <<'EOF' | kubectl --context kind-platform-engineering-green apply -f -
   apiVersion: cert-manager.io/v1
   kind: Certificate
   metadata:
     name: canary-check
     namespace: platform-smoke-test
   spec:
     secretName: canary-check-tls
     dnsNames: [canary-check.localtest.me]
     issuerRef:
       name: platform-ca
       kind: ClusterIssuer
   EOF
   kubectl --context kind-platform-engineering-green -n platform-smoke-test wait --for=condition=Ready certificate/canary-check --timeout=120s
   ```
   Expected result: `certificate.cert-manager.io/canary-check condition met`. Delete it afterwards with
   `kubectl --context kind-platform-engineering-green -n platform-smoke-test delete certificate canary-check`.
6. **Promote or abort.** See the criteria below.
   Check: promote only if none of the [abort criteria](#abort-criteria) hold.

### Promotion

Promotion means moving blue to the new version and handing green's cert-manager back to Helm:

1. Upgrade blue with the same command Lab 01's `setup.sh` uses, at the new version, run from
   `labs/lab-08-platform-upgrade`:
   ```bash
   helm upgrade --install cert-manager cert-manager \
     --kube-context kind-platform-engineering \
     --repo https://charts.jetstack.io --version <new-version> \
     --namespace cert-manager --create-namespace \
     --values ../lab-01-cluster-setup/values/cert-manager.yaml \
     --wait --timeout 10m
   ```
   Expected result: `STATUS: deployed`. Then confirm blue is healthy: the step 2 loop shows `:<new-version>`
   for both clusters.
2. Delete `cert-manager-canary` without cascading, so its resources stay in place:
   `argocd app delete cert-manager-canary --cascade=false --yes`. Do not run `helm uninstall` on green: it
   would remove cert-manager from green.
3. Run the same `helm upgrade --install` command against green (`--kube-context kind-platform-engineering-green`)
   so that green's Helm release record matches the version that is running.
   Expected result: `STATUS: deployed`.
4. Bump `CERT_MANAGER_VERSION` in `labs/lab-01-cluster-setup/setup.sh` (and the Lab 01 README pin) in your
   fork, so a rebuilt cluster gets the new version.

### Abort

Abort by deleting `cert-manager-canary` with `argocd app delete cert-manager-canary --cascade=false --yes`, then
reverting green to the old version with step 4a's cert-manager command from
[blue-green-deploy.md](blue-green-deploy.md#4a-install-traefik-and-cert-manager-on-green) (`--version v1.21.3`).
Expected result: `STATUS: deployed`, and the step 2 loop shows `:v1.21.3` for green again. Blue was never touched.

---

## Metrics to watch

These names are the ones this lab expects. Confirm each one on your own `/metrics` endpoint before relying on it: names can change between chart versions.

| Source | Metric | What it tells you |
|--------|--------|-------------------|
| cert-manager | `certmanager_certificate_ready_status` | Certificate readiness by condition. Should stay `1` for `Ready=True` |
| Argo CD | `argocd_app_info` | Sync and health status per application (labels: `sync_status`, `health_status`) |
| kube-state-metrics | `kube_pod_container_status_restarts_total` | Restart counts; a steady climb is a crash loop |
| kube-state-metrics | `kube_deployment_status_replicas_unavailable` | Replicas not yet available during a rollout |
| Traefik | `traefik_service_requests_total`, `traefik_service_request_duration_seconds` | Request volume, status codes and latency through the ingress |

Lab 06 (observability) provides Prometheus. Without it, use `kubectl get events -A --sort-by=.lastTimestamp` and `kubectl logs` on the add-on's pods, and look for the same signals.

## Abort criteria

Abort the canary and keep the old version if any of these hold during the soak:

- A Certificate on green is not Ready for more than 10 minutes after it is requested
- Any add-on pod restarts more than 3 times in 15 minutes, or sits in `CrashLoopBackOff`
- The Argo CD app for the canary reports `Degraded`, or stays `Progressing` for more than 15 minutes
- A CRD change in the new version reports a validation or conversion error
- Anything on blue degrades while the canary runs (stop; the canary is not the cause until you know)

---

## Part 2: Kubernetes minor upgrade ordering

Blue-green is how this lab upgrades Kubernetes. In-place upgrades (common on managed providers, and with kubeadm) follow these rules.

### Rules

1. **Control plane first, then workers.** The API server must be at the new minor before any kubelet moves to it.
2. **One minor at a time.** 1.35 to 1.36, then 1.36 to 1.37. Skipping a minor is not supported.
3. **Check deprecated APIs before the control plane moves.** A workload using an API removed in the target version will break after the control-plane upgrade.
4. **Upgrade one node at a time** for workers. Drain it, upgrade it, uncordon it, then move on.

### kubeadm sequence (illustrative; match your provider's flow)

Control plane, one node at a time:

```bash
kubeadm upgrade plan
kubeadm upgrade apply v1.36.x          # use the exact patch version from `kubeadm upgrade plan`
# then upgrade kubelet and kubectl packages on the control-plane node and restart kubelet
```

Each worker, one at a time:

```bash
kubectl drain <node> --ignore-daemonsets --delete-emptydir-data
# on the worker: upgrade kubeadm, then `kubeadm upgrade node`, then the kubelet and kubectl packages, then restart kubelet
kubectl uncordon <node>
```

`upgrade-automation.sh` runs the cordon and drain steps for you, but only with `--execute` and a `y` answer for each one. It does not run the upgrade step itself.

### Version skew policy

From the Kubernetes version skew policy (stated here as of this writing; check [kubernetes.io](https://kubernetes.io/releases/version-skew-policy/) before you rely on it):

- **kubelet** may be up to **three minor versions older** than the API server
- **kubectl** should be within **one minor version** of the API server, older or newer
- **kube-proxy** must not be newer than the API server (Lab 08 uses Cilium, so kube-proxy is absent by design)

So for a 1.36 control plane, kubelets may run 1.33 through 1.36. The upgrade order keeps you inside that window: after the control plane moves to 1.36, workers can lag behind while you upgrade them one by one.

---

## Part 3: Pre-flight checks

Run before any minor upgrade, from `labs/lab-08-platform-upgrade`. The script is read-only unless you add `--execute`.

```bash
./upgrade-automation.sh --target-version 1.36 --context kind-platform-engineering
```

It runs:

| Check | Result when it fails or can't run |
|-------|-----------------------------------|
| Current server version and skew (current minor or exactly one above) | FAIL if the target is more than one minor away, or is a downgrade |
| Deprecated APIs with kubent, or with pluto if kubent is absent | SKIP if neither tool is installed. A SKIP is not a pass |
| PodDisruptionBudgets with zero disruptions allowed | WARN: drain will block until the pods recover |
| Node health (Ready, and cordoned nodes) | FAIL for NotReady; WARN for cordoned |

Expected result: `[PASS]`, `[WARN]`, `[FAIL]` or `[SKIP]` lines for each check, then
`Summary: PASS=<n> WARN=<n> FAIL=<n> SKIP=<n>`. The script exits 0 when there is no FAIL, and 1 otherwise.
With no FAIL it prints the planned cordon and drain steps without running them.

Deprecated-API findings from kubent or pluto are WARN lines that you must read. The script does not parse their output, so it cannot tell that a clean run means zero findings.

You can also check the API server's own count of deprecated-API requests:

```bash
kubectl --context kind-platform-engineering get --raw /metrics | grep apiserver_requested_deprecated_apis
```

Expected result: no output on a clean cluster. Non-zero series mean clients are still calling deprecated APIs.
Find them before upgrading.

---

## Summary

| Question | Answer |
|----------|--------|
| What do I canary first? | One add-on on green, then promote it to blue |
| What is the cutover canary? | Green as a whole, through the ingress switch |
| What order for a minor? | Control plane, then workers one at a time, one minor per cycle |
| Can I skip a minor? | No |
| Is the check passed? | Only if kubent or pluto ran and you read the output |
