# Platform Engineering on Kubernetes - Learning Resource Plan

## Overview
Create a comprehensive learning platform for Platform Engineering concepts, patterns, and implementations on Kubernetes.

## Directory Structure

```
platform-engineering-on-kubernetes/
├── README.md
├── CONTRIBUTING.md
├── LICENSE
├── docs/
│   ├── 01-kubernetes-fundamentals/
│   ├── 02-container-runtime/
│   ├── 03-networking-fundamentals/
│   ├── 04-infrastructure-as-code/
│   ├── 05-gitops/
│   ├── 06-secrets-management/
│   ├── 07-internal-developer-platform/
│   ├── 08-self-service/
│   ├── 09-platform-apis/
│   ├── 10-observability/
│   ├── 11-reliability/
│   ├── 12-security-compliance/
│   ├── 13-multi-cluster/
│   ├── 14-platform-evolution/
│   └── 15-case-studies/
├── labs/
│   ├── lab-01-cluster-setup/
│   ├── lab-02-gitops-bootstrap/
│   ├── lab-03-platform-api/
│   ├── lab-04-service-catalog/
│   ├── lab-05-policy-engine/
│   ├── lab-06-observability-stack/
│   ├── lab-07-multi-tenancy/
│   └── lab-08-platform-upgrade/
└── code/
    ├── platform-api/
    ├── service-catalog/
    ├── policy-engine/
    ├── platform-cli/
    ├── terraform-modules/
    ├── crossplane-compositions/
    ├── helm-charts/
    ├── tenant-operator/
    ├── environment-operator/
    └── workload-operator/
```

## Phase 1: Foundation Documentation (Week 1-2)

### 01-kubernetes-fundamentals
- kubernetes-architecture.md - Control plane, nodes, components
- api-primer.md - Resources, verbs, API groups
- controllers-operators.md - Controller pattern, operator pattern
- scheduling.md - Scheduler, predicates, priorities, custom schedulers
- storage.md - PV, PVC, StorageClass, CSI drivers
- rbac.md - Roles, ClusterRoles, bindings, impersonation

### 02-container-runtime
- container-runtimes.md - containerd, CRI-O, Docker Engine
- image-spec.md - OCI image spec, manifests, layers
- image-security.md - Signing (cosign), scanning (trivy), SBOM
- runtime-classes.md - RuntimeClass, kata containers, gVisor

### 03-networking-fundamentals
- cni-overview.md - CNI spec, popular plugins (Calico, Cilium, Flannel)
- service-mesh.md - Istio, Linkerd, Cilium Service Mesh
- ingress-controllers.md - NGINX, Traefik, Gateway API
- network-policies.md - Kubernetes NetworkPolicy, CiliumNetworkPolicy

## Phase 2: Platform Building Blocks (Week 3-4)

### 04-infrastructure-as-code
- terraform-kubernetes.md - TF k8s provider, helm provider
- crossplane.md - Crossplane architecture, XR, compositions
- gitops-tool-comparison.md - ArgoCD vs Flux vs Fleet
- drift-detection.md - Detecting and remediating drift

### 05-gitops
- gitops-principles.md - Declarative, versioned, automated
- argocd-deep-dive.md - App of Apps, ApplicationSets, projects
- flux-deep-dive.md - Kustomize, Helm, Image automation
- multi-cluster-gitops.md - Managing multiple clusters

### 06-secrets-management
- vault-integration.md - Vault Agent Injector, CSI driver
- external-secrets.md - External Secrets Operator
- sealed-secrets.md - Bitnami Sealed Secrets
- secrets-rotation.md - Automated rotation patterns

## Phase 3: Developer Platform (Week 5-6)

### 07-internal-developer-platform
- backstage.md - Backstage architecture, plugins, software templates
- port.md - Port.io alternative
- custom-portal.md - Building custom developer portals
- platform-capabilities.md - Defining platform capabilities

### 08-self-service
- service-catalog.md - Service catalog patterns
- scaffolding.md - Template engines, cookiecutter, scaffolder
- environment-provisioning.md - Ephemeral environments, preview envs
- developer-experience.md - DX metrics, feedback loops

### 09-platform-apis
- platform-api-design.md - REST vs gRPC, API versioning
- kubernetes-crds.md - CRD design, validation, conversion
- controller-runtime.md - Kubebuilder, controller-runtime
- api-gateway.md - Kong, Traefik, Envoy as platform gateway

## Phase 4: Observability & Reliability (Week 7-8)

### 10-observability
- logging-stack.md - Loki, Elasticsearch, Vector/Fluent Bit
- metrics-stack.md - Prometheus, Thanos, Cortex, VictoriaMetrics
- tracing-stack.md - Tempo, Jaeger, OpenTelemetry
- profiling.md - Pyroscope, continuous profiling
- alerting.md - Alertmanager, routing, inhibition, silencing

### 11-reliability
- slos-slis.md - Defining SLOs, SLIs, error budgets
- chaos-engineering.md - LitmusChaos, Chaos Mesh, Gremlin
- disaster-recovery.md - Backup/restore (Velero), DR strategies
- capacity-planning.md - VPA, HPA, cluster autoscaler, Karpenter

### 12-security-compliance
- policy-engine.md - OPA Gatekeeper, Kyverno, CEL
- pod-security.md - PSA, Pod Security Standards
- supply-chain.md - SLSA, Sigstore, in-toto
- compliance.md - CIS benchmarks, Kyverno policies

## Phase 5: Advanced Topics (Week 9-10)

