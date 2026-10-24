# platform-core

The control plane for the platform, installed once per cluster:

- `platform-system` namespace with the Pod Security Standard `restricted` profile
- **platform-api**: Deployment, Service, PodDisruptionBudget and ServiceAccount. Its only cluster
  permissions are `impersonate` on users and groups, and `get`/`list` on namespaces. Every resource
  request runs as the caller through impersonation, so the API server's RBAC decides what the caller can do.
- **tenant-operator**, **environment-operator**, **workload-operator**: one ServiceAccount, ClusterRole,
  ClusterRoleBinding and Deployment each. Images, args and RBAC rules are values.
- CRDs in `crds/` (see [crds/README.md](crds/README.md))

## Install

```bash
helm lint --strict code/helm-charts/platform-core
helm upgrade --install platform-core code/helm-charts/platform-core \
  --namespace platform-system --create-namespace \
  --set platformApi.auth.existingSecret=platform-api-tokens \
  --set operators.tenant.image.tag=0.3.0 \
  --wait
helm test platform-core --namespace platform-system
```

Create the token Secret before installing, or platform-api rejects every request with 401:

```bash
kubectl -n platform-system create secret generic platform-api-tokens \
  --from-file=tokens=./tokens.txt   # lines: token,subject[,groups]
```

## Values

| Key | Default | Description |
| --- | ------- | ----------- |
| `controlPlane.namespace` | `platform-system` | Namespace for all control-plane objects. |
| `platformApi.enabled` | `true` | Render platform-api. |
| `platformApi.replicaCount` | `2` | Replicas. |
| `platformApi.image.repository` / `.tag` / `.pullPolicy` | `ghcr.io/gitopshub/platform-api` / `""` (AppVersion) / `IfNotPresent` | platform-api image. |
| `platformApi.containerPort` | `8080` | Listen port (`--addr`). |
| `platformApi.service.port` | `80` | Service port. |
| `platformApi.platformAdminGroup` | `platform-admins` | Group that may create and delete Tenants. |
| `platformApi.auth.existingSecret` | `""` | Secret with key `tokens` (`--tokens-file`). |
| `platformApi.cors.origins` | `""` | Allowed browser origins. |
| `platformApi.podDisruptionBudget.minAvailable` | `1` | PDB floor. |
| `operators.<name>.enabled` | `true` | `<name>` is `tenant`, `environment` or `workload`. |
| `operators.<name>.image.repository` / `.tag` | `ghcr.io/gitopshub/<name>-operator` / `""` | Operator image. |
| `operators.<name>.args` | health and metrics bind flags | Passed to the manager binary. |
| `operators.<name>.rules` | per operator | ClusterRole rules for the operator. |

The schema (`values.schema.json`) rejects invalid values, for example `service.port: 0` or an unknown operator name.

## Tests

`helm test` checks that platform-api answers `GET /healthz` through its Service.

## Notes

- Cluster-scoped names (ClusterRoles, the Namespace) are fixed. Install the chart once per cluster.
- Environment and Workload CRDs are placeholders until their operators generate real ones.
- Operator images use the names above. The tenant operator's Makefile publishes `ghcr.io/gitopshub/tenant-operator`; the others are assumed to follow the same pattern.
