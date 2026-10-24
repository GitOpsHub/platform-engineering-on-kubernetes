# networking/azure — VNet for AKS

Creates a VNet, an AKS node subnet (default outbound access disabled) with an NSG, a Standard NAT gateway
with a static public IP for egress, and an optional delegated subnet for API-server VNet integration.
Designed for Azure CNI Overlay, so Pods do not consume VNet addresses.

## Usage

```hcl
module "network" {
  source = "../networking/azure"

  name                = "platform-dev"
  resource_group_name = azurerm_resource_group.this.name
  location            = "westeurope"
}
```

A complete root module using this module lives in [`examples/`](../../examples).

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
| [azurerm_nat_gateway.this](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/nat_gateway) | resource |
| [azurerm_nat_gateway_public_ip_association.this](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/nat_gateway_public_ip_association) | resource |
| [azurerm_network_security_group.nodes](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/network_security_group) | resource |
| [azurerm_public_ip.nat](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/public_ip) | resource |
| [azurerm_subnet.api_server](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/subnet) | resource |
| [azurerm_subnet.nodes](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/subnet) | resource |
| [azurerm_subnet_nat_gateway_association.nodes](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/subnet_nat_gateway_association) | resource |
| [azurerm_subnet_network_security_group_association.nodes](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/subnet_network_security_group_association) | resource |
| [azurerm_virtual_network.this](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/virtual_network) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_location"></a> [location](#input\_location) | Azure region, e.g. westeurope. | `string` | n/a | yes |
| <a name="input_name"></a> [name](#input\_name) | Name prefix for the VNet, subnets, NAT gateway and NSG. | `string` | n/a | yes |
| <a name="input_resource_group_name"></a> [resource\_group\_name](#input\_resource\_group\_name) | Existing resource group to create the network in. | `string` | n/a | yes |
| <a name="input_address_space"></a> [address\_space](#input\_address\_space) | Address space of the virtual network. | `list(string)` | <pre>[<br/>  "10.30.0.0/16"<br/>]</pre> | no |
| <a name="input_api_server_subnet_cidr"></a> [api\_server\_subnet\_cidr](#input\_api\_server\_subnet\_cidr) | Optional CIDR for an API-server VNet-integration subnet (delegated to Microsoft.ContainerService/managedClusters). Null to skip. | `string` | `null` | no |
| <a name="input_nat_idle_timeout_minutes"></a> [nat\_idle\_timeout\_minutes](#input\_nat\_idle\_timeout\_minutes) | Idle timeout for the NAT gateway. | `number` | `4` | no |
| <a name="input_nodes_subnet_cidr"></a> [nodes\_subnet\_cidr](#input\_nodes\_subnet\_cidr) | CIDR of the AKS node subnet. With Azure CNI Overlay, Pods do not consume VNet IPs, so a /22 is plenty. | `string` | `"10.30.0.0/22"` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Tags applied to all resources. | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_api_server_subnet_id"></a> [api\_server\_subnet\_id](#output\_api\_server\_subnet\_id) | ID of the API-server VNet-integration subnet, or null. |
| <a name="output_nat_public_ip"></a> [nat\_public\_ip](#output\_nat\_public\_ip) | Public egress IP of the NAT gateway. |
| <a name="output_nodes_subnet_id"></a> [nodes\_subnet\_id](#output\_nodes\_subnet\_id) | ID of the AKS node subnet. |
| <a name="output_vnet_id"></a> [vnet\_id](#output\_vnet\_id) | ID of the virtual network. |
| <a name="output_vnet_name"></a> [vnet\_name](#output\_vnet\_name) | Name of the virtual network. |
<!-- END_TF_DOCS -->
