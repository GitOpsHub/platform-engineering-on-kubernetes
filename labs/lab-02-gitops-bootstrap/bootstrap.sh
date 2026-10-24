#!/usr/bin/env bash
# bootstrap.sh - install Argo CD and hand the cluster over to Git.
#
# Usage:
#   ./bootstrap.sh                                   # use this repo (upstream)
#   ./bootstrap.sh --repo-url https://github.com/<you>/platform-engineering-on-kubernetes
#   ./bootstrap.sh --repo-url <fork> --rewrite       # also rewrite repoURLs in the
#                                                    # lab files (commit + push after!)
#   ./bootstrap.sh --revision my-branch              # track a branch other than main
#   ./bootstrap.sh --context kind-platform-engineering
#
# Idempotent: re-running re-applies the pinned Argo CD manifests and the root app.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

ARGOCD_VERSION="v3.5.6"   # keep in sync with argocd-install/kustomization.yaml
UPSTREAM_REPO="https://github.com/GitOpsHub/platform-engineering-on-kubernetes"
REPO_URL="${UPSTREAM_REPO}"
REVISION="main"
REWRITE="false"
KCTX="$(kubectl config current-context 2>/dev/null || true)"

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mWARN:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-url)   REPO_URL="${2:-}"; shift 2 ;;
    --repo-url=*) REPO_URL="${1#*=}"; shift ;;
    --revision)   REVISION="${2:-}"; shift 2 ;;
    --revision=*) REVISION="${1#*=}"; shift ;;
    --context)    KCTX="${2:-}"; shift 2 ;;
    --context=*)  KCTX="${1#*=}"; shift ;;
    --rewrite)    REWRITE="true"; shift ;;
    -h|--help)    sed -n '2,13p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done

REPO_URL="${REPO_URL%/}"
REPO_URL="${REPO_URL%.git}"
[[ -n "${KCTX}" ]] || die "no kubectl context; run Lab 01 first or pass --context"

k() { kubectl --context "${KCTX}" "$@"; }

# --- Prerequisites ------------------------------------------------------------
for bin in kubectl sed; do
  command -v "${bin}" >/dev/null 2>&1 || die "missing required tool: ${bin}"
done
k cluster-info >/dev/null 2>&1 || die "cannot reach cluster via context '${KCTX}'"
command -v argocd >/dev/null 2>&1 || warn "argocd CLI not found - optional, but the README uses it"

# --- Optional: point the lab files at a fork ----------------------------------
if [[ "${REWRITE}" == "true" ]]; then
  [[ "${REPO_URL}" != "${UPSTREAM_REPO}" ]] || die "--rewrite needs --repo-url pointing at your fork"
  log "Rewriting repoURL/targetRevision in lab files -> ${REPO_URL} @ ${REVISION}"
  while IFS= read -r -d '' f; do
    sed -i.bak \
      -e "s#${UPSTREAM_REPO}#${REPO_URL}#g" \
      -e "s#targetRevision: main#targetRevision: ${REVISION}#g" \
      -e "s#revision: main#revision: ${REVISION}#g" \
      "${f}"
    rm -f "${f}.bak"
  done < <(find "${SCRIPT_DIR}/app-of-apps.yaml" "${SCRIPT_DIR}/applications" -maxdepth 1 -name '*.yaml' -print0)
  warn "Files rewritten. Commit and push them to ${REPO_URL} (branch ${REVISION}) - Argo CD reads Git, not your disk."
fi

# Warn if child Applications still point somewhere else than the root app.
if [[ "${REPO_URL}" != "${UPSTREAM_REPO}" ]] && grep -q "${UPSTREAM_REPO}" "${SCRIPT_DIR}"/applications/*.yaml; then
  warn "applications/*.yaml still reference ${UPSTREAM_REPO}."
  warn "Child apps will sync from upstream, not your fork. Re-run with --rewrite, then commit + push."
fi

# --- Install Argo CD ----------------------------------------------------------
log "Installing Argo CD ${ARGOCD_VERSION} (server-side apply)"
k apply --server-side --force-conflicts -k "${SCRIPT_DIR}/argocd-install"

log "Waiting for Argo CD CRDs and components"
k wait --for=condition=Established \
  crd/applications.argoproj.io crd/applicationsets.argoproj.io crd/appprojects.argoproj.io --timeout=120s
for d in argocd-server argocd-repo-server argocd-applicationset-controller argocd-redis argocd-dex-server argocd-notifications-controller; do
  k -n argocd rollout status "deployment/${d}" --timeout=300s
done
k -n argocd rollout status statefulset/argocd-application-controller --timeout=300s

# --- Root application ---------------------------------------------------------
log "Applying AppProjects and the root Application (${REPO_URL} @ ${REVISION})"
sed -e "s#${UPSTREAM_REPO}#${REPO_URL}#g" \
    -e "s#targetRevision: main#targetRevision: ${REVISION}#g" \
    "${SCRIPT_DIR}/app-of-apps.yaml" | k apply -f -

log "Waiting for platform-root to sync (this pulls from Git; can take ~2 min)"
for _ in $(seq 1 60); do
  sync="$(k -n argocd get application platform-root -o jsonpath='{.status.sync.status}' 2>/dev/null || true)"
  health="$(k -n argocd get application platform-root -o jsonpath='{.status.health.status}' 2>/dev/null || true)"
  if [[ "${sync}" == "Synced" && "${health}" == "Healthy" ]]; then
    break
  fi
  sleep 5
done
k -n argocd get applications

PASSWORD="$(k -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' 2>/dev/null | base64 --decode || true)"

cat <<EOF

Argo CD is up.
  UI:        http://argocd.localtest.me:8080      (needs the Lab 01 Traefik ingress)
             or: kubectl -n argocd port-forward svc/argocd-server 8081:80  ->  http://localhost:8081
  Username:  admin
  Password:  ${PASSWORD:-<run: argocd admin initial-password -n argocd>}
  CLI:       argocd login argocd.localtest.me:8080 --plaintext --grpc-web --username admin

Watch the apps:  kubectl -n argocd get applications -w
EOF
