variable "cluster_name" {
  description = "Cluster name; used for Cilium cluster.name and labels."
  type        = string
}

variable "cloud" {
  description = "Target cloud (eks, gke, aks or generic); selects sensible per-cloud defaults for Cilium."
  type        = string
  default     = "generic"

  validation {
    condition     = contains(["eks", "gke", "aks", "generic"], var.cloud)
    error_message = "cloud must be eks, gke, aks or generic."
  }
}

variable "addons" {
  description = <<-EOT
    Add-ons to install. Each key toggles one Helm release:
      enabled   - install or not
      version   - pinned chart version
      namespace - target namespace (created if missing)
      values    - list of extra YAML values documents merged after the module defaults
  EOT
  type = object({
    cilium = optional(object({
      enabled   = optional(bool, false)
      version   = optional(string, "1.20.4")
      namespace = optional(string, "kube-system")
      values    = optional(list(string), [])
    }), {})
    cert_manager = optional(object({
      enabled   = optional(bool, true)
      version   = optional(string, "v1.21.3")
      namespace = optional(string, "cert-manager")
      values    = optional(list(string), [])
    }), {})
    external_secrets = optional(object({
      enabled   = optional(bool, true)
      version   = optional(string, "2.14.0")
      namespace = optional(string, "external-secrets")
      values    = optional(list(string), [])
    }), {})
    metrics_server = optional(object({
      enabled   = optional(bool, true)
      version   = optional(string, "3.14.0")
      namespace = optional(string, "kube-system")
      values    = optional(list(string), [])
    }), {})
    kube_prometheus_stack = optional(object({
      enabled   = optional(bool, false)
      version   = optional(string, "92.11.0")
      namespace = optional(string, "monitoring")
      values    = optional(list(string), [])
    }), {})
  })
  default = {}
}

variable "external_secrets_service_account_annotations" {
  description = "Annotations for the external-secrets controller ServiceAccount (e.g. eks.amazonaws.com/role-arn, iam.gke.io/gcp-service-account, azure.workload.identity/client-id)."
  type        = map(string)
  default     = {}
}

variable "cert_manager_service_account_annotations" {
  description = "Annotations for the cert-manager controller ServiceAccount (cloud identity for DNS-01 solvers)."
  type        = map(string)
  default     = {}
}

variable "eks_api_server_host" {
  description = "EKS only: API server hostname (no scheme) for Cilium kube-proxy replacement. Required when cloud = eks and Cilium is enabled."
  type        = string
  default     = null
}

variable "atomic" {
  description = "Roll back a release automatically if the install/upgrade fails."
  type        = bool
  default     = true
}

variable "timeout_seconds" {
  description = "Helm operation timeout per release."
  type        = number
  default     = 600
}
