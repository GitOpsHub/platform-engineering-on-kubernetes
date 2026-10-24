# addons — platform add-ons via Helm

Installs the baseline platform add-ons with the Helm provider (3.x). Each add-on is toggled and pinned through
the `addons` object; extra values are appended as YAML documents.

| Add-on | Chart | Default version | Enabled by default |
| ------ | ----- | --------------- | :----------------: |
| Cilium | `cilium/cilium` | 1.20.4 | no |
| cert-manager | `jetstack/cert-manager` | v1.21.3 | yes |
| external-secrets | `external-secrets/external-secrets` | 2.14.0 | yes |
| metrics-server | `metrics-server/metrics-server` | 3.14.0 | yes (skipped on GKE/AKS, which ship it) |
| kube-prometheus-stack | `prometheus-community/kube-prometheus-stack` | 92.11.0 | no |

The module has **no provider block**: configure `provider "helm"` in the root module (see `../examples`).
`cloud` selects per-cloud Cilium settings (ENI IPAM + kube-proxy replacement on EKS, BYO-CNI on AKS, node-init on GKE).
On GKE and AKS prefer the managed Cilium data planes (Dataplane V2 / Azure CNI powered by Cilium) and leave Cilium off.

Cloud identities: pass `external_secrets_service_account_annotations` / `cert_manager_service_account_annotations`,
e.g. `{ "eks.amazonaws.com/role-arn" = aws_iam_role.eso.arn }` (IRSA), `{ "iam.gke.io/gcp-service-account" = "..." }`
or `{ "azure.workload.identity/client-id" = "..." }`.

## Usage

```hcl
module "addons" {
  source = "../addons"

  cluster_name = module.eks.cluster_name
  cloud        = "eks"

  addons = {
    cert_manager          = { enabled = true }
    external_secrets      = { enabled = true }
    kube_prometheus_stack = { enabled = true, values = [file("${path.module}/values/kps.yaml")] }
  }
}
```

A complete root module using this module lives in [`examples/`](../examples).

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.6.0 |
| <a name="requirement_helm"></a> [helm](#requirement\_helm) | >= 3.0, < 4.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_helm"></a> [helm](#provider\_helm) | >= 3.0, < 4.0 |

## Resources

| Name | Type |
| ---- | ---- |
| [helm_release.cert_manager](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [helm_release.cilium](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [helm_release.external_secrets](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [helm_release.kube_prometheus_stack](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [helm_release.metrics_server](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_cluster_name"></a> [cluster\_name](#input\_cluster\_name) | Cluster name; used for Cilium cluster.name and labels. | `string` | n/a | yes |
| <a name="input_addons"></a> [addons](#input\_addons) | Add-ons to install. Each key toggles one Helm release:<br/>  enabled   - install or not<br/>  version   - pinned chart version<br/>  namespace - target namespace (created if missing)<br/>  values    - list of extra YAML values documents merged after the module defaults | <pre>object({<br/>    cilium = optional(object({<br/>      enabled   = optional(bool, false)<br/>      version   = optional(string, "1.20.4")<br/>      namespace = optional(string, "kube-system")<br/>      values    = optional(list(string), [])<br/>    }), {})<br/>    cert_manager = optional(object({<br/>      enabled   = optional(bool, true)<br/>      version   = optional(string, "v1.21.3")<br/>      namespace = optional(string, "cert-manager")<br/>      values    = optional(list(string), [])<br/>    }), {})<br/>    external_secrets = optional(object({<br/>      enabled   = optional(bool, true)<br/>      version   = optional(string, "2.14.0")<br/>      namespace = optional(string, "external-secrets")<br/>      values    = optional(list(string), [])<br/>    }), {})<br/>    metrics_server = optional(object({<br/>      enabled   = optional(bool, true)<br/>      version   = optional(string, "3.14.0")<br/>      namespace = optional(string, "kube-system")<br/>      values    = optional(list(string), [])<br/>    }), {})<br/>    kube_prometheus_stack = optional(object({<br/>      enabled   = optional(bool, false)<br/>      version   = optional(string, "92.11.0")<br/>      namespace = optional(string, "monitoring")<br/>      values    = optional(list(string), [])<br/>    }), {})<br/>  })</pre> | `{}` | no |
| <a name="input_atomic"></a> [atomic](#input\_atomic) | Roll back a release automatically if the install/upgrade fails. | `bool` | `true` | no |
| <a name="input_cert_manager_service_account_annotations"></a> [cert\_manager\_service\_account\_annotations](#input\_cert\_manager\_service\_account\_annotations) | Annotations for the cert-manager controller ServiceAccount (cloud identity for DNS-01 solvers). | `map(string)` | `{}` | no |
| <a name="input_cloud"></a> [cloud](#input\_cloud) | Target cloud (eks, gke, aks or generic); selects sensible per-cloud defaults for Cilium. | `string` | `"generic"` | no |
| <a name="input_eks_api_server_host"></a> [eks\_api\_server\_host](#input\_eks\_api\_server\_host) | EKS only: API server hostname (no scheme) for Cilium kube-proxy replacement. Required when cloud = eks and Cilium is enabled. | `string` | `null` | no |
| <a name="input_external_secrets_service_account_annotations"></a> [external\_secrets\_service\_account\_annotations](#input\_external\_secrets\_service\_account\_annotations) | Annotations for the external-secrets controller ServiceAccount (e.g. eks.amazonaws.com/role-arn, iam.gke.io/gcp-service-account, azure.workload.identity/client-id). | `map(string)` | `{}` | no |
| <a name="input_timeout_seconds"></a> [timeout\_seconds](#input\_timeout\_seconds) | Helm operation timeout per release. | `number` | `600` | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_cert_manager_namespace"></a> [cert\_manager\_namespace](#output\_cert\_manager\_namespace) | Namespace of cert-manager, or null if disabled. |
| <a name="output_monitoring_namespace"></a> [monitoring\_namespace](#output\_monitoring\_namespace) | Namespace of kube-prometheus-stack, or null if disabled. |
| <a name="output_releases"></a> [releases](#output\_releases) | Installed releases: name => { namespace, chart version, status }. |
<!-- END_TF_DOCS -->
