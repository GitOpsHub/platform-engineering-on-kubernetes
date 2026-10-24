# Complete AKS platform: VNet + NAT -> AKS (Azure CNI Overlay + Cilium, Workload Identity) -> add-ons.

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 5.11"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 3.1"
    }
  }

  # backend "azurerm" {
  #   resource_group_name  = "rg-tfstate"
  #   storage_account_name = "mycompanytfstate"
  #   container_name       = "tfstate"
  #   key                  = "platform/aks-complete.tfstate"
  #   use_azuread_auth     = true
  # }
}

provider "azurerm" {
  features {}
  subscription_id = var.subscription_id
}

resource "azurerm_resource_group" "this" {
  name     = "rg-${var.cluster_name}"
  location = var.location

  tags = {
    "platform.mycompany.io/environment" = var.environment
  }
}

module "network" {
  source = "../../networking/azure"

  name                = var.cluster_name
  resource_group_name = azurerm_resource_group.this.name
  location            = azurerm_resource_group.this.location
}

module "aks" {
  source = "../../aks"

  cluster_name        = var.cluster_name
  resource_group_name = azurerm_resource_group.this.name
  location            = azurerm_resource_group.this.location
  kubernetes_version  = var.kubernetes_version
  sku_tier            = var.environment == "prod" ? "Standard" : "Free"
  nodes_subnet_id     = module.network.nodes_subnet_id

  authorized_ip_ranges   = var.api_allowed_cidrs
  admin_group_object_ids = var.admin_group_object_ids
  network_data_plane     = "cilium"

  user_node_pools = {
    workloads = {
      vm_size   = "Standard_D8s_v5"
      min_count = 1
      max_count = 10
    }
  }

  # NAT gateway must be associated with the subnet before AKS uses userAssignedNATGateway.
  depends_on = [module.network]
}

# local_account_disabled = true, so Helm authenticates via Entra ID using kubelogin.
provider "helm" {
  kubernetes = {
    host                   = module.aks.kube_config_host
    cluster_ca_certificate = base64decode(module.aks.cluster_ca_certificate)
    exec = {
      api_version = "client.authentication.k8s.io/v1beta1"
      command     = "kubelogin"
      args        = ["get-token", "--login", "azurecli", "--server-id", "6dae42f8-4368-4678-94ff-3960e28e3630"]
    }
  }
}

module "addons" {
  source = "../../addons"

  cluster_name = module.aks.cluster_name
  cloud        = "aks"

  # AKS already runs Cilium (network_data_plane) and metrics-server.
  addons = {
    cilium                = { enabled = false }
    cert_manager          = { enabled = true }
    external_secrets      = { enabled = true }
    metrics_server        = { enabled = false }
    kube_prometheus_stack = { enabled = var.enable_monitoring }
  }
}
