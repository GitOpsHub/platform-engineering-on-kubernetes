# 11 - Reliability

Reliability engineering for the platform and the workloads running on it: define what "reliable enough" means, prove it under failure, recover when things go badly wrong, and keep enough capacity to meet demand.

## Documents

| Document | What it covers |
|----------|----------------|
| [SLOs, SLIs and Error Budgets](slos-slis.md) | Choosing SLIs, error budget math, multi-window multi-burn-rate alerts, Sloth/Pyrra, error budget policies and platform-owned SLOs. |
| [Chaos Engineering](chaos-engineering.md) | Hypothesis-driven experiments with Chaos Mesh and LitmusChaos, Gremlin overview, GameDays and self-service chaos guardrails. |
| [Disaster Recovery](disaster-recovery.md) | RPO/RTO, DR strategies from backup/restore to active-active, Velero backups and restores, etcd snapshots and DR drills. |
| [Capacity Planning](capacity-planning.md) | Requests/limits, HPA, KEDA, VPA, Cluster Autoscaler vs Karpenter, PDBs, headroom, quotas and forecasting. |

## Reading Order

1. [SLOs, SLIs and Error Budgets](slos-slis.md)
2. [Chaos Engineering](chaos-engineering.md)
3. [Disaster Recovery](disaster-recovery.md)
4. [Capacity Planning](capacity-planning.md)

## Related Labs

- [Lab 06: Observability Stack](../../labs/lab-06-observability-stack/)
- [Lab 08: Platform Upgrade](../../labs/lab-08-platform-upgrade/)

## Where This Leads

- Observability foundations: [10 - Observability](../10-observability/README.md)
- Security & compliance: [12 - Security & Compliance](../12-security-compliance/README.md)
