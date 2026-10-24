# Lab 05: Policy Engine - Admission Control with Gatekeeper, Kyverno and CEL

## Objectives
- Enforce the same platform rules three ways: **OPA Gatekeeper** (Rego `ConstraintTemplate`s), **Kyverno** (validate, mutate, generate and verify-images policies) and native **ValidatingAdmissionPolicy** (CEL)
- Require a `platform.mycompany.io/team` label, block the `:latest` tag and untagged images, and allow images only from approved registries
- Generate guardrails (a default-deny NetworkPolicy and a ResourceQuota) into namespaces that opt in with a label
- Verify every rule with offline tests (`gator`, `kyverno test`) and with live server-side dry-run requests
- Understand the trade-offs between the three engines (see [`cel-expressions.md`](cel-expressions.md))

## Prerequisites
| Tool | Version | Notes |
|------|---------|-------|
| Lab 01 cluster | `platform-engineering` (kind) | Context `kind-platform-engineering` must exist |
| kubectl | 1.33+ | ValidatingAdmissionPolicy is GA from Kubernetes 1.30 |
| Helm | 3.12+ | Installs Gatekeeper and Kyverno |
| `gator` CLI | matching Gatekeeper | Optional, for offline Gatekeeper tests. Install from the Gatekeeper releases page |
| `kyverno` CLI | matching Kyverno | Optional, for offline Kyverno tests (`kyverno test`) |
| Internet access | - | Needed for Helm repos and, with `--with-verify-images`, Sigstore |

Background reading: [Policy Engine](../../docs/12-security-compliance/policy-engine.md),
[Pod Security](../../docs/12-security-compliance/pod-security.md),
[Compliance](../../docs/12-security-compliance/compliance.md).
Lab 01 is the prerequisite: [Cluster Setup](../lab-01-cluster-setup/README.md).

## Estimated Time: 60 minutes
(About 15 minutes is Gatekeeper and Kyverno image pulls on the first run.)

---

## Files in this lab

| File | Purpose |
|------|---------|
| [`gatekeeper-policies/templates/`](gatekeeper-policies/templates/) | Three `ConstraintTemplate`s (`templates.gatekeeper.sh/v1`): required labels, disallowed tags, allowed repos |
| [`gatekeeper-policies/constraints/`](gatekeeper-policies/constraints/) | Constraints (`constraints.gatekeeper.sh/v1beta1`) that set the parameters and scope them to `policy-test-gatekeeper` |
| [`gatekeeper-policies/tests/`](gatekeeper-policies/tests/) | `gator` suite and resources for offline tests |
| [`kyverno-policies/`](kyverno-policies/) | `ClusterPolicy` validate, mutate and generate rules, a `ValidatingPolicy` (CEL), and the verify-images policy (not applied by default) |
| [`kyverno-policies/tests/`](kyverno-policies/tests/) | `kyverno-test.yaml` and resources for `kyverno test` |
| [`cel-policies/`](cel-policies/) | `ValidatingAdmissionPolicy` and `ValidatingAdmissionPolicyBinding` pairs, plus parameter `ConfigMap`s |
| [`cel-expressions.md`](cel-expressions.md) | Walkthrough of the CEL policies and the admissionregistration.k8s.io/v1 objects |
| [`tests/fixtures/`](tests/fixtures/) | Shared good and bad manifests, each annotated with its expected result |
| [`setup.sh`](setup.sh) | Installs Gatekeeper and Kyverno, creates test namespaces, applies all policies |
| [`test-policies.sh`](test-policies.sh) | Asserts each fixture is allowed or denied (live) or runs offline suites (`--offline`) |
| [`teardown.sh`](teardown.sh) | Removes the policies, test namespaces and both engines |

### Pinned versions

| Component | Version | Installed by |
|-----------|---------|--------------|
| Gatekeeper | 3.23.1 (`GATEKEEPER_VERSION`) | Helm chart `gatekeeper/gatekeeper` |
| Kyverno | chart 3.9.2 (`KYVERNO_CHART_VERSION`) | Helm chart `kyverno/kyverno` |
| ValidatingAdmissionPolicy | `admissionregistration.k8s.io/v1` | Built into Kubernetes 1.30+ |

Versions are set at the top of `setup.sh`; override them with environment variables.

> **Scope.** Each engine only evaluates the test namespace it owns
> (`policy-test-gatekeeper`, `policy-test-kyverno`, `policy-test-cel`), so the three
> engines do not interfere with each other on one cluster. Production policies would
> normally target the platform-managed namespace label instead.

---

## The rules

