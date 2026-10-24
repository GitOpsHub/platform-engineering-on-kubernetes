#!/usr/bin/env bash
# teardown.sh - remove the green cluster and the Lab 08 leftovers created by the runbooks.
#
# Deletes the green kind cluster, the local S3-compatible Velero store, the smoke-test
# restore namespace and the blue-side proxy Service used for the traffic switch.
# Everything is idempotent, so re-running is safe. Blue (the Lab 01 cluster) is NOT
# deleted: move Argo CD off it and delete it with Lab 01's teardown.sh instead.
#
# Usage:
#   ./teardown.sh
#   ./teardown.sh --help
set -euo pipefail

# --- Pinned values (keep in sync with green-kind-cluster.yaml) ----------------
GREEN_CLUSTER="platform-engineering-green"
BLUE_CONTEXT="kind-platform-engineering"
VELERO_STORE="velero-s3"

# --- Helpers ------------------------------------------------------------------
log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mWARN:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
  exit 0
}

case "${1:-}" in
  -h|--help) usage ;;
  "") ;;
  *) die "unknown argument: $1 (see --help)" ;;
esac

command -v kind >/dev/null 2>&1 || die "kind is required"

# Blue-side resources: only if the blue context still exists.
if kubectl config get-contexts -o name 2>/dev/null | grep -qx "${BLUE_CONTEXT}"; then
  log "Removing blue-side Lab 08 resources"
  kubectl --context "${BLUE_CONTEXT}" -n platform-smoke-test delete service hello-green-proxy --ignore-not-found
  kubectl --context "${BLUE_CONTEXT}" delete namespace platform-smoke-test-restore --ignore-not-found
else
  warn "kubectl context '${BLUE_CONTEXT}' not found - skipping blue-side cleanup"
fi

log "Deleting green kind cluster '${GREEN_CLUSTER}'"
if kind get clusters 2>/dev/null | grep -qx "${GREEN_CLUSTER}"; then
  kind delete cluster --name "${GREEN_CLUSTER}"
else
  warn "green cluster not found - nothing to delete"
fi

log "Removing local Velero store container"
if command -v docker >/dev/null 2>&1; then
  docker rm -f "${VELERO_STORE}" >/dev/null 2>&1 || warn "container '${VELERO_STORE}' not found"
else
  warn "docker not found - skipping ${VELERO_STORE}"
fi

log "Done. Blue cluster left in place; see the decommission step in blue-green-deploy.md."
