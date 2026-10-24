package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	platformv1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/tenant-operator/api/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := platformv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

const testOperatorNamespace = "tenant-operator-system"

func newReconciler(t *testing.T, objs ...client.Object) (*TenantReconciler, client.Client) {
	t.Helper()
	s := newScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&platformv1alpha1.Tenant{}).
		Build()
	return &TenantReconciler{Client: c, Scheme: s, OperatorNamespace: testOperatorNamespace}, c
}

func conditionReason(t *testing.T, tn *platformv1alpha1.Tenant, condType string) (metav1.ConditionStatus, string) {
	t.Helper()
	cond := meta.FindStatusCondition(tn.Status.Conditions, condType)
	if cond == nil {
		t.Fatalf("condition %s missing; conditions=%+v", condType, tn.Status.Conditions)
	}
	return cond.Status, cond.Reason
}

func sampleTenant() *platformv1alpha1.Tenant {
	return &platformv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "team-a", UID: "uid-team-a"},
		Spec: platformv1alpha1.TenantSpec{
			DisplayName:      "Team A",
			Owners:           []string{"alice@example.com", "group:team-a-admins"},
			Namespaces:       []string{"team-a-dev", "team-a-prod"},
			Quota:            platformv1alpha1.TenantQuota{CPU: "4", Memory: "8Gi", Storage: "50Gi", Pods: 20},
			NetworkIsolation: true,
		},
	}
}

func reconcile(t *testing.T, r *TenantReconciler, name string) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: name}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	return res
}

func getTenant(t *testing.T, c client.Client, name string) *platformv1alpha1.Tenant {
	t.Helper()
	tn := &platformv1alpha1.Tenant{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: name}, tn); err != nil {
		t.Fatalf("get tenant: %v", err)
	}
	return tn
}

func TestReconcileCreatesTenantResources(t *testing.T) {
	ctx := context.Background()
	r, c := newReconciler(t, sampleTenant())
	reconcile(t, r, "team-a")

	tn := getTenant(t, c, "team-a")
	if tn.Status.Phase != platformv1alpha1.PhaseReady {
		t.Fatalf("phase = %q, want Ready; conditions=%+v", tn.Status.Phase, tn.Status.Conditions)
	}
	if !meta.IsStatusConditionTrue(tn.Status.Conditions, platformv1alpha1.ConditionReady) {
		t.Fatalf("Ready condition not true: %+v", tn.Status.Conditions)
	}
	if len(tn.Status.Namespaces) != 2 {
		t.Fatalf("status.namespaces = %v", tn.Status.Namespaces)
	}
	if len(tn.Finalizers) != 1 || tn.Finalizers[0] != platformv1alpha1.TenantFinalizer {
		t.Fatalf("finalizers = %v", tn.Finalizers)
	}

	for _, nsName := range []string{"team-a-dev", "team-a-prod"} {
		ns := &corev1.Namespace{}
		if err := c.Get(ctx, client.ObjectKey{Name: nsName}, ns); err != nil {
			t.Fatalf("namespace %s: %v", nsName, err)
		}
		if ns.Labels[platformv1alpha1.LabelTenant] != "team-a" ||
			ns.Labels[platformv1alpha1.LabelManagedBy] != platformv1alpha1.ManagedByValue {
			t.Errorf("namespace %s labels = %v", nsName, ns.Labels)
		}
		if len(ns.OwnerReferences) != 1 || ns.OwnerReferences[0].Name != "team-a" {
			t.Errorf("namespace %s ownerRefs = %+v", nsName, ns.OwnerReferences)
		}

		rq := &corev1.ResourceQuota{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: nsName, Name: ResourceQuotaName}, rq); err != nil {
			t.Fatalf("quota: %v", err)
		}
		if got := rq.Spec.Hard[corev1.ResourceLimitsCPU]; got.Cmp(resource.MustParse("4")) != 0 {
			t.Errorf("limits.cpu = %s", got.String())
		}
		if got := rq.Spec.Hard[corev1.ResourcePods]; got.Value() != 20 {
			t.Errorf("pods = %s", got.String())
		}

		if err := c.Get(ctx, client.ObjectKey{Namespace: nsName, Name: LimitRangeName}, &corev1.LimitRange{}); err != nil {
			t.Errorf("limitrange: %v", err)
		}
		for _, np := range []string{DefaultDenyPolicyName, AllowTenantPolicyName, AllowDNSPolicyName} {
			if err := c.Get(ctx, client.ObjectKey{Namespace: nsName, Name: np}, &networkingv1.NetworkPolicy{}); err != nil {
				t.Errorf("networkpolicy %s: %v", np, err)
			}
		}

		rb := &rbacv1.RoleBinding{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: nsName, Name: OwnersRoleBindingName}, rb); err != nil {
			t.Fatalf("rolebinding: %v", err)
		}
		if rb.RoleRef.Name != "admin" || len(rb.Subjects) != 2 {
			t.Errorf("rolebinding = %+v", rb)
		}
		if rb.Subjects[0].Kind != rbacv1.UserKind || rb.Subjects[1].Kind != rbacv1.GroupKind || rb.Subjects[1].Name != "team-a-admins" {
			t.Errorf("subjects = %+v", rb.Subjects)
		}
	}

	// Reconcile is idempotent.
	reconcile(t, r, "team-a")
}