| Rule | Gatekeeper | Kyverno | CEL |
|------|-----------|---------|-----|
| Pods need `platform.mycompany.io/team` | `K8sRequiredLabels` | `require-team-label` (validate) | `lab05-require-team-label` |
| No `:latest` tag, image must have a tag or digest | `K8sDisallowedTags` | `disallow-latest-tag` (validate) | `lab05-disallow-latest-tag` |
| Images from approved registries only | `K8sAllowedRepos` | `restrict-image-registries` (validate) | `lab05-allowed-registries` (ConfigMap parameter) |
| No privileged containers | - | `disallow-privileged-containers` (ValidatingPolicy, CEL) | - |
| Default labels and `imagePullPolicy` | - | `add-default-labels` (mutate) | - |
| Max replicas per Deployment | - | - | `lab05-replica-limit` (ConfigMap parameter, audit annotation) |
| Default-deny NetworkPolicy and ResourceQuota on opt-in namespaces | - | `generate-tenant-defaults` (generate) | - |
| Images must have a valid Sigstore signature | - | `verify-image-signatures` (verifyImages, opt-in) | - |

The approved registry list is identical in all three engines
(`registry.k8s.io/`, `ghcr.io/mycompany/`, `ghcr.io/kyverno/`), so the same fixtures give
the same answer everywhere.

---

## Quick start

```bash
cd labs/lab-05-policy-engine

./setup.sh                        # Gatekeeper + Kyverno + all policies
# or, to also apply image signature verification (needs internet):
./setup.sh --with-verify-images

./test-policies.sh                # live server-side dry-run assertions
./test-policies.sh --offline      # gator + kyverno CLI suites
```

The rest of this README walks through the same steps by hand.

---

## Step 1 - Install the engines

Gatekeeper:

```bash
helm upgrade --install gatekeeper gatekeeper \
  --repo https://open-policy-agent.github.io/gatekeeper/charts --version 3.23.1 \
  --namespace gatekeeper-system --create-namespace --set replicas=1 --wait
```

Kyverno:

```bash
helm upgrade --install kyverno kyverno \
  --repo https://kyverno.github.io/kyverno --version 3.9.2 \
  --namespace kyverno --create-namespace --wait
```

Create the three test namespaces, each labelled so you can find them later:

```bash
for ns in policy-test-gatekeeper policy-test-kyverno policy-test-cel; do
  kubectl create namespace "$ns" --dry-run=client -o yaml | kubectl apply -f -
  kubectl label namespace "$ns" platform.mycompany.io/lab=lab-05-policy-engine --overwrite
done
```

## Step 2 - Gatekeeper: templates, then constraints

Gatekeeper creates a CRD for each template, so the templates must be established first:

```bash
kubectl apply -f gatekeeper-policies/templates/
kubectl wait --for=condition=Established crd/k8srequiredlabels.constraints.gatekeeper.sh --timeout=60s
kubectl apply -f gatekeeper-policies/constraints/
kubectl get constraints
```

The `enforcementAction: deny` field in each constraint is what rejects requests. Set it to
`warn` or `dryrun` to roll a policy out without blocking anything, then read the audit
results with `kubectl get <kind> <name> -o yaml`.

## Step 3 - Kyverno: validate, mutate, generate

Kyverno's generate rules need extra RBAC for the background controller. Apply it first:

```bash
kubectl apply -f kyverno-policies/rbac-generate.yaml
kubectl apply -f kyverno-policies/require-labels.yaml \
  -f kyverno-policies/disallow-latest-tag.yaml \
  -f kyverno-policies/restrict-registries.yaml \
  -f kyverno-policies/add-default-labels.yaml \
  -f kyverno-policies/generate-tenant-defaults.yaml \
  -f kyverno-policies/validatingpolicy-disallow-privileged.yaml
kubectl get clusterpolicy
```

Kyverno's `add-default-labels` policy uses the `+( )` anchor, so a value a developer sets
always wins over the default.

## Step 4 - CEL: ValidatingAdmissionPolicy

No extra controller is needed. The API server evaluates CEL in-process:

```bash
kubectl api-resources --api-group=admissionregistration.k8s.io | grep validatingadmissionpolic
kubectl apply -f cel-policies/
kubectl -n policy-test-cel get configmap allowed-registries replica-limit
```

[`cel-expressions.md`](cel-expressions.md) explains each file, including why the
parameters live in a `ConfigMap`.

## Step 5 - Optional: image signature verification

```bash
kubectl apply -f kyverno-policies/verify-images.yaml
```

This needs outbound access to `ghcr.io` and the public Sigstore services (Fulcio and
Rekor). Without it, the signed-image test is skipped.

---

## Verification

### Live checks (server-side dry-run)

`kubectl apply --dry-run=server` sends the request through every admission webhook and
policy, but does not store the object:

