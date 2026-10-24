# 01 — Kubernetes Fundamentals

The Kubernetes building blocks every platform is assembled from: the control plane, the API, the controller
pattern, and the scheduling, storage, and access-control primitives your platform will wrap in higher-level
abstractions.

## Reading Order

| # | Document | Summary |
|---|----------|---------|
| 1 | [Kubernetes Architecture](./kubernetes-architecture.md) | Control-plane and node components, CRI, and a first look at the controller loop |
| 2 | [Kubernetes API Primer](./api-primer.md) | Groups, versions, resources, verbs, discovery, and client libraries |
| 3 | [Controllers and Operators](./controllers-operators.md) | Level-triggered reconciliation, informers and work queues, owner references, finalizers, and a `Tenant` controller in controller-runtime |
| 4 | [Scheduling](./scheduling.md) | Scheduling framework, affinity, taints, topology spread, priority/preemption, and scheduler profiles |
| 5 | [Storage](./storage.md) | Volumes, PV/PVC lifecycle, StorageClasses as a platform API, CSI architecture, snapshots, and expansion |
| 6 | [RBAC](./rbac.md) | Subjects, roles and bindings, aggregation, escalation prevention, impersonation, and a tenant access model |

## Related Labs

- [Lab 01: Cluster Setup](../../labs/lab-01-cluster-setup/README.md) — the local multi-node cluster used by the exercises in this section
- [Lab 03: Platform API](../../labs/lab-03-platform-api/) — build the `Tenant`/`Environment`/`Workload` controllers
- [Lab 07: Multi-Tenancy](../../labs/lab-07-multi-tenancy/) — quotas, RBAC, and placement per tenant

## Next Section

➡️ [02 — Container Runtime](../02-container-runtime/README.md)