func TestDisablingIsolationRemovesPolicies(t *testing.T) {
	ctx := context.Background()
	r, c := newReconciler(t, sampleTenant())
	reconcile(t, r, "team-a")

	tn := getTenant(t, c, "team-a")
	tn.Spec.NetworkIsolation = false
	if err := c.Update(ctx, tn); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, "team-a")

	err := c.Get(ctx, client.ObjectKey{Namespace: "team-a-dev", Name: DefaultDenyPolicyName}, &networkingv1.NetworkPolicy{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected default-deny to be removed, got %v", err)
	}
}

func TestRemovedNamespaceIsPruned(t *testing.T) {
	ctx := context.Background()
	r, c := newReconciler(t, sampleTenant())
	reconcile(t, r, "team-a")

	tn := getTenant(t, c, "team-a")
	tn.Spec.Namespaces = []string{"team-a-dev"}
	if err := c.Update(ctx, tn); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, "team-a")

	if err := c.Get(ctx, client.ObjectKey{Name: "team-a-prod"}, &corev1.Namespace{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected team-a-prod to be pruned, got %v", err)
	}
	if got := getTenant(t, c, "team-a").Status.Namespaces; len(got) != 1 || got[0] != "team-a-dev" {
		t.Fatalf("status.namespaces = %v", got)
	}
}

func TestInvalidQuotaSetsFailed(t *testing.T) {
	tn := sampleTenant()
	tn.Spec.Quota.CPU = "four"
	r, c := newReconciler(t, tn)

	// Terminal spec errors are not returned (no hot requeue loop).
	reconcile(t, r, "team-a")

	got := getTenant(t, c, "team-a")
	if got.Status.Phase != platformv1alpha1.PhaseFailed {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, platformv1alpha1.ConditionQuotaReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "InvalidQuota" {
		t.Fatalf("QuotaReady = %+v", cond)
	}
}

func TestNamespaceConflict(t *testing.T) {
	foreign := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "team-a-dev",
		Labels: map[string]string{platformv1alpha1.LabelTenant: "team-b"},
	}}
	r, c := newReconciler(t, sampleTenant(), foreign)
	reconcile(t, r, "team-a")

	got := getTenant(t, c, "team-a")
	cond := meta.FindStatusCondition(got.Status.Conditions, platformv1alpha1.ConditionNamespacesReady)
	if cond == nil || cond.Reason != "NamespaceConflict" {
		t.Fatalf("NamespacesReady = %+v", cond)
	}

	ns := &corev1.Namespace{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: "team-a-dev"}, ns); err != nil {
		t.Fatal(err)
	}
	if ns.Labels[platformv1alpha1.LabelTenant] != "team-b" {
		t.Fatalf("foreign namespace was hijacked: %v", ns.Labels)
	}
}

func TestDeletionCleansUpNamespaces(t *testing.T) {
	ctx := context.Background()
	r, c := newReconciler(t, sampleTenant())
	reconcile(t, r, "team-a")

	// The fake client honours finalizers: Delete only sets deletionTimestamp.
	if err := c.Delete(ctx, getTenant(t, c, "team-a")); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, "team-a")

	if err := c.Get(ctx, client.ObjectKey{Name: "team-a"}, &platformv1alpha1.Tenant{}); !apierrors.IsNotFound(err) {
		t.Fatalf("tenant should be gone after finalizer removal, got %v", err)
	}
	list := &corev1.NamespaceList{}
	if err := c.List(ctx, list, client.MatchingLabels{platformv1alpha1.LabelTenant: "team-a"}); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("expected tenant namespaces to be deleted, found %d", len(list.Items))
	}
}

