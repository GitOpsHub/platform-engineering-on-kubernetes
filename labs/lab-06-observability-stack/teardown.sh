#!/usr/bin/env bash
# teardown.sh - removes the Lab 06 app, Grafana datasources and observability releases.
#
# Keeps the kind cluster and the platform-monitoring namespace (both owned by Lab 01).
# Helm does not remove CRDs; see README (Cleanup) if you want those gone too.
set -euo pipefail

LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NS="platform-monitoring"

ctx="$(kubectl config current-context 2>/dev/null || echo none)"
if [[ "$ctx" != "kind-platform-engineering" ]]; then
  echo "WARNING: current context is '$ctx' (expected kind-platform-engineering)"
fi

echo "NOTE: the '${NS}' namespace is KEPT on purpose. Lab 01 creates it and other labs use it."
echo "      This script removes only the Lab 06 releases, PVCs, the checkout namespace and the datasource ConfigMap."
echo

echo "==> Removing sample app (namespace checkout)"
kubectl delete -f "$LAB_DIR/apps/checkout-api/k8s/" --ignore-not-found || true
kubectl delete namespace checkout --ignore-not-found

echo "==> Removing Grafana datasource ConfigMap"
kubectl delete -f "$LAB_DIR/grafana/datasources-configmap.yaml" --ignore-not-found || true

echo "==> Uninstalling Helm releases"
helm uninstall kps -n "$NS" || true
helm uninstall alloy -n "$NS" || true
helm uninstall otel-collector -n "$NS" || true
helm uninstall tempo -n "$NS" || true
helm uninstall loki -n "$NS" || true

echo "==> Deleting PVCs left behind by StatefulSets (Loki, Tempo, Prometheus)"
# helm uninstall keeps StatefulSet PVCs. Match by name: <template>-<statefulset>-<ordinal>.
while read -r pvc; do
  case "$pvc" in
    storage-loki-*|storage-tempo-*|prometheus-kps-kube-prometheus-stack-prometheus-db-*)
      kubectl -n "$NS" delete pvc "$pvc" --ignore-not-found ;;
  esac
done < <(kubectl -n "$NS" get pvc -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null)

echo "==> Done. Lab 06 removed."
echo "    KEPT: the kind cluster and the '${NS}' namespace (owned by Lab 01)."
echo "    To delete the namespace as well, run it yourself: kubectl delete namespace ${NS}"
