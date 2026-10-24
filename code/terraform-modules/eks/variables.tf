variable "cluster_name" {
  description = "Name of the EKS cluster."
  type        = string

  validation {
    condition     = can(regex("^[a-zA-Z][a-zA-Z0-9-]{0,99}$", var.cluster_name))
    error_message = "cluster_name must start with a letter and contain only letters, digits and hyphens (max 100 chars)."
  }
}

variable "kubernetes_version" {
  description = "Kubernetes minor version for the control plane, e.g. \"1.34\". The repo targets 1.28+."
  type        = string
  default     = "1.34"

  validation {
    condition     = can(regex("^1\\.(2[89]|[3-9][0-9])$", var.kubernetes_version))
    error_message = "kubernetes_version must be a minor version string >= 1.28, e.g. \"1.34\"."
  }
}

variable "vpc_id" {
  description = "VPC ID the cluster runs in (from the networking/aws module)."
  type        = string
}

variable "subnet_ids" {
  description = "Private subnet IDs for the control-plane ENIs and the node groups (at least two AZs)."
  type        = list(string)

  validation {
    condition     = length(var.subnet_ids) >= 2
    error_message = "EKS requires subnets in at least two availability zones."
  }
}

variable "endpoint_public_access" {
  description = "Expose the Kubernetes API publicly. The private endpoint is always enabled; set false for a fully private cluster."
  type        = bool
  default     = false
}

variable "public_access_cidrs" {
  description = "CIDRs allowed to reach the public API endpoint (only used when endpoint_public_access = true)."
  type        = list(string)
  default     = []

  validation {
    condition     = alltrue([for c in var.public_access_cidrs : can(cidrhost(c, 0)) && c != "0.0.0.0/0"])
    error_message = "public_access_cidrs must be valid CIDRs and must not be 0.0.0.0/0; restrict API access to known ranges."
  }
}

variable "enabled_log_types" {
  description = "Control-plane log types shipped to CloudWatch."
  type        = list(string)
  default     = ["api", "audit", "authenticator"]

  validation {
    condition     = alltrue([for t in var.enabled_log_types : contains(["api", "audit", "authenticator", "controllerManager", "scheduler"], t)])
    error_message = "enabled_log_types entries must be one of api, audit, authenticator, controllerManager, scheduler."
  }
}

variable "log_retention_days" {
  description = "Retention for the control-plane CloudWatch log group."
  type        = number
  default     = 90
}

variable "kms_key_arn" {
  description = "Existing KMS key ARN for envelope encryption of Kubernetes Secrets. If null, the module creates a dedicated key."
  type        = string
  default     = null
}

variable "admin_principal_arns" {
  description = "IAM principal ARNs (roles/users) granted cluster-admin via EKS access entries."
  type        = list(string)
  default     = []
}

variable "node_groups" {
  description = <<-EOT
    Managed node groups keyed by name. Each entry:
      instance_types - list of EC2 instance types
      capacity_type  - ON_DEMAND or SPOT
      min_size / max_size / desired_size
      disk_size_gib  - root volume size
      ami_type       - e.g. AL2023_x86_64_STANDARD, AL2023_ARM_64_STANDARD, BOTTLEROCKET_x86_64
      labels         - Kubernetes node labels
      taints         - list of { key, value, effect } (effect: NO_SCHEDULE | NO_EXECUTE | PREFER_NO_SCHEDULE)
  EOT
  type = map(object({
    instance_types = optional(list(string), ["m6i.large"])
    capacity_type  = optional(string, "ON_DEMAND")
    min_size       = optional(number, 2)
    max_size       = optional(number, 5)
    desired_size   = optional(number, 2)
    disk_size_gib  = optional(number, 50)
    ami_type       = optional(string, "AL2023_x86_64_STANDARD")
    labels         = optional(map(string), {})
    taints = optional(list(object({
      key    = string
      value  = optional(string)
      effect = string
    })), [])
  }))
  default = {
    system = {}
  }

  validation {
    condition     = alltrue([for ng in values(var.node_groups) : contains(["ON_DEMAND", "SPOT"], ng.capacity_type)])
    error_message = "node_groups[*].capacity_type must be ON_DEMAND or SPOT."
  }

  validation {
    condition     = alltrue([for ng in values(var.node_groups) : ng.min_size <= ng.desired_size && ng.desired_size <= ng.max_size])
    error_message = "node_groups[*] must satisfy min_size <= desired_size <= max_size."
  }
}

variable "cluster_addons" {
  description = "EKS managed add-ons to install, keyed by add-on name. Value is an optional pinned version (null = default for the cluster version)."
  type        = map(string)
  default = {
    "vpc-cni"                = null
    "coredns"                = null
    "kube-proxy"             = null
    "eks-pod-identity-agent" = null
    "aws-ebs-csi-driver"     = null
  }
}

variable "tags" {
  description = "Tags applied to all resources."
  type        = map(string)
  default     = {}
}
