# Start Here: A Step-by-Step Learning Path

This guide takes you through the whole repo in order. Each step has three parts:

- **Read**: the doc to study
- **Do**: the lab or command to run
- **Check**: how you know the step worked

You can also work through it interactively with Claude Code. Open this repo, then ask:

> Walk me through START-HERE.md one step at a time. After each step, quiz me with two questions before we move on.

Claude will pace you through it and check your answers.

---

## Before You Begin

### Tools

| Tool | Version | Used in |
|------|---------|---------|
| Docker Desktop or Docker Engine | recent | Labs 01–08 |
| kubectl | 1.28+ | Labs 01–08 |
| kind | 0.33+ | Lab 01 |
| k3d | 5.6+ (optional) | Lab 01 |
| Helm | 3.12+ | Labs 01, 04, 06 |
| Go | 1.22+ (1.26 for the Go modules that need it) | Labs 03, 07; `code/` |
| Terraform or OpenTofu | 1.6+ | `code/terraform-modules` |

Resources: 8 GB RAM and 4 CPU cores are the minimum for the full stack. Labs 01 and 02 run on less.

### Ground rules

- Labs share one kind cluster, named `platform-engineering`, created in Lab 01. Tear down a lab only when its **Cleanup** section says so.
- Never run lab scripts against a production cluster. Check your context first: `kubectl config current-context`.
- Lab 01 owns host ports 8080 and 8443. Do not port-forward to them in later labs. Use 8081 instead.

---

## Part 1: Foundations (Phase 1)

### Step 1. Kubernetes architecture

- **Read:** [docs/01-kubernetes-fundamentals/kubernetes-architecture.md](docs/01-kubernetes-fundamentals/kubernetes-architecture.md)
- **Do:** nothing yet. Draw the control plane and node components on paper.
- **Check:** you can name what etcd, the scheduler, and the kubelet each do without looking.

### Step 2. API primer, controllers and scheduling

- **Read:** [api-primer.md](docs/01-kubernetes-fundamentals/api-primer.md), [controllers-operators.md](docs/01-kubernetes-fundamentals/controllers-operators.md), [scheduling.md](docs/01-kubernetes-fundamentals/scheduling.md)
- **Check:** you can explain the reconcile loop in one sentence.

### Step 3. Storage and RBAC

- **Read:** [storage.md](docs/01-kubernetes-fundamentals/storage.md), [rbac.md](docs/01-kubernetes-fundamentals/rbac.md)
- **Check:** you can write a Role and RoleBinding that lets one user read Pods in one namespace, and say why a ClusterRole is the wrong tool for that.

### Step 4. Containers, images and runtime security

- **Read:** [container-runtimes.md](docs/02-container-runtime/container-runtimes.md), [image-spec.md](docs/02-container-runtime/image-spec.md), [image-security.md](docs/02-container-runtime/image-security.md), [runtime-classes.md](docs/02-container-runtime/runtime-classes.md)
- **Check:** you can list three ways to reduce image attack surface.

### Step 5. Networking

- **Read:** [cni-overview.md](docs/03-networking-fundamentals/cni-overview.md), [service-mesh.md](docs/03-networking-fundamentals/service-mesh.md), [ingress-controllers.md](docs/03-networking-fundamentals/ingress-controllers.md), [network-policies.md](docs/03-networking-fundamentals/network-policies.md)
- **Check:** you can say what a default-deny NetworkPolicy does to a Pod with no policies selecting it.

### Step 6. Your first cluster (Lab 01)

- **Read:** [labs/lab-01-cluster-setup/README.md](labs/lab-01-cluster-setup/README.md)
- **Do:**
  ```bash
  cd labs/lab-01-cluster-setup
  ./setup.sh
  ./verify.sh
  ```
- **Check:** `kubectl get nodes` shows the nodes as `Ready`, and `verify.sh` reports all checks passing.

**Checkpoint.** You have a working platform-engineering cluster. Everything after this builds on it.

---

## Part 2: Platform Building Blocks (Phase 2)

### Step 7. Infrastructure as code

