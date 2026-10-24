#!/usr/bin/env bash
# teardown.sh - delete the Lab 01 cluster (and the k3d registry).
#
# Usage: ./teardown.sh [--provider kind|k3d|all]
# Default is "all": deletes whichever of the two lab clusters exists.
set -euo pipefail

CLUSTER_NAME="platform-engineering"
PROVIDER="all"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --provider)   PROVIDER="${2:-}"; shift 2 ;;
    --provider=*) PROVIDER="${1#*=}"; shift ;;
    kind|k3d|all) PROVIDER="$1"; shift ;;
    -h|--help)    sed -n '2,5p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ "${PROVIDER}" == "kind" || "${PROVIDER}" == "all" ]] && command -v kind >/dev/null 2>&1; then
  if kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
    echo "==> Deleting kind cluster ${CLUSTER_NAME}"
    kind delete cluster --name "${CLUSTER_NAME}"
  else
    echo "==> No kind cluster named ${CLUSTER_NAME}"
  fi
fi

if [[ "${PROVIDER}" == "k3d" || "${PROVIDER}" == "all" ]] && command -v k3d >/dev/null 2>&1; then
  if k3d cluster get "${CLUSTER_NAME}" >/dev/null 2>&1; then
    echo "==> Deleting k3d cluster ${CLUSTER_NAME}"
    k3d cluster delete "${CLUSTER_NAME}"
  else
    echo "==> No k3d cluster named ${CLUSTER_NAME}"
  fi
  # The registry is normally removed with the cluster; clean up if it lingers.
  if k3d registry get platform-registry.localhost >/dev/null 2>&1; then
    echo "==> Deleting k3d registry platform-registry.localhost"
    k3d registry delete platform-registry.localhost
  fi
fi

echo "==> Teardown complete"
