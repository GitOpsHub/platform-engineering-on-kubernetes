output "cluster_name" {
  description = "AKS cluster name."
  value       = module.aks.cluster_name
}

output "kubeconfig_command" {
  description = "Command to fetch kubeconfig (then run `kubelogin convert-kubeconfig -l azurecli`)."
  value       = "az aks get-credentials --resource-group ${azurerm_resource_group.this.name} --name ${module.aks.cluster_name}"
}

output "oidc_issuer_url" {
  description = "OIDC issuer for Workload Identity federated credentials."
  value       = module.aks.oidc_issuer_url
}

output "addon_releases" {
  description = "Installed Helm add-ons."
  value       = module.addons.releases
}
