#!/usr/bin/env bash
# Lab 05 policy tests. Uses server-side dry-run so admission webhooks and
# ValidatingAdmissionPolicies evaluate requests without creating objects.
# Usage: ./test-policies.sh [--offline]   (--offline runs gator + kyverno CLI only)
set -euo pipefail
LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FX="$LAB_DIR/tests/fixtures"
PASS=0; FAIL=0
ok()  { echo "  PASS: $*"; PASS=$((PASS+1)); }
bad() { echo "  FAIL: $*"; FAIL=$((FAIL+1)); }

expect_allow() { # ns fixture
  local out
  if out=$(kubectl apply --dry-run=server -n "$1" -f "$FX/$2" 2>&1); then
    ok "$1 allows $2"
  else
    bad "$1 should allow $2: $out"
  fi
}

expect_deny() { # ns fixture pattern
  local out
  if out=$(kubectl apply --dry-run=server -n "$1" -f "$FX/$2" 2>&1); then
    bad "$1 should deny $2"
  elif grep -q -- "$3" <<<"$out"; then
    ok "$1 denies $2"
  else
    bad "$1 denied $2 for unexpected reason: $out"
  fi
}

offline() {
  echo "== Offline tests"
  if command -v gator >/dev/null; then
    if gator verify "$LAB_DIR/gatekeeper-policies/tests/..."; then ok "gator verify"; else bad "gator verify"; fi
  else
    echo "  SKIP: gator not installed"
  fi
  if command -v kyverno >/dev/null; then
    if kyverno test "$LAB_DIR/kyverno-policies/tests"; then ok "kyverno test"; else bad "kyverno test"; fi
  else
    echo "  SKIP: kyverno CLI not installed"
  fi
}

live() {
  for engine in gatekeeper kyverno cel; do
    local ns="policy-test-$engine" tag="\[$engine\]"
    echo "== $engine ($ns)"
    expect_allow "$ns" pod-compliant.yaml
    expect_deny  "$ns" pod-missing-team-label.yaml "$tag"
    expect_deny  "$ns" pod-latest-tag.yaml "$tag"
    expect_deny  "$ns" pod-untagged.yaml "$tag"
    expect_deny  "$ns" pod-untrusted-registry.yaml "$tag"
  done

  echo "== kyverno extras"
  expect_deny policy-test-kyverno pod-privileged.yaml "kyverno-cel"

  local env_label
  env_label=$(kubectl apply --dry-run=server -n policy-test-kyverno -f "$FX/pod-compliant.yaml" \
    -o jsonpath='{.metadata.labels.platform\.mycompany\.io/environment}' 2>/dev/null || true)
  if [[ "$env_label" == "dev" ]]; then
    ok "mutate added environment=dev"
  else
    bad "mutate label missing (got '$env_label')"
  fi

  kubectl apply -f "$FX/namespace-generate.yaml" >/dev/null
  for _ in $(seq 1 30); do
    kubectl get networkpolicy default-deny-all -n policy-test-generated >/dev/null 2>&1 && break
    sleep 2
  done
  if kubectl get networkpolicy default-deny-all -n policy-test-generated >/dev/null 2>&1; then
    ok "generate created NetworkPolicy"
  else
    bad "generate NetworkPolicy missing"
  fi
  if kubectl get resourcequota default-quota -n policy-test-generated >/dev/null 2>&1; then
    ok "generate created ResourceQuota"
  else
    bad "generate ResourceQuota missing"
  fi
  kubectl delete -f "$FX/namespace-generate.yaml" --wait=false >/dev/null

  if kubectl get clusterpolicy verify-image-signatures >/dev/null 2>&1; then
    expect_allow policy-test-kyverno pod-signed-image.yaml
  fi

  echo "== cel extras"
  expect_allow policy-test-cel deployment-compliant.yaml
  expect_deny  policy-test-cel deployment-too-many-replicas.yaml "replicas must be"
}

if [[ "${1:-}" == "--offline" ]]; then
  offline
else
  live
fi
echo "Result: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
