data "azurerm_client_config" "current" {}

locals {
  tags = merge(var.tags, {
    "platform.mycompany.io/managed-by" = "terraform"
    "platform.mycompany.io/module"     = "aks"
    "platform.mycompany.io/cluster"    = var.cluster_name
  })
}

# User-assigned identity for the control plane, so role assignments
# (e.g. Network Contributor on the subnet) can be created before the cluster.
resource "azurerm_user_assigned_identity" "control_plane" {
  name                = "${var.cluster_name}-control-plane"
  resource_group_name = var.resource_group_name
  location            = var.location
  tags                = local.tags
}

resource "azurerm_role_assignment" "subnet_network_contributor" {
  scope                = var.nodes_subnet_id
  role_definition_name = "Network Contributor"
  principal_id         = azurerm_user_assigned_identity.control_plane.principal_id
}

resource "azurerm_kubernetes_cluster" "this" {
  name                = var.cluster_name
  resource_group_name = var.resource_group_name
  location            = var.location
  dns_prefix          = var.cluster_name
  kubernetes_version  = var.kubernetes_version
  sku_tier            = var.sku_tier

  automatic_upgrade_channel = var.automatic_upgrade_channel
  node_os_upgrade_channel   = "NodeImage"

  private_cluster_enabled = var.private_cluster_enabled

  dynamic "api_server_access_profile" {
    for_each = !var.private_cluster_enabled && length(var.authorized_ip_ranges) > 0 ? [1] : []
    content {
      authorized_ip_ranges = var.authorized_ip_ranges
    }
  }

  # Entra ID auth + Azure RBAC; local (static kubeconfig) accounts disabled.
  local_account_disabled            = true
  role_based_access_control_enabled = true
  azure_active_directory_role_based_access_control {
    tenant_id              = data.azurerm_client_config.current.tenant_id
    azure_rbac_enabled     = true
    admin_group_object_ids = var.admin_group_object_ids
  }

  # Workload Identity: federated credentials let KSAs act as Entra identities.
  oidc_issuer_enabled       = true
  workload_identity_enabled = true

  azure_policy_enabled = true

  identity {
    type         = "UserAssigned"
    identity_ids = [azurerm_user_assigned_identity.control_plane.id]
  }

  default_node_pool {
    name                         = "system"
    vm_size                      = var.system_node_pool.vm_size
    vnet_subnet_id               = var.nodes_subnet_id
    auto_scaling_enabled         = true
    min_count                    = var.system_node_pool.min_count
    max_count                    = var.system_node_pool.max_count
    os_disk_size_gb              = var.system_node_pool.os_disk_size_gb
    os_disk_type                 = "Managed"
    zones                        = var.system_node_pool.zones
    only_critical_addons_enabled = true
    host_encryption_enabled      = false
    node_labels                  = { "platform.mycompany.io/node-pool" = "system" }

    upgrade_settings {
      max_surge = "33%"
    }
  }

  # Required block in azurerm 5.x. "Manual" = classic cluster-autoscaler node pools;
  # "Auto" enables Node Auto Provisioning (Karpenter-based).
  node_provisioning_profile {
    mode = "Manual"
  }

  network_profile {
    network_plugin      = "azure"
    network_plugin_mode = "overlay"
    network_data_plane  = var.network_data_plane
    network_policy      = var.network_data_plane == "cilium" ? "cilium" : "azure"
    pod_cidr            = var.pod_cidr
    service_cidr        = var.service_cidr
    dns_service_ip      = var.dns_service_ip
    outbound_type       = "userAssignedNATGateway"
    load_balancer_sku   = "standard"
  }

  dynamic "oms_agent" {
    for_each = var.log_analytics_workspace_id == null ? [] : [1]
    content {
      log_analytics_workspace_id      = var.log_analytics_workspace_id
      msi_auth_for_monitoring_enabled = true
    }
  }

  key_vault_secrets_provider {
    secret_rotation_enabled = true
  }

  maintenance_window_auto_upgrade {
    frequency   = "Weekly"
    interval    = 1
    day_of_week = "Saturday"
    start_time  = "02:00"
    duration    = 4
  }

  tags = local.tags

  depends_on = [azurerm_role_assignment.subnet_network_contributor]

  lifecycle {
    ignore_changes = [default_node_pool[0].node_count]
  }
}

resource "azurerm_kubernetes_cluster_node_pool" "user" {
  for_each = var.user_node_pools

  name                  = each.key
  kubernetes_cluster_id = azurerm_kubernetes_cluster.this.id
  vm_size               = each.value.vm_size
  vnet_subnet_id        = var.nodes_subnet_id
  mode                  = "User"
  auto_scaling_enabled  = true
  min_count             = each.value.min_count
  max_count             = each.value.max_count
  os_disk_size_gb       = each.value.os_disk_size_gb
  zones                 = each.value.zones
  priority              = each.value.spot ? "Spot" : "Regular"
  eviction_policy       = each.value.spot ? "Delete" : null
  spot_max_price        = each.value.spot ? -1 : null
  node_labels           = merge(each.value.labels, { "platform.mycompany.io/node-pool" = each.key })
  node_taints           = each.value.taints
  tags                  = local.tags

  dynamic "upgrade_settings" {
    for_each = each.value.spot ? [] : [1]
    content {
      max_surge = "33%"
    }
  }

  lifecycle {
    ignore_changes = [node_count]
  }
}
