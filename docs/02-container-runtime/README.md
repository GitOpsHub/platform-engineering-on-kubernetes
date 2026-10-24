# 02 — Container Runtime

What actually runs your workloads on each node, how images are described and trusted, and how
stronger isolation is added when shared-kernel containers are not enough for a multi-tenant platform.

## Reading Order

| # | Document | Summary |
|---|----------|---------|
| 1 | [Container Runtimes](./container-runtimes.md) | The CRI boundary, containerd and CRI-O internals, Docker Engine via cri-dockerd, and runtime configuration |
| 2 | [OCI Image Specification](./image-spec.md) | Index, manifest, config, and layers; digests as the unit of trust; multi-architecture images |
| 3 | [Image Security](./image-security.md) | Threats across build, registry, and deploy; signing and verification with cosign; SBOMs with syft and trivy |
| 4 | [RuntimeClass, gVisor, and Kata](./runtime-classes.md) | Stronger isolation per workload with `node.k8s.io/v1` RuntimeClass and sandboxed runtime handlers |

## Related Labs

- [Lab 01: Cluster Setup](../../labs/lab-01-cluster-setup/README.md) — the local cluster used to inspect runtimes and images
- [Lab 05: Policy Engine](../../labs/lab-05-policy-engine/README.md) — enforce signed-image and registry rules at admission
- [Lab 07: Multi-Tenancy](../../labs/lab-07-multi-tenancy/README.md) — per-tenant RuntimeClass selection
- [Lab 08: Platform Upgrade](../../labs/lab-08-platform-upgrade/) — rolling a containerd upgrade across node pools

## Related Code

- [`code/policy-engine/`](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/policy-engine/) — admission policies that consume image signatures and SBOM attestations
- [`code/workload-operator/`](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/workload-operator/) — the `Workload` API that selects a RuntimeClass per workload

## Where This Leads

- [Networking Fundamentals](../03-networking-fundamentals/README.md) — how the sandboxed and standard runtimes attach to the Pod network
- [Pod Security](../12-security-compliance/pod-security.md) — admission-time enforcement of runtime and image policy
- [Supply Chain](../12-security-compliance/supply-chain.md) — SLSA provenance and attestation flows built on the image material here

---

## Next Section

➡️ [03 — Networking Fundamentals](../03-networking-fundamentals/README.md)
