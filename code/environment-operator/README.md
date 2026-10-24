# environment-operator

Reconciles namespaced `Environment` resources (`platform.mycompany.io/v1alpha1`, short name `env`) into a per-tenant namespace named `<tenant>-<name>`.

## Environment API

```yaml
apiVersion: platform.mycompany.io/v1alpha1
kind: Environment              # namespaced, plural: environments
metadata:
  name: pr-123
  namespace: platform-envs
spec:
  tenant: team-payments        # required; namespace becomes team-payments-pr-123
  type: preview                # required enum: dev | staging | prod | preview (default dev)
  ttl: 72h                     # optional; environment is deleted this long after creation
  template: baseline           # optional template set (default "baseline")
  cluster: ""                  # optional; with --cluster-name, only matching operators reconcile it
status:
  phase: Ready                 # Pending | Ready | Failed | Expired | Terminating
  namespace: team-payments-pr-123
  url: https://pr-123.team-payments.apps.example.internal
  expiresAt: "2030-01-04T12:00:00Z"   # creationTimestamp + ttl; absent when no ttl
  conditions: []               # Ready, NamespaceReady, TemplatesReady, Expired
```

Spec fields, exactly: `tenant` (string, required), `type` (enum, required), `ttl` (duration, optional), `template` (string, optional), `cluster` (string, optional).

## Reconcile behaviour

0. Verify the tenant server-side. The tenant is read from the `platform.mycompany.io/tenant` label of the Environment's own namespace, and `spec.tenant` must match it. A mismatch, a missing namespace or a missing label sets `TenantVerified=False` with reason `TenantMismatch`, and nothing is created. `config/policy/` holds an optional ValidatingAdmissionPolicy that enforces the same rule at admission time.
1. Validate the generated namespace name (DNS-1123 label, at most 63 characters, not `default`, `kube-*`, `platform-system`, or the operator's namespace). Invalid → `Failed`, reason `InvalidNamespace`, no retry.
2. Create the namespace, or adopt it when it already carries `managed-by=environment-operator`, `tenant=<tenant>` and `environment=<name>` and has no owner-uid for a different Environment. Otherwise `NamespaceReady=False` reason `NamespaceConflict` and nothing is touched.
3. Render `templates/<template>/<type>/*.yaml.tmpl` and apply the objects: a `ResourceQuota` (`env-quota`), a `NetworkPolicy` (`env-baseline`) and a `ConfigMap` (`env-config`). Each existing object that is not owned by this Environment is a conflict (`ObjectConflict`) and is left alone.
4. Set `status.url` to `https://<name>.<tenant>.<base-domain>`, set `status.expiresAt`, and requeue with `RequeueAfter` until expiry.
5. At expiry, set phase `Expired` and delete the Environment. The finalizer then deletes the namespace only if it is owned by this Environment.

## Ownership model

- Generated namespaces and child objects are labelled `platform.mycompany.io/managed-by=environment-operator`, `…/tenant`, `…/environment`, and annotated `platform.mycompany.io/owner-uid=<Environment UID>`.
- A controller reference is not used. A namespaced Environment cannot own cluster-scoped namespaces, and a cross-namespace owner reference is invalid, so the owner UID annotation stands in for it.
- Every adoption and delete checks the owner-uid annotation (`IsOwnedBy`) and the labels. Labels alone never allow a delete.

## Layout

| Path | Purpose |
|------|---------|
| `api/v1alpha1` | Environment types, markers, deepcopy, namespace/ownership helpers |
| `templates/` | `embed.go` plus `baseline/<type>/*.yaml.tmpl` (quota, network policy, configmap per type) |
| `internal/controller` | Reconciler, template rendering, unit tests (fake client) |
| `cmd/main.go` | Manager: metrics, probes, leader election |
| `config/` | CRD, RBAC, manager, samples (kustomize) |

## Usage

```bash
make test                       # generate, fmt, vet, unit tests (fake client)
make build                      # bin/manager
make install run BASE_DOMAIN=apps.example.internal
make docker-build deploy IMG=<registry>/environment-operator:dev
kubectl apply -f config/samples/platform_v1alpha1_environment.yaml
kubectl get environments -n platform-envs
```

Flags: `--operator-namespace` (default `$POD_NAMESPACE`), `--cluster-name` (default `$CLUSTER_NAME`), `--base-domain`, `--leader-elect`, `--metrics-bind-address`, `--health-probe-bind-address`.
