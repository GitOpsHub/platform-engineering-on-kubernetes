# gke — Google Kubernetes Engine cluster

Native-resource GKE module:

- **Private nodes** always; private control-plane endpoint optional (`enable_private_endpoint`), master authorized networks.
- **Workload Identity** enabled (`PROJECT.svc.id.goog`) and node metadata set to `GKE_METADATA`.
- **Dataplane V2** (eBPF/Cilium) by default, giving NetworkPolicy enforcement without a self-managed CNI.
- Dedicated least-privilege node service account (not the Compute default SA), Shielded Nodes with Secure Boot.
- Optional application-layer Secrets encryption with Cloud KMS (`database_encryption_key`).
- Release channel upgrades + weekend maintenance window; Managed Prometheus on.
- `deletion_protection = true` by default; set false for labs.

## Usage

```hcl
module "gke" {
  source = "../gke"

  project_id          = "my-project"
  cluster_name        = "platform-dev"
  location            = "europe-west1"
  network             = module.network.network_id
  subnetwork          = module.network.subnetwork_id
  pods_range_name     = module.network.pods_range_name
  services_range_name = module.network.services_range_name

  master_authorized_networks = [{ cidr_block = "203.0.113.0/24", display_name = "vpn" }]
}
```

A complete root module using this module lives in [`examples/`](../examples).

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.6.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 7.0, < 9.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_google"></a> [google](#provider\_google) | >= 7.0, < 9.0 |

## Resources

| Name | Type |
| ---- | ---- |
| [google_container_cluster.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/container_cluster) | resource |
| [google_container_node_pool.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/container_node_pool) | resource |
| [google_project_iam_member.nodes](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_service_account.nodes](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_account) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_cluster_name"></a> [cluster\_name](#input\_cluster\_name) | Name of the GKE cluster. | `string` | n/a | yes |
| <a name="input_location"></a> [location](#input\_location) | Region (regional, HA control plane - recommended) or zone (zonal) for the cluster. | `string` | n/a | yes |
| <a name="input_network"></a> [network](#input\_network) | VPC network self-link or ID (from networking/gcp). | `string` | n/a | yes |
| <a name="input_pods_range_name"></a> [pods\_range\_name](#input\_pods\_range\_name) | Name of the subnet secondary range for Pods. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID for the cluster. | `string` | n/a | yes |
| <a name="input_services_range_name"></a> [services\_range\_name](#input\_services\_range\_name) | Name of the subnet secondary range for Services. | `string` | n/a | yes |
| <a name="input_subnetwork"></a> [subnetwork](#input\_subnetwork) | Subnetwork self-link or ID (from networking/gcp). | `string` | n/a | yes |
| <a name="input_database_encryption_key"></a> [database\_encryption\_key](#input\_database\_encryption\_key) | Cloud KMS key ID (projects/.../cryptoKeys/...) for application-layer Secrets encryption. Null uses Google-managed encryption. | `string` | `null` | no |
| <a name="input_deletion_protection"></a> [deletion\_protection](#input\_deletion\_protection) | Prevent Terraform from destroying the cluster. Set false for labs/ephemeral clusters. | `bool` | `true` | no |
| <a name="input_enable_dataplane_v2"></a> [enable\_dataplane\_v2](#input\_enable\_dataplane\_v2) | Use GKE Dataplane V2 (eBPF/Cilium-based; built-in NetworkPolicy enforcement). Set false to self-manage a CNI. | `bool` | `true` | no |
| <a name="input_enable_private_endpoint"></a> [enable\_private\_endpoint](#input\_enable\_private\_endpoint) | Make the control plane reachable only on its private IP. Nodes are always private. | `bool` | `false` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | GCP resource labels applied to the cluster and nodes. | `map(string)` | `{}` | no |
| <a name="input_master_authorized_networks"></a> [master\_authorized\_networks](#input\_master\_authorized\_networks) | CIDRs allowed to reach the control plane endpoint. | <pre>list(object({<br/>    cidr_block   = string<br/>    display_name = string<br/>  }))</pre> | `[]` | no |
| <a name="input_master_ipv4_cidr_block"></a> [master\_ipv4\_cidr\_block](#input\_master\_ipv4\_cidr\_block) | /28 range for the control-plane private endpoint. | `string` | `"172.16.0.0/28"` | no |
| <a name="input_min_master_version"></a> [min\_master\_version](#input\_min\_master\_version) | Optional minimum control-plane version (e.g. "1.34"). Null lets the release channel decide. | `string` | `null` | no |
| <a name="input_node_pools"></a> [node\_pools](#input\_node\_pools) | Node pools keyed by name. Each entry:<br/>  machine\_type, min\_count, max\_count (per zone), disk\_size\_gb, disk\_type, spot,<br/>  labels (map), taints (list of { key, value, effect } with effect NO\_SCHEDULE \| PREFER\_NO\_SCHEDULE \| NO\_EXECUTE) | <pre>map(object({<br/>    machine_type = optional(string, "e2-standard-4")<br/>    min_count    = optional(number, 1)<br/>    max_count    = optional(number, 3)<br/>    disk_size_gb = optional(number, 100)<br/>    disk_type    = optional(string, "pd-balanced")<br/>    spot         = optional(bool, false)<br/>    labels       = optional(map(string), {})<br/>    taints = optional(list(object({<br/>      key    = string<br/>      value  = string<br/>      effect = string<br/>    })), [])<br/>  }))</pre> | <pre>{<br/>  "system": {}<br/>}</pre> | no |
| <a name="input_release_channel"></a> [release\_channel](#input\_release\_channel) | GKE release channel: RAPID, REGULAR, STABLE or EXTENDED. | `string` | `"REGULAR"` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_cluster_ca_certificate"></a> [cluster\_ca\_certificate](#output\_cluster\_ca\_certificate) | Base64-encoded cluster CA certificate. |
| <a name="output_cluster_endpoint"></a> [cluster\_endpoint](#output\_cluster\_endpoint) | Control-plane endpoint (IP). Prefix with https:// for clients. |
| <a name="output_cluster_id"></a> [cluster\_id](#output\_cluster\_id) | ID of the GKE cluster. |
| <a name="output_cluster_name"></a> [cluster\_name](#output\_cluster\_name) | Name of the GKE cluster. |
| <a name="output_location"></a> [location](#output\_location) | Location (region or zone) of the cluster. |
| <a name="output_node_service_account_email"></a> [node\_service\_account\_email](#output\_node\_service\_account\_email) | Email of the node service account. |
| <a name="output_workload_identity_pool"></a> [workload\_identity\_pool](#output\_workload\_identity\_pool) | Workload Identity pool (PROJECT\_ID.svc.id.goog) for binding KSAs to GSAs. |
<!-- END_TF_DOCS -->
