variable "region" {
  description = "AWS region."
  type        = string
  default     = "eu-west-1"
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
  description = "EKS cluster name."
  type        = string
  default     = "platform-dev"
}

variable "kubernetes_version" {
  description = "Kubernetes minor version."
  type        = string
  default     = "1.34"
}

variable "api_allowed_cidrs" {
  description = "CIDRs allowed to reach the public API endpoint. Empty = private endpoint only (requires VPN/bastion)."
  type        = list(string)
  default     = []
}

variable "admin_principal_arns" {
  description = "IAM role/user ARNs granted cluster-admin."
  type        = list(string)
  default     = []
}

variable "enable_monitoring" {
  description = "Install kube-prometheus-stack."
  type        = bool
  default     = true
}
