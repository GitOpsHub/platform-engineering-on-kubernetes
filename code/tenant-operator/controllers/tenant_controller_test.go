package controllers

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

func newReconciler(t *testing.T, objs ...client.Object) (*TenantReconciler, client.Client) {
	t.Helper()
	s := newScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&platformv1alpha1.Tenant{}).
		Build()
	return &TenantReconciler{Client: c, Scheme: s}, c
}

func sampleTenant() *platformv1alpha1.Tenant {
	return &platformv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "team-a"},
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
