variable "project_id" {
  description = "GCP project ID for the cluster."
  type        = string
}

variable "cluster_name" {
  description = "Name of the GKE cluster."
  type        = string

  validation {
    condition     = can(regex("^[a-z]([-a-z0-9]{0,38}[a-z0-9])?$", var.cluster_name))
    error_message = "cluster_name must be 1-40 chars, start with a letter, and contain only lowercase letters, digits and hyphens."
  }
}

variable "location" {
  description = "Region (regional, HA control plane - recommended) or zone (zonal) for the cluster."
  type        = string
}

variable "network" {
  description = "VPC network self-link or ID (from networking/gcp)."
  type        = string
}

variable "subnetwork" {
  description = "Subnetwork self-link or ID (from networking/gcp)."
  type        = string
}

variable "pods_range_name" {
  description = "Name of the subnet secondary range for Pods."
  type        = string
}

variable "services_range_name" {
  description = "Name of the subnet secondary range for Services."
  type        = string
}

variable "release_channel" {
  description = "GKE release channel: RAPID, REGULAR, STABLE or EXTENDED."
  type        = string
  default     = "REGULAR"

  validation {
    condition     = contains(["RAPID", "REGULAR", "STABLE", "EXTENDED"], var.release_channel)
    error_message = "release_channel must be RAPID, REGULAR, STABLE or EXTENDED."
  }
}

variable "min_master_version" {
  description = "Optional minimum control-plane version (e.g. \"1.34\"). Null lets the release channel decide."
  type        = string
  default     = null
}

variable "enable_private_endpoint" {
  description = "Make the control plane reachable only on its private IP. Nodes are always private."
  type        = bool
  default     = false
}

variable "master_ipv4_cidr_block" {
  description = "/28 range for the control-plane private endpoint."
  type        = string
  default     = "172.16.0.0/28"

  validation {
    condition     = can(cidrhost(var.master_ipv4_cidr_block, 0)) && endswith(var.master_ipv4_cidr_block, "/28")
    error_message = "master_ipv4_cidr_block must be a /28."
  }
}

variable "master_authorized_networks" {
  description = "CIDRs allowed to reach the control plane endpoint."
  type = list(object({
    cidr_block   = string
    display_name = string
  }))
  default = []
}

variable "enable_dataplane_v2" {
  description = "Use GKE Dataplane V2 (eBPF/Cilium-based; built-in NetworkPolicy enforcement). Set false to self-manage a CNI."
  type        = bool
  default     = true
}

variable "database_encryption_key" {
  description = "Cloud KMS key ID (projects/.../cryptoKeys/...) for application-layer Secrets encryption. Null uses Google-managed encryption."
  type        = string
  default     = null
}

variable "deletion_protection" {
  description = "Prevent Terraform from destroying the cluster. Set false for labs/ephemeral clusters."
  type        = bool
  default     = true
}

variable "node_pools" {
  description = <<-EOT
    Node pools keyed by name. Each entry:
      machine_type, min_count, max_count (per zone), disk_size_gb, disk_type, spot,
      labels (map), taints (list of { key, value, effect } with effect NO_SCHEDULE | PREFER_NO_SCHEDULE | NO_EXECUTE)
  EOT
  type = map(object({
    machine_type = optional(string, "e2-standard-4")
    min_count    = optional(number, 1)
    max_count    = optional(number, 3)
    disk_size_gb = optional(number, 100)
    disk_type    = optional(string, "pd-balanced")
    spot         = optional(bool, false)
    labels       = optional(map(string), {})
    taints = optional(list(object({
      key    = string
      value  = string
      effect = string
    })), [])
  }))
  default = {
    system = {}
  }

  validation {
    condition     = alltrue([for np in values(var.node_pools) : np.min_count <= np.max_count])
    error_message = "node_pools[*].min_count must be <= max_count."
  }
}

variable "labels" {
  description = "GCP resource labels applied to the cluster and nodes."
  type        = map(string)
  default     = {}
}
