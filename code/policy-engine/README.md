# policy-engine

A Kubernetes ValidatingWebhook that enforces platform policy on `Tenant`,
`Environment` and `Workload` objects. The policies are Rego (v1 syntax), evaluated
in-process with OPA's Go API. The webhook fails closed: if policy evaluation fails or
the engine is unreachable, the change is rejected.

## Policies

All rules live in `policies/` and add to the set `data.platform.deny`.

| File | Policy | Applies to | Rule |
|---|---|---|---|
| `labels.rego` | `required-labels` | Tenant, Environment, Workload | `platform.mycompany.io/owner` must be set; Workloads also need `app.kubernetes.io/name` |
| `registry.rego` | `registry-allowlist` | Workload | `spec.image` must start with an allowed registry prefix |
| `limits.rego` | `resource-limits` | Workload | `spec.resources.limits` must set `cpu` and `memory`; `spec.replicas` must not exceed the ceiling |
| `privileged.rego` | `no-privileged` | any | No `privileged: true` anywhere in the object |

Each `*.rego` has a matching `*_test.rego` with positive and negative cases.

## Layout

| Path | Purpose |
|---|---|
| `engine.go` | Compiles the policies and evaluates an object, returning sorted violations |
| `policies/` | Rego policies and tests. `embed.go` embeds the policies into the binary |
| `admission/` | AdmissionReview (`admission.k8s.io/v1`) handler and TLS certificate reloader |
| `metrics/` | Prometheus metrics: decisions, violations, evaluation latency |
| `cmd/policy-engine/` | Webhook entry point. Serves `/validate` on HTTPS and `/metrics` on HTTP |
| `deploy/` | Deployment, Service, ValidatingWebhookConfiguration, cert-manager Certificate |

## How it runs

- `POST /validate` on `:8443` (HTTPS). The API server calls this for CREATE and UPDATE on
  `platform.mycompany.io` `tenants`, `environments` and `workloads`. DELETE is always allowed.
- `GET /metrics` and `GET /healthz` on `:8080` (plain HTTP, cluster-internal only).
- Certificates are re-read from disk when cert-manager rotates them. A failed reload keeps the
  last good certificate in service.

Metrics: `policy_engine_decisions_total{kind,decision}`,
`policy_engine_violations_total{policy}`, `policy_engine_evaluation_duration_seconds{kind}`.

## Configuration

| Flag | Default | Meaning |
|---|---|---|
| `--addr` | `:8443` | HTTPS listen address |
| `--metrics-addr` | `:8080` | Metrics and health address |
| `--tls-cert` / `--tls-key` | `/tls/tls.crt`, `/tls/tls.key` | Serving certificate |
| `--allowed-registries` | `ghcr.io/acme/` | Comma-separated image prefixes |
| `--max-replicas` | `10` | Workload replica ceiling |
| `--log-level` | `info` | `debug`, `info`, `warn`, `error` |

## Develop

```sh
make test        # Go tests with -race. This includes the Rego suite via OPA's tester.
make opa-test    # the same Rego suite via the opa CLI, if it is installed
make build
```

To try it locally, generate a self-signed certificate under `certs/`, run
`make run`, then POST an AdmissionReview to `https://localhost:8443/validate`.

## Deploy

1. Install cert-manager and create a `ClusterIssuer` named `platform-ca`.
2. Apply `deploy/`. The Certificate creates `policy-engine-tls`, and cert-manager injects the CA bundle
   into the webhook configuration.
3. Add a rule that exempts namespaces you do not want evaluated. The shipped config
   already skips `kube-system` and `policy-system`.

## Notes

- The Rego rule for privileged containers walks the whole object. It is generic on purpose,
  because the Workload contract has no fixed container list.
- The Deployment pins `ghcr.io/gitopshub/policy-engine:v0.1.0`. That tag does not exist yet: build and push it (`make docker-build TAG=v0.1.0`) before applying `deploy/`. Bump the tag with each release and never use `:latest`.
