# platform-api

REST front door for the Internal Developer Platform. It serves the
`platform.mycompany.io/v1alpha1` resources reconciled by the operators in this
repository (`Tenant`, `Environment`, `Workload`) as a small JSON API, so portals
and CLIs never need direct Kubernetes access.

- `GET /api/v1alpha1/platform`, `/version`, `/healthz`, `/readyz`, `/openapi.yaml`, `/metrics`
- Tenants (cluster-scoped): `/api/v1alpha1/tenants[/{name}]`
- Environments and Workloads (namespaced): `/api/v1alpha1/[namespaces/{ns}/]{environments|workloads}[/{name}]`
- Full contract: [openapi.yaml](openapi.yaml)

## Security model

platform-api does **not** use its own identity to touch platform resources.

1. **Authentication.** A bearer token is verified (static tokens file, or an OIDC
   verifier via the `middleware.TokenVerifier` seam). Requests without a valid
   token get `401`.
2. **Caller-scoped clients.** For each request, `handlers.Options.ClientFor` builds a
   dynamic client that impersonates the caller. Before it does, it:
   - refuses a principal whose subject or any group starts with `system:` (403, no client is built)
   - drops any group that is not on the `--impersonate-groups` allowlist, and never forwards IdP groups verbatim
   - writes an audit log line (`"audit": true`) for every impersonation and every refusal

   Cluster RBAC is then evaluated for the real user. Requests with no principal never
   reach Kubernetes. The chart scopes the `impersonate` grant to the same allowlist; see
   [the chart README](deploy/chart/platform-api/README.md) for the scope and the one
   residual broad permission (`impersonate` on `users`).
3. **Platform-admin gate.** Creating and deleting Tenants requires membership of the
   platform-admin group (`--platform-admin-group`, default `platform-admins`).
   Anyone else gets `403`.
4. **Least-privilege ServiceAccount.** The chart's ClusterRole grants only
   `impersonate` on `users` and `groups` in the core API group. It grants nothing on
   `platform.mycompany.io` resources. Be aware that `impersonate` lets the server act
   as any subject it has authenticated, so protect the server's own token and the
   tokens Secret.

### Dev-only escape hatch

`--insecure-skip-authz` (default **off**, not read from the environment) lets
unauthenticated requests run with platform-api's own credentials and disables the
admin check. A warning is logged at startup while it is enabled. Never enable it in a
shared cluster.

### Live updates and "Can I?"

- `GET <collection>?watch=true` streams changes as newline-delimited JSON. The stream is
  opened as the caller, so it only contains what RBAC lets that caller see.
- `POST /api/v1alpha1/access-reviews` answers "can the caller do this?" by creating a
  `SelfSubjectAccessReview` with the caller's impersonated client.

## Layout

| Path | Purpose |
|---|---|
| `main.go` | Flags, config, server wiring, impersonating client factory |
| `handlers/` | Route table, auth guards, generic resource handlers |
| `middleware/` | Auth, request ID, logging, panic recovery, CORS, metrics, body validation |
| `types/` | Wire types and validation, no Kubernetes dependencies |
| `client/` | Typed Go SDK over HTTP |
| `deploy/chart/platform-api/` | Helm chart (Deployment, Service, PDB, impersonation RBAC, ServiceMonitor) |
| `openapi.yaml` | API description, embedded in the binary and served at `/openapi.yaml` |

## Configuration

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `--addr` | `PLATFORM_API_ADDR` | `:8080` | Listen address |
| `--kubeconfig` | `KUBECONFIG` | in-cluster | Kubeconfig path |
| `--tokens-file` | `PLATFORM_API_TOKENS_FILE` | empty | `token,subject[,group1;group2]` lines. Empty means every resource request gets 401 |
| `--platform-admin-group` | `PLATFORM_API_ADMIN_GROUP` | `platform-admins` | Group required for Tenant create and delete |
| `--impersonate-groups` | `PLATFORM_API_IMPERSONATE_GROUPS` | `platform-admins` | Allowlist of groups that may be impersonated. `system:*` is refused at start-up. The admin group is always added |
| `--insecure-skip-authz` | none | `false` | DEV ONLY, see above |
| `--cors-origins` | `PLATFORM_API_CORS_ORIGINS` | empty | Exact allowed origins. `*` never sends credentials |
| `--cors-credentials` | none | `false` | Allow credentials for explicit origins |
| `--profiling` | none | `false` | pprof on `/debug/pprof/`. Do not expose publicly |
| `--metrics` | none | `true` | Prometheus metrics on `/metrics` |
| `--log-level` | `PLATFORM_API_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

## Develop

```sh
make help          # list targets
make test          # go test -race with coverage
make lint          # go vet (+ golangci-lint when installed)
make run           # local run against ~/.kube/config, with --insecure-skip-authz
make docker-build  # distroless image
make helm-lint helm-template
```

## Deploy

```sh
kubectl create secret generic platform-api-tokens --from-file=tokens=tokens.csv -n platform-system
helm upgrade --install platform-api deploy/chart/platform-api -n platform-system \
  --create-namespace --set auth.existingSecret=platform-api-tokens
```

Callers must be granted RBAC on the CRDs, for example a `RoleBinding` for a team
group. Platform admins also need membership of `platform-admins`, or the value set
by `platformAdminGroup`.

## Known limits

- Static tokens are for labs. Production should wire an OIDC verifier that
  implements `middleware.TokenVerifier`.
- A new dynamic client is built per request, so connection reuse depends on the
  client-go transport cache. Measure before optimizing.
