# crossplane-compositions

Platform APIs built with Crossplane: a `Database` (AWS RDS PostgreSQL) and an `Environment`
(namespace plus resource quota). Developers request these with a claim. The platform team owns
the XRDs, Compositions, Functions and Providers.

```
crossplane-compositions/
├── providers/       Provider + ProviderConfig (provider-aws-rds, provider-kubernetes)
├── functions/       Function packages (function-patch-and-transform, function-auto-ready)
├── xrd/             CompositeResourceDefinitions (platform.mycompany.io)
├── compositions/    Pipeline-mode Compositions
└── claims/          Example developer claims
```

## Versions

| Component | Version | Notes |
| --------- | ------- | ----- |
| Crossplane | 2.0.x (chart `crossplane` 2.0.2) | XRDs here use the `apiextensions.crossplane.io/v1` API, which 2.x still serves. |
| function-patch-and-transform | `v0.8.2` | Same tag as `docs/04-infrastructure-as-code/crossplane.md`. |
| function-auto-ready | `v0.5.0` | Same tag as the docs. |
| provider-aws-rds | `v2.0.0` | **Verify the tag on marketplace.upbound.io before applying.** |
| provider-kubernetes | `v0.16.0` | **Verify the tag on github.com/crossplane-contrib/provider-kubernetes.** |

All compositions use `mode: Pipeline`. Resources-mode compositions are not used anywhere in this repo.

## Install

Prerequisites: a Kubernetes 1.28+ cluster, `helm`, `kubectl`, and the `crossplane` CLI.

```bash
# 1. Crossplane core (pin the chart version in production, and install it through GitOps).
helm repo add crossplane-stable https://charts.crossplane.io/stable
helm repo update
helm install crossplane crossplane-stable/crossplane \
  --version 2.0.2 \
  --namespace crossplane-system --create-namespace --wait

# 2. Functions and providers.
kubectl apply -f functions/functions.yaml
kubectl apply -f providers/providers.yaml
kubectl apply -f providers/environment-provisioner.yaml

# 2a. Kubeconfig for the environment-provisioner identity (namespaces and resourcequotas only).
#     Write a kubeconfig whose server and CA match your cluster, and whose user token comes from:
TOKEN=$(kubectl -n crossplane-system create token environment-provisioner --duration=24h)
#     Put the token into that kubeconfig, then store it as the Secret the ProviderConfig reads:
kubectl -n crossplane-system create secret generic environment-provisioner-kubeconfig \
  --from-file=kubeconfig=./environment-provisioner.kubeconfig
# Tokens expire. Rotate the Secret on a schedule, or switch to a long-lived token Secret you manage.

# 3. Wait until everything is healthy.
kubectl wait --for=condition=Healthy function --all --timeout=5m
kubectl wait --for=condition=Healthy provider --all --timeout=10m

# 4. XRDs, then Compositions.
kubectl apply -f xrd/
kubectl wait --for=condition=Established xrd --all --timeout=2m
kubectl apply -f compositions/
```

### Credentials

- **provider-aws-rds** uses `WebIdentity` (IRSA on EKS). Attach an IAM role to the provider's
  ServiceAccount with a `DeploymentRuntimeConfig` and let the role manage only RDS resources in the
  allowed regions. The `ProviderConfig` in `providers/providers.yaml` does not contain any static keys.
- **provider-kubernetes** uses `InjectedIdentity`, which is the Crossplane pod's own identity. Bind
  its ServiceAccount to a ClusterRole that allows `create/get/update/patch/delete` on the kinds the
  compositions produce (`namespaces`, `resourcequotas`). Nothing broader.

### Tenant admission

`policies/tenant-namespace-match.yaml` is a Kyverno policy. It denies an Environment claim whose
`spec.parameters.tenant` does not match the `platform.mycompany.io/tenant` label on the claim's namespace.
Install Kyverno 1.11 or later first. The policy is not applied by the install steps above.

## Render compositions locally

`crossplane render` runs the Pipeline with the real functions in Docker, without a cluster. Use it
in CI and for code review.

```bash
# Requires Docker. crossplane render takes a composite resource (XR), not a claim.
crossplane render claims/xenvironment.yaml compositions/environment-kubernetes.yaml functions/functions.yaml \
  --include-function-results

crossplane render claims/xdatabase.yaml compositions/database-aws.yaml functions/functions.yaml \
  --include-function-results
```

The XR files in `claims/` (`xdatabase.yaml`, `xenvironment.yaml`) are the composite form of the
claims in `claims.yaml`. Use `--extra-resources=<dir>` if a pipeline step reads existing objects.

Validate the XRDs against a live cluster with `kubectl apply --dry-run=server -f xrd/`.

## Debug a live claim

```bash
kubectl -n team-orders get database orders-db -o yaml          # claim conditions and events
kubectl get xdatabase.platform.mycompany.io                    # the composite
crossplane beta trace database orders-db -n team-orders        # full resource tree and status
```

## Platform API conventions

- Group `platform.mycompany.io`, version `v1alpha1`.
- Composed Kubernetes objects carry `platform.mycompany.io/managed-by: crossplane` and `platform.mycompany.io/component` labels. The RDS instance is labelled through its composite's tenant label when one is set.
- Deletion policy: `Orphan` for RDS (databases are never dropped by accident) and `Delete` for Kubernetes objects.
