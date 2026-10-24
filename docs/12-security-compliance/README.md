# 12 - Security & Compliance

Guardrails that keep tenant workloads safe by default, and the evidence that proves it. Read in order: decide *how* pods may run, *what* may run, *which* policies enforce it, then how to *prove* compliance.

| # | Document | Summary |
|---|----------|---------|
| 1 | [Policy Engine](./policy-engine.md) | OPA Gatekeeper, Kyverno and CEL ValidatingAdmissionPolicy — validation, mutation, generation and policy testing |
| 2 | [Pod Security](./pod-security.md) | Pod Security Standards and Pod Security Admission, restricted-compliant workloads, closing gaps with CEL/Kyverno, migrating from PSP |
| 3 | [Supply Chain Security](./supply-chain.md) | SLSA build levels, Sigstore keyless signing, in-toto attestations, SBOMs, GitHub provenance and admission-time image verification |
| 4 | [Compliance](./compliance.md) | CIS benchmarks with kube-bench, Trivy Operator, Kubescape, PolicyReports, audit logging, Falco and mapping controls to SOC 2 / PCI DSS / ISO 27001 |

## Related

- Lab: [Lab 05 - Policy Engine](../../labs/lab-05-policy-engine/)
- Code: [code/policy-engine](../../code/policy-engine/)
- Previous section: [11 - Reliability](../11-reliability/README.md)
- Next section: [13 - Multi-Cluster](../13-multi-cluster/)
