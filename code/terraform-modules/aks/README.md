# aks — Azure Kubernetes Service cluster

Native-resource AKS module (requires azurerm 5.x):

- **Entra ID + Azure RBAC**, `local_account_disabled = true` (no static admin kubeconfig; use `kubelogin`).
- **Workload Identity** + OIDC issuer enabled; Key Vault Secrets Store CSI driver with rotation.
- **Azure CNI Overlay powered by Cilium** by default; egress via the NAT gateway from `networking/azure`.
- Private API server optional (`private_cluster_enabled`) or authorized IP ranges for a public one.
- User-assigned control-plane identity with Network Contributor on the node subnet.
- System pool tainted for critical add-ons only; user pools (incl. Spot) via `user_node_pools`.
- Patch auto-upgrades + NodeImage OS upgrades inside a weekly maintenance window; Azure Policy add-on on.

## Usage

```hcl
module "aks" {
  source = "../aks"

  cluster_name           = "platform-dev"
  resource_group_name    = azurerm_resource_group.this.name
  location               = "westeurope"
  nodes_subnet_id        = module.network.nodes_subnet_id
  authorized_ip_ranges   = ["203.0.113.0/24"]
  admin_group_object_ids = ["<entra-group-object-id>"]

  user_node_pools = {
    workloads = { vm_size = "Standard_D8s_v5", max_count = 10 }
  }
}
```

