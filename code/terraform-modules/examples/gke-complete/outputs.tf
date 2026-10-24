output "cluster_name" {
  description = "GKE cluster name."
  value       = module.gke.cluster_name
}

output "kubeconfig_command" {
  description = "Command to add the cluster to your kubeconfig."
  value       = "gcloud container clusters get-credentials ${module.gke.cluster_name} --region ${var.region} --project ${var.project_id}"
}

output "workload_identity_pool" {
  description = "Workload Identity pool."
  value       = module.gke.workload_identity_pool
}

output "addon_releases" {
  description = "Installed Helm add-ons."
  value       = module.addons.releases
}