### 13-multi-cluster
- cluster-federation.md - Karmada, Cluster API, Fleet
- multi-cluster-networking.md - Submariner, Istio multi-cluster
- multi-cluster-gitops.md - ArgoCD multi-cluster, Flux multi-cluster
- workload-distribution.md - Scheduling across clusters

### 14-platform-evolution
- maturity-model.md - Platform maturity levels
- team-topologies.md - Stream-aligned, platform, enabling teams
- platform-product.md - Platform as a product
- measuring-success.md - DORA metrics, platform metrics

### 15-case-studies
- netflix.md - Netflix platform engineering
- spotify.md - Spotify Backstage journey
- airbnb.md - Airbnb service platform
- generic-enterprise.md - Enterprise platform patterns

## Hands-On Labs

### Lab 01: Cluster Setup (Beginner)
- kind-cluster.yaml - Multi-node Kind config
- k3d-cluster.yaml - k3d with registries
- platform-components.yaml - Core platform components
- setup.sh - Automated setup script

### Lab 02: GitOps Bootstrap (Beginner)
- argocd-install.yaml - ArgoCD installation
- app-of-apps.yaml - App of Apps pattern
- applications/ - Sample applications
- bootstrap.sh - Bootstrap script

### Lab 03: Platform API (Intermediate)
- api-types.go - Go types for platform API
- controllers/ - Kubernetes controllers
- crds/ - Custom Resource Definitions
- Makefile - Build and deploy targets

### Lab 04: Service Catalog (Intermediate)
- backstage-config.yaml - Backstage app config
- templates/ - Software templates
- catalog-entities.yaml - Catalog entities
- deploy.sh - Deployment script

### Lab 05: Policy Engine (Intermediate)
- gatekeeper-policies/ - OPA Gatekeeper constraints
- kyverno-policies/ - Kyverno policies
- cel-expressions.md - CEL validation examples
- test-policies.sh - Policy testing script

### Lab 06: Observability Stack (Intermediate)
- loki-stack.yaml - Loki deployment
- prometheus-stack.yaml - Prometheus + Thanos
- tempo-stack.yaml - Tempo deployment
- otel-collector.yaml - OpenTelemetry Collector

### Lab 07: Multi-Tenancy (Advanced)
- tenant-operator/ - Tenant controller
- namespace-templates/ - Namespace templates
- resource-quotas/ - Quota management
- network-isolation/ - Network policies

### Lab 08: Platform Upgrade (Advanced)
- blue-green-deploy.md - Blue-green platform upgrades
- canary-upgrade.md - Canary platform upgrades
- rollback-procedures.md - Rollback runbooks
- upgrade-automation.sh - Automated upgrade script

## Code Examples

### platform-api
- main.go - HTTP/gRPC server
- handlers/ - API handlers
- middleware/ - Auth, logging, validation
- client/ - Generated client SDK

### service-catalog
- catalog.go - Catalog core logic
- templates/ - Go templates for scaffolding
- registry/ - Template registry
- cli/ - Catalog CLI

### policy-engine
- engine.go - Policy evaluation engine
- policies/ - Rego policies
- admission/ - Admission webhook
- metrics/ - Policy metrics

### platform-cli
- main.go - CLI entry point
- commands/ - Cobra commands
- config/ - Configuration management
- output/ - Formatters (table, json, yaml)

### terraform-modules
- eks/ - EKS cluster module
- gke/ - GKE cluster module
- aks/ - AKS cluster module
- networking/ - VPC, subnets, NAT
- addons/ - CNI, CSI, monitoring

### crossplane-compositions
- xrd/ - Composite Resource Definitions
- compositions/ - Composition templates
- functions/ - Composition functions
- claims/ - Example claims

### helm-charts
- platform-core/ - Core platform chart
- platform-addons/ - Addons chart
- developer-tools/ - Dev tools chart

### tenant-operator
- api/ - Tenant API types
- controllers/ - Tenant controller
- webhooks/ - Validation webhooks
- tests/ - Integration tests

### environment-operator
- api/ - Environment API types
- controllers/ - Environment controller
- templates/ - Environment templates

### workload-operator
- api/ - Workload API types
- controllers/ - Workload controller
- traits/ - Workload traits

## Documentation Standards

Each doc should include:
- Overview and learning objectives
- Prerequisites
- Conceptual explanation with diagrams (Mermaid)
- Code examples with explanations
- Hands-on exercises
- Further reading links
- Troubleshooting section

## Lab Standards

Each lab should include:
- README.md with objectives and prerequisites
- Setup/teardown scripts
- Step-by-step instructions
- Verification steps
- Cleanup instructions
- Expected time to complete

## Code Standards

- Go 1.21+ for controllers/CLI
- Kubernetes 1.28+ APIs
- Controller-runtime v0.17+
- Comprehensive tests
- Makefile with standard targets
- Dockerfile for containerization
- Helm chart for deployment

## Timeline

- Week 1-2: Phase 1 docs + Lab 01
- Week 3-4: Phase 2 docs + Lab 02
- Week 5-6: Phase 3 docs + Lab 03-04
- Week 7-8: Phase 4 docs + Lab 05-06
- Week 9-10: Phase 5 docs + Lab 07-08
- Ongoing: Code examples, case studies

## Success Metrics

- All 15 doc sections complete with exercises
- 8 labs working end-to-end
- 10 code examples compiling and deploying
- Cross-references between docs, labs, code
- Searchable, navigable structure
