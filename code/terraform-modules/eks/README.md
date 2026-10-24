# eks — Amazon EKS cluster

Native-resource EKS module (no registry wrapper) so every security decision is visible:

- **API access**: private endpoint always on; public endpoint opt-in and must be CIDR-restricted (`0.0.0.0/0` is rejected).
- **AuthN/Z**: EKS access entries (`authentication_mode = "API"`), no `aws-auth` ConfigMap, creator gets no implicit admin.
- **Encryption**: Secrets envelope-encrypted with a dedicated, rotating KMS key (or bring your own); gp3 root volumes encrypted.
- **Workload identity**: IRSA OIDC provider **and** the EKS Pod Identity agent add-on; the EBS CSI driver uses Pod Identity.
- **Nodes**: managed node groups via launch templates enforcing IMDSv2 with hop limit 1; AL2023 AMIs by default.
- **Logging**: api/audit/authenticator logs to a CloudWatch group with bounded retention.

`bootstrap_self_managed_addons = false`: networking comes from the managed `vpc-cni` add-on in `cluster_addons`.
To run Cilium instead, remove `vpc-cni` and `kube-proxy` from `cluster_addons` and enable Cilium in the `addons` module
(set `eks_api_server_host`).

## Usage

```hcl
module "eks" {
  source = "../eks"

  cluster_name       = "platform-dev"
  kubernetes_version = "1.34"
  vpc_id             = module.network.vpc_id
  subnet_ids         = module.network.private_subnet_ids

  endpoint_public_access = true
  public_access_cidrs    = ["203.0.113.0/24"]
  admin_principal_arns   = ["arn:aws:iam::111122223333:role/PlatformAdmins"]

  node_groups = {
    system    = { instance_types = ["m6i.large"] }
    workloads = { instance_types = ["m6i.xlarge"], capacity_type = "SPOT", max_size = 10 }
  }
}
```

