# 03 — Networking Fundamentals

How traffic moves in and around a Kubernetes platform: Pod networking, getting traffic into the cluster,
segmenting it, and securing service-to-service communication.

## Reading Order

| # | Document | Summary |
|---|----------|---------|
| 1 | [CNI Overview](cni-overview.md) | The Kubernetes network model, the CNI spec, and how Flannel, Calico, and Cilium implement Pod networking |
| 2 | [Ingress Controllers & Gateway API](ingress-controllers.md) | North-south traffic with Ingress, popular controllers, and the role-oriented Gateway API v1 |
| 3 | [Network Policies](network-policies.md) | NetworkPolicy semantics, a default-deny tenant baseline, and CiliumNetworkPolicy L7/FQDN rules |
| 4 | [Service Mesh](service-mesh.md) | mTLS, identity-based authorization, and traffic management with Istio ambient, Linkerd, and Cilium |

## Related Labs

- [Lab 01: Cluster Setup](../../labs/lab-01-cluster-setup/README.md) — kind cluster with Cilium and Hubble
- [Lab 07: Multi-Tenancy](../../labs/lab-07-multi-tenancy/) — network isolation per tenant

## Where This Leads

- [Multi-Cluster Networking](../13-multi-cluster/multi-cluster-networking.md)
- [API Gateway](../09-platform-apis/api-gateway.md)
- [Policy Engine](../12-security-compliance/policy-engine.md)

Previous section: [02 — Container Runtime](../02-container-runtime/README.md) ·
Next section: [04 — Infrastructure as Code](../04-infrastructure-as-code/README.md)
