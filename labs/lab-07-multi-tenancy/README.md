# Lab 07: Multi-Tenancy - Isolated Team Namespaces on One Cluster

## Objectives
- Provision two tenants (`acme` and `globex`) on the Lab 01 cluster, each with its own namespace, quota, network policies and owner RBAC
- Enforce the restricted Pod Security profile on every tenant namespace
- Show that a `ResourceQuota` plus `LimitRange` cap what a tenant can request
- Show that default-deny `NetworkPolicy` blocks cross-tenant traffic while same-tenant traffic and DNS still work
- Show that a tenant owner group is admin inside its own namespaces and has no cluster-scoped access
- Understand the `Tenant` API that the [tenant-operator](tenant-operator/README.md) reconciles, and run the lab either with plain manifests or with the operator

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Lab 01 cluster | `kind-platform-engineering` | Kubernetes v1.35, Cilium (NetworkPolicy enforced), Traefik, cert-manager. Run [Lab 01](../lab-01-cluster-setup/README.md) first |
| kubectl | 1.33+ | Within one minor version of the cluster |
| bash | 4+ | `setup.sh`, `teardown.sh`, `tests/verify-isolation.sh` |
| shellcheck, Python with PyYAML | any | Optional, only for linting the lab's files |
| tenant-operator | optional | Only needed for operator mode. The lab works without it |

Background reading:
[Pod Security](../../docs/12-security-compliance/pod-security.md),
[RBAC](../../docs/01-kubernetes-fundamentals/rbac.md),
[Network Policies](../../docs/03-networking-fundamentals/network-policies.md),
[Platform API design](../../docs/09-platform-apis/platform-api-design.md).

## Estimated Time: 30-40 minutes
(About 5 minutes of that is pulling the demo images on the first run.)

---

## Steps

Everything lives in this directory. The scripts target the context `kind-platform-engineering` explicitly, so a different current context never receives the manifests.

### Step 1 - Confirm the cluster

```bash
kubectl config get-contexts -o name | grep kind-platform-engineering
kubectl --context kind-platform-engineering get namespace platform-system
```

If the context is missing, run Lab 01 first. Nothing here creates or deletes clusters.

### Step 2 - Read the tenant building blocks

| Piece | File | What it does |
|-------|------|--------------|
| Namespace template | [`namespace-templates/namespace.yaml`](namespace-templates/namespace.yaml) | Namespace labelled `platform.mycompany.io/tenant` and `platform.mycompany.io/managed-by: tenant-operator`, PSA `enforce/audit/warn: restricted` at `v1.35` |
| Quota and limits | [`resource-quotas/quota.yaml`](resource-quotas/quota.yaml) | `ResourceQuota tenant-quota` (requests and limits for cpu and memory, `requests.storage`, `pods`) and `LimitRange tenant-limits` (container defaults) |
| Network policies | [`network-isolation/network-policies.yaml`](network-isolation/network-policies.yaml) | `tenant-default-deny`, `tenant-allow-same-tenant`, `tenant-allow-dns`, plus the lab-only `tenant-allow-ingress-from-traefik` |
| Owner RBAC | [`rbac/tenant-owner.yaml`](rbac/tenant-owner.yaml) | `RoleBinding tenant-owners` to ClusterRole `admin`, bound to group `tenant:<tenant>:owners` |
| Demo app | [`tests/demo-app.yaml`](tests/demo-app.yaml) | `echo` Deployment and Service, `client` pod with curl |
| Tenant CRs | [`tenant-operator/`](tenant-operator/) | `Tenant` objects for `acme` and `globex`, used only in operator mode |

The names of the quota, LimitRange, network policies and RoleBinding are the same as the operator's, so the two modes produce the same objects. The differences are listed in [tenant-operator/README.md](tenant-operator/README.md).

The templates use placeholders, filled in by `setup.sh` with `sed`:

| Placeholder | Value | Used in |
|-------------|-------|---------|
| `__NAMESPACE__` | `tenant-<tenant>-dev` | `namespace.yaml` |
| `__TENANT__` | `acme` or `globex` | `namespace.yaml`, `network-policies.yaml`, `tenant-owner.yaml` |
| `__ECHO_IMAGE__`, `__CLIENT_IMAGE__` | pinned image tags | `tests/demo-app.yaml` |

Other objects contain no tenant name, so they are applied with `kubectl -n <namespace>`.

### Step 3 - Provision both tenants

```bash
cd labs/lab-07-multi-tenancy
./setup.sh
```

`setup.sh` checks whether the `tenants.platform.mycompany.io` CRD exists and chooses a mode:

