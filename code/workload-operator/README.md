# workload-operator

Reconciles namespaced `Workload` resources (`platform.mycompany.io/v1alpha1`, short name `wl`) into a Deployment and a Service with the same name, plus optional trait objects.

## Workload API

```yaml
apiVersion: platform.mycompany.io/v1alpha1
kind: Workload                    # namespaced, plural: workloads
metadata:
  name: checkout-api
  namespace: team-payments-prod
spec:
  image: ghcr.io/example/checkout-api:1.4.2   # required
  replicas: 2                                 # optional, default 1; owned by the HPA when autoscaling is set
  port: 8080                                  # required, 1-65535; container port and Service port
  env: [{name: LOG_LEVEL, value: info}]       # optional, []corev1.EnvVar
  resources: {requests: {cpu: 100m}}          # optional, corev1.ResourceRequirements
  traits:                                     # optional, each type at most once
  - type: ingress                             # ingress | autoscaling | pdb
    config: {host: checkout.example.com}
  - type: autoscaling
    config: {minReplicas: 2, maxReplicas: 10, targetCPUUtilizationPercentage: 70}
  - type: pdb
    config: {minAvailable: 1}
status:
  phase: Ready                                # Pending | Progressing | Ready | Failed
  availableReplicas: 2                        # copied from the Deployment
  url: https://checkout.example.com           # set when the ingress trait is present
  observedGeneration: 1
  conditions: []                              # Ready, DeploymentReady, TraitsApplied
```

Spec fields, exactly: `image` (string, required), `replicas` (int32, optional), `port` (int32, required), `env` ([]EnvVar, optional), `resources` (ResourceRequirements, optional), `traits` ([]{`type` enum, `config` JSON object, optional}).

## Generated objects

| Object | apiVersion | Name | When |
|--------|-----------|------|------|
| Deployment | apps/v1 | `<name>` | always |
| Service (ClusterIP) | v1 | `<name>` | always |
| Ingress | networking.k8s.io/v1 | `<name>` | `ingress` trait |
| HorizontalPodAutoscaler | autoscaling/v2 | `<name>` | `autoscaling` trait |
| PodDisruptionBudget | policy/v1 | `<name>` | `pdb` trait |

Trait configs are decoded strictly: unknown fields are rejected.

- `ingress`: `host` (required, DNS name), `path` (default `/`), `className`, `tlsSecretName`.
- `autoscaling`: `maxReplicas` (required), `minReplicas` (default 1), `targetCPUUtilizationPercentage` (default 80, 1-100). The Deployment's replica count is then owned by the HPA.
- `pdb`: `minAvailable` or `maxUnavailable` (int or percentage). Defaults to `minAvailable: 1`. Set at most one.

Labels on generated objects: `app.kubernetes.io/name`, `platform.mycompany.io/workload=<name>`, `platform.mycompany.io/managed-by=workload-operator`, plus `platform.mycompany.io/tenant` and `platform.mycompany.io/environment` copied from the Workload's labels when present.

## Behaviour

- Every generated object has a controller reference to the Workload (same namespace), so deleting the Workload garbage-collects its children.
- Spec problems (bad trait config, duplicate or unknown trait, port out of range) are validated before anything is written. The Workload goes to `Failed` with a reason, and the operator does not retry until the spec changes.
- An existing Deployment, Service or trait object without a controller reference to the Workload is a conflict (`ObjectConflict`). It is never overwritten or deleted.
- A trait that is removed from the spec has its object deleted, but only if the Workload controls it.
- Status comes from the live Deployment. `Ready` means available replicas meet the desired count. `Failed` means the Deployment exceeded its progress deadline.
- The controller watches its children with `Owns()`, so status updates come from child changes without polling.

## Layout

| Path | Purpose |
|------|---------|
| `api/v1alpha1` | Workload types, markers, deepcopy |
| `traits/` | `Trait` interface, registry, and the ingress, autoscaling and pdb implementations, with unit tests |
| `internal/controller` | Reconciler (Deployment, Service, traits, status), fake-client unit tests |
| `cmd/main.go` | Manager: metrics, probes, leader election |
| `config/` | CRD, RBAC, manager, samples (kustomize) |

## Usage

```bash
make test                    # generate, fmt, vet, unit tests (fake client)
make build                   # bin/manager
make install run            # install CRD, run against ~/.kube/config
make docker-build deploy IMG=<registry>/workload-operator:dev
kubectl apply -f config/samples/platform_v1alpha1_workload.yaml
kubectl get workloads -n team-payments-prod
```
