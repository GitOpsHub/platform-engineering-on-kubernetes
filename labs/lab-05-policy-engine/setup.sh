#!/usr/bin/env bash
# Lab 05 setup: installs Gatekeeper + Kyverno and applies all lab policies.
# Usage: ./setup.sh [--with-verify-images]
set -euo pipefail
LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATEKEEPER_VERSION="${GATEKEEPER_VERSION:-3.23.1}"
KYVERNO_CHART_VERSION="${KYVERNO_CHART_VERSION:-3.9.2}"
WITH_VERIFY_IMAGES=false
[[ "${1:-}" == "--with-verify-images" ]] && WITH_VERIFY_IMAGES=true

ctx="$(kubectl config current-context)"
[[ "$ctx" == "kind-platform-engineering" ]] || echo "WARNING: current context is '$ctx' (expected kind-platform-engineering)"

echo "==> Creating test namespaces"
for ns in policy-test-gatekeeper policy-test-kyverno policy-test-cel; do
  kubectl create namespace "$ns" --dry-run=client -o yaml | kubectl apply -f -
  kubectl label namespace "$ns" platform.mycompany.io/lab=lab-05-policy-engine --overwrite
done

echo "==> Installing Gatekeeper ${GATEKEEPER_VERSION}"
helm upgrade --install gatekeeper gatekeeper \
  --repo https://open-policy-agent.github.io/gatekeeper/charts --version "$GATEKEEPER_VERSION" \
  --namespace gatekeeper-system --create-namespace --set replicas=1 --wait

echo "==> Installing Kyverno chart ${KYVERNO_CHART_VERSION}"
helm upgrade --install kyverno kyverno \
  --repo https://kyverno.github.io/kyverno --version "$KYVERNO_CHART_VERSION" \
  --namespace kyverno --create-namespace --wait

echo "==> Applying Gatekeeper templates"
kubectl apply -f "$LAB_DIR/gatekeeper-policies/templates/"
for crd in k8srequiredlabels k8sdisallowedtags k8sallowedrepos; do
  until kubectl get crd "${crd}.constraints.gatekeeper.sh" >/dev/null 2>&1; do sleep 2; done
  kubectl wait --for=condition=Established "crd/${crd}.constraints.gatekeeper.sh" --timeout=60s
done
kubectl apply -f "$LAB_DIR/gatekeeper-policies/constraints/"

echo "==> Applying Kyverno policies"
kubectl apply -f "$LAB_DIR/kyverno-policies/rbac-generate.yaml"
for p in require-labels disallow-latest-tag restrict-registries add-default-labels generate-tenant-defaults validatingpolicy-disallow-privileged; do
  kubectl apply -f "$LAB_DIR/kyverno-policies/${p}.yaml"
done
if [[ "$WITH_VERIFY_IMAGES" == "true" ]]; then
  kubectl apply -f "$LAB_DIR/kyverno-policies/verify-images.yaml"
fi
kubectl wait --for=condition=Ready clusterpolicy --all --timeout=120s || true

echo "==> Applying CEL ValidatingAdmissionPolicies (requires Kubernetes >= 1.30)"
if kubectl api-resources --api-group=admissionregistration.k8s.io -o name | grep -q '^validatingadmissionpolicies'; then
  kubectl apply -f "$LAB_DIR/cel-policies/"
else
  echo "WARNING: ValidatingAdmissionPolicy API not available; skipping CEL policies."
fi
echo "==> Done. Run ./test-policies.sh"
