#!/usr/bin/env bash
# teardown.sh - remove what deploy.sh created: the Backstage release, Postgres (and its data),
# the Secrets and the app-config ConfigMap. The developer-platform namespace and the kind
# cluster are kept (Lab 01 owns them).
#
# Usage: ./teardown.sh
set -euo pipefail

CLUSTER_NAME="platform-engineering"
KCTX="kind-${CLUSTER_NAME}"
NAMESPACE="developer-platform"
RELEASE="backstage"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

case "${1:-}" in
  -h|--help) sed -n '2,6p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
  "") ;;
  *) echo "unknown argument: $1" >&2; exit 2 ;;
esac

k() { kubectl --context "${KCTX}" "$@"; }

if ! kubectl config get-contexts -o name | grep -qx "${KCTX}"; then
  echo "==> No kubectl context '${KCTX}'; nothing to tear down"
  exit 0
fi

if helm --kube-context "${KCTX}" -n "${NAMESPACE}" status "${RELEASE}" >/dev/null 2>&1; then
  echo "==> Uninstalling Helm release '${RELEASE}'"
  helm --kube-context "${KCTX}" -n "${NAMESPACE}" uninstall "${RELEASE}" --wait
else
  echo "==> No Helm release '${RELEASE}' in ${NAMESPACE}"
fi

echo "==> Deleting Postgres (including its data) and lab Secrets/ConfigMaps"
k delete -f "${SCRIPT_DIR}/postgres.yaml" --ignore-not-found --wait=true
k -n "${NAMESPACE}" delete secret backstage-env backstage-postgres --ignore-not-found
k -n "${NAMESPACE}" delete configmap backstage-lab-config --ignore-not-found

echo "==> Teardown complete (cluster and namespace '${NAMESPACE}' kept)"
