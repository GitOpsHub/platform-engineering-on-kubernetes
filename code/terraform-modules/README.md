# terraform-modules

Reference Terraform/OpenTofu modules for provisioning Kubernetes platforms on AWS, GCP and Azure,
plus a baseline of platform add-ons.

```
terraform-modules/
├── networking/
│   ├── aws/      VPC, public/private subnets, NAT, flow logs (for EKS)
│   ├── gcp/      VPC, subnet with Pod/Service secondary ranges, Cloud NAT (for GKE)
│   └── azure/    VNet, node subnet + NSG, NAT gateway (for AKS)
├── eks/          EKS: KMS Secrets encryption, access entries, IRSA + Pod Identity, managed node groups
├── gke/          GKE: private nodes, Workload Identity, Dataplane V2, least-privilege node SA
├── aks/          AKS: Entra ID + Azure RBAC, Workload Identity, CNI Overlay + Cilium
├── addons/       Helm: Cilium, cert-manager, external-secrets, metrics-server, kube-prometheus-stack
└── examples/
    ├── eks-complete/   network -> eks -> addons
    ├── gke-complete/   network -> gke -> addons
    └── aks-complete/   resource group -> network -> aks -> addons
```

## Design decisions

- **Native resources rather than registry wrappers.** The modules use provider resources
  (`aws_eks_cluster`, `google_container_cluster`, `azurerm_kubernetes_cluster`, ...) and do not wrap
  `terraform-aws-modules/eks`, `terraform-google-modules/kubernetes-engine` or `Azure/aks`. This is a
  learning repo, so you can see every security-relevant setting in about 300 lines per cloud. In production,
  the community modules are a good choice when you want more features and less code to own.
- **Networking per cloud.** `networking/` has one submodule per cloud because the three VPC models
  differ too much to share one interface. Each one outputs exactly what its cluster module needs.
- **No provider blocks in modules.** Only `examples/` configure providers and backends, so modules
  compose and can be used with `for_each`.
- **Secure by default.** Nodes are always private, public API access is opt-in and CIDR-restricted,
  Secrets are encrypted, workload identity (IRSA/Pod Identity, GKE Workload Identity, AKS Workload
  Identity) is on, IMDSv2 is enforced, local/static credentials are disabled where the cloud allows it,
  and control-plane logs are kept with bounded retention.

## Versions

| Component | Constraint in modules | Pinned in examples | Validated with |
| --------- | --------------------- | ------------------ | -------------- |
| Terraform / OpenTofu | `>= 1.6.0` | | OpenTofu 1.13.2 |
| hashicorp/aws | `>= 6.0, < 7.0` | `~> 6.70` | 6.70.0 |
| hashicorp/google | `>= 7.0, < 9.0` | `~> 8.8` | 8.8.1 |
| hashicorp/azurerm | `>= 5.0, < 6.0` | `~> 5.11` | 5.11.0 |
| hashicorp/helm | `>= 3.0, < 4.0` | `~> 3.1` | 3.3.0 |
| hashicorp/tls | `>= 4.0, < 5.0` | `~> 4.1` | latest 4.x |

Kubernetes 1.28+ is assumed; the EKS example defaults to 1.34, GKE follows the REGULAR release channel,
and AKS uses the region default with patch auto-upgrades.

## Usage

```bash
cd examples/eks-complete
cp terraform.tfvars.example terraform.tfvars   # edit values
terraform init
terraform plan -out tfplan
terraform apply tfplan
```

Uncomment and configure the remote `backend` block in each example before you use it for anything shared.

> **Two-phase apply note.** The examples configure the Helm provider from cluster outputs in the same
> root module. A first `apply` works, but if the cluster is replaced later, the provider cannot connect
> during plan. Production setups usually split infrastructure (`network` + cluster) and in-cluster add-ons into
> separate root modules or stacks.

## Local checks (no cloud credentials needed)

```bash
make fmt        # terraform fmt -recursive
make validate   # init -backend=false && validate in every example
make docs       # regenerate README input/output tables (terraform-docs)
```

Set `TF=tofu` to use OpenTofu: `make validate TF=tofu`.
