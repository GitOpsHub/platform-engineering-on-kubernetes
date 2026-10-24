#!/usr/bin/env bash
# setup.sh - create tenants acme and globex on the Lab 01 cluster (idempotent).
#
# Two modes, chosen by whether the Tenant CRD is installed:
#   - no operator (default): applies the plain manifests for each tenant - namespace,
#     ResourceQuota + LimitRange, NetworkPolicies, owner RoleBinding.
#   - operator installed: applies only the Tenant CRs and waits for the operator to
#     create those objects. Plain child manifests are NOT applied in this mode, because
#     tenant-operator refuses to adopt an existing ResourceQuota, LimitRange, NetworkPolicy
#     or RoleBinding that it did not create (ObjectConflict).
# In both modes the demo app is applied last.
#
# Usage:
#   ./setup.sh           # create or update both tenants
#   ./setup.sh --help
#
# Safe to re-run: every object is applied with `kubectl apply`.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- Pinned values (keep in sync with README.md) ------------------------------
KUBE_CONTEXT="kind-platform-engineering"   # Lab 01 cluster
TENANTS=(acme globex)                      # namespaces are tenant-<name>-dev
ECHO_IMAGE="nginxinc/nginx-unprivileged:1.27-alpine"
CLIENT_IMAGE="curlimages/curl:8.11.1"
ROLLOUT_TIMEOUT="180s"

# --- Helpers ------------------------------------------------------------------
log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mWARN:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
  exit 0
}

case "${1:-}" in
  -h|--help) usage ;;
  "") ;;
  *) die "unknown argument: $1 (see --help)" ;;
esac

# Every call names the context explicitly, so a different current-context never
# receives these manifests.
k() { kubectl --context "${KUBE_CONTEXT}" "$@"; }

# render <template> <tenant> [namespace]: fill __TENANT__ and __NAMESPACE__.
render() {
  local template="$1" tenant="$2" namespace="${3:-}"
  sed -e "s|__TENANT__|${tenant}|g" -e "s|__NAMESPACE__|${namespace}|g" "${template}"
}

render_demo() {
  sed -e "s|__ECHO_IMAGE__|${ECHO_IMAGE}|g" -e "s|__CLIENT_IMAGE__|${CLIENT_IMAGE}|g" \
    "${SCRIPT_DIR}/tests/demo-app.yaml"
}

# --- Checks -------------------------------------------------------------------
check_prereqs() {
  command -v kubectl >/dev/null 2>&1 || die "kubectl is required"

  local contexts current
  contexts="$(kubectl config get-contexts -o name 2>/dev/null || true)"
  grep -qx "${KUBE_CONTEXT}" <<<"${contexts}" \
    || die "kubectl context '${KUBE_CONTEXT}' not found - run Lab 01 setup.sh first"

  current="$(kubectl config current-context 2>/dev/null || true)"
  if [[ "${current}" != "${KUBE_CONTEXT}" ]]; then
    warn "current kubectl context is '${current}'; this script targets '${KUBE_CONTEXT}' explicitly"
  fi

  k get namespace platform-system >/dev/null 2>&1 \
    || die "cannot reach '${KUBE_CONTEXT}' or Lab 01 namespaces are missing"
}

operator_installed() {
  k get crd tenants.platform.mycompany.io >/dev/null 2>&1
}

# --- Provisioning -------------------------------------------------------------
provision_manual() {
  local tenant="$1" ns="$2"

  log "Tenant '${tenant}': namespace ${ns}"
  render "${SCRIPT_DIR}/namespace-templates/namespace.yaml" "${tenant}" "${ns}" | k apply -f -

  log "Tenant '${tenant}': quota and limits"
  k -n "${ns}" apply -f "${SCRIPT_DIR}/resource-quotas/quota.yaml"

  log "Tenant '${tenant}': network policies"
  render "${SCRIPT_DIR}/network-isolation/network-policies.yaml" "${tenant}" | k -n "${ns}" apply -f -

  log "Tenant '${tenant}': owner RBAC (group tenant:${tenant}:owners -> ClusterRole admin)"
  render "${SCRIPT_DIR}/rbac/tenant-owner.yaml" "${tenant}" | k -n "${ns}" apply -f -
}

provision_via_operator() {
  local tenant="$1" ns="$2"

  log "Tenant '${tenant}': applying Tenant CR (the operator creates ${ns} and its children)"
  k apply -f "${SCRIPT_DIR}/tenant-operator/tenant-${tenant}.yaml"

  for _ in $(seq 1 60); do
    k get namespace "${ns}" >/dev/null 2>&1 && break
    sleep 2
  done
  k get namespace "${ns}" >/dev/null 2>&1 || die "operator did not create namespace ${ns} within 120s"

  log "Tenant '${tenant}': waiting for the operator to report Ready"
  k wait "tenant/${tenant}" --for=jsonpath='{.status.phase}'=Ready --timeout="${ROLLOUT_TIMEOUT}" \
    || die "Tenant '${tenant}' did not reach phase Ready - see: kubectl describe tenant ${tenant}"
}

provision_tenant() {
  local tenant="$1"
  local ns="tenant-${tenant}-dev"

  if operator_installed; then
    provision_via_operator "${tenant}" "${ns}"
  else
    provision_manual "${tenant}" "${ns}"
  fi

  log "Tenant '${tenant}': demo app"
  render_demo | k -n "${ns}" apply -f -
  k -n "${ns}" rollout status deployment/echo --timeout="${ROLLOUT_TIMEOUT}"
  k -n "${ns}" wait --for=condition=Ready pod/client --timeout="${ROLLOUT_TIMEOUT}"
}

main() {
  check_prereqs
  if operator_installed; then
    log "Tenant CRD found: operator mode (plain child manifests are skipped)"
  else
    warn "Tenant CRD not installed: manifest mode (the lab works without the operator)"
  fi
  for tenant in "${TENANTS[@]}"; do
    provision_tenant "${tenant}"
  done

  log "Done. Run ./tests/verify-isolation.sh to check isolation."
}

main