**Manifest mode (no operator, the default).** For each tenant, `setup.sh`:

1. Renders and applies the namespace template.
2. Applies the quota and LimitRange into the namespace.
3. Renders and applies the four NetworkPolicies.
4. Renders and applies the owner RoleBinding.
5. Applies the demo app and waits for `echo` to roll out and `client` to become Ready.

**Operator mode (CRD installed).** For each tenant, `setup.sh` applies the `Tenant` CR, waits for the namespace and for `status.phase=Ready`, and then applies the demo app. It does not apply the child manifests, because the operator owns them (see [Operator mode](#operator-mode-optional)).

Both modes are safe to re-run.

### Step 4 - Do it by hand (optional)

This is what `setup.sh` does in manifest mode for `acme`, so you can see every object:

```bash
CTX=kind-platform-engineering
T=acme
NS=tenant-$T-dev

sed -e "s|__TENANT__|$T|g" -e "s|__NAMESPACE__|$NS|g" namespace-templates/namespace.yaml | kubectl --context $CTX apply -f -
kubectl --context $CTX -n $NS apply -f resource-quotas/quota.yaml
sed -e "s|__TENANT__|$T|g" network-isolation/network-policies.yaml | kubectl --context $CTX -n $NS apply -f -
sed -e "s|__TENANT__|$T|g" rbac/tenant-owner.yaml | kubectl --context $CTX -n $NS apply -f -
sed -e 's|__ECHO_IMAGE__|nginxinc/nginx-unprivileged:1.27-alpine|' -e 's|__CLIENT_IMAGE__|curlimages/curl:8.11.1|' tests/demo-app.yaml \
  | kubectl --context $CTX -n $NS apply -f -
```

The rendered RoleBinding for `acme` is in [`rbac/examples/tenant-acme-owner.yaml`](rbac/examples/tenant-acme-owner.yaml).

### Step 5 - Look at the result

```bash
kubectl --context kind-platform-engineering get namespaces -l platform.mycompany.io/tenant --show-labels
kubectl --context kind-platform-engineering -n tenant-acme-dev describe resourcequota tenant-quota
kubectl --context kind-platform-engineering -n tenant-acme-dev get networkpolicy
kubectl --context kind-platform-engineering -n tenant-acme-dev get rolebinding tenant-owners -o wide
kubectl --context kind-platform-engineering -n tenant-acme-dev get pods
```

---

## Verification

```bash
./tests/verify-isolation.sh
```

The script runs five checks:

| Check | What it proves | How |
|-------|----------------|-----|
| (a) same-tenant HTTP | Same-tenant traffic is allowed | `curl` from `client` to `echo` in the same namespace |
| (b) cross-tenant HTTP | Cross-tenant traffic is denied | `curl --max-time 5` from `tenant-acme-dev` to `tenant-globex-dev`, expects a non-zero exit |
| (c) DNS | Egress to CoreDNS is allowed | `curl` from `client` to the API Service name. Exit 6 means the name did not resolve; any other exit means DNS worked |
| (d) quota | The quota rejects an oversized pod | Server-side dry run of a pod requesting `cpu: 4` on top of the 100m already used. The error must contain `exceeded quota` |
| (e) RBAC scope | The owner group is admin only inside its namespace | `kubectl auth can-i --as-group tenant:acme:owners`: `no` for `list nodes`, `yes` for `create deployments` in the namespace |

Expected end of output: `=== 6 passed, 0 failed ===`. Check (e) runs two assertions, so there are six in total.

> **Check (b) depends on the CNI.** NetworkPolicy is only enforced because Lab 01 uses Cilium. On a CNI that ignores NetworkPolicy (kindnet, flannel without policy) the cross-tenant `curl` succeeds and (b) fails. That is a real isolation gap, not a test bug.

Manual checks:

```bash
kubectl --context kind-platform-engineering -n tenant-acme-dev exec client -- \
  curl -sS --max-time 5 http://echo.tenant-globex-dev.svc.cluster.local/ ; echo "exit=$?"    # expect a timeout, exit=28
kubectl --context kind-platform-engineering -n tenant-acme-dev exec client -- \
  curl -fsS http://echo.tenant-acme-dev.svc.cluster.local/ -o /dev/null && echo "same-tenant OK"
```

---

## Operator mode (optional)

Use this mode only if you have built and installed `tenant-operator` from `code/tenant-operator`. Do not run the manifest mode and operator mode on the same tenant at the same time.

The operator adopts a namespace that carries its labels, but it never adopts an existing ResourceQuota, LimitRange, NetworkPolicy or RoleBinding that it did not create. If those already exist, it reports `ObjectConflict`. So to switch a cluster that ran the lab in manifest mode:

```bash
./teardown.sh          # delete the manifest-mode tenants
# install the operator (see tenant-operator/README.md)
./setup.sh             # now runs in operator mode
kubectl --context kind-platform-engineering get tenants
```

Confirm the operator owns the children:

```bash
kubectl --context kind-platform-engineering -n tenant-acme-dev get resourcequota tenant-quota -o jsonpath='{.metadata.ownerReferences[0].kind}{"\n"}'
# expect: Tenant
```

---

## Cleanup

```bash
./teardown.sh
```

This deletes the `Tenant` CRs (if the CRD exists) and the namespaces `tenant-acme-dev` and `tenant-globex-dev`. Everything in a tenant namespace goes with it. The Lab 01 cluster and its platform namespaces are untouched. Re-running `teardown.sh` is safe because every delete uses `--ignore-not-found`.

---

## Troubleshooting

### `setup.sh` says the context is not found
Lab 01 has not run, or its kubeconfig entry was removed. Run [Lab 01](../lab-01-cluster-setup/README.md) or check `kubectl config get-contexts`.

### Check (b) reports that cross-tenant HTTP succeeded
NetworkPolicy is not enforced, or the policies were not applied:
```bash
kubectl --context kind-platform-engineering -n tenant-acme-dev get networkpolicy
kubectl --context kind-platform-engineering -n kube-system get pods -l k8s-app=cilium
```
Also confirm the Cilium agent is healthy (see Lab 01 troubleshooting).

### Check (c) fails: DNS does not resolve
The `tenant-allow-dns` policy allows egress to any pod in `kube-system` on UDP and TCP 53, which is the same rule tenant-operator uses. If DNS fails, check that CoreDNS runs in `kube-system` and that the namespace carries the standard name label:
```bash
kubectl --context kind-platform-engineering -n kube-system get pods -l k8s-app=kube-dns
kubectl --context kind-platform-engineering get namespace kube-system --show-labels
```

### `echo` or `client` pod is rejected or stuck
PSA `restricted` rejects pods without `runAsNonRoot`, a `seccompProfile`, `allowPrivilegeEscalation: false`, or `drop: [ALL]`. The demo pods set all four. Look at the events:
```bash
kubectl --context kind-platform-engineering -n tenant-acme-dev get events --sort-by=.lastTimestamp | tail -20
```
A pod stuck at `ImagePullBackOff` usually means the pinned tag in `setup.sh` does not exist in the registry. Check the tag and rerun.

### `tenant-acme-dev` already exists
`kubectl apply` merges the tenant labels into an existing namespace, so a namespace created by hand or by another tool can end up half-owned. Check its labels first:
```bash
kubectl --context kind-platform-engineering get namespace tenant-acme-dev --show-labels
```
If it is not part of this lab, pick another tenant name in `setup.sh` and `teardown.sh`.

### Operator reports `ObjectConflict` on a child object
Manifest-mode objects already exist in the namespace, and the operator will not adopt them. Run `./teardown.sh`, then `./setup.sh` again, as described in [Operator mode](#operator-mode-optional).

### `kubectl auth can-i` returns `yes` for `list nodes`
The owners group is bound to the `admin` ClusterRole by a namespaced RoleBinding, so it cannot list cluster-scoped objects. If it can, look for a ClusterRoleBinding that grants the group:
```bash
kubectl --context kind-platform-engineering get clusterrolebindings -o json \
  | grep -n 'tenant:acme:owners' || echo "no ClusterRoleBinding for the owners group"
```

### `Tenant` CRs are skipped
The CRD is not installed, so `setup.sh` is in manifest mode. That is expected without the operator. See [tenant-operator/README.md](tenant-operator/README.md).

---

## Next Steps

1. **Read**: [Platform API design](../../docs/09-platform-apis/platform-api-design.md) for why tenants are an API object and not just a namespace convention
2. **Explore**: [`tenant-operator/`](tenant-operator/README.md) for the `Tenant` model, the remaining differences from this lab, and what the operator reconciles
3. **Lab 08**: [Platform Upgrade](../lab-08-platform-upgrade/README.md). The tenant policies in this lab must survive a platform upgrade intact

---

## Resources

- [Kubernetes Pod Security Admission](https://kubernetes.io/docs/concepts/security/pod-security-admission/)
- [Kubernetes NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- [Kubernetes ResourceQuota and LimitRange](https://kubernetes.io/docs/concepts/policy/resource-quotas/)
- [Cilium NetworkPolicy](https://docs.cilium.io/en/stable/security/policy/)
