# 12 - Security & Compliance

Guardrails that keep tenant workloads safe by default, and the evidence that proves it. The pages
cover which policies enforce the rules, how pods may run, what may run, and how to prove
compliance.

## Documents

| Document | What it covers |
|----------|----------------|
| [Policy Engine](./policy-engine.md) | OPA Gatekeeper, Kyverno and CEL ValidatingAdmissionPolicy: validation, mutation, generation and policy testing |
| [Pod Security](./pod-security.md) | Pod Security Standards and Pod Security Admission, restricted-compliant workloads, closing gaps with CEL/Kyverno, migrating from PSP |
| [Supply Chain Security](./supply-chain.md) | SLSA build levels, Sigstore keyless signing, in-toto attestations, SBOMs, GitHub provenance and admission-time image verification |
| [Compliance](./compliance.md) | CIS benchmarks with kube-bench, Trivy Operator, Kubescape, PolicyReports, audit logging, Falco and mapping controls to SOC 2 / PCI DSS / ISO 27001 |

## Reading Order

1. [Policy Engine](./policy-engine.md): choose the admission engine that enforces everything else.
2. [Pod Security](./pod-security.md): decide how pods may run.
3. [Supply Chain Security](./supply-chain.md): decide what may run.
4. [Compliance](./compliance.md): prove the controls work and map them to frameworks.

## Related Labs

- [Lab 05 - Policy Engine](../../labs/lab-05-policy-engine/)
- Code: [code/policy-engine](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/policy-engine/)

## Where This Leads

- Previous section: [11 - Reliability](../11-reliability/README.md)
- Next section: [13 - Multi-Cluster](../13-multi-cluster/README.md)
