# Runbook: Canary Upgrades

Two kinds of canary appear in this lab:

1. **Add-on canary**: upgrade one platform add-on (cert-manager here) on green first, while blue keeps the old version. Promote only when it is healthy.
2. **Kubernetes minor canary**: the blue-green cutover in [blue-green-deploy.md](blue-green-deploy.md) is the minor-level canary. This runbook covers the ordering rules and the checks you run before any minor upgrade.

---

## Part 1: Canary add-on rollout

Use this for any single add-on: Traefik, cert-manager, External Secrets, metrics-server. This example uses cert-manager, moving from v1.21.2 to the next patch release you choose.

### Principle

Only one place runs the new version at a time. Green is the canary cluster because it takes no production traffic until the cutover. Blue keeps the old add-on until green is proven.

### Steps

1. **Pick the target version** and check its release notes and upgrade notes. For cert-manager, check the CRD changes first.
2. **Keep the shared ApplicationSet on the old version.** Blue and green both run v1.21.2.
3. **Add a canary Application that targets only green** with the new chart version. Give it a different name from the shared app so the two never fight over one Helm release.
   ```yaml
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
       targetRevision: <new-version>   # the canary version, unverified until you check the chart index
       helm:
         valuesObject:
           crds:
             enabled: true
             keep: true
     destination:
       name: platform-engineering-green
       namespace: cert-manager
     syncPolicy:
       syncOptions:
         - CreateNamespace=true
   ```
   Keep the shared ApplicationSet's selector from matching green for this add-on. Otherwise two Applications own one release. Label green's cluster Secret with a canary marker and exclude it in the selector, or keep the canary out of the set entirely.
4. **Sync the canary** and wait for it to be Healthy:
   ```bash
   argocd app sync cert-manager-canary
   argocd app wait cert-manager-canary --health --timeout 600
   ```
5. **Watch the metrics** (below) for one full soak period. Run the cert-manager canary check: issue a test Certificate from the `platform-ca` ClusterIssuer and confirm it becomes Ready.
6. **Promote or abort.** See the criteria below.

### Promotion

Promotion means moving the shared ApplicationSet to the new version and deleting the canary Application:

1. Bump `targetRevision` in the shared app to the canary version. Commit it in Git.
2. Let Argo CD sync blue first, then confirm blue is healthy.
3. Delete `cert-manager-canary` without cascading, so its resources stay for the shared app to own: `argocd app delete cert-manager-canary --cascade=false`. Do not run `helm uninstall` on the canary: it would remove the CRDs.

### Abort

Abort by deleting `cert-manager-canary` with `argocd app delete cert-manager-canary --cascade=false`, then reverting green to the old version through the shared app (the shared app will re-adopt the release). Blue was never touched.

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

Run before any minor upgrade. The script is read-only unless you add `--execute`.

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

Deprecated-API findings from kubent or pluto are WARN lines that you must read. The script does not parse their output, so it cannot tell that a clean run means zero findings.

You can also check the API server's own count of deprecated-API requests:

```bash
kubectl --context kind-platform-engineering get --raw /metrics | grep apiserver_requested_deprecated_apis
```

Non-zero series mean clients are still calling deprecated APIs. Find them before upgrading.

---

## Summary

| Question | Answer |
|----------|--------|
| What do I canary first? | One add-on on green, then promote through the shared app |
| What is the cutover canary? | Green as a whole, through the ingress switch |
| What order for a minor? | Control plane, then workers one at a time, one minor per cycle |
| Can I skip a minor? | No |
| Is the check passed? | Only if kubent or pluto ran and you read the output |
