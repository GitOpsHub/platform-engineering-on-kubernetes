# tenant-operator

Reconciles cluster-scoped `Tenant` resources (`platform.mycompany.io/v1alpha1`) into:

- labelled Namespaces (`platform.mycompany.io/tenant`, `platform.mycompany.io/managed-by`) owned by the Tenant
- a `ResourceQuota` (`tenant-quota`) and `LimitRange` (`tenant-limits`) in each namespace
- NetworkPolicies when `spec.networkIsolation: true`: default-deny, allow-same-tenant, allow-DNS
- a `RoleBinding` (`tenant-owners`) granting the `admin` ClusterRole to `spec.owners` (`group:` prefix means a Group)
- a finalizer that deletes tenant namespaces when the Tenant is deleted; Ready/NamespacesReady/QuotaReady/NetworkPolicyReady/RBACReady conditions

A validating webhook (`webhooks/`) rejects invalid quotas and namespace collisions with other tenants or with unmanaged namespaces.

## Layout

| Path | Purpose |
|------|---------|
| `api/v1alpha1` | Tenant types, kubebuilder/CEL markers, generated deepcopy |
| `controllers/` | Tenant reconciler + fake-client unit tests |
| `webhooks/` | Validating webhook + unit tests |
| `tests/` | envtest integration tests (skipped when `KUBEBUILDER_ASSETS` is unset) |
| `config/` | CRDs, RBAC, manager, webhook, cert-manager, samples (kustomize) |

## Usage

```bash
make test                 # manifests, generate, fmt, vet, unit + envtest tests
make install run          # install CRDs, run locally (webhooks disabled)
make docker-build deploy IMG=<registry>/tenant-operator:dev   # needs cert-manager
kubectl apply -k config/samples/
kubectl get tenants
```
