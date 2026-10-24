# tenant-operator

Reconciles cluster-scoped `Tenant` resources (`platform.mycompany.io/v1alpha1`) into per-namespace guardrails.

## Tenant API

```yaml
apiVersion: platform.mycompany.io/v1alpha1
kind: Tenant                      # cluster-scoped, plural: tenants
metadata:
  name: team-payments
  labels: {cost-center: "42"}     # optional; copied onto generated namespaces (reserved prefixes dropped)
  annotations: {team-contact: a@example.com}   # optional; same rules
spec:
  displayName: Payments Team      # required, 1-128 chars
  owners: [alice@mycompany.io, "group:payments-admins"]   # optional; "group:" prefix = Group
  namespaces: [payments-dev, payments-prod]               # optional; <=32 DNS-1123 labels
  quota:                          # optional; applied as a ResourceQuota per namespace
    cpu: "8"                      # -> requests.cpu / limits.cpu
    memory: 16Gi                  # -> requests.memory / limits.memory
    storage: 100Gi                # -> requests.storage
    pods: 50                      # -> pods
  networkIsolation: true          # optional; default-deny + same-tenant + DNS NetworkPolicies
status:
  phase: Ready                    # Pending | Ready | Failed | Terminating
  observedGeneration: 3
  namespaces: [payments-dev, payments-prod]   # informational; ownership is decided by controller references
  conditions: []                  # Ready, NamespacesReady, QuotaReady, NetworkPolicyReady, RBACReady
```

Spec fields, exactly: `displayName` (string, required), `owners` ([]string), `namespaces` ([]string), `quota` ({`cpu`, `memory`, `storage` strings; `pods` int}), `networkIsolation` (bool).

## What it creates

- Namespaces named exactly as in `spec.namespaces`, labelled `platform.mycompany.io/tenant=<name>` and `platform.mycompany.io/managed-by=tenant-operator`, with a controller reference to the Tenant.
- `ResourceQuota/tenant-quota` (only when `quota` is set) and `LimitRange/tenant-limits` in each namespace.
- `NetworkPolicy` set when `networkIsolation: true`: `tenant-default-deny`, `tenant-allow-same-tenant`, `tenant-allow-dns`.
- `RoleBinding/tenant-owners` binding the `admin` ClusterRole to `owners`.
- A finalizer (`platform.mycompany.io/tenant-cleanup`) that deletes the tenant's namespaces before the Tenant goes away.

## Security model

- **Namespace names** must match `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` and be at most 63 characters. Reserved names are rejected: `default`, `platform-system`, any `kube-*`, and the operator's own namespace (`--operator-namespace`, defaults to `$POD_NAMESPACE`). Checked by the webhook and the controller.
- **Adoption**: an existing namespace is adopted only if it has `managed-by=tenant-operator` AND `tenant=<name>`, or already has a controller reference to the Tenant. Otherwise the Tenant gets `NamespacesReady=False` with reason `NamespaceConflict` and nothing is changed.
- **Deletion**: before every Delete (finalizer, stale-namespace pruning, child objects) the controller checks that the object has a controller reference to the Tenant. Labels alone never authorize a delete. Child objects (quota, limit range, policies, rolebinding) with the same name but no controller reference are reported as `ObjectConflict` and left alone.
- **Label precedence**: user labels and annotations are applied first, then the operator's labels, so users cannot override ownership labels. Keys under `platform.mycompany.io/`, `pod-security.kubernetes.io/`, `kubernetes.io/`, `k8s.io/` are rejected by the webhook and silently dropped by the controller. `kubectl.kubernetes.io/*` bookkeeping is never copied to namespaces.

## Layout

| Path | Purpose |
|------|---------|
| `api/v1alpha1` | Tenant types, kubebuilder/CEL markers, deepcopy, namespace/ownership helpers |
| `cmd/main.go` | Manager: metrics, health/readiness probes, leader election, webhook server |
| `internal/controller` | Tenant reconciler + fake-client unit tests |
| `webhooks/` | Validating webhook + unit tests |
| `tests/` | envtest integration tests (skipped unless `KUBEBUILDER_ASSETS` is set) |
| `config/` | CRDs, RBAC, manager, webhook, cert-manager, samples (kustomize) |

## Usage

```bash
make test                    # manifests, generate, fmt, vet, unit + envtest tests (downloads envtest assets)
make test-unit               # unit tests only; envtest suites skip
make build                   # bin/manager
make install run             # install CRDs, run locally with webhooks disabled
make docker-build deploy IMG=<registry>/tenant-operator:dev   # needs cert-manager for webhook certs
kubectl apply -f config/samples/platform_v1alpha1_tenant.yaml
kubectl get tenants
```

Envtest: set `KUBEBUILDER_ASSETS` to a directory with `kube-apiserver` and `etcd` to run `tests/` directly, e.g.
`KUBEBUILDER_ASSETS=$(bin/setup-envtest use 1.34.x --bin-dir bin -p path) go test ./tests/`.
