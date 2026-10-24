variable "project_id" {
  description = "GCP project ID that owns the network."
  type        = string
}

variable "name" {
  description = "Name prefix for the VPC, subnet, router and NAT."
  type        = string

  validation {
    condition     = can(regex("^[a-z]([-a-z0-9]{1,38}[a-z0-9])$", var.name))
    error_message = "name must be 3-40 chars, start with a letter, and contain only lowercase letters, digits and hyphens."
  }
}

variable "region" {
  description = "Region for the subnet, Cloud Router and Cloud NAT."
  type        = string
}

variable "subnet_cidr" {
  description = "Primary IPv4 range for the node subnet."
  type        = string
  default     = "10.10.0.0/20"

  validation {
    condition     = can(cidrhost(var.subnet_cidr, 0))
    error_message = "subnet_cidr must be a valid IPv4 CIDR."
  }
}

variable "pods_cidr" {
  description = "Secondary range used for GKE Pod IPs (VPC-native / alias IPs)."
  type        = string
  default     = "10.20.0.0/14"

  validation {
    condition     = can(cidrhost(var.pods_cidr, 0))
    error_message = "pods_cidr must be a valid IPv4 CIDR."
  }
}

variable "services_cidr" {
  description = "Secondary range used for GKE Service ClusterIPs."
  type        = string
  default     = "10.24.0.0/20"

  validation {
    condition     = can(cidrhost(var.services_cidr, 0))
    error_message = "services_cidr must be a valid IPv4 CIDR."
  }
}

variable "enable_flow_logs" {
  description = "Enable VPC flow logs on the subnet."
  type        = bool
  default     = true
}

variable "nat_log_filter" {
  description = "Cloud NAT logging filter: ERRORS_ONLY, TRANSLATIONS_ONLY or ALL. Set to null to disable NAT logging."
  type        = string
  default     = "ERRORS_ONLY"

  validation {
    condition     = var.nat_log_filter == null || contains(["ERRORS_ONLY", "TRANSLATIONS_ONLY", "ALL"], coalesce(var.nat_log_filter, "ALL"))
    error_message = "nat_log_filter must be ERRORS_ONLY, TRANSLATIONS_ONLY, ALL or null."
  }
}
