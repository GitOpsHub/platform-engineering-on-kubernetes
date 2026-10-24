# 05 — GitOps

Git as the source of truth for cluster state, and the controllers that reconcile clusters to match it.
This section covers the principles, the two leading reconcilers (Argo CD and Flux), and how the same
model extends from one cluster to a fleet.

## Reading Order

| # | Document | Summary |
|---|----------|---------|
| 1 | [GitOps Principles and Patterns](./gitops-principles.md) | The four principles, push versus pull, secrets in GitOps, drift remediation, and the shared troubleshooting model |
| 2 | [Argo CD Deep Dive](./argocd-deep-dive.md) | Applications, projects, App of Apps, ApplicationSets, RBAC, health assessment, and metrics |
| 3 | [Flux Deep Dive](./flux-deep-dive.md) | Sources, Kustomizations, HelmReleases, image automation, multi-tenancy, and notifications |
| 4 | [Multi-Cluster GitOps](./multi-cluster-gitops.md) | Hub-and-spoke versus per-cluster topologies, repository layout, cluster registration, and progressive rollout |

## Related Labs

- [Lab 02: GitOps Bootstrap](../../labs/lab-02-gitops-bootstrap/README.md) — bootstrap Argo CD with App of Apps and reconcile sample applications
- [Lab 07: Multi-Tenancy](../../labs/lab-07-multi-tenancy/README.md) — tenant-scoped projects and namespaces managed through Git
- [Lab 08: Platform Upgrade](../../labs/lab-08-platform-upgrade/) — upgrading platform components through Git-driven rollouts

## Related Code

- [`code/helm-charts/`](../../code/helm-charts/) — the platform charts that GitOps tools deploy
- [`code/tenant-operator/`](../../code/tenant-operator/) — the `Tenant` resources that GitOps repositories create per team

## Where This Leads

- [GitOps Tool Comparison](../04-infrastructure-as-code/gitops-tool-comparison.md) — Argo CD, Flux, and Fleet side by side
- [Drift Detection](../04-infrastructure-as-code/drift-detection.md) — what reconcilers can and cannot see
- [Secrets Management](../06-secrets-management/external-secrets.md) — keeping secret values out of Git
- [Multi-Cluster Networking](../13-multi-cluster/multi-cluster-networking.md) — connectivity between the clusters a fleet manages

---

## Next Section

➡️ [06 — Secrets Management](../06-secrets-management/README.md)
