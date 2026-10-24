#!/usr/bin/env bash
# Lab 05 teardown: removes lab policies, test namespaces, Kyverno and Gatekeeper.
set -euo pipefail
LAB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
kubectl delete -f "$LAB_DIR/cel-policies/" --ignore-not-found || true
kubectl delete -f "$LAB_DIR/kyverno-policies/" --ignore-not-found || true
kubectl delete -f "$LAB_DIR/gatekeeper-policies/constraints/" --ignore-not-found || true
kubectl delete -f "$LAB_DIR/gatekeeper-policies/templates/" --ignore-not-found || true
kubectl delete namespace policy-test-gatekeeper policy-test-kyverno policy-test-cel policy-test-generated --ignore-not-found
helm uninstall kyverno -n kyverno || true
helm uninstall gatekeeper -n gatekeeper-system || true
kubectl delete namespace kyverno gatekeeper-system --ignore-not-found
