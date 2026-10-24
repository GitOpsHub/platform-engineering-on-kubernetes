#!/usr/bin/env bash
# teardown.sh - delete the acme and globex tenants created by setup.sh.
#
# Deletes Tenant CRs first (if the CRD exists) so an operator can clean up, then the
# tenant namespaces. Everything is --ignore-not-found, so re-running is safe. The Lab 01
# cluster and its platform namespaces are not touched.
#
# Usage:
#   ./teardown.sh
#   ./teardown.sh --help
set -euo pipefail

# --- Pinned values (keep in sync with setup.sh) -------------------------------
KUBE_CONTEXT="kind-platform-engineering"
TENANTS=(acme globex)

# --- Helpers ------------------------------------------------------------------
log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mWARN:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
  exit 0
}

case "${1:-}" in
  -h|--help) usage ;;
  "") ;;
  *) die "unknown argument: $1 (see --help)" ;;
esac

k() { kubectl --context "${KUBE_CONTEXT}" "$@"; }

command -v kubectl >/dev/null 2>&1 || die "kubectl is required"
kubectl config get-contexts -o name 2>/dev/null | grep -qx "${KUBE_CONTEXT}" \
  || die "kubectl context '${KUBE_CONTEXT}' not found - nothing to tear down"

if k get crd tenants.platform.mycompany.io >/dev/null 2>&1; then
  log "Deleting Tenant CRs"
  for tenant in "${TENANTS[@]}"; do
    k delete tenants.platform.mycompany.io "${tenant}" --ignore-not-found
  done
else
  warn "Tenant CRD not installed - skipping Tenant CRs"
fi

log "Deleting tenant namespaces"
for tenant in "${TENANTS[@]}"; do
  k delete namespace "tenant-${tenant}-dev" --ignore-not-found
done

log "Done. Lab 01 cluster left in place."
