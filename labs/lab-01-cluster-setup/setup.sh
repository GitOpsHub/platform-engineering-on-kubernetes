#!/usr/bin/env bash
# setup.sh - create the Lab 01 cluster and install the platform baseline.
#
# Usage:
#   ./setup.sh                    # kind (default) + Cilium + all components
#   ./setup.sh --provider k3d     # k3d (flannel CNI) + all components
#   ./setup.sh --cluster-only     # just create the cluster (and CNI on kind)
#   ./setup.sh --help
#
# Safe to re-run: existing clusters are reused and every Helm release uses
# `helm upgrade --install`, so a second run converges instead of failing.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- Pinned versions (keep in sync with README.md) ----------------------------
CILIUM_VERSION="1.20.2"
TRAEFIK_CHART_VERSION="42.1.3"          # Traefik v3.7
CERT_MANAGER_VERSION="v1.21.2"
EXTERNAL_SECRETS_VERSION="2.10.0"
SEALED_SECRETS_VERSION="v0.41.1"
METRICS_SERVER_CHART_VERSION="3.14.0"   # metrics-server v0.9.0

CLUSTER_NAME="platform-engineering"
PROVIDER="kind"
CLUSTER_ONLY="false"
HELM_TIMEOUT="10m"

# --- Helpers ------------------------------------------------------------------
log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mWARN:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
  exit 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --provider)     PROVIDER="${2:-}"; shift 2 ;;
    --provider=*)   PROVIDER="${1#*=}"; shift ;;
    kind|k3d)       PROVIDER="$1"; shift ;;
    --cluster-only) CLUSTER_ONLY="true"; shift ;;
    -h|--help)      usage ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done

case "${PROVIDER}" in
  kind) KCTX="kind-${CLUSTER_NAME}" ;;
  k3d)  KCTX="k3d-${CLUSTER_NAME}" ;;
  *) die "--provider must be 'kind' or 'k3d' (got '${PROVIDER}')" ;;
esac

k() { kubectl --context "${KCTX}" "$@"; }
h() { helm --kube-context "${KCTX}" "$@"; }