A complete root module using this module lives in [`examples/`](../examples).

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.6.0 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 6.0, < 7.0 |
| <a name="requirement_tls"></a> [tls](#requirement\_tls) | >= 4.0, < 5.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_aws"></a> [aws](#provider\_aws) | >= 6.0, < 7.0 |
| <a name="provider_tls"></a> [tls](#provider\_tls) | >= 4.0, < 5.0 |

## Resources

| Name | Type |
| ---- | ---- |
| [aws_cloudwatch_log_group.cluster](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_log_group) | resource |
| [aws_eks_access_entry.admins](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_access_entry) | resource |
| [aws_eks_access_policy_association.admins](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_access_policy_association) | resource |
| [aws_eks_addon.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_addon) | resource |
| [aws_eks_cluster.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_cluster) | resource |
| [aws_eks_node_group.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eks_node_group) | resource |
| [aws_iam_openid_connect_provider.this](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_openid_connect_provider) | resource |
| [aws_iam_role.cluster](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role) | resource |
| [aws_iam_role.ebs_csi](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role) | resource |
| [aws_iam_role.node](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role) | resource |
| [aws_iam_role_policy.cluster_kms](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy) | resource |
| [aws_iam_role_policy_attachment.cluster](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy_attachment) | resource |
| [aws_iam_role_policy_attachment.ebs_csi](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy_attachment) | resource |
| [aws_iam_role_policy_attachment.node](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy_attachment) | resource |
| [aws_kms_alias.secrets](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_alias) | resource |
| [aws_kms_key.secrets](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_key) | resource |
| [aws_launch_template.node](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/launch_template) | resource |
| [aws_security_group.cluster](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/security_group) | resource |
| [aws_caller_identity.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/caller_identity) | data source |
| [aws_iam_policy_document.cluster_assume](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/iam_policy_document) | data source |
| [aws_iam_policy_document.cluster_kms](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/iam_policy_document) | data source |
| [aws_iam_policy_document.ebs_csi_pod_identity](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/iam_policy_document) | data source |
| [aws_iam_policy_document.node_assume](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/iam_policy_document) | data source |
| [aws_partition.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/partition) | data source |
| [tls_certificate.oidc](https://registry.terraform.io/providers/hashicorp/tls/latest/docs/data-sources/certificate) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_cluster_name"></a> [cluster\_name](#input\_cluster\_name) | Name of the EKS cluster. | `string` | n/a | yes |
| <a name="input_subnet_ids"></a> [subnet\_ids](#input\_subnet\_ids) | Private subnet IDs for the control-plane ENIs and the node groups (at least two AZs). | `list(string)` | n/a | yes |
| <a name="input_vpc_id"></a> [vpc\_id](#input\_vpc\_id) | VPC ID the cluster runs in (from the networking/aws module). | `string` | n/a | yes |
| <a name="input_admin_principal_arns"></a> [admin\_principal\_arns](#input\_admin\_principal\_arns) | IAM principal ARNs (roles/users) granted cluster-admin via EKS access entries. | `list(string)` | `[]` | no |
| <a name="input_cluster_addons"></a> [cluster\_addons](#input\_cluster\_addons) | EKS managed add-ons to install, keyed by add-on name. Value is an optional pinned version (null = default for the cluster version). | `map(string)` | <pre>{<br/>  "aws-ebs-csi-driver": null,<br/>  "coredns": null,<br/>  "eks-pod-identity-agent": null,<br/>  "kube-proxy": null,<br/>  "vpc-cni": null<br/>}</pre> | no |
| <a name="input_enabled_log_types"></a> [enabled\_log\_types](#input\_enabled\_log\_types) | Control-plane log types shipped to CloudWatch. | `list(string)` | <pre>[<br/>  "api",<br/>  "audit",<br/>  "authenticator"<br/>]</pre> | no |
| <a name="input_endpoint_public_access"></a> [endpoint\_public\_access](#input\_endpoint\_public\_access) | Expose the Kubernetes API publicly. The private endpoint is always enabled; set false for a fully private cluster. | `bool` | `false` | no |
| <a name="input_kms_key_arn"></a> [kms\_key\_arn](#input\_kms\_key\_arn) | Existing KMS key ARN for envelope encryption of Kubernetes Secrets. If null, the module creates a dedicated key. | `string` | `null` | no |
| <a name="input_kubernetes_version"></a> [kubernetes\_version](#input\_kubernetes\_version) | Kubernetes minor version for the control plane, e.g. "1.34". The repo targets 1.28+. | `string` | `"1.34"` | no |
| <a name="input_log_retention_days"></a> [log\_retention\_days](#input\_log\_retention\_days) | Retention for the control-plane CloudWatch log group. | `number` | `90` | no |
| <a name="input_node_groups"></a> [node\_groups](#input\_node\_groups) | Managed node groups keyed by name. Each entry:<br/>  instance\_types - list of EC2 instance types<br/>  capacity\_type  - ON\_DEMAND or SPOT<br/>  min\_size / max\_size / desired\_size<br/>  disk\_size\_gib  - root volume size<br/>  ami\_type       - e.g. AL2023\_x86\_64\_STANDARD, AL2023\_ARM\_64\_STANDARD, BOTTLEROCKET\_x86\_64<br/>  labels         - Kubernetes node labels<br/>  taints         - list of { key, value, effect } (effect: NO\_SCHEDULE \| NO\_EXECUTE \| PREFER\_NO\_SCHEDULE) | <pre>map(object({<br/>    instance_types = optional(list(string), ["m6i.large"])<br/>    capacity_type  = optional(string, "ON_DEMAND")<br/>    min_size       = optional(number, 2)<br/>    max_size       = optional(number, 5)<br/>    desired_size   = optional(number, 2)<br/>    disk_size_gib  = optional(number, 50)<br/>    ami_type       = optional(string, "AL2023_x86_64_STANDARD")<br/>    labels         = optional(map(string), {})<br/>    taints = optional(list(object({<br/>      key    = string<br/>      value  = optional(string)<br/>      effect = string<br/>    })), [])<br/>  }))</pre> | <pre>{<br/>  "system": {}<br/>}</pre> | no |
| <a name="input_public_access_cidrs"></a> [public\_access\_cidrs](#input\_public\_access\_cidrs) | CIDRs allowed to reach the public API endpoint (only used when endpoint\_public\_access = true). | `list(string)` | `[]` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Tags applied to all resources. | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_account_id"></a> [account\_id](#output\_account\_id) | AWS account ID the cluster was created in. |
| <a name="output_cluster_arn"></a> [cluster\_arn](#output\_cluster\_arn) | ARN of the EKS cluster. |
| <a name="output_cluster_certificate_authority_data"></a> [cluster\_certificate\_authority\_data](#output\_cluster\_certificate\_authority\_data) | Base64-encoded cluster CA certificate. |
| <a name="output_cluster_endpoint"></a> [cluster\_endpoint](#output\_cluster\_endpoint) | Kubernetes API server endpoint. |
| <a name="output_cluster_name"></a> [cluster\_name](#output\_cluster\_name) | Name of the EKS cluster. |
| <a name="output_cluster_security_group_id"></a> [cluster\_security\_group\_id](#output\_cluster\_security\_group\_id) | EKS-managed cluster security group (attached to control-plane ENIs and managed nodes). |
| <a name="output_cluster_version"></a> [cluster\_version](#output\_cluster\_version) | Kubernetes version of the control plane. |
| <a name="output_kms_key_arn"></a> [kms\_key\_arn](#output\_kms\_key\_arn) | KMS key ARN used for Secrets encryption. |
| <a name="output_node_role_arn"></a> [node\_role\_arn](#output\_node\_role\_arn) | IAM role ARN used by managed node groups. |
| <a name="output_oidc_issuer_url"></a> [oidc\_issuer\_url](#output\_oidc\_issuer\_url) | OIDC issuer URL of the cluster. |
| <a name="output_oidc_provider_arn"></a> [oidc\_provider\_arn](#output\_oidc\_provider\_arn) | ARN of the IAM OIDC provider for IRSA. |
<!-- END_TF_DOCS -->
