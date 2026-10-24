variable "name" {
  description = "Name prefix for the VNet, subnets, NAT gateway and NSG."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$", var.name))
    error_message = "name must be 3-40 chars of lowercase letters, digits and hyphens."
  }
}

variable "resource_group_name" {
  description = "Existing resource group to create the network in."
  type        = string
}

variable "location" {
  description = "Azure region, e.g. westeurope."
  type        = string
}

variable "address_space" {
  description = "Address space of the virtual network."
  type        = list(string)
  default     = ["10.30.0.0/16"]

  validation {
    condition     = alltrue([for c in var.address_space : can(cidrhost(c, 0))])
    error_message = "address_space entries must be valid IPv4 CIDRs."
  }
}

variable "nodes_subnet_cidr" {
  description = "CIDR of the AKS node subnet. With Azure CNI Overlay, Pods do not consume VNet IPs, so a /22 is plenty."
  type        = string
  default     = "10.30.0.0/22"

  validation {
    condition     = can(cidrhost(var.nodes_subnet_cidr, 0))
    error_message = "nodes_subnet_cidr must be a valid IPv4 CIDR."
  }
}

variable "api_server_subnet_cidr" {
  description = "Optional CIDR for an API-server VNet-integration subnet (delegated to Microsoft.ContainerService/managedClusters). Null to skip."
  type        = string
  default     = null
}

variable "nat_idle_timeout_minutes" {
  description = "Idle timeout for the NAT gateway."
  type        = number
  default     = 4

  validation {
    condition     = var.nat_idle_timeout_minutes >= 4 && var.nat_idle_timeout_minutes <= 120
    error_message = "nat_idle_timeout_minutes must be between 4 and 120."
  }
}

variable "tags" {
  description = "Tags applied to all resources."
  type        = map(string)
  default     = {}
}
