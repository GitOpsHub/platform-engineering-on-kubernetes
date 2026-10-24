# platform-addons

An umbrella chart for the cluster add-ons the platform depends on. Each dependency is pinned and
toggled with its own `condition:`.

| Add-on | Chart version | Repository | Toggle |
| ------ | ------------- | ---------- | ------ |
| cert-manager | `v1.21.3` | https://charts.jetstack.io | `cert-manager.enabled` |
| external-secrets | `2.14.0` | https://charts.external-secrets.io | `external-secrets.enabled` |
| ingress-nginx | `4.15.1` | https://kubernetes.github.io/ingress-nginx | `ingress-nginx.enabled` |
| kube-prometheus-stack | `92.13.1` | https://prometheus-community.github.io/helm-charts | `kube-prometheus-stack.enabled` |

These are the same versions the Terraform `addons` module pins (for cert-manager, external-secrets and
kube-prometheus-stack), so both paths install the same charts.

> **ingress-nginx is retired.** The Kubernetes project stopped best-effort maintenance in March 2026.
> It receives no releases, bug fixes or security updates from upstream. It stays in this chart so existing
> clusters keep working. For new clusters, use Gateway API with a controller that is still maintained, and
> set `ingress-nginx.enabled: false`.

## Install

```bash
make -C code/helm-charts deps          # helm dependency build (needs network)
helm lint --strict code/helm-charts/platform-addons
helm upgrade --install platform-addons code/helm-charts/platform-addons \
  --namespace platform-addons --create-namespace \
  --set ingress-nginx.enabled=false \
  --wait
helm test platform-addons --namespace platform-addons
```

Create the Grafana credentials Secret before installing kube-prometheus-stack:

```bash
kubectl -n platform-addons create secret generic grafana-admin \
  --from-literal=admin-user=admin --from-literal=admin-password="$(openssl rand -base64 24)"
```

## Values

Top-level keys are subchart names. Each has an `enabled` flag. Everything else under a key goes to that
subchart, so see the upstream chart's `values.yaml` for the full list of options.

| Key | Default | Notes |
| --- | ------- | ----- |
| `cert-manager.enabled` | `true` | `crds.enabled` and `crds.keep` are both `true`. |
| `external-secrets.enabled` | `true` | `installCRDs: true`. |
| `ingress-nginx.enabled` | `true` | See the retirement note above. |
| `kube-prometheus-stack.enabled` | `true` | Grafana reads its admin credentials from Secret `grafana-admin`. Prometheus discovers monitors in all namespaces. |

## Tests

`helm test` checks that the CRDs for each enabled add-on exist (`certificates.cert-manager.io`,
`externalsecrets.external-secrets.io`, `prometheuses.monitoring.coreos.com`). The test runs as a
ServiceAccount with `get` on CustomResourceDefinitions and nothing else.
