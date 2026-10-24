output "network_id" {
  description = "Self-link style ID of the VPC network."
  value       = google_compute_network.this.id
}

output "network_name" {
  description = "Name of the VPC network."
  value       = google_compute_network.this.name
}

output "subnetwork_id" {
  description = "ID of the node subnetwork."
  value       = google_compute_subnetwork.nodes.id
}

output "subnetwork_name" {
  description = "Name of the node subnetwork."
  value       = google_compute_subnetwork.nodes.name
}

output "pods_range_name" {
  description = "Name of the secondary range for Pods (pass to the gke module)."
  value       = local.pods_range_name
}

output "services_range_name" {
  description = "Name of the secondary range for Services (pass to the gke module)."
  value       = local.services_range_name
}
