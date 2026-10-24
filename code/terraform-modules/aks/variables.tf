variable "cluster_name" {
  description = "Name of the AKS cluster."
  type        = string

  validation {
    condition     = can(regex("^[a-zA-Z0-9][a-zA-Z0-9-]{0,61}[a-zA-Z0-9]$", var.cluster_name))
    error_message = "cluster_name must be 2-63 chars of letters, digits and hyphens, not starting or ending with a hyphen."
  }
}

variable "resource_group_name" {
  description = "Resource group for the cluster."
  type        = string
}

variable "location" {
  description = "Azure region."
  type        = string
}

variable "kubernetes_version" {
  description = "Kubernetes version (e.g. \"1.34\"). Null uses the AKS default; patch upgrades follow automatic_upgrade_channel."
  type        = string
  default     = null

  validation {
    condition     = var.kubernetes_version == null || can(regex("^1\\.(2[89]|[3-9][0-9])(\\.[0-9]+)?$", coalesce(var.kubernetes_version, "1.30")))
    error_message = "kubernetes_version must be >= 1.28, e.g. \"1.34\"."
  }
}

variable "automatic_upgrade_channel" {
  description = "AKS auto-upgrade channel: none, patch, rapid, stable or node-image."
  type        = string
  default     = "patch"

  validation {
    condition     = contains(["none", "patch", "rapid", "stable", "node-image"], var.automatic_upgrade_channel)
    error_message = "automatic_upgrade_channel must be none, patch, rapid, stable or node-image."
  }
}

variable "sku_tier" {
  description = "AKS control-plane tier: Free (labs), Standard (uptime SLA) or Premium (LTS)."
  type        = string
  default     = "Standard"

  validation {
    condition     = contains(["Free", "Standard", "Premium"], var.sku_tier)
    error_message = "sku_tier must be Free, Standard or Premium."
  }
}

variable "nodes_subnet_id" {
  description = "Subnet ID for node NICs (from networking/azure)."
  type        = string
}

variable "private_cluster_enabled" {
  description = "Expose the API server only on a private endpoint inside the VNet."
  type        = bool
  default     = false
}

variable "authorized_ip_ranges" {
  description = "CIDRs allowed to reach a public API server. Ignored when private_cluster_enabled = true."
  type        = list(string)
  default     = []
}

variable "admin_group_object_ids" {
  description = "Entra ID group object IDs granted cluster admin (Azure RBAC for Kubernetes)."
  type        = list(string)
  default     = []
}

variable "network_data_plane" {
  description = "Data plane: \"cilium\" (Azure CNI powered by Cilium) or \"azure\"."
  type        = string
  default     = "cilium"

  validation {
    condition     = contains(["cilium", "azure"], var.network_data_plane)
    error_message = "network_data_plane must be cilium or azure."
  }
}

variable "pod_cidr" {
  description = "Overlay Pod CIDR (Azure CNI Overlay; not routable in the VNet)."
  type        = string
  default     = "192.168.0.0/16"
}

variable "service_cidr" {
  description = "Service CIDR; must not overlap the VNet."
  type        = string
  default     = "172.20.0.0/16"
}

variable "dns_service_ip" {
  description = "ClusterIP for CoreDNS; must be inside service_cidr."
  type        = string
  default     = "172.20.0.10"
}

variable "system_node_pool" {
  description = "Settings for the default (system) node pool."
  type = object({
    vm_size         = optional(string, "Standard_D4s_v5")
    min_count       = optional(number, 2)
    max_count       = optional(number, 5)
    os_disk_size_gb = optional(number, 128)
    zones           = optional(list(string), ["1", "2", "3"])
  })
  default = {}
}

variable "user_node_pools" {
  description = "Additional user node pools keyed by name (max 12 lowercase alphanumeric chars)."
  type = map(object({
    vm_size         = optional(string, "Standard_D4s_v5")
    min_count       = optional(number, 1)
    max_count       = optional(number, 5)
    os_disk_size_gb = optional(number, 128)
    zones           = optional(list(string), ["1", "2", "3"])
    spot            = optional(bool, false)
    labels          = optional(map(string), {})
    taints          = optional(list(string), [])
  }))
  default = {}

  validation {
    condition     = alltrue([for k in keys(var.user_node_pools) : can(regex("^[a-z][a-z0-9]{0,11}$", k))])
    error_message = "user_node_pools keys must be 1-12 lowercase alphanumeric chars starting with a letter."
  }
}

variable "log_analytics_workspace_id" {
  description = "Optional Log Analytics workspace ID for Container Insights. Null disables the oms_agent."
  type        = string
  default     = null
}

variable "tags" {
  description = "Tags applied to all resources."
  type        = map(string)
  default     = {}
}
