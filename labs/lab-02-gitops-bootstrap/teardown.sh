#!/usr/bin/env bash
# teardown.sh - remove everything Lab 02 created, leaving the Lab 01 cluster.
#
# Usage: ./teardown.sh [--context <kubectl-context>] [--keep-argocd]
#
# Order matters: the root Application is deleted first so its finalizer
# cascades to child Applications and their workloads; only then is Argo CD
# itself removed (otherwise nothing would be left to process the finalizers).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KCTX="$(kubectl config current-context 2>/dev/null || true)"
KEEP_ARGOCD="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --context)     KCTX="${2:-}"; shift 2 ;;
    --context=*)   KCTX="${1#*=}"; shift ;;
    --keep-argocd) KEEP_ARGOCD="true"; shift ;;
    -h|--help)     sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

[[ -n "${KCTX}" ]] || { echo "no kubectl context" >&2; exit 1; }
k() { kubectl --context "${KCTX}" "$@"; }

if k get crd applications.argoproj.io >/dev/null 2>&1; then
  echo "==> Deleting root Application (cascades to children)"
  k -n argocd delete application platform-root --ignore-not-found --wait=true --timeout=300s || true

  echo "==> Deleting any remaining ApplicationSets and Applications"
  k -n argocd delete applicationsets --all --ignore-not-found --wait=true --timeout=180s || true
  k -n argocd delete applications --all --ignore-not-found --wait=true --timeout=300s || true

  echo "==> Deleting AppProjects"
  k -n argocd delete appprojects platform apps --ignore-not-found --wait=true --timeout=60s || true
fi

if [[ "${KEEP_ARGOCD}" == "false" ]]; then
  echo "==> Uninstalling Argo CD"
  k delete -k "${SCRIPT_DIR}/argocd-install" --ignore-not-found --wait=true || true
fi

# Safety net in case a finalizer could not run (e.g. Argo CD was already gone).
for ns in guestbook podinfo sample-app-dev sample-app-staging; do
  k delete namespace "${ns}" --ignore-not-found --wait=false
done

echo "==> Lab 02 teardown complete"
