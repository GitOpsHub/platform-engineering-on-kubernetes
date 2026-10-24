output "cluster_name" {
  description = "Name of the GKE cluster."
  value       = google_container_cluster.this.name
}

output "cluster_id" {
  description = "ID of the GKE cluster."
  value       = google_container_cluster.this.id
}

output "cluster_endpoint" {
  description = "Control-plane endpoint (IP). Prefix with https:// for clients."
  value       = google_container_cluster.this.endpoint
}

output "cluster_ca_certificate" {
  description = "Base64-encoded cluster CA certificate."
  value       = google_container_cluster.this.master_auth[0].cluster_ca_certificate
}

output "location" {
  description = "Location (region or zone) of the cluster."
  value       = google_container_cluster.this.location
}

output "workload_identity_pool" {
  description = "Workload Identity pool (PROJECT_ID.svc.id.goog) for binding KSAs to GSAs."
  value       = "${var.project_id}.svc.id.goog"
}

output "node_service_account_email" {
  description = "Email of the node service account."
  value       = google_service_account.nodes.email
}
