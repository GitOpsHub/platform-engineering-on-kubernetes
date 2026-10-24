variable "project_id" {
  description = "GCP project ID."
  type        = string
}

variable "region" {
  description = "GCP region (regional cluster)."
  type        = string
  default     = "europe-west1"
}

variable "cluster_name" {
  description = "GKE cluster name."
  type        = string
  default     = "platform-dev"
}

variable "master_authorized_networks" {
  description = "CIDRs allowed to reach the control plane."
  type = list(object({
    cidr_block   = string
    display_name = string
  }))
  default = []
}

variable "deletion_protection" {
  description = "Protect the cluster from terraform destroy."
  type        = bool
  default     = true
}

variable "enable_monitoring" {
  description = "Install kube-prometheus-stack (GKE Managed Prometheus is also enabled)."
  type        = bool
  default     = false
}
