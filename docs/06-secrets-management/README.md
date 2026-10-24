# 06 — Secrets Management

How credentials reach workloads without being stored in Git or baked into images: a central secret
store synchronized into Kubernetes, direct injection from Vault, encrypted secrets that are safe to
commit, and automated rotation that keeps running workloads current.

## Reading Order

| # | Document | Summary |
|---|----------|---------|
| 1 | [External Secrets Operator](./external-secrets.md) | `ClusterSecretStore` and `ExternalSecret` against Vault and cloud secret managers, templating, PushSecret, and refresh behavior |
| 2 | [Vault Integration](./vault-integration.md) | Vault Agent Injector, the Vault CSI provider, and Kubernetes auth roles that bind service accounts to policies |
| 3 | [Sealed Secrets](./sealed-secrets.md) | Asymmetric encryption of Secrets for Git, controller key management, scopes, and rotating the sealing key |
| 4 | [Secrets Rotation](./secrets-rotation.md) | Rotation patterns: ESO refresh intervals, Vault dynamic secrets and leases, Reloader for restarts, and cert-manager for certificates |

## Related Labs

- [Lab 02: GitOps Bootstrap](../../labs/lab-02-gitops-bootstrap/README.md) — the GitOps flow that delivers ExternalSecret and SealedSecret manifests
- [Lab 05: Policy Engine](../../labs/lab-05-policy-engine/README.md) — policies that reject plaintext Secrets in Git-managed namespaces
- [Lab 07: Multi-Tenancy](../../labs/lab-07-multi-tenancy/README.md) — per-tenant secret stores and Vault roles

## Related Code

- [`code/platform-api/`](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/platform-api/) — the platform API that provisions secret stores for new environments
- [`code/helm-charts/`](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/helm-charts/) — chart values that wire service accounts to Vault roles

## Where This Leads

- [GitOps Principles](../05-gitops/gitops-principles.md) — why secret values must never appear in the repository
- [Pod Security](../12-security-compliance/pod-security.md) — restricting what a workload can read from mounted secrets
- [Supply Chain](../12-security-compliance/supply-chain.md) — signing and attestation for the artifacts that carry configuration

---

## Next Section

➡️ [07 — Internal Developer Platform](../07-internal-developer-platform/README.md)
