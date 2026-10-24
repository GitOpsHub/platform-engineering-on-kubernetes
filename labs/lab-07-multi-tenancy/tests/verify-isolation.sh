#!/usr/bin/env bash
# verify-isolation.sh - pass/fail checks for the acme and globex demo tenants.
# Run after setup.sh.
#
#   (a) same-tenant HTTP succeeds
#   (b) cross-tenant HTTP fails (needs NetworkPolicy enforcement, e.g. Cilium)
#   (c) DNS resolves from a tenant pod
#   (d) a pod that exceeds the namespace ResourceQuota is rejected (server dry run)
#   (e) the owners group cannot read cluster-scoped resources (kubectl auth can-i)
#
# Usage: ./verify-isolation.sh   (KUBE_CONTEXT=... to override the context)
set -euo pipefail

KUBE_CONTEXT="${KUBE_CONTEXT:-kind-platform-engineering}"
NS_A="tenant-acme-dev"
NS_B="tenant-globex-dev"
OWNERS_GROUP="tenant:acme:owners"
PROBE_IMAGE="curlimages/curl:8.11.1"   # image name only; a dry run never pulls it

PASS=0
FAIL=0

# --- Helpers ------------------------------------------------------------------
pass() { PASS=$((PASS + 1)); printf '  \033[1;32m[PASS]\033[0m %s\n' "$*"; }
fail() { FAIL=$((FAIL + 1)); printf '  \033[1;31m[FAIL]\033[0m %s\n' "$*"; }

k() { kubectl --context "${KUBE_CONTEXT}" "$@"; }

# A pod that asks for more CPU than the tenant quota (requests.cpu: 4) allows.
# LimitRange max cpu is 4, so the pod itself is legal; only the namespace total is exceeded.
probe_pod() {
  cat <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: quota-probe
spec:
  securityContext:
    runAsNonRoot: true
    runAsUser: 1000
    seccompProfile:
      type: RuntimeDefault
  containers:
    - name: probe
      image: ${PROBE_IMAGE}
      securityContext:
        allowPrivilegeEscalation: false
        capabilities:
          drop: ["ALL"]
      resources:
        requests:
          cpu: "4"
          memory: 64Mi
        limits:
          cpu: "4"
          memory: 128Mi
EOF
}

# --- Checks -------------------------------------------------------------------
check_same_tenant_http() {
  if k -n "${NS_A}" exec client -- curl -fsS --max-time 5 -o /dev/null "http://echo.${NS_A}.svc.cluster.local/"; then
    pass "(a) same-tenant HTTP succeeds (${NS_A} client -> echo)"
  else
    fail "(a) same-tenant HTTP failed - check pods and the same-tenant policy"
  fi
}

check_cross_tenant_blocked() {
  local rc=0
  k -n "${NS_A}" exec client -- curl -fsS --max-time 5 -o /dev/null "http://echo.${NS_B}.svc.cluster.local/" || rc=$?
  if (( rc != 0 )); then
    pass "(b) cross-tenant HTTP blocked (${NS_A} client -> ${NS_B} echo, curl exit ${rc})"
  else
    fail "(b) cross-tenant HTTP succeeded - NetworkPolicy is not enforced by the CNI"
  fi
}

check_dns() {
  # The client image is curl-based and the echo image has no nslookup, so resolve the
  # name with curl. curl exit 6 means "could not resolve host"; any other exit code
  # (for example 28, timeout, or 7, connection refused) means DNS worked and only the
  # connection failed. The API server Service is used because it always exists.
  local rc=0
  k -n "${NS_A}" exec client -- curl -sS --max-time 5 -o /dev/null \
    "https://kubernetes.default.svc.cluster.local/" 2>/dev/null || rc=$?
  if (( rc != 6 )); then
    pass "(c) DNS resolves from a tenant pod (curl exit ${rc}, not 6)"
  else
    fail "(c) DNS lookup failed from ${NS_A}/client - check the allow-dns policy"
  fi
}

check_quota_rejects() {
  local out rc=0
  out="$(probe_pod | k -n "${NS_A}" apply --dry-run=server -f - 2>&1)" || rc=$?
  if (( rc != 0 )) && grep -q 'exceeded quota' <<<"${out}"; then
    pass "(d) pod exceeding the ResourceQuota is rejected"
  else
    fail "(d) oversized pod was not rejected by the quota: ${out}"
  fi
}

check_rbac_scope() {
  local out
  out="$(k auth can-i list nodes --as tenant-probe --as-group "${OWNERS_GROUP}" 2>/dev/null || true)"
  if [[ "${out}" == "no" ]]; then
    pass "(e) ${OWNERS_GROUP} cannot list nodes (cluster-scoped)"
  else
    fail "(e) ${OWNERS_GROUP} can list nodes (answer: '${out}')"
  fi

  out="$(k auth can-i create deployments -n "${NS_A}" --as tenant-probe --as-group "${OWNERS_GROUP}" 2>/dev/null || true)"
  if [[ "${out}" == "yes" ]]; then
    pass "(e) ${OWNERS_GROUP} can create deployments in ${NS_A}"
  else
    fail "(e) ${OWNERS_GROUP} cannot create deployments in ${NS_A} (answer: '${out}')"
  fi
}

# --- Main ---------------------------------------------------------------------
printf '=== Tenant isolation verification (context: %s) ===\n' "${KUBE_CONTEXT}"
check_same_tenant_http
check_cross_tenant_blocked
check_dns
check_quota_rejects
check_rbac_scope

printf '\n=== %d passed, %d failed ===\n' "${PASS}" "${FAIL}"
(( FAIL == 0 ))