# --- Prerequisites ------------------------------------------------------------
check_prereqs() {
  log "Checking prerequisites for provider '${PROVIDER}'"
  local missing=()
  for bin in docker kubectl helm "${PROVIDER}"; do
    command -v "${bin}" >/dev/null 2>&1 || missing+=("${bin}")
  done
  [[ ${#missing[@]} -eq 0 ]] || die "missing required tools: ${missing[*]}"

  docker info >/dev/null 2>&1 || die "Docker daemon is not reachable - is Docker Desktop / dockerd running?"

  # Docker Desktop memory check (warn only).
  local mem_bytes
  mem_bytes="$(docker info --format '{{.MemTotal}}' 2>/dev/null || echo 0)"
  if [[ "${mem_bytes}" =~ ^[0-9]+$ ]] && (( mem_bytes > 0 && mem_bytes < 7500000000 )); then
    warn "Docker has $((mem_bytes / 1024 / 1024)) MiB of memory; 8 GiB+ is recommended."
  fi

  local helm_major
  helm_major="$(helm version --template '{{.Version}}' | sed -E 's/^v([0-9]+).*/\1/')"
  (( helm_major >= 3 )) || die "Helm 3.12+ is required"

  # Linux hosts: multi-node kind clusters exhaust the default inotify limits.
  if [[ "$(uname -s)" == "Linux" ]]; then
    local watches
    watches="$(cat /proc/sys/fs/inotify/max_user_instances 2>/dev/null || echo 0)"
    if (( watches < 512 )); then
      warn "fs.inotify.max_user_instances=${watches}; run: sudo sysctl fs.inotify.max_user_instances=512"
    fi
  fi
}

# --- Cluster ------------------------------------------------------------------
create_kind_cluster() {
  if kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
    log "kind cluster '${CLUSTER_NAME}' already exists - reusing it"
    kind export kubeconfig --name "${CLUSTER_NAME}" >/dev/null
  else
    log "Creating kind cluster '${CLUSTER_NAME}'"
    kind create cluster --config "${SCRIPT_DIR}/kind-cluster.yaml" --wait 0s
  fi
}

create_k3d_cluster() {
  if k3d cluster get "${CLUSTER_NAME}" >/dev/null 2>&1; then
    log "k3d cluster '${CLUSTER_NAME}' already exists - reusing it"
    k3d cluster start "${CLUSTER_NAME}" >/dev/null 2>&1 || true
    k3d kubeconfig merge "${CLUSTER_NAME}" --kubeconfig-merge-default --kubeconfig-switch-context >/dev/null
  else
    log "Creating k3d cluster '${CLUSTER_NAME}'"
    k3d cluster create --config "${SCRIPT_DIR}/k3d-cluster.yaml"
  fi
}

install_cilium() {
  log "Installing Cilium ${CILIUM_VERSION} (kube-proxy replacement)"
  h upgrade --install cilium cilium \
    --repo https://helm.cilium.io \
    --version "${CILIUM_VERSION}" \
    --namespace kube-system \
    --values "${SCRIPT_DIR}/values/cilium.yaml" \
    --wait --timeout "${HELM_TIMEOUT}"
  log "Waiting for nodes to become Ready"
  k wait --for=condition=Ready nodes --all --timeout=300s
}

# --- Platform components ------------------------------------------------------
install_components() {
  log "Installing Traefik ingress controller (chart ${TRAEFIK_CHART_VERSION})"
  h upgrade --install traefik traefik \
    --repo https://traefik.github.io/charts \
    --version "${TRAEFIK_CHART_VERSION}" \
    --namespace traefik --create-namespace \
    --values "${SCRIPT_DIR}/values/traefik.yaml" \
    --wait --timeout "${HELM_TIMEOUT}"

  log "Installing cert-manager ${CERT_MANAGER_VERSION}"
  h upgrade --install cert-manager cert-manager \
    --repo https://charts.jetstack.io \
    --version "${CERT_MANAGER_VERSION}" \
    --namespace cert-manager --create-namespace \
    --values "${SCRIPT_DIR}/values/cert-manager.yaml" \
    --wait --timeout "${HELM_TIMEOUT}"

  log "Installing External Secrets Operator ${EXTERNAL_SECRETS_VERSION}"
  h upgrade --install external-secrets external-secrets \
    --repo https://charts.external-secrets.io \
    --version "${EXTERNAL_SECRETS_VERSION}" \
    --namespace external-secrets --create-namespace \
    --values "${SCRIPT_DIR}/values/external-secrets.yaml" \
    --wait --timeout "${HELM_TIMEOUT}"

  log "Installing Sealed Secrets ${SEALED_SECRETS_VERSION}"
  k apply --server-side --force-conflicts \
    -f "https://github.com/bitnami-labs/sealed-secrets/releases/download/${SEALED_SECRETS_VERSION}/controller.yaml"
  k -n kube-system rollout status deployment/sealed-secrets-controller --timeout=300s

  log "Installing metrics-server (chart ${METRICS_SERVER_CHART_VERSION})"
  h upgrade --install metrics-server metrics-server \
    --repo https://kubernetes-sigs.github.io/metrics-server \
    --version "${METRICS_SERVER_CHART_VERSION}" \
    --namespace kube-system \
    --values "${SCRIPT_DIR}/values/metrics-server.yaml" \
    --wait --timeout "${HELM_TIMEOUT}"

  log "Applying platform baseline (namespaces, PKI, smoke test)"
  k apply -f "${SCRIPT_DIR}/platform-components.yaml"
  k -n cert-manager wait --for=condition=Ready certificate/platform-root-ca --timeout=120s
  k wait --for=condition=Ready clusterissuer/platform-ca --timeout=120s
  k -n platform-smoke-test rollout status deployment/hello --timeout=180s
}

main() {
  check_prereqs
  if [[ "${PROVIDER}" == "kind" ]]; then
    create_kind_cluster
    install_cilium
  else
    create_k3d_cluster
    k wait --for=condition=Ready nodes --all --timeout=300s
  fi

  if [[ "${CLUSTER_ONLY}" == "true" ]]; then
    log "Cluster ready (--cluster-only). Context: ${KCTX}"
    exit 0
  fi

  install_components

  log "Done. kubectl context: ${KCTX}"
  echo
  echo "  Next: ./verify.sh --provider ${PROVIDER}"
  echo "  Smoke test: curl http://hello.localtest.me:8080/hostname"
}

main
