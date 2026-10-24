locals {
  a = var.addons

  common_labels = {
    "platform.mycompany.io/managed-by" = "terraform"
    "platform.mycompany.io/component"  = "addons"
  }

  # Per-cloud Cilium defaults. On GKE prefer Dataplane V2 (gke module) instead of
  # self-managed Cilium; this mode chains Cilium for features DPv2 lacks.
  cilium_cloud_values = {
    eks = {
      # ENI IPAM keeps Pods on VPC IPs; kube-proxy replacement requires the API host.
      eni                        = { enabled = true }
      ipam                       = { mode = "eni" }
      routingMode                = "native"
      egressMasqueradeInterfaces = "eth+"
      kubeProxyReplacement       = true
      k8sServiceHost             = var.eks_api_server_host
      k8sServicePort             = 443
    }
    aks = {
      # Bring-your-own-CNI AKS clusters (network_plugin = "none").
      aksbyocni = { enabled = true }
      nodeinit  = { enabled = true }
      ipam      = { mode = "cluster-pool" }
    }
    gke = {
      nodeinit = { enabled = true, reconfigureKubelet = true, removeCbrBridge = true }
      cni      = { binPath = "/home/kubernetes/bin" }
      ipam     = { mode = "kubernetes" }
    }
    generic = {}
  }
}

resource "helm_release" "cilium" {
  count = local.a.cilium.enabled ? 1 : 0

  name             = "cilium"
  repository       = "https://helm.cilium.io"
  chart            = "cilium"
  version          = local.a.cilium.version
  namespace        = local.a.cilium.namespace
  create_namespace = true
  atomic           = var.atomic
  timeout          = var.timeout_seconds

  values = concat([
    yamlencode(merge({
      cluster = { name = var.cluster_name }
      hubble = {
        enabled = true
        relay   = { enabled = true }
        ui      = { enabled = false }
        metrics = { enabled = ["dns", "drop", "tcp", "flow", "httpV2"] }
      }
      operator   = { replicas = 2 }
      prometheus = { enabled = true }
      encryption = { enabled = false }
    }, local.cilium_cloud_values[var.cloud]))
  ], local.a.cilium.values)

  lifecycle {
    precondition {
      condition     = var.cloud != "eks" || var.eks_api_server_host != null
      error_message = "eks_api_server_host is required when installing Cilium on EKS."
    }
  }
}

resource "helm_release" "cert_manager" {
  count = local.a.cert_manager.enabled ? 1 : 0

  name             = "cert-manager"
  repository       = "https://charts.jetstack.io"
  chart            = "cert-manager"
  version          = local.a.cert_manager.version
  namespace        = local.a.cert_manager.namespace
  create_namespace = true
  atomic           = var.atomic
  timeout          = var.timeout_seconds

  values = concat([
    yamlencode({
      crds                = { enabled = true, keep = true }
      replicaCount        = 2
      podDisruptionBudget = { enabled = true }
      prometheus          = { enabled = true }
      serviceAccount      = { annotations = var.cert_manager_service_account_annotations }
      webhook             = { replicaCount = 2 }
      global              = { commonLabels = local.common_labels }
    })
  ], local.a.cert_manager.values)

  depends_on = [helm_release.cilium]
}

resource "helm_release" "external_secrets" {
  count = local.a.external_secrets.enabled ? 1 : 0

  name             = "external-secrets"
  repository       = "https://charts.external-secrets.io"
  chart            = "external-secrets"
  version          = local.a.external_secrets.version
  namespace        = local.a.external_secrets.namespace
  create_namespace = true
  atomic           = var.atomic
  timeout          = var.timeout_seconds

  values = concat([
    yamlencode({
      installCRDs         = true
      replicaCount        = 2
      podDisruptionBudget = { enabled = true }
      serviceAccount      = { annotations = var.external_secrets_service_account_annotations }
      commonLabels        = local.common_labels
    })
  ], local.a.external_secrets.values)

  depends_on = [helm_release.cilium]
}

resource "helm_release" "metrics_server" {
  # GKE and AKS ship metrics-server; installing a second copy conflicts with the managed one.
  count = local.a.metrics_server.enabled && !contains(["gke", "aks"], var.cloud) ? 1 : 0

  name             = "metrics-server"
  repository       = "https://kubernetes-sigs.github.io/metrics-server"
  chart            = "metrics-server"
  version          = local.a.metrics_server.version
  namespace        = local.a.metrics_server.namespace
  create_namespace = true
  atomic           = var.atomic
  timeout          = var.timeout_seconds

  values = concat([
    yamlencode({
      replicas            = 2
      podDisruptionBudget = { enabled = true, minAvailable = 1 }
      commonLabels        = local.common_labels
    })
  ], local.a.metrics_server.values)

  depends_on = [helm_release.cilium]
}

resource "helm_release" "kube_prometheus_stack" {
  count = local.a.kube_prometheus_stack.enabled ? 1 : 0

  name             = "kube-prometheus-stack"
  repository       = "https://prometheus-community.github.io/helm-charts"
  chart            = "kube-prometheus-stack"
  version          = local.a.kube_prometheus_stack.version
  namespace        = local.a.kube_prometheus_stack.namespace
  create_namespace = true
  atomic           = var.atomic
  timeout          = var.timeout_seconds

  values = concat([
    yamlencode({
      commonLabels = local.common_labels
      # Managed control planes do not expose these components.
      kubeControllerManager = { enabled = false }
      kubeScheduler         = { enabled = false }
      kubeEtcd              = { enabled = false }
      kubeProxy             = { enabled = !(local.a.cilium.enabled && var.cloud == "eks") }
      grafana = {
        # The chart's default admin password is well known: override it via
        # grafana.admin.existingSecret (e.g. synced by external-secrets) or wire SSO.
        defaultDashboardsTimezone = "utc"
      }
      prometheus = {
        prometheusSpec = {
          retention = "15d"
          # Pick up ServiceMonitors/PodMonitors from every namespace, not only this release.
          serviceMonitorSelectorNilUsesHelmValues = false
          podMonitorSelectorNilUsesHelmValues     = false
          ruleSelectorNilUsesHelmValues           = false
        }
      }
    })
  ], local.a.kube_prometheus_stack.values)

  depends_on = [helm_release.cilium]
}