A complete root module using this module lives in [`examples/`](../examples).

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.6.0 |
| <a name="requirement_azurerm"></a> [azurerm](#requirement\_azurerm) | >= 5.0, < 6.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_azurerm"></a> [azurerm](#provider\_azurerm) | >= 5.0, < 6.0 |

## Resources

| Name | Type |
| ---- | ---- |
| [azurerm_kubernetes_cluster.this](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/kubernetes_cluster) | resource |
| [azurerm_kubernetes_cluster_node_pool.user](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/kubernetes_cluster_node_pool) | resource |
| [azurerm_role_assignment.subnet_network_contributor](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/role_assignment) | resource |
| [azurerm_user_assigned_identity.control_plane](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/user_assigned_identity) | resource |
| [azurerm_client_config.current](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/data-sources/client_config) | data source |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_cluster_name"></a> [cluster\_name](#input\_cluster\_name) | Name of the AKS cluster. | `string` | n/a | yes |
| <a name="input_location"></a> [location](#input\_location) | Azure region. | `string` | n/a | yes |
| <a name="input_nodes_subnet_id"></a> [nodes\_subnet\_id](#input\_nodes\_subnet\_id) | Subnet ID for node NICs (from networking/azure). | `string` | n/a | yes |
| <a name="input_resource_group_name"></a> [resource\_group\_name](#input\_resource\_group\_name) | Resource group for the cluster. | `string` | n/a | yes |
| <a name="input_admin_group_object_ids"></a> [admin\_group\_object\_ids](#input\_admin\_group\_object\_ids) | Entra ID group object IDs granted cluster admin (Azure RBAC for Kubernetes). | `list(string)` | `[]` | no |
| <a name="input_authorized_ip_ranges"></a> [authorized\_ip\_ranges](#input\_authorized\_ip\_ranges) | CIDRs allowed to reach a public API server. Ignored when private\_cluster\_enabled = true. | `list(string)` | `[]` | no |
| <a name="input_automatic_upgrade_channel"></a> [automatic\_upgrade\_channel](#input\_automatic\_upgrade\_channel) | AKS auto-upgrade channel: none, patch, rapid, stable or node-image. | `string` | `"patch"` | no |
| <a name="input_dns_service_ip"></a> [dns\_service\_ip](#input\_dns\_service\_ip) | ClusterIP for CoreDNS; must be inside service\_cidr. | `string` | `"172.20.0.10"` | no |
| <a name="input_kubernetes_version"></a> [kubernetes\_version](#input\_kubernetes\_version) | Kubernetes version (e.g. "1.34"). Null uses the AKS default; patch upgrades follow automatic\_upgrade\_channel. | `string` | `null` | no |
| <a name="input_log_analytics_workspace_id"></a> [log\_analytics\_workspace\_id](#input\_log\_analytics\_workspace\_id) | Optional Log Analytics workspace ID for Container Insights. Null disables the oms\_agent. | `string` | `null` | no |
| <a name="input_network_data_plane"></a> [network\_data\_plane](#input\_network\_data\_plane) | Data plane: "cilium" (Azure CNI powered by Cilium) or "azure". | `string` | `"cilium"` | no |
| <a name="input_pod_cidr"></a> [pod\_cidr](#input\_pod\_cidr) | Overlay Pod CIDR (Azure CNI Overlay; not routable in the VNet). | `string` | `"192.168.0.0/16"` | no |
| <a name="input_private_cluster_enabled"></a> [private\_cluster\_enabled](#input\_private\_cluster\_enabled) | Expose the API server only on a private endpoint inside the VNet. | `bool` | `false` | no |
| <a name="input_service_cidr"></a> [service\_cidr](#input\_service\_cidr) | Service CIDR; must not overlap the VNet. | `string` | `"172.20.0.0/16"` | no |
| <a name="input_sku_tier"></a> [sku\_tier](#input\_sku\_tier) | AKS control-plane tier: Free (labs), Standard (uptime SLA) or Premium (LTS). | `string` | `"Standard"` | no |
| <a name="input_system_node_pool"></a> [system\_node\_pool](#input\_system\_node\_pool) | Settings for the default (system) node pool. | <pre>object({<br/>    vm_size         = optional(string, "Standard_D4s_v5")<br/>    min_count       = optional(number, 2)<br/>    max_count       = optional(number, 5)<br/>    os_disk_size_gb = optional(number, 128)<br/>    zones           = optional(list(string), ["1", "2", "3"])<br/>  })</pre> | `{}` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Tags applied to all resources. | `map(string)` | `{}` | no |
| <a name="input_user_node_pools"></a> [user\_node\_pools](#input\_user\_node\_pools) | Additional user node pools keyed by name (max 12 lowercase alphanumeric chars). | <pre>map(object({<br/>    vm_size         = optional(string, "Standard_D4s_v5")<br/>    min_count       = optional(number, 1)<br/>    max_count       = optional(number, 5)<br/>    os_disk_size_gb = optional(number, 128)<br/>    zones           = optional(list(string), ["1", "2", "3"])<br/>    spot            = optional(bool, false)<br/>    labels          = optional(map(string), {})<br/>    taints          = optional(list(string), [])<br/>  }))</pre> | `{}` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_cluster_ca_certificate"></a> [cluster\_ca\_certificate](#output\_cluster\_ca\_certificate) | Base64-encoded cluster CA certificate. |
| <a name="output_cluster_fqdn"></a> [cluster\_fqdn](#output\_cluster\_fqdn) | Public FQDN of the API server (null for private clusters). |
| <a name="output_cluster_id"></a> [cluster\_id](#output\_cluster\_id) | Resource ID of the AKS cluster. |
| <a name="output_cluster_name"></a> [cluster\_name](#output\_cluster\_name) | Name of the AKS cluster. |
| <a name="output_kube_config_host"></a> [kube\_config\_host](#output\_kube\_config\_host) | API server URL. |
| <a name="output_kubelet_identity_object_id"></a> [kubelet\_identity\_object\_id](#output\_kubelet\_identity\_object\_id) | Object ID of the kubelet managed identity (grant AcrPull to this). |
| <a name="output_node_resource_group"></a> [node\_resource\_group](#output\_node\_resource\_group) | Auto-generated resource group holding node resources. |
| <a name="output_oidc_issuer_url"></a> [oidc\_issuer\_url](#output\_oidc\_issuer\_url) | OIDC issuer URL for Workload Identity federated credentials. |
| <a name="output_private_fqdn"></a> [private\_fqdn](#output\_private\_fqdn) | Private FQDN of the API server (private clusters only). |
<!-- END_TF_DOCS -->
