#!/usr/bin/env bash
# verify.sh - check that every Lab 01 component is installed and healthy.
#
# Usage: ./verify.sh [--provider kind|k3d]
# Exits non-zero if any check fails, so it can be used in CI.
set -euo pipefail

CLUSTER_NAME="platform-engineering"
PROVIDER="kind"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --provider)   PROVIDER="${2:-}"; shift 2 ;;
    --provider=*) PROVIDER="${1#*=}"; shift ;;
    kind|k3d)     PROVIDER="$1"; shift ;;
    -h|--help)    sed -n '2,5p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

case "${PROVIDER}" in
  kind) KCTX="kind-${CLUSTER_NAME}" ;;
  k3d)  KCTX="k3d-${CLUSTER_NAME}" ;;
  *) echo "--provider must be kind or k3d" >&2; exit 2 ;;
esac

k() { kubectl --context "${KCTX}" "$@"; }

PASS=0
FAIL=0
check() {
  local name="$1"; shift
  if "$@" >/dev/null 2>&1; then
    printf '  \033[32m[PASS]\033[0m %s\n' "${name}"
    PASS=$((PASS + 1))
  else
    printf '  \033[31m[FAIL]\033[0m %s\n' "${name}"
    FAIL=$((FAIL + 1))
  fi
}

rollout() { k -n "$1" rollout status "$2" --timeout=60s; }

k cluster-info >/dev/null 2>&1 || { echo "Cannot reach cluster context ${KCTX}. Run ./setup.sh first." >&2; exit 1; }

echo "=== Lab 01 verification (context: ${KCTX}) ==="

echo "Cluster"
check "3 nodes registered"            bash -c "[[ \$(kubectl --context ${KCTX} get nodes --no-headers | wc -l) -eq 3 ]]"
check "all nodes Ready"               k wait --for=condition=Ready nodes --all --timeout=60s
check "CoreDNS running"               rollout kube-system deployment/coredns
check "default StorageClass present"  bash -c "kubectl --context ${KCTX} get storageclass -o jsonpath='{.items[*].metadata.annotations.storageclass\.kubernetes\.io/is-default-class}' | grep -q true"

if [[ "${PROVIDER}" == "kind" ]]; then
  echo "Networking (Cilium)"
  check "cilium DaemonSet ready"      rollout kube-system daemonset/cilium
  check "cilium-operator ready"       rollout kube-system deployment/cilium-operator
  check "hubble-relay ready"          rollout kube-system deployment/hubble-relay
  check "kube-proxy absent (KPR)"     bash -c "! kubectl --context ${KCTX} -n kube-system get daemonset kube-proxy"
fi

echo "Platform components"
check "Traefik ready"                 rollout traefik deployment/traefik
check "cert-manager ready"            rollout cert-manager deployment/cert-manager
check "cert-manager webhook ready"    rollout cert-manager deployment/cert-manager-webhook
check "External Secrets ready"        rollout external-secrets deployment/external-secrets
check "Sealed Secrets ready"          rollout kube-system deployment/sealed-secrets-controller
check "metrics-server ready"          rollout kube-system deployment/metrics-server
check "kubectl top nodes works"       k top nodes

echo "Platform baseline"
for ns in platform-system platform-monitoring platform-security developer-platform; do
  check "namespace ${ns}"             k get namespace "${ns}"
done
check "ClusterIssuer platform-ca Ready" k wait --for=condition=Ready clusterissuer/platform-ca --timeout=30s
check "smoke-test TLS cert issued"    k -n platform-smoke-test wait --for=condition=Ready certificate/hello-tls --timeout=60s
check "smoke-test app ready"          rollout platform-smoke-test deployment/hello
check "ingress reachable on :8080"    curl -fsS --max-time 5 -H "Host: hello.localtest.me" http://127.0.0.1:8080/hostname
check "in-cluster DNS + Service routing" k -n platform-smoke-test exec deploy/hello -- /agnhost connect kubernetes.default.svc.cluster.local:443 --timeout=5s

echo
echo "=== ${PASS} passed, ${FAIL} failed ==="
[[ ${FAIL} -eq 0 ]]
