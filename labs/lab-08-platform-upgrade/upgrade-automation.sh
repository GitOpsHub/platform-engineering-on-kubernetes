#!/usr/bin/env bash
# upgrade-automation.sh - pre-flight checks before a Kubernetes minor upgrade.
#
# DRY RUN BY DEFAULT. Without --execute the script only reads cluster state and
# prints the upgrade plan. With --execute it still asks before every cordon,
# drain and uncordon (default answer: no). It never upgrades a node itself:
# the upgrade step is provider- or kubeadm-specific and is printed as a manual step.
#
# Checks: server version and skew, deprecated APIs (kubent or pluto), PodDisruptionBudgets,
# node health. Result tags: PASS, WARN, FAIL, SKIP (check not run). Exit 0 if
# no FAIL, 1 otherwise.
#
# Usage:
#   ./upgrade-automation.sh --target-version 1.36
#   ./upgrade-automation.sh --target-version 1.36 --context kind-platform-engineering-green
#   ./upgrade-automation.sh --target-version 1.36 --execute
#   ./upgrade-automation.sh --help
set -euo pipefail

CONTEXT="kind-platform-engineering"
TARGET=""
EXECUTE="false"
CUR=""
PASS=0; WARN=0; FAIL=0; SKIP=0
KCFG_TMP=""

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }
usage() { awk 'NR==1{next} /^#/{sub(/^# ?/,""); print; next} {exit}' "$0"; exit 0; }

report() { # report STATUS message...
  local status="$1"; shift
  case "${status}" in
    PASS) PASS=$((PASS + 1)) ;;
    WARN) WARN=$((WARN + 1)) ;;
    FAIL) FAIL=$((FAIL + 1)) ;;
    SKIP) SKIP=$((SKIP + 1)) ;;
  esac
  printf '  [%s] %s\n' "${status}" "$*"
}

k() { kubectl --context "${CONTEXT}" --request-timeout=20s "$@"; }

trap 'rm -f "${KCFG_TMP}"' EXIT

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target-version)   TARGET="${2:-}"; shift 2 ;;
    --target-version=*) TARGET="${1#*=}"; shift ;;
    --context)          CONTEXT="${2:-}"; shift 2 ;;
    --context=*)        CONTEXT="${1#*=}"; shift ;;
    --execute)          EXECUTE="true"; shift ;;
    -h|--help)          usage ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done

[[ -n "${TARGET}" ]] || die "--target-version <minor> is required, e.g. --target-version 1.36"
[[ "${TARGET}" =~ ^[0-9]+\.[0-9]+$ ]] || die "--target-version must be a minor such as 1.36 (got '${TARGET}')"
command -v kubectl >/dev/null 2>&1 || die "kubectl is not in PATH"
CONTEXTS="$(kubectl config get-contexts -o name)"
grep -qxF "${CONTEXT}" <<<"${CONTEXTS}" \
  || die "kubectl context '${CONTEXT}' does not exist (see: kubectl config get-contexts)"

log "Pre-flight for upgrade to v${TARGET} on ${CONTEXT} (mode: $([[ ${EXECUTE} == true ]] && echo EXECUTE || echo dry-run))"

# --- 1. Version and skew -------------------------------------------------------
check_version() {
  # Pull "1.35" out of the serverVersion block of `kubectl version -o json`.
  CUR="$(k version -o json 2>/dev/null \
    | sed -n '/"serverVersion"/,/}/p' \
    | sed -n 's/.*"gitVersion": *"v\([0-9]*\.[0-9]*\)\..*/\1/p' || true)"
  if [[ -z "${CUR}" ]]; then
    report FAIL "cannot read server version from ${CONTEXT} (is the cluster reachable?)"
    return
  fi
  report PASS "current server minor is ${CUR}"

  local cur_major="${CUR%%.*}" cur_minor="${CUR#*.}"
  local tgt_major="${TARGET%%.*}" tgt_minor="${TARGET#*.}"
  if [[ "${cur_major}" != "${tgt_major}" ]]; then
    report FAIL "major version change ${CUR} -> ${TARGET} is not a supported upgrade path"
  elif (( tgt_minor == cur_minor )); then
    report PASS "target ${TARGET} equals current minor: nothing to upgrade"
  elif (( tgt_minor == cur_minor + 1 )); then
    report PASS "skew OK: exactly one minor above current (${CUR} -> ${TARGET})"
  else
    report FAIL "skew: upgrade one minor at a time (current ${CUR}, target ${TARGET})"
  fi
}

