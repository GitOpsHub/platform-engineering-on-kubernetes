# Complete GKE platform: VPC -> private GKE (Dataplane V2, Workload Identity) -> add-ons.

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 8.8"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 3.1"
    }
  }

  # backend "gcs" {
  #   bucket = "mycompany-tfstate"
  #   prefix = "platform/gke-complete"
  # }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

module "network" {
  source = "../../networking/gcp"

  project_id = var.project_id
  name       = var.cluster_name
  region     = var.region
}

module "gke" {
  source = "../../gke"

  project_id          = var.project_id
  cluster_name        = var.cluster_name
  location            = var.region
  network             = module.network.network_id
  subnetwork          = module.network.subnetwork_id
  pods_range_name     = module.network.pods_range_name
  services_range_name = module.network.services_range_name

  release_channel            = "REGULAR"
  enable_dataplane_v2        = true
  master_authorized_networks = var.master_authorized_networks
  deletion_protection        = var.deletion_protection

  node_pools = {
    system = {
      machine_type = "e2-standard-4"
      min_count    = 1
      max_count    = 3
    }
    workloads = {
      machine_type = "e2-standard-8"
      min_count    = 0
      max_count    = 5
      spot         = true
    }
  }
}

data "google_client_config" "current" {}

provider "helm" {
  kubernetes = {
    host                   = "https://${module.gke.cluster_endpoint}"
    token                  = data.google_client_config.current.access_token
    cluster_ca_certificate = base64decode(module.gke.cluster_ca_certificate)
  }
}

module "addons" {
  source = "../../addons"

  cluster_name = module.gke.cluster_name
  cloud        = "gke"

  # Dataplane V2 already provides Cilium; GKE already runs metrics-server.
  addons = {
    cilium                = { enabled = false }
    cert_manager          = { enabled = true }
    external_secrets      = { enabled = true }
    metrics_server        = { enabled = false }
    kube_prometheus_stack = { enabled = var.enable_monitoring }
  }
}