- **Read:** [terraform-kubernetes.md](docs/04-infrastructure-as-code/terraform-kubernetes.md), [crossplane.md](docs/04-infrastructure-as-code/crossplane.md), [gitops-tool-comparison.md](docs/04-infrastructure-as-code/gitops-tool-comparison.md), [drift-detection.md](docs/04-infrastructure-as-code/drift-detection.md)
- **Explore:** [code/terraform-modules](code/terraform-modules/README.md), [code/crossplane-compositions](code/crossplane-compositions/README.md). You don't need to apply anything.
- **Check:** you can explain the difference between a Terraform module and a Crossplane composition in one paragraph.

### Step 8. GitOps principles

- **Read:** [gitops-principles.md](docs/05-gitops/gitops-principles.md), [argocd-deep-dive.md](docs/05-gitops/argocd-deep-dive.md), [flux-deep-dive.md](docs/05-gitops/flux-deep-dive.md)
- **Check:** you can explain the difference between push-based and pull-based deployment.

### Step 9. Bootstrap GitOps (Lab 02)

- **Read:** [labs/lab-02-gitops-bootstrap/README.md](labs/lab-02-gitops-bootstrap/README.md)
- **Do:** follow the README from the top. Run `bootstrap.sh`, then open Argo CD.
- **Check:** the root Application shows `Synced` and `Healthy` in Argo CD.

### Step 10. Secrets

- **Read:** [external-secrets.md](docs/06-secrets-management/external-secrets.md), [vault-integration.md](docs/06-secrets-management/vault-integration.md), [sealed-secrets.md](docs/06-secrets-management/sealed-secrets.md), [secrets-rotation.md](docs/06-secrets-management/secrets-rotation.md)
- **Check:** you can say why secrets never go into Git in plain text, and name two ways to avoid it.

**Checkpoint.** Changes to the cluster now flow through Git, and secrets are handled properly.

---

## Part 3: Developer Platform (Phase 3)

### Step 11. Internal developer platform

- **Read:** [backstage.md](docs/07-internal-developer-platform/backstage.md), [port.md](docs/07-internal-developer-platform/port.md), [custom-portal.md](docs/07-internal-developer-platform/custom-portal.md), [platform-capabilities.md](docs/07-internal-developer-platform/platform-capabilities.md)
- **Check:** you can explain why a portal should act as the signed-in user and not with its own cluster credentials.

### Step 12. Self-service

- **Read:** [service-catalog.md](docs/08-self-service/service-catalog.md), [scaffolding.md](docs/08-self-service/scaffolding.md), [environment-provisioning.md](docs/08-self-service/environment-provisioning.md), [developer-experience.md](docs/08-self-service/developer-experience.md)
- **Check:** you can describe what a preview environment is and when it should expire.

### Step 13. Platform APIs and CRDs

- **Read:** [platform-api-design.md](docs/09-platform-apis/platform-api-design.md), [kubernetes-crds.md](docs/09-platform-apis/kubernetes-crds.md), [controller-runtime.md](docs/09-platform-apis/controller-runtime.md), [api-gateway.md](docs/09-platform-apis/api-gateway.md)
- **Check:** you can name the three platform CRDs (`Tenant`, `Environment`, `Workload`) and say which one is cluster-scoped.

### Step 14. Your first custom resource (Lab 03)

- **Read:** [labs/lab-03-platform-api/README.md](labs/lab-03-platform-api/README.md)
- **Do:**
  ```bash
  cd labs/lab-03-platform-api
  make test
  make deploy
  kubectl apply -f samples/webapp-hello.yaml
  kubectl get webapps
  ```
- **Check:** the `WebApp` shows a `Ready` condition, and `kubectl get deploy,svc` shows the objects it created.

### Step 15. Service catalog (Lab 04)

- **Read:** [labs/lab-04-service-catalog/README.md](labs/lab-04-service-catalog/README.md)
- **Check:** you understand the two Software Templates and can say what each one scaffolds.
- **Do (optional):** run `deploy.sh` after you've built the Backstage image as the README describes.

**Checkpoint.** You can define a new platform API and expose it to developers.