func TestReconcileMissingTenantIsNoop(t *testing.T) {
	r, _ := newReconciler(t)
	reconcile(t, r, "does-not-exist")
}

// An unmanaged namespace with the requested name must be neither adopted
// (no owner reference, labels untouched) nor deleted when the tenant goes away.
func TestUnmanagedNamespaceIsNeitherAdoptedNorDeleted(t *testing.T) {
	ctx := context.Background()
	legacy := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "team-a-dev",
		Labels: map[string]string{"owner": "someone-else"},
	}}
	r, c := newReconciler(t, sampleTenant(), legacy)
	reconcile(t, r, "team-a")

	got := getTenant(t, c, "team-a")
	if st, reason := conditionReason(t, got, platformv1alpha1.ConditionNamespacesReady); st != metav1.ConditionFalse || reason != "NamespaceConflict" {
		t.Fatalf("NamespacesReady = %v/%s, want False/NamespaceConflict", st, reason)
	}
	if got.Status.Phase != platformv1alpha1.PhaseFailed {
		t.Fatalf("phase = %q", got.Status.Phase)
	}

	ns := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: "team-a-dev"}, ns); err != nil {
		t.Fatal(err)
	}
	if len(ns.OwnerReferences) != 0 {
		t.Fatalf("unmanaged namespace was adopted: ownerRefs=%+v", ns.OwnerReferences)
	}
	if _, ok := ns.Labels[platformv1alpha1.LabelTenant]; ok || ns.Labels["owner"] != "someone-else" {
		t.Fatalf("unmanaged namespace labels were modified: %v", ns.Labels)
	}

	// Deleting the tenant must not delete the namespace it never owned.
	if err := c.Delete(ctx, getTenant(t, c, "team-a")); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, "team-a")
	if err := c.Get(ctx, client.ObjectKey{Name: "team-a-dev"}, &corev1.Namespace{}); err != nil {
		t.Fatalf("unmanaged namespace was deleted: %v", err)
	}
}

// A namespace that only carries the tenant label (no managed-by, no owner ref)
// is not adoptable. Pruning must not delete it either.
func TestTenantLabelAloneDoesNotGrantOwnership(t *testing.T) {
	ctx := context.Background()
	spoofed := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   "team-a-prod",
		Labels: map[string]string{platformv1alpha1.LabelTenant: "team-a"},
	}}
	tn := sampleTenant()
	tn.Spec.Namespaces = []string{"team-a-dev"}
	r, c := newReconciler(t, tn, spoofed)
	reconcile(t, r, "team-a")

	if err := c.Get(ctx, client.ObjectKey{Name: "team-a-prod"}, &corev1.Namespace{}); err != nil {
		t.Fatalf("label-only namespace was deleted: %v", err)
	}
	if st, _ := conditionReason(t, getTenant(t, c, "team-a"), platformv1alpha1.ConditionNamespacesReady); st != metav1.ConditionTrue {
		t.Fatalf("NamespacesReady should be true for team-a-dev")
	}
}

// Managed-by + tenant labels on an existing namespace allow adoption; the
// adopted namespace then carries a controller reference.
func TestManagedNamespaceIsAdopted(t *testing.T) {
	ctx := context.Background()
	existing := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "team-a-dev",
		Labels: map[string]string{
			platformv1alpha1.LabelTenant:    "team-a",
			platformv1alpha1.LabelManagedBy: platformv1alpha1.ManagedByValue,
		},
	}}
	r, c := newReconciler(t, sampleTenant(), existing)
	reconcile(t, r, "team-a")

	ns := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: "team-a-dev"}, ns); err != nil {
		t.Fatal(err)
	}
	if !metav1.IsControlledBy(ns, getTenant(t, c, "team-a")) {
		t.Fatalf("adopted namespace has no controller reference to the tenant: %+v", ns.OwnerReferences)
	}
}

