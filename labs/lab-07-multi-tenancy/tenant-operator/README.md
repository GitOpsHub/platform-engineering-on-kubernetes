# Tenant Operator (optional)

The operator source lives in [`code/tenant-operator`](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/tenant-operator/). This directory holds only the sample `Tenant` objects the lab applies.

**The lab works without the operator.** [`setup.sh`](../setup.sh) creates every tenant object with plain manifests. When the operator is installed, `setup.sh` applies the same tenants as `Tenant` CRs and lets the operator create the child objects. The two modes are described below.

## The Tenant model

`Tenant` is a cluster-scoped custom resource in the group `platform.mycompany.io`, version `v1alpha1`, short name `tn`. Its source of truth is [`tenant_types.go`](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/tenant-operator/api/v1alpha1/tenant_types.go).

```yaml
apiVersion: platform.mycompany.io/v1alpha1
kind: Tenant
metadata:
  name: acme
spec:
  displayName: Acme Corp        # required, 1-128 chars
  owners:                       # users, or groups prefixed "group:"
    - alice@example.com
    - group:tenant:acme:owners  # the lab's owner group
  namespaces:                   # DNS labels, max 32, no default/kube-* names
    - tenant-acme-dev
  quota:                        # applied as a ResourceQuota in each namespace
    cpu: "4"
    memory: 8Gi
    storage: 20Gi
    pods: 20
  networkIsolation: true        # installs default-deny NetworkPolicies
```

The sample files are [`tenant-acme.yaml`](tenant-acme.yaml) and [`tenant-globex.yaml`](tenant-globex.yaml). The operator reports progress in `status.phase` (`Pending`, `Ready`, `Failed`, `Terminating`) and in conditions for namespaces, quota, network policy and RBAC.

## What the operator creates, and how it matches the lab

| Object | Name | Lab equivalent | Status |
|--------|------|----------------|--------|
| Namespace per entry in `spec.namespaces` | as listed | [`namespace-templates/namespace.yaml`](../namespace-templates/namespace.yaml) | Matches: same tenant and `managed-by` labels, so the operator adopts a lab-created namespace |
| ResourceQuota | `tenant-quota` | [`resource-quotas/quota.yaml`](../resource-quotas/quota.yaml) | Matches: keys `requests/limits.cpu`, `requests/limits.memory`, `requests.storage`, `pods` |
| LimitRange | `tenant-limits` | same file | Matches: default and defaultRequest only |
| NetworkPolicy | `tenant-default-deny`, `tenant-allow-same-tenant`, `tenant-allow-dns` | [`network-isolation/network-policies.yaml`](../network-isolation/network-policies.yaml) | Matches: same names and scopes, DNS allows all of `kube-system` |
| RoleBinding | `tenant-owners` (to ClusterRole `admin`) | [`rbac/tenant-owner.yaml`](../rbac/tenant-owner.yaml) | Matches: same name, role and group |

The lab adds one object the operator does not create: `tenant-allow-ingress-from-traefik`, which lets the Lab 01 ingress controller reach tenant pods. It is additive and has a different name.

## Two modes, and why they cannot share one namespace

`setup.sh` picks a mode from whether the `tenants.platform.mycompany.io` CRD exists.

- **Manifest mode (no operator).** `setup.sh` applies the namespace, quota, LimitRange, network policies and RoleBinding directly.
- **Operator mode.** `setup.sh` applies only the `Tenant` CRs. It then waits for the operator to create the namespace and for `status.phase` to be `Ready`, and applies the demo app.

**Do not mix the two in one namespace.** The operator adopts a namespace that carries its labels, but it never adopts the child objects. If a ResourceQuota, LimitRange, NetworkPolicy or RoleBinding with a matching name already exists without a controller reference to the Tenant, the operator reports `ObjectConflict` and does not change it. So to move a tenant from manifest mode to operator mode, run `teardown.sh` first, then `setup.sh`.

## Differences that remain

These are the differences that are left after the lab's manifests were aligned with the operator:

1. **Admin is broad.** The operator's owner binding grants the built-in `admin` ClusterRole, which includes creating RoleBindings within the namespace. The lab keeps that behaviour to match the operator. A narrower Role is a stricter alternative, but it would not match the operator, so the two modes would then grant different access.
2. **DNS scope is broad.** The operator's `tenant-allow-dns` allows egress to every pod in `kube-system`. A tighter rule would select only `k8s-app=kube-dns` pods. The lab uses the operator's rule. To tighten it, change both this lab and the operator together.
3. **PSA labels come only from the lab.** The operator does not set `pod-security.kubernetes.io/*` labels, and it filters those keys out of `Tenant.metadata.labels` (`FilterUserMetadata`), so a Tenant CR cannot add them either. In operator mode the tenant namespaces are therefore not PSA-restricted. The demo pods still meet the restricted profile, but nothing enforces it at the namespace level.
4. **Traefik ingress policy is lab-only.** `tenant-allow-ingress-from-traefik` is not created by the operator. It stays in place in both modes.
5. **Not tested end to end.** This lab was checked by reading the operator code and rendering the manifests. The operator itself was not built or run against a cluster in this lab.

## Using the operator (optional)

The operator is not installed by Lab 01, and this lab does not install it. To try it, build and deploy it from `code/tenant-operator` (see its [README](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/tenant-operator/README.md) and `config/`), then run `teardown.sh` if you have already run the lab in manifest mode, and run `setup.sh` again. The
quickest route is `make install run` from `code/tenant-operator`, as described in the
[Lab 07 operator mode section](../README.md#operator-mode-optional).

The operator's CRD is generated at [`config/crd/bases/platform.mycompany.io_tenants.yaml`](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/tenant-operator/config/crd/bases/platform.mycompany.io_tenants.yaml). Its `spec` fields match the sample files in this directory.

## Further reading

- [Platform API design](../../../docs/09-platform-apis/platform-api-design.md)
- [Lab 07 README](../README.md)