```bash
./test-policies.sh
```

Expected output (abridged):

```
== gatekeeper (policy-test-gatekeeper)
  PASS: policy-test-gatekeeper allows pod-compliant.yaml
  PASS: policy-test-gatekeeper denies pod-missing-team-label.yaml
  PASS: policy-test-gatekeeper denies pod-latest-tag.yaml
  ...
== kyverno extras
  PASS: mutate added environment=dev
  PASS: generate created NetworkPolicy
  PASS: generate created ResourceQuota
== cel extras
  PASS: policy-test-cel allows deployment-compliant.yaml
  PASS: policy-test-cel denies deployment-too-many-replicas.yaml
Result: 21 passed, 0 failed   (22 with the optional signed-image check)
```

Look at a denial yourself, to see which engine rejected it:

```bash
kubectl apply --dry-run=server -n policy-test-gatekeeper -f tests/fixtures/pod-latest-tag.yaml
kubectl apply --dry-run=server -n policy-test-cel -f tests/fixtures/pod-untrusted-registry.yaml
```

### Offline checks

```bash
./test-policies.sh --offline
```

These run the `gator` suite (`gatekeeper-policies/tests/suite.yaml`) and the `kyverno test`
suite (`kyverno-policies/tests/kyverno-test.yaml`). Each is skipped with a message if its
CLI is not installed.

### Confirm the generate rule

```bash
kubectl apply -f tests/fixtures/namespace-generate.yaml
kubectl -n policy-test-generated get networkpolicy,resourcequota
```

---

## Cleanup

```bash
./teardown.sh
```

This deletes the policies, the test namespaces and both helm releases. Remove the test
namespaces only with `kubectl delete namespace policy-test-gatekeeper policy-test-kyverno policy-test-cel policy-test-generated`
if you want to keep the engines installed.

---

## Troubleshooting

### `no matches for kind "K8sRequiredLabels"`
The constraint was applied before its template's CRD existed. Apply the templates, wait for
the CRD (`kubectl get crd | grep constraints.gatekeeper.sh`), then apply the constraints.

### Gatekeeper templates fail with a Rego error
The templates use the OPA v1 syntax (`violation contains ... if { ... }`). Check the
Gatekeeper version with `helm list -n gatekeeper-system` and see the template status:
`kubectl get constrainttemplate k8srequiredlabels -o yaml`.

### Kyverno policy ignored (no denial)
Check the policy is `Ready` and that the test namespace has the label Kyverno matches:
```bash
kubectl get clusterpolicy
kubectl get clusterpolicy require-team-label -o jsonpath='{.status.conditions}'
kubectl -n kyverno logs deploy/kyverno-admission-controller | tail -30
```

### `ValidatingAdmissionPolicy` denial is missing
The binding must select the namespace. Check the binding and its parameters:
```bash
kubectl get validatingadmissionpolicybinding
kubectl -n policy-test-cel get configmap allowed-registries
```
With `parameterNotFoundAction: Deny`, a missing ConfigMap blocks every request in that namespace, so restore the ConfigMap rather than removing the binding.

### `test-policies.sh` reports `denied for unexpected reason`
Read the message the script printed. Each engine prefixes its message (`[gatekeeper]`,
`[kyverno]`, `[cel]`), so a mismatch usually means the wrong engine rejected the object.

### `verify-image-signatures` test is skipped
The policy was not applied. Run `./setup.sh --with-verify-images` or apply the file by hand, and
confirm the cluster can reach `ghcr.io` and `rekor.sigstore.dev`.

---

## Next Steps

1. **Read**: [Policy Engine concepts](../../docs/12-security-compliance/policy-engine.md) and [Pod Security](../../docs/12-security-compliance/pod-security.md)
2. **Lab 06**: [Observability Stack](../lab-06-observability-stack/README.md), to watch admission metrics and logs
3. **Lab 07**: [Multi-Tenancy](../lab-07-multi-tenancy/README.md), where the generate rule and quotas become tenant guardrails
4. **Exercise**: Change the Gatekeeper constraint to `enforcementAction: warn` and confirm a bad pod is admitted with a warning.

---

## Resources

- [Gatekeeper documentation](https://open-policy-agent.github.io/gatekeeper/website/docs/)
- [Gatekeeper policy library](https://open-policy-agent.github.io/gatekeeper-library/website/)
- [Kyverno documentation](https://kyverno.io/docs/)
- [Kyverno CLI test](https://kyverno.io/docs/kyverno-cli/usage/test/)
- [ValidatingAdmissionPolicy](https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/)
- [CEL in Kubernetes](https://kubernetes.io/docs/reference/using-api/cel/)
