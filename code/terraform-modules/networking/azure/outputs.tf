output "vnet_id" {
  description = "ID of the virtual network."
  value       = azurerm_virtual_network.this.id
}

output "vnet_name" {
  description = "Name of the virtual network."
  value       = azurerm_virtual_network.this.name
}

output "nodes_subnet_id" {
  description = "ID of the AKS node subnet."
  value       = azurerm_subnet.nodes.id
}

output "api_server_subnet_id" {
  description = "ID of the API-server VNet-integration subnet, or null."
  value       = try(azurerm_subnet.api_server[0].id, null)
}

output "nat_public_ip" {
  description = "Public egress IP of the NAT gateway."
  value       = azurerm_public_ip.nat.ip_address
}
