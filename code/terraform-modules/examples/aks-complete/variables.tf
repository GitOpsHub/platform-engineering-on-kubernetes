variable "subscription_id" {
  description = "Azure subscription ID (required by azurerm 4.x+)."
  type        = string
}

variable "location" {
  description = "Azure region."
  type        = string
  default     = "westeurope"
}

variable "environment" {
  description = "Environment name (dev, staging, prod)."
  type        = string
  default     = "dev"

  validation {
    condition     = contains(["dev", "staging", "prod"], var.environment)
    error_message = "environment must be dev, staging or prod."
  }
}

variable "cluster_name" {
  description = "AKS cluster name."
  type        = string
  default     = "platform-dev"
}

variable "kubernetes_version" {
  description = "Kubernetes version; null = AKS default."
  type        = string
  default     = null
}

variable "api_allowed_cidrs" {
  description = "CIDRs allowed to reach the API server."
  type        = list(string)
  default     = []
}

variable "admin_group_object_ids" {
  description = "Entra ID group object IDs with cluster-admin."
  type        = list(string)
  default     = []
}

variable "enable_monitoring" {
  description = "Install kube-prometheus-stack."
  type        = bool
  default     = true
}
