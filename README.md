# Platform Engineering on Kubernetes - Learning Resource

A comprehensive learning platform for Platform Engineering concepts, patterns, and implementations on Kubernetes.

**New here?** Follow the guided, step-by-step path in [START-HERE.md](START-HERE.md).

## 📚 Learning Path

### Phase 1: Foundations
- [01-kubernetes-fundamentals](docs/01-kubernetes-fundamentals/) - Core Kubernetes concepts for platform engineers
- [02-container-runtime](docs/02-container-runtime/) - Container runtimes, images, and security
- [03-networking-fundamentals](docs/03-networking-fundamentals/) - CNI, Service Mesh, Ingress

### Phase 2: Platform Building Blocks
- [04-infrastructure-as-code](docs/04-infrastructure-as-code/) - Terraform, Crossplane, GitOps
- [05-gitops](docs/05-gitops/) - ArgoCD, Flux, GitOps patterns
- [06-secrets-management](docs/06-secrets-management/) - Vault, External Secrets, Sealed Secrets

### Phase 3: Developer Platform
- [07-internal-developer-platform](docs/07-internal-developer-platform/) - Backstage, Port, custom portals
- [08-self-service](docs/08-self-service/) - Service catalogs, templates, scaffolding
- [09-platform-apis](docs/09-platform-apis/) - Platform APIs, CRDs, controllers

### Phase 4: Observability & Reliability
- [10-observability](docs/10-observability/) - Logging, metrics, tracing, profiling
- [11-reliability](docs/11-reliability/) - SLOs, chaos engineering, disaster recovery
- [12-security-compliance](docs/12-security-compliance/) - Policy enforcement, compliance, RBAC

### Phase 5: Advanced Topics
- [13-multi-cluster](docs/13-multi-cluster/) - Cluster federation, multi-cluster management
- [14-platform-evolution](docs/14-platform-evolution/) - Platform maturity models, team topologies
- [15-case-studies](docs/15-case-studies/) - Real-world platform engineering case studies

## 🛠️ Hands-On Labs

| Lab | Description | Difficulty |
|-----|-------------|------------|
| [lab-01-cluster-setup](labs/lab-01-cluster-setup/) | Kind/k3d cluster with platform components | Beginner |
| [lab-02-gitops-bootstrap](labs/lab-02-gitops-bootstrap/) | Bootstrap ArgoCD with App of Apps pattern | Beginner |
| [lab-03-platform-api](labs/lab-03-platform-api/) | Build a platform API with Kubernetes controllers | Intermediate |
| [lab-04-service-catalog](labs/lab-04-service-catalog/) | Create a service catalog with Backstage | Intermediate |
| [lab-05-policy-engine](labs/lab-05-policy-engine/) | Implement Gatekeeper, Kyverno and CEL admission policies | Intermediate |
| [lab-06-observability-stack](labs/lab-06-observability-stack/) | Deploy full observability stack | Intermediate |
| [lab-07-multi-tenancy](labs/lab-07-multi-tenancy/) | Multi-tenant platform with namespaces | Advanced |
| [lab-08-platform-upgrade](labs/lab-08-platform-upgrade/) | Blue-green platform upgrades | Advanced |

## 📦 Code Examples

### Platform Components
- [platform-api](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/platform-api/) - Reference platform API implementation
- [service-catalog](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/service-catalog/) - Service catalog with templating
- [policy-engine](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/policy-engine/) - Custom policy controller
- [platform-cli](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/platform-cli/) - Developer CLI for platform interaction

### Infrastructure
- [terraform-modules](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/terraform-modules/) - Reusable Terraform modules
- [crossplane-compositions](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/crossplane-compositions/) - Crossplane XR definitions
- [helm-charts](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/helm-charts/) - Platform Helm charts

### Operators & Controllers
- [tenant-operator](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/tenant-operator/) - Multi-tenancy operator
- [environment-operator](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/environment-operator/) - Environment lifecycle operator
- [workload-operator](https://github.com/GitOpsHub/platform-engineering-on-kubernetes/tree/main/code/workload-operator/) - Workload abstraction operator

## 🚀 Quick Start

```bash
# 1. Start a local cluster
cd labs/lab-01-cluster-setup
./setup.sh

# 2. Bootstrap GitOps
cd ../lab-02-gitops-bootstrap
./bootstrap.sh

# 3. Build and deploy the Lab 03 WebApp operator
cd ../lab-03-platform-api
make deploy

# 4. Try the platform CLI
cd ../../code/platform-cli
go build -o platform-cli
./platform-cli --help   # lists the config, env, tenant, workload, resource and version commands
```

## 📖 Recommended Reading Order

1. Start with **Phase 1** docs for Kubernetes fundamentals
2. Follow **Phase 2** for platform building blocks
3. Complete **labs 01-02** for hands-on GitOps experience
4. Explore **Phase 3** for developer platform concepts
5. Build **lab 03-04** for platform API and service catalog
6. Study **Phase 4** for production readiness
7. Complete advanced labs **05-08**

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on adding new content.

## 📄 License

MIT License - See [LICENSE](LICENSE) for details.
