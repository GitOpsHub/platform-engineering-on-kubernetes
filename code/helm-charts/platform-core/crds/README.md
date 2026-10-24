# CRDs

These files are the CustomResourceDefinitions the platform-core operators need.

| File | Source | Status |
| ---- | ------ | ------ |
| `platform.mycompany.io_tenants.yaml` | `tenant-operator/config/crd/bases/` (controller-gen) | Copied from the operator. Cluster-scoped. |
| `platform.mycompany.io_environments.yaml` | none yet | **Placeholder.** Namespaced, `x-kubernetes-preserve-unknown-fields`. |
| `platform.mycompany.io_workloads.yaml` | none yet | **Placeholder.** Namespaced, `x-kubernetes-preserve-unknown-fields`. |

## Keeping them in sync

The operators own their CRDs. Do not edit the copies here by hand. Regenerate them in the operator and run:

```bash
make -C code/helm-charts sync-crds
```

`sync-crds` copies every `*.yaml` in each operator's `config/crd/bases/`. An operator without generated
CRDs keeps its placeholder. Once `environment-operator` or `workload-operator` generates a CRD, the next sync replaces the placeholder.

## Helm behaviour

Helm installs the `crds/` directory on the **first install only**. It never upgrades or deletes these
CRDs. On upgrades, apply them yourself, preferably with server-side apply:

```bash
kubectl apply --server-side -f code/helm-charts/platform-core/crds/
```

Deleting the chart does not remove the CRDs, so deleting Tenants, Environments or Workloads needs a separate step.
