# Complete EKS platform: VPC -> EKS -> platform add-ons.
# Usage: see ../README.md. Never commit real state; configure a remote backend in backend.tf.

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.70"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.1"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 3.1"
    }
  }

  # backend "s3" {
  #   bucket       = "mycompany-tfstate"
  #   key          = "platform/eks-complete.tfstate"
  #   region       = "eu-west-1"
  #   use_lockfile = true
  #   encrypt      = true
  # }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      "platform.mycompany.io/environment" = var.environment
      "platform.mycompany.io/owner"       = "platform-team"
    }
  }
}

module "network" {
  source = "../../networking/aws"

  name               = var.cluster_name
  cidr_block         = "10.0.0.0/16"
  az_count           = 3
  single_nat_gateway = var.environment != "prod"
}

module "eks" {
  source = "../../eks"

  cluster_name       = var.cluster_name
  kubernetes_version = var.kubernetes_version
  vpc_id             = module.network.vpc_id
  subnet_ids         = module.network.private_subnet_ids

  endpoint_public_access = length(var.api_allowed_cidrs) > 0
  public_access_cidrs    = var.api_allowed_cidrs
  admin_principal_arns   = var.admin_principal_arns

  node_groups = {
    system = {
      instance_types = ["m6i.large"]
      min_size       = 2
      max_size       = 4
      desired_size   = 2
      labels         = { "platform.mycompany.io/workload-class" = "system" }
    }
    workloads = {
      instance_types = ["m6i.xlarge", "m6a.xlarge"]
      capacity_type  = "SPOT"
      min_size       = 1
      max_size       = 10
      desired_size   = 2
    }
  }
}

# Helm talks to the new cluster with short-lived tokens from `aws eks get-token`.
provider "helm" {
  kubernetes = {
    host                   = module.eks.cluster_endpoint
    cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)
    exec = {
      api_version = "client.authentication.k8s.io/v1beta1"
      command     = "aws"
      args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name, "--region", var.region]
    }
  }
}

module "addons" {
  source = "../../addons"

  cluster_name = module.eks.cluster_name
  cloud        = "eks"

  addons = {
    cert_manager          = { enabled = true }
    external_secrets      = { enabled = true }
    metrics_server        = { enabled = true }
    kube_prometheus_stack = { enabled = var.enable_monitoring }
  }
}