---

## Part 4: Observability, Reliability and Security (Phase 4)

### Step 16. Observability

- **Read:** [logging-stack.md](docs/10-observability/logging-stack.md), [metrics-stack.md](docs/10-observability/metrics-stack.md), [tracing-stack.md](docs/10-observability/tracing-stack.md), [profiling.md](docs/10-observability/profiling.md), [alerting.md](docs/10-observability/alerting.md)
- **Do:** follow [Lab 06](labs/lab-06-observability-stack/README.md) and open Grafana.
- **Check:** you can move from a metric spike to the matching trace and log lines in Grafana.

### Step 17. Reliability

- **Read:** [slos-slis.md](docs/11-reliability/slos-slis.md), [chaos-engineering.md](docs/11-reliability/chaos-engineering.md), [disaster-recovery.md](docs/11-reliability/disaster-recovery.md), [capacity-planning.md](docs/11-reliability/capacity-planning.md)
- **Check:** you can write an SLO and calculate the error budget it leaves.

### Step 18. Policy enforcement (Lab 05)

- **Read:** [policy-engine.md](docs/12-security-compliance/policy-engine.md), then [labs/lab-05-policy-engine/README.md](labs/lab-05-policy-engine/README.md)
- **Do:** run `setup.sh`, then `test-policies.sh`.
- **Check:** `test-policies.sh` shows the bad fixtures denied and the good ones allowed.

### Step 19. Pod security, supply chain and compliance

- **Read:** [pod-security.md](docs/12-security-compliance/pod-security.md), [supply-chain.md](docs/12-security-compliance/supply-chain.md), [compliance.md](docs/12-security-compliance/compliance.md)
- **Check:** you can name the three Pod Security Standard levels and say which one a platform namespace should enforce.

### Step 20. Tenancy (Lab 07)

- **Read:** [labs/lab-07-multi-tenancy/README.md](labs/lab-07-multi-tenancy/README.md)
- **Do:** run `setup.sh`, then `tests/verify-isolation.sh`.
- **Check:** a request from one tenant's pod to another's fails.

**Checkpoint.** You can enforce policy and isolate tenants.

---

## Part 5: Advanced Topics (Phase 5)

### Step 21. Multi-cluster

- **Read:** [cluster-federation.md](docs/13-multi-cluster/cluster-federation.md), [multi-cluster-networking.md](docs/13-multi-cluster/multi-cluster-networking.md), [workload-distribution.md](docs/13-multi-cluster/workload-distribution.md), and the GitOps view in [docs/05-gitops/multi-cluster-gitops.md](docs/05-gitops/multi-cluster-gitops.md)
- **Check:** you can explain what changes when a second cluster joins the fleet.

### Step 22. Platform evolution

- **Read:** [maturity-model.md](docs/14-platform-evolution/maturity-model.md), [team-topologies.md](docs/14-platform-evolution/team-topologies.md), [platform-product.md](docs/14-platform-evolution/platform-product.md), [measuring-success.md](docs/14-platform-evolution/measuring-success.md)
- **Check:** you can place your current team on the maturity model and name one next step.

### Step 23. Upgrades (Lab 08)

- **Read:** [labs/lab-08-platform-upgrade/README.md](labs/lab-08-platform-upgrade/README.md), then the three runbooks in that folder.
- **Do:** run `upgrade-automation.sh --help`. The script is read-only unless you pass `--execute`.
- **Check:** you can say which runbook to use for a blue/green upgrade and which for a canary.

### Step 24. Case studies

- **Read:** [docs/15-case-studies/](docs/15-case-studies/README.md). Start with the generic enterprise case, then compare it with the public stories.
- **Check:** you can name one decision each company made and the trade-off it accepted.

**Checkpoint.** You've covered the whole curriculum.

---

## Where to Go Next

- Reference implementations: [code/](code/) (platform-api, platform-cli, service-catalog, policy-engine, the three operators, Terraform, Crossplane and Helm)
- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md)
- Clean up everything: run each lab's `teardown.sh`, then `kind delete cluster --name platform-engineering`
