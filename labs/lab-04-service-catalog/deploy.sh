#!/usr/bin/env bash
# deploy.sh - install Backstage on the Lab 01 kind cluster using the official Backstage Helm chart.
#
# Usage:
#   BACKSTAGE_IMAGE=backstage-lab:local ./deploy.sh
#   BACKSTAGE_IMAGE=backstage-lab:local GITHUB_TOKEN=ghp_... ./deploy.sh --repo-url https://github.com/<you>/platform-engineering-on-kubernetes
#   ./deploy.sh --ref my-branch            # catalog locations read from another branch
#   ./deploy.sh --help
#
# Prerequisites: the Lab 01 cluster (context kind-platform-engineering), kubectl, helm 3.8+ (or 4),
# kind, docker, openssl, and a Backstage image built locally (README, "Build the Backstage image").
#
# Safe to re-run: secrets are reused, ConfigMaps are re-applied, and
# `helm upgrade --install` converges on the same release.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CLUSTER_NAME="platform-engineering"
KCTX="kind-${CLUSTER_NAME}"
NAMESPACE="developer-platform"          # created by Lab 01 (platform-components.yaml)
RELEASE="backstage"
CHART_REF="oci://ghcr.io/backstage/charts/backstage"
CHART_VERSION="2.10.2"                  # https://github.com/backstage/charts/releases
BACKSTAGE_IMAGE="${BACKSTAGE_IMAGE:-backstage-lab:local}"

UPSTREAM_REPO_URL="https://github.com/GitOpsHub/platform-engineering-on-kubernetes"
REPO_URL="${UPSTREAM_REPO_URL}"
REF="main"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-url)   REPO_URL="${2:-}"; shift 2 ;;
    --repo-url=*) REPO_URL="${1#*=}"; shift ;;
    --ref)        REF="${2:-}"; shift 2 ;;
    --ref=*)      REF="${1#*=}"; shift ;;
    -h|--help)    sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done
REPO_URL="${REPO_URL%/}"
REPO_URL="${REPO_URL%.git}"

k() { kubectl --context "${KCTX}" "$@"; }

# --- Prerequisites ------------------------------------------------------------
check_prereqs() {
  log "Checking prerequisites"
  local bin
  for bin in kubectl helm kind docker openssl sed; do
    command -v "${bin}" >/dev/null 2>&1 || die "missing required tool: ${bin}"
  done

  local helm_major
  helm_major="$(helm version --template '{{.Version}}' | sed -E 's/^v([0-9]+).*/\1/')"
  (( helm_major >= 3 )) || die "Helm 3.8+ is required for OCI charts"

  kubectl config get-contexts -o name | grep -qx "${KCTX}" \
    || die "kubectl context '${KCTX}' not found. Run Lab 01 first (labs/lab-01-cluster-setup/setup.sh)."
  k cluster-info >/dev/null 2>&1 || die "cannot reach cluster '${KCTX}'"
  k get namespace "${NAMESPACE}" >/dev/null 2>&1 \
    || die "namespace '${NAMESPACE}' is missing. It is created by Lab 01 platform-components.yaml."

  docker image inspect "${BACKSTAGE_IMAGE}" >/dev/null 2>&1 \
    || die "image '${BACKSTAGE_IMAGE}' not found locally. Build it first (see README, 'Build the Backstage image') or set BACKSTAGE_IMAGE."
}

# --- Secrets and config --------------------------------------------------------
# apply_secret <name> <key=value>...   (idempotent: create --dry-run | apply)
# Values go through a mode-600 temp file, not --from-literal, so tokens and passwords
# never appear in the process list (ps). The file is removed when the function returns.
apply_secret() {
  local name="$1"; shift
  local envfile
  envfile="$(mktemp)"
  chmod 600 "${envfile}"
  # shellcheck disable=SC2064  # expand now: the temp file path is fixed
  trap "rm -f '${envfile}'" RETURN
  printf '%s\n' "$@" > "${envfile}"
  k -n "${NAMESPACE}" create secret generic "${name}" --from-env-file="${envfile}" \
    --dry-run=client -o yaml | k apply -f - >/dev/null
}

deploy_postgres() {
  log "Deploying Postgres for Backstage"
  # Reuse the existing password on re-runs, so the running database keeps working.
  local pg_password
  pg_password="$(k -n "${NAMESPACE}" get secret backstage-postgres -o jsonpath='{.data.password}' 2>/dev/null | base64 --decode 2>/dev/null || true)"
  [[ -n "${pg_password}" ]] || pg_password="$(openssl rand -hex 24)"

  apply_secret backstage-postgres "password=${pg_password}"
  apply_secret backstage-env \
    "POSTGRES_HOST=backstage-postgres" \
    "POSTGRES_PORT=5432" \
    "POSTGRES_USER=backstage" \
    "POSTGRES_PASSWORD=${pg_password}" \
    "GITHUB_TOKEN=${GITHUB_TOKEN:-}"

  k apply -f "${SCRIPT_DIR}/postgres.yaml" >/dev/null
  k -n "${NAMESPACE}" rollout status deployment/backstage-postgres --timeout=180s
}

deploy_app_config() {
  log "Publishing backstage-config.yaml (repo ${REPO_URL}, ref ${REF})"
  local rendered
  rendered="$(mktemp)"
  # shellcheck disable=SC2064  # expand now: the temp file path is fixed
  trap "rm -f '${rendered}'" RETURN
  sed -e "s#${UPSTREAM_REPO_URL}#${REPO_URL}#g" \
      -e "s#blob/main/#blob/${REF}/#g" \
      "${SCRIPT_DIR}/backstage-config.yaml" > "${rendered}"

  k -n "${NAMESPACE}" create configmap backstage-lab-config \
    --from-file=app-config.lab.yaml="${rendered}" --dry-run=client -o yaml | k apply -f - >/dev/null
}

# --- Chart --------------------------------------------------------------------
install_chart() {
  local repo tag
  if [[ "${BACKSTAGE_IMAGE}" == *:* ]]; then
    repo="${BACKSTAGE_IMAGE%:*}"
    tag="${BACKSTAGE_IMAGE##*:}"
  else
    repo="${BACKSTAGE_IMAGE}"
    tag="latest"
  fi

  log "Loading ${BACKSTAGE_IMAGE} into kind cluster '${CLUSTER_NAME}'"
  kind load docker-image "${BACKSTAGE_IMAGE}" --name "${CLUSTER_NAME}"

  log "Installing Backstage chart ${CHART_VERSION} as release '${RELEASE}'"
  helm upgrade --install "${RELEASE}" "${CHART_REF}" \
    --version "${CHART_VERSION}" \
    --kube-context "${KCTX}" \
    --namespace "${NAMESPACE}" \
    --values "${SCRIPT_DIR}/helm-values.yaml" \
    --set-string "backstage.image.repository=${repo}" \
    --set-string "backstage.image.tag=${tag}" \
    --wait --timeout 10m

  k -n "${NAMESPACE}" rollout status "deployment/${RELEASE}" --timeout=300s
}

main() {
  check_prereqs
  deploy_postgres
  deploy_app_config
  install_chart

  cat <<MSG

Backstage is running.
  UI:        http://backstage.localtest.me:8080   (sign in with "Continue as guest")
  Catalog:   http://backstage.localtest.me:8080/catalog
  Templates: http://backstage.localtest.me:8080/create

Check the catalog import:  kubectl -n ${NAMESPACE} logs deploy/${RELEASE} | grep -i catalog | tail -5
MSG
}

main
