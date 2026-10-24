# 04 — Infrastructure as Code

How a platform describes and provisions what it runs on: Terraform for cloud and cluster foundations,
Crossplane for Kubernetes-native platform APIs, the GitOps tools that keep both in step, and drift
detection to catch what changes outside the declared state.

## Reading Order

| # | Document | Summary |
|---|----------|---------|
| 1 | [Terraform for Kubernetes Platforms](./terraform-kubernetes.md) | Where Terraform fits, the `kubernetes` and `helm` providers, and splitting state into layers to avoid provider-ordering traps |
| 2 | [Crossplane](./crossplane.md) | Core, providers, managed resources, XRDs as platform APIs, and Pipeline-mode Compositions with functions |
| 3 | [GitOps Tool Comparison](./gitops-tool-comparison.md) | Argo CD, Flux, and Rancher Fleet side by side, with the same deployment expressed in each |
| 4 | [Drift Detection and Remediation](./drift-detection.md) | What each tool can and cannot see, legitimate versus real drift, and how to configure ignore rules |

## Related Labs

- [Lab 02: GitOps Bootstrap](../../labs/lab-02-gitops-bootstrap/README.md) — bootstrap Argo CD or Flux and reconcile the sample applications
- [Lab 03: Platform API](../../labs/lab-03-platform-api/README.md) — define and reconcile a platform API with XRDs and controllers
- [Lab 05: Policy Engine](../../labs/lab-05-policy-engine/README.md) — policy checks that catch drifted or non-compliant resources
- [Lab 07: Multi-Tenancy](../../labs/lab-07-multi-tenancy/README.md) — per-tenant composition and quota resources

## Related Code

- [`code/terraform-modules/`](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/terraform-modules/) — EKS, GKE, AKS, networking, and addon modules
- [`code/crossplane-compositions/`](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/crossplane-compositions/) — XRDs, Compositions, functions, and example claims

## Where This Leads

- [GitOps Principles](../05-gitops/gitops-principles.md) — the reconciliation model that Terraform and Crossplane are often paired with
- [Secrets Management](../06-secrets-management/external-secrets.md) — getting credentials into provisioned resources without storing them in state
- [Internal Developer Platform](../07-internal-developer-platform/README.md) — exposing these platform APIs to developers

---

## Next Section

➡️ [05 — GitOps](../05-gitops/README.md)