# --- 2. Deprecated APIs ---------------------------------------------------------
check_deprecated_apis() {
  # Give each tool a kubeconfig that holds only the chosen context.
  KCFG_TMP="$(mktemp)"
  kubectl config view --minify --flatten --context "${CONTEXT}" > "${KCFG_TMP}"

  if command -v kubent >/dev/null 2>&1; then
    log "Deprecated APIs: kubent (target ${TARGET})"
    KUBECONFIG="${KCFG_TMP}" kubent --target-version "${TARGET}" || true
    report WARN "kubent ran; any resource it lists uses an API removed by v${TARGET} - fix it before upgrading"
  elif command -v pluto >/dev/null 2>&1; then
    log "Deprecated APIs: pluto (target v${TARGET})"
    KUBECONFIG="${KCFG_TMP}" pluto detect-all-in-cluster --target-versions "k8s=v${TARGET}" || true
    report WARN "pluto ran; review its findings above - fix any removed API before upgrading"
  else
    report SKIP "deprecated-API check was SKIPPED: install kubent or pluto (not a pass)"
  fi
}

# --- 3. PodDisruptionBudgets ----------------------------------------------------
check_pdbs() {
  local out zero pdb
  out="$(k get pdb -A -o jsonpath='{range .items[*]}{.metadata.namespace}{"/"}{.metadata.name}{" "}{.status.disruptionsAllowed}{"\n"}{end}')" \
    || { report FAIL "could not list PodDisruptionBudgets"; return; }
  if [[ -z "${out}" ]]; then
    report PASS "no PodDisruptionBudgets in the cluster"
    return
  fi
  # An empty status field means the controller has not reported yet; treat it as zero.
  zero="$(awk '($2 == "" || $2 == 0) {print $1}' <<<"${out}")"
  if [[ -z "${zero}" ]]; then
    report PASS "all PodDisruptionBudgets allow at least one disruption"
  else
    while read -r pdb; do
      report WARN "PDB ${pdb} allows 0 disruptions: drain will block until its pods are healthy"
    done <<<"${zero}"
  fi
}

# --- 4. Node health -------------------------------------------------------------
check_nodes() {
  local out name unsched ready
  out="$(k get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"|"}{.spec.unschedulable}{"|"}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}')" \
    || { report FAIL "could not list nodes"; return; }
  while IFS='|' read -r name unsched ready; do
    [[ -n "${name}" ]] || continue
    if [[ "${ready}" != "True" ]]; then
      report FAIL "node ${name} is NotReady"
    elif [[ "${unsched}" == "true" ]]; then
      report WARN "node ${name} is cordoned (spec.unschedulable=true)"
    else
      report PASS "node ${name} is Ready"
    fi
  done <<<"${out}"
}

confirm() { # reads y/N from the terminal; anything but y is no
  local ans=""
  printf '  Run now? [y/N] '
  if [[ -t 0 ]]; then read -r ans; fi
  [[ "${ans}" =~ ^[Yy]$ ]]
}

# --- Plan (dry run) and execution ---------------------------------------------
NODES=""
load_nodes() { NODES="$(k get nodes -o jsonpath='{.items[*].metadata.name}')"; }

print_plan() {
  log "Planned steps (dry run: nothing was changed)"
  echo "  1. Upgrade the control plane to v${TARGET}.x first (kubeadm upgrade apply, or your provider's API). Manual step."
  echo "  2. Then each node, one at a time:"
  for node in ${NODES}; do
    echo "     kubectl --context ${CONTEXT} cordon ${node}"
    echo "     kubectl --context ${CONTEXT} drain ${node} --ignore-daemonsets --delete-emptydir-data"
    echo "     (upgrade ${node} to v${TARGET}.x: manual step)"
    echo "     kubectl --context ${CONTEXT} uncordon ${node}"
  done
  echo "  Re-run with --execute to be prompted before each cordon, drain and uncordon."
}

execute_plan() {
  log "Executing: each step needs a 'y' answer (default no)"
  for node in ${NODES}; do
    echo "  cordon ${node}?"
    if confirm; then k cordon "${node}"; else echo "  not run: cordon ${node}"; continue; fi
    echo "  drain ${node}?"
    if confirm; then
      k drain "${node}" --ignore-daemonsets --delete-emptydir-data
    else
      echo "  not run: drain ${node} (node stays cordoned)"; continue
    fi
    echo "  Upgrade ${node} to v${TARGET}.x now (manual step), then answer to uncordon."
    echo "  uncordon ${node}?"
    if confirm; then k uncordon "${node}"; else echo "  not run: uncordon ${node}"; fi
  done
}

# --- Main -----------------------------------------------------------------------
check_version
if [[ -z "${CUR}" ]]; then
  log "Cannot reach the API server; stopping after the version check."
else
  check_deprecated_apis
  check_pdbs
  check_nodes
fi

log "Summary: PASS=${PASS} WARN=${WARN} FAIL=${FAIL} SKIP=${SKIP}"
if (( FAIL > 0 )); then
  log "FAIL checks present: not planning or executing the upgrade."
  exit 1
fi
if [[ -z "${CUR}" ]]; then
  exit 1
fi

load_nodes
if [[ "${EXECUTE}" == "true" ]]; then
  execute_plan
else
  print_plan
fi
exit 0
