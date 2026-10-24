# networking/gcp — VPC for GKE

Creates a custom-mode VPC, one regional node subnet with secondary ranges for Pods and Services
(VPC-native GKE), Private Google Access, flow logs, a Cloud Router + Cloud NAT for egress from private nodes,
and a firewall rule for Google load-balancer health checks.

## Usage

```hcl
module "network" {
  source = "../networking/gcp"

  project_id = "my-project"
  name       = "platform-dev"
  region     = "europe-west1"
}
```

A complete root module using this module lives in [`examples/`](../../examples).

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
| [google_compute_firewall.health_checks](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_firewall) | resource |
| [google_compute_network.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_network) | resource |
| [google_compute_router.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_router) | resource |
| [google_compute_router_nat.this](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_router_nat) | resource |
| [google_compute_subnetwork.nodes](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_subnetwork) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_name"></a> [name](#input\_name) | Name prefix for the VPC, subnet, router and NAT. | `string` | n/a | yes |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | GCP project ID that owns the network. | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Region for the subnet, Cloud Router and Cloud NAT. | `string` | n/a | yes |
| <a name="input_enable_flow_logs"></a> [enable\_flow\_logs](#input\_enable\_flow\_logs) | Enable VPC flow logs on the subnet. | `bool` | `true` | no |
| <a name="input_nat_log_filter"></a> [nat\_log\_filter](#input\_nat\_log\_filter) | Cloud NAT logging filter: ERRORS\_ONLY, TRANSLATIONS\_ONLY or ALL. Set to null to disable NAT logging. | `string` | `"ERRORS_ONLY"` | no |
| <a name="input_pods_cidr"></a> [pods\_cidr](#input\_pods\_cidr) | Secondary range used for GKE Pod IPs (VPC-native / alias IPs). | `string` | `"10.20.0.0/14"` | no |
| <a name="input_services_cidr"></a> [services\_cidr](#input\_services\_cidr) | Secondary range used for GKE Service ClusterIPs. | `string` | `"10.24.0.0/20"` | no |
| <a name="input_subnet_cidr"></a> [subnet\_cidr](#input\_subnet\_cidr) | Primary IPv4 range for the node subnet. | `string` | `"10.10.0.0/20"` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_network_id"></a> [network\_id](#output\_network\_id) | Self-link style ID of the VPC network. |
| <a name="output_network_name"></a> [network\_name](#output\_network\_name) | Name of the VPC network. |
| <a name="output_pods_range_name"></a> [pods\_range\_name](#output\_pods\_range\_name) | Name of the secondary range for Pods (pass to the gke module). |
| <a name="output_services_range_name"></a> [services\_range\_name](#output\_services\_range\_name) | Name of the secondary range for Services (pass to the gke module). |
| <a name="output_subnetwork_id"></a> [subnetwork\_id](#output\_subnetwork\_id) | ID of the node subnetwork. |
| <a name="output_subnetwork_name"></a> [subnetwork\_name](#output\_subnetwork\_name) | Name of the node subnetwork. |
<!-- END_TF_DOCS -->
