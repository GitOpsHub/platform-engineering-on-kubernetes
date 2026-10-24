output "releases" {
  description = "Installed releases: name => { namespace, chart version, status }."
  value = {
    for r in concat(
      helm_release.cilium,
      helm_release.cert_manager,
      helm_release.external_secrets,
      helm_release.metrics_server,
      helm_release.kube_prometheus_stack,
      ) : r.name => {
      namespace = r.namespace
      version   = r.version
      status    = r.status
    }
  }
}

output "cert_manager_namespace" {
  description = "Namespace of cert-manager, or null if disabled."
  value       = try(helm_release.cert_manager[0].namespace, null)
}

output "monitoring_namespace" {
  description = "Namespace of kube-prometheus-stack, or null if disabled."
  value       = try(helm_release.kube_prometheus_stack[0].namespace, null)
}
