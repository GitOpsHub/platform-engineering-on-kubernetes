#!/usr/bin/env bash
# setup.sh - Lab 06 observability stack on the Lab 01 kind cluster.
#
# Installs, in order:
#   Loki (logs), Tempo (traces), OpenTelemetry Collector (OTLP fan-out),
#   Grafana Alloy (pod log shipping), kube-prometheus-stack (Prometheus + Thanos sidecar + Grafana),
#   then the checkout-api sample app and the Grafana datasource ConfigMap.
#
# Usage:
#   ./setup.sh               # full install
#   ./setup.sh --skip-app    # observability stack only (no sample app, no image build)
#   ./setup.sh --help
#
# Safe to re-run: every Helm release uses `helm upgrade --install`, and kubectl apply converges.
set -euo pipefail

LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- Pinned versions (keep in sync with README.md) ----------------------------
LOKI_CHART_VERSION="7.3.0"              # Loki 3.6.12
ALLOY_CHART_VERSION="1.12.1"            # Grafana Alloy v1.19.2
TEMPO_CHART_VERSION="3.2.1"             # grafana-community/tempo chart, Tempo 3.1.1 (replaces grafana/tempo 1.24.4, deprecated)
OTEL_COLLECTOR_CHART_VERSION="0.173.1"  # OpenTelemetry Collector 0.160.0
KPS_CHART_VERSION="91.4.1"              # kube-prometheus-stack
THANOS_VERSION="v0.42.4"                # Thanos sidecar image, set inside prometheus-stack.yaml
APP_IMAGE="checkout-api:0.1.0"

CLUSTER_NAME="platform-engineering"
NS="platform-monitoring"
GRAFANA_REPO="https://grafana.github.io/helm-charts"
TEMPO_REPO="https://grafana-community.github.io/helm-charts"   # grafana/tempo is deprecated
PROM_REPO="https://prometheus-community.github.io/helm-charts"
OTEL_REPO="https://open-telemetry.github.io/opentelemetry-helm-charts"
HELM_TIMEOUT="10m"
SKIP_APP="false"

# --- Helpers ------------------------------------------------------------------
log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mWARN:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-app) SKIP_APP="true"; shift ;;
    -h|--help)  sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done

ctx="$(kubectl config current-context 2>/dev/null || echo none)"
if [[ "$ctx" != "kind-${CLUSTER_NAME}" ]]; then
  warn "current context is '${ctx}' (expected kind-${CLUSTER_NAME})"
fi

# --- Prerequisites ------------------------------------------------------------
check_prereqs() {
  log "Checking prerequisites"
  local bin missing=()
  for bin in kubectl helm; do
    command -v "$bin" >/dev/null 2>&1 || missing+=("$bin")
  done
  if [[ "$SKIP_APP" == "false" ]]; then
    for bin in docker kind; do
      command -v "$bin" >/dev/null 2>&1 || missing+=("$bin")
    done
  fi
  [[ ${#missing[@]} -eq 0 ]] || die "missing required tools: ${missing[*]}"

  kubectl cluster-info >/dev/null 2>&1 \
    || die "cannot reach the API server for context '${ctx}' - is the Lab 01 cluster running?"
}

# --- Platform pieces ----------------------------------------------------------
ensure_namespace() {
  # Created by Lab 01 already. The privileged PSA level is needed by node-exporter.
  log "Ensuring namespace ${NS} (privileged PSA for node-exporter)"
  kubectl create namespace "$NS" --dry-run=client -o yaml | kubectl apply -f -
  kubectl label namespace "$NS" \
    platform.mycompany.io/managed=true \
    platform.mycompany.io/tier=observability \
    pod-security.kubernetes.io/enforce=privileged \
    --overwrite
}

helm_install() {
  # Usage: helm_install <release> <chart> <repo> <version> <values-file>
  local release="$1" chart="$2" repo="$3" version="$4" values="$5"
  helm upgrade --install "$release" "$chart" \
    --repo "$repo" --version "$version" \
    --namespace "$NS" --values "$values" \
    --wait --timeout "$HELM_TIMEOUT"
}

build_and_load_app() {
  log "Building ${APP_IMAGE} and loading it into kind cluster '${CLUSTER_NAME}'"
  docker build -t "$APP_IMAGE" "$LAB_DIR/apps/checkout-api"
  kind load docker-image "$APP_IMAGE" --name "$CLUSTER_NAME"
}

# --- Main ---------------------------------------------------------------------
main() {
  check_prereqs
  ensure_namespace

  log "Installing Loki ${LOKI_CHART_VERSION} (single binary, filesystem storage)"
  helm_install loki loki "$GRAFANA_REPO" "$LOKI_CHART_VERSION" "$LAB_DIR/loki-stack.yaml"

  log "Installing Tempo ${TEMPO_CHART_VERSION} from grafana-community (single binary, OTLP 4317/4318)"
  helm_install tempo tempo "$TEMPO_REPO" "$TEMPO_CHART_VERSION" "$LAB_DIR/tempo-stack.yaml"

  log "Installing OpenTelemetry Collector ${OTEL_COLLECTOR_CHART_VERSION} (deployment mode)"
  helm_install otel-collector opentelemetry-collector "$OTEL_REPO" "$OTEL_COLLECTOR_CHART_VERSION" \
    "$LAB_DIR/otel-collector.yaml"

  log "Installing Grafana Alloy ${ALLOY_CHART_VERSION} (pod logs -> Loki)"
  helm_install alloy alloy "$GRAFANA_REPO" "$ALLOY_CHART_VERSION" "$LAB_DIR/alloy-logs.yaml"

  log "Installing kube-prometheus-stack ${KPS_CHART_VERSION} (Thanos sidecar ${THANOS_VERSION}, Grafana)"
  helm_install kps kube-prometheus-stack "$PROM_REPO" "$KPS_CHART_VERSION" "$LAB_DIR/prometheus-stack.yaml"

  if [[ "$SKIP_APP" == "true" ]]; then
    log "Skipping sample app (--skip-app)"
  else
    build_and_load_app
    log "Deploying checkout-api and the load generator"
    kubectl apply -f "$LAB_DIR/apps/checkout-api/k8s/00-namespace.yaml"
    kubectl apply -f "$LAB_DIR/apps/checkout-api/k8s/"
    kubectl -n checkout rollout status deployment/checkout-api --timeout=180s
  fi

  log "Applying Grafana datasource ConfigMap (Grafana sidecar loads it)"
  kubectl apply -f "$LAB_DIR/grafana/datasources-configmap.yaml"

  log "Done. Context: ${ctx}"
  echo
  echo "  Grafana:     kubectl -n ${NS} port-forward svc/kps-grafana 3000:80   (http://localhost:3000)"
  echo "  Admin login: admin / see README (Verification)"
}

main