// Reserved and invalid namespace names are rejected before anything is created.
func TestReservedNamespaceNamesAreRejected(t *testing.T) {
	ctx := context.Background()
	for _, bad := range []string{"kube-system", "kube-public", "default", "platform-system", testOperatorNamespace, "Team_A"} {
		t.Run(bad, func(t *testing.T) {
			tn := sampleTenant()
			tn.Spec.Namespaces = []string{bad}
			r, c := newReconciler(t, tn)
			reconcile(t, r, "team-a")

			st, reason := conditionReason(t, getTenant(t, c, "team-a"), platformv1alpha1.ConditionNamespacesReady)
			if st != metav1.ConditionFalse || reason != ReasonInvalidNamespace {
				t.Fatalf("NamespacesReady = %v/%s", st, reason)
			}
			if err := c.Get(ctx, client.ObjectKey{Name: bad}, &corev1.Namespace{}); !apierrors.IsNotFound(err) {
				t.Fatalf("namespace %q should not exist, got %v", bad, err)
			}
		})
	}
}

// User labels cannot override operator ownership labels, and reserved keys are dropped.
func TestUserLabelsCannotOverrideOperatorLabels(t *testing.T) {
	ctx := context.Background()
	tn := sampleTenant()
	tn.Labels = map[string]string{
		platformv1alpha1.LabelTenant:         "someone-else",
		platformv1alpha1.LabelManagedBy:      "nobody",
		"pod-security.kubernetes.io/enforce": "privileged",
		"cost-center":                        "42",
	}
	tn.Annotations = map[string]string{
		"platform.mycompany.io/x":                          "bad",
		"kubectl.kubernetes.io/last-applied-configuration": "{}",
		"team-contact":                                     "a@example.com",
	}
	r, c := newReconciler(t, tn)
	reconcile(t, r, "team-a")

	ns := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: "team-a-dev"}, ns); err != nil {
		t.Fatal(err)
	}
	if ns.Labels[platformv1alpha1.LabelTenant] != "team-a" || ns.Labels[platformv1alpha1.LabelManagedBy] != platformv1alpha1.ManagedByValue {
		t.Fatalf("operator labels overridden: %v", ns.Labels)
	}
	if _, ok := ns.Labels["pod-security.kubernetes.io/enforce"]; ok {
		t.Fatalf("reserved label copied: %v", ns.Labels)
	}
	if ns.Labels["cost-center"] != "42" {
		t.Fatalf("user label missing: %v", ns.Labels)
	}
	if _, ok := ns.Annotations["platform.mycompany.io/x"]; ok {
		t.Fatalf("reserved annotation copied: %v", ns.Annotations)
	}
	if _, ok := ns.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		t.Fatalf("kubectl bookkeeping copied: %v", ns.Annotations)
	}
	if ns.Annotations["team-contact"] != "a@example.com" {
		t.Fatalf("user annotation missing: %v", ns.Annotations)
	}
}

// Pre-existing child objects that the tenant does not own are not overwritten.
func TestForeignQuotaIsNotOverwritten(t *testing.T) {
	ctx := context.Background()
	foreignQuota := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: ResourceQuotaName, Namespace: "team-a-dev"}}
	r, c := newReconciler(t, sampleTenant(), foreignQuota)
	reconcile(t, r, "team-a")

	got := getTenant(t, c, "team-a")
	if st, reason := conditionReason(t, got, platformv1alpha1.ConditionQuotaReady); st != metav1.ConditionFalse || reason != ReasonObjectConflict {
		t.Fatalf("QuotaReady = %v/%s", st, reason)
	}
	rq := &corev1.ResourceQuota{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: "team-a-dev", Name: ResourceQuotaName}, rq); err != nil {
		t.Fatal(err)
	}
	if len(rq.OwnerReferences) != 0 || len(rq.Spec.Hard) != 0 {
		t.Fatalf("foreign quota was modified: %+v", rq)
	}
}

// Stale cleanup only deletes namespaces that carry a controller reference to the tenant.
func TestPruneSkipsNamespacesNotOwnedByTenant(t *testing.T) {
	ctx := context.Background()
	tn := sampleTenant()
	tn.Spec.Namespaces = []string{"team-a-dev"}
	labelled := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "team-a-old",
		Labels: map[string]string{
			platformv1alpha1.LabelTenant:    "team-a",
			platformv1alpha1.LabelManagedBy: platformv1alpha1.ManagedByValue,
		},
	}}
	r, c := newReconciler(t, tn, labelled)
	reconcile(t, r, "team-a")

	if err := c.Get(ctx, client.ObjectKey{Name: "team-a-old"}, &corev1.Namespace{}); err != nil {
		t.Fatalf("namespace without controller reference was pruned: %v", err)
	}
}
