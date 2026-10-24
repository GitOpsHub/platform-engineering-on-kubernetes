variable "name" {
  description = "Name prefix for all networking resources (usually the cluster name)."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$", var.name))
    error_message = "name must be 3-40 chars of lowercase letters, digits and hyphens."
  }
}

variable "cidr_block" {
  description = "IPv4 CIDR block for the VPC."
  type        = string
  default     = "10.0.0.0/16"

  validation {
    condition     = can(cidrhost(var.cidr_block, 0)) && tonumber(split("/", var.cidr_block)[1]) <= 20
    error_message = "cidr_block must be a valid IPv4 CIDR of /20 or larger."
  }
}

variable "az_count" {
  description = "Number of availability zones to spread subnets across."
  type        = number
  default     = 3

  validation {
    condition     = var.az_count >= 2 && var.az_count <= 6
    error_message = "az_count must be between 2 and 6 (EKS requires at least two AZs)."
  }
}

variable "private_subnet_newbits" {
  description = "Bits added to the VPC prefix for private (node/pod) subnets. 4 on a /16 gives /20 subnets."
  type        = number
  default     = 4
}

variable "public_subnet_newbits" {
  description = "Bits added to the VPC prefix for public (load balancer) subnets. 8 on a /16 gives /24 subnets."
  type        = number
  default     = 8
}

variable "single_nat_gateway" {
  description = "Use one shared NAT gateway (cheaper, not AZ-resilient) instead of one per AZ."
  type        = bool
  default     = true
}

variable "cluster_name" {
  description = "EKS cluster name used for subnet discovery tags (kubernetes.io/cluster/<name>). Defaults to var.name."
  type        = string
  default     = null
}

variable "enable_flow_logs" {
  description = "Send VPC flow logs to a CloudWatch log group."
  type        = bool
  default     = true
}

variable "flow_logs_retention_days" {
  description = "Retention in days for the VPC flow log group."
  type        = number
  default     = 30

  validation {
    condition     = contains([1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1827, 3653], var.flow_logs_retention_days)
    error_message = "flow_logs_retention_days must be a CloudWatch-supported retention value."
  }
}

variable "tags" {
  description = "Tags applied to all resources."
  type        = map(string)
  default     = {}
}
