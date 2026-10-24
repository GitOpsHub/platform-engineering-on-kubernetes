package controller

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	platformv1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/environment-operator/api/v1alpha1"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/environment-operator/templates"
)

const (
	testOperatorNS = "environment-operator-system"
	testNamespace  = "platform-envs"
)

var fixedNow = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

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

func homeNamespace(tenant string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   testNamespace,
		Labels: map[string]string{platformv1alpha1.LabelTenant: tenant},
	}}
}

func newReconciler(t *testing.T, objs ...client.Object) (*EnvironmentReconciler, client.Client) {
	t.Helper()
	s := newScheme(t)
	// The Environment's own namespace carries the tenant label the controller trusts.
	hasHome := false
	for _, o := range objs {
		if o.GetName() == testNamespace {
			hasHome = true
		}
	}
	if !hasHome {
		objs = append(objs, homeNamespace("team-a"))
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&platformv1alpha1.Environment{}).
		Build()
	return &EnvironmentReconciler{
		Client:            c,
		Scheme:            s,
		OperatorNamespace: testOperatorNS,
		BaseDomain:        "apps.test",
		Now:               func() time.Time { return fixedNow },
	}, c
}

func sampleEnv(envType platformv1alpha1.EnvironmentType) *platformv1alpha1.Environment {
	return &platformv1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pr-1",
			Namespace:         testNamespace,
			UID:               "uid-pr-1",
			CreationTimestamp: metav1.NewTime(fixedNow.Add(-time.Hour)),
		},
		Spec: platformv1alpha1.EnvironmentSpec{Tenant: "team-a", Type: envType},
	}
}

func reconcileKey(t *testing.T, r *EnvironmentReconciler, name string) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNamespace, Name: name}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	return res
}

func getEnv(t *testing.T, c client.Client, name string) *platformv1alpha1.Environment {
	t.Helper()
	env := &platformv1alpha1.Environment{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNamespace, Name: name}, env); err != nil {
		t.Fatalf("get environment: %v", err)
	}
	return env
}

func condition(t *testing.T, env *platformv1alpha1.Environment, typ string) metav1.Condition {
	t.Helper()
	c := meta.FindStatusCondition(env.Status.Conditions, typ)
	if c == nil {
		t.Fatalf("condition %s missing: %+v", typ, env.Status.Conditions)
	}
	return *c
}

func TestRenderBaselinePerType(t *testing.T) {
	for _, typ := range []platformv1alpha1.EnvironmentType{"dev", "staging", "prod", "preview"} {
		t.Run(string(typ), func(t *testing.T) {
			env := sampleEnv(typ)
			env.Spec.Template = ""
			objs, err := renderBaseline(templates.FS, env, "team-a-pr-1", "https://pr-1.team-a.apps.test")
			if err != nil {
				t.Fatal(err)
			}
			if len(objs) != 3 {
				t.Fatalf("got %d objects, want 3", len(objs))
			}
			kinds := map[string]bool{}
			for _, o := range objs {
				kinds[o.GetObjectKind().GroupVersionKind().Kind+"/"+o.GetName()] = true
				if o.GetNamespace() != "team-a-pr-1" {
					t.Errorf("%s rendered in namespace %q", o.GetName(), o.GetNamespace())
				}
				if o.GetAnnotations()[platformv1alpha1.OwnerUIDAnnotation] != "uid-pr-1" {
					t.Errorf("%s missing owner-uid annotation", o.GetName())
				}
			}
			for _, want := range []string{"ResourceQuota/env-quota", "NetworkPolicy/env-baseline", "ConfigMap/env-config"} {
				if !kinds[want] {
					t.Errorf("missing %s; got %v", want, kinds)
				}
			}
		})
	}
}

func TestProdQuotaIsLargerThanDev(t *testing.T) {
	dev, err := renderBaseline(templates.FS, sampleEnv("dev"), "ns", "u")
	if err != nil {
		t.Fatal(err)
	}
	prod, err := renderBaseline(templates.FS, sampleEnv("prod"), "ns", "u")
	if err != nil {
		t.Fatal(err)
	}
	quotaCPU := func(objs []client.Object) resource.Quantity {
		for _, o := range objs {
			if rq, ok := o.(*corev1.ResourceQuota); ok {
				return rq.Spec.Hard[corev1.ResourceRequestsCPU]
			}
		}
		t.Fatal("no quota")
		return resource.Quantity{}
	}
	if d, p := quotaCPU(dev), quotaCPU(prod); p.Cmp(d) <= 0 {
		t.Fatalf("prod cpu %s should exceed dev %s", p.String(), d.String())
	}
}

func TestReconcileCreatesNamespaceAndBaseline(t *testing.T) {
	ctx := context.Background()
	r, c := newReconciler(t, sampleEnv("dev"))
	res := reconcileKey(t, r, "pr-1")

	env := getEnv(t, c, "pr-1")
	if env.Status.Phase != platformv1alpha1.PhaseReady {
		t.Fatalf("phase = %q; conditions=%+v", env.Status.Phase, env.Status.Conditions)
	}
	if env.Status.Namespace != "team-a-pr-1" {
		t.Fatalf("namespace = %q", env.Status.Namespace)
	}
	if env.Status.URL != "https://pr-1.team-a.apps.test" {
		t.Fatalf("url = %q", env.Status.URL)
	}
	if !meta.IsStatusConditionTrue(env.Status.Conditions, platformv1alpha1.ConditionReady) {
		t.Fatalf("Ready not true: %+v", env.Status.Conditions)
	}
	if !controllerFinalizerPresent(env) {
		t.Fatalf("finalizer missing: %v", env.Finalizers)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("no ttl set, should not requeue; got %v", res.RequeueAfter)
	}

	ns := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: "team-a-pr-1"}, ns); err != nil {
		t.Fatal(err)
	}
	if ns.Labels[platformv1alpha1.LabelManagedBy] != platformv1alpha1.ManagedByValue ||
		ns.Labels[platformv1alpha1.LabelTenant] != "team-a" || ns.Labels[platformv1alpha1.LabelEnvironment] != "pr-1" {
		t.Fatalf("namespace labels = %v", ns.Labels)
	}
	if ns.Annotations[platformv1alpha1.OwnerUIDAnnotation] != "uid-pr-1" {
		t.Fatalf("owner annotation = %v", ns.Annotations)
	}
	for _, check := range []client.Object{&corev1.ResourceQuota{}, &networkingv1.NetworkPolicy{}, &corev1.ConfigMap{}} {
		name := map[string]string{"*v1.ResourceQuota": "env-quota", "*v1.NetworkPolicy": "env-baseline", "*v1.ConfigMap": "env-config"}[typeName(check)]
		if err := c.Get(ctx, client.ObjectKey{Namespace: "team-a-pr-1", Name: name}, check); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func typeName(o client.Object) string {
	switch o.(type) {
	case *corev1.ResourceQuota:
		return "*v1.ResourceQuota"
	case *networkingv1.NetworkPolicy:
		return "*v1.NetworkPolicy"
	case *corev1.ConfigMap:
		return "*v1.ConfigMap"
	}
	return ""
}

func controllerFinalizerPresent(env *platformv1alpha1.Environment) bool {
	for _, f := range env.Finalizers {
		if f == platformv1alpha1.EnvironmentFinalizer {
			return true
		}
	}
	return false
}

// An unmanaged namespace with the target name is never adopted or modified.
func TestUnmanagedNamespaceIsNotAdopted(t *testing.T) {
	ctx := context.Background()
	legacy := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "team-a-pr-1", Labels: map[string]string{"owner": "someone"},
	}}
	r, c := newReconciler(t, sampleEnv("dev"), legacy)
	reconcileKey(t, r, "pr-1")

	env := getEnv(t, c, "pr-1")
	if env.Status.Phase != platformv1alpha1.PhaseFailed {
		t.Fatalf("phase = %q", env.Status.Phase)
	}
	if cond := condition(t, env, platformv1alpha1.ConditionNamespaceReady); cond.Status != metav1.ConditionFalse || cond.Reason != ReasonNamespaceConflict {
		t.Fatalf("NamespaceReady = %+v", cond)
	}
	ns := &corev1.Namespace{}
	if err := c.Get(ctx, client.ObjectKey{Name: "team-a-pr-1"}, ns); err != nil {
		t.Fatal(err)
	}
	if len(ns.Labels) != 1 || ns.Labels["owner"] != "someone" || ns.Annotations[platformv1alpha1.OwnerUIDAnnotation] != "" {
		t.Fatalf("unmanaged namespace modified: labels=%v annotations=%v", ns.Labels, ns.Annotations)
	}
	// Nothing was written into it.
	if err := c.Get(ctx, client.ObjectKey{Namespace: "team-a-pr-1", Name: "env-quota"}, &corev1.ResourceQuota{}); !apierrors.IsNotFound(err) {
		t.Fatalf("baseline written into unmanaged namespace: %v", err)
	}
}

// Deleting the Environment must not delete a namespace it does not own.
func TestFinalizerDoesNotDeleteUnownedNamespace(t *testing.T) {
	ctx := context.Background()
	env := sampleEnv("dev")
	env.Finalizers = []string{platformv1alpha1.EnvironmentFinalizer}
	env.DeletionTimestamp = &metav1.Time{Time: fixedNow}
	env.Status.Namespace = "team-a-pr-1"
	foreign := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "team-a-pr-1",
		Labels: map[string]string{
			platformv1alpha1.LabelManagedBy: platformv1alpha1.ManagedByValue,
			platformv1alpha1.LabelTenant:    "team-a",
		},
		Annotations: map[string]string{platformv1alpha1.OwnerUIDAnnotation: "someone-else"},
	}}
	r, c := newReconciler(t, env, foreign)
	reconcileKey(t, r, "pr-1")

	if err := c.Get(ctx, client.ObjectKey{Name: "team-a-pr-1"}, &corev1.Namespace{}); err != nil {
		t.Fatalf("namespace owned by another environment was deleted: %v", err)
	}
	// The finalizer is released so the Environment can go away.
	err := c.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: "pr-1"}, &platformv1alpha1.Environment{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("environment should be gone after finalizer release, got %v", err)
	}
}

func TestFinalizerDeletesOwnedNamespace(t *testing.T) {
	ctx := context.Background()
	env := sampleEnv("dev")
	env.Finalizers = []string{platformv1alpha1.EnvironmentFinalizer}
	env.DeletionTimestamp = &metav1.Time{Time: fixedNow}
	env.Status.Namespace = "team-a-pr-1"
	owned := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name: "team-a-pr-1",
		Labels: map[string]string{
			platformv1alpha1.LabelManagedBy: platformv1alpha1.ManagedByValue,
			platformv1alpha1.LabelTenant:    "team-a",
		},
		Annotations: map[string]string{platformv1alpha1.OwnerUIDAnnotation: "uid-pr-1"},
	}}
	r, c := newReconciler(t, env, owned)
	// First pass issues the delete; the fake client removes it immediately, so
	// the second pass sees it gone and releases the finalizer.
	reconcileKey(t, r, "pr-1")
	if err := c.Get(ctx, client.ObjectKey{Name: "team-a-pr-1"}, &corev1.Namespace{}); !apierrors.IsNotFound(err) {
		t.Fatalf("owned namespace should be deleted, got %v", err)
	}
}

func TestInvalidNamespaceIsRejected(t *testing.T) {
	for _, tc := range []struct{ tenant, name string }{
		{"kube", "system"},                  // kube-system is reserved
		{"team-a", "UPPER"},                 // not a DNS-1123 label
		{"Team", "a"},                       // uppercase tenant
		{"team-a", strings.Repeat("x", 60)}, // longer than 63 characters
		{"environment", "operator-system"},  // equals the operator namespace below
	} {
		t.Run(tc.tenant+"-"+tc.name, func(t *testing.T) {
			env := sampleEnv("dev")
			env.Spec.Tenant = tc.tenant
			env.Name = tc.name
			r, c := newReconciler(t, env, homeNamespace(tc.tenant))
			r.OperatorNamespace = "environment-operator-system"
			reconcileKey(t, r, tc.name)
			got := getEnv(t, c, tc.name)
			if got.Status.Phase != platformv1alpha1.PhaseFailed {
				t.Fatalf("phase = %q", got.Status.Phase)
			}
			if cond := condition(t, got, platformv1alpha1.ConditionNamespaceReady); cond.Reason != ReasonInvalidNamespace {
				t.Fatalf("reason = %q", cond.Reason)
			}
		})
	}
}

func TestUnknownTemplateSetFails(t *testing.T) {
	env := sampleEnv("dev")
	env.Spec.Template = "nope"
	r, c := newReconciler(t, env)
	reconcileKey(t, r, "pr-1")
	got := getEnv(t, c, "pr-1")
	if cond := condition(t, got, platformv1alpha1.ConditionTemplatesReady); cond.Reason != ReasonUnknownTemplate {
		t.Fatalf("reason = %q", cond.Reason)
	}
}

func TestForeignChildObjectIsNotOverwritten(t *testing.T) {
	ctx := context.Background()
	foreign := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "env-config", Namespace: "team-a-pr-1"},
		Data: map[string]string{"keep": "me"}}
	r, c := newReconciler(t, sampleEnv("dev"), foreign)
	reconcileKey(t, r, "pr-1")

	got := getEnv(t, c, "pr-1")
	if cond := condition(t, got, platformv1alpha1.ConditionTemplatesReady); cond.Reason != ReasonObjectConflict {
		t.Fatalf("reason = %q", cond.Reason)
	}
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: "team-a-pr-1", Name: "env-config"}, cm); err != nil {
		t.Fatal(err)
	}
	if cm.Data["keep"] != "me" || len(cm.Data) != 1 {
		t.Fatalf("foreign configmap overwritten: %v", cm.Data)
	}
}

func TestTTLRequeuesUntilExpiry(t *testing.T) {
	env := sampleEnv("preview")
	ttl := metav1.Duration{Duration: 3 * time.Hour}
	env.Spec.TTL = &ttl
	r, c := newReconciler(t, env)
	res := reconcileKey(t, r, "pr-1")

	got := getEnv(t, c, "pr-1")
	if got.Status.ExpiresAt == nil || !got.Status.ExpiresAt.Time.Equal(fixedNow.Add(2*time.Hour)) {
		t.Fatalf("expiresAt = %v", got.Status.ExpiresAt)
	}
	if res.RequeueAfter != 2*time.Hour {
		t.Fatalf("RequeueAfter = %v, want 2h", res.RequeueAfter)
	}
}

func TestExpiredEnvironmentIsDeleted(t *testing.T) {
	ctx := context.Background()
	env := sampleEnv("preview")
	ttl := metav1.Duration{Duration: 30 * time.Minute}
	env.Spec.TTL = &ttl
	env.Finalizers = []string{platformv1alpha1.EnvironmentFinalizer}
	r, c := newReconciler(t, env)
	// Created an hour ago with a 30 minute TTL: expired.
	reconcileKey(t, r, "pr-1")

	err := c.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: "pr-1"}, &platformv1alpha1.Environment{})
	if err == nil {
		// Still present because the finalizer is held until the namespace is gone.
		got := getEnv(t, c, "pr-1")
		if got.DeletionTimestamp.IsZero() {
			t.Fatalf("expired environment was not deleted")
		}
		if got.Status.Phase != platformv1alpha1.PhaseExpired {
			t.Fatalf("phase = %q, want Expired", got.Status.Phase)
		}
		return
	}
	if !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
}

func TestEnvironmentForOtherClusterIsSkipped(t *testing.T) {
	env := sampleEnv("dev")
	env.Spec.Cluster = "eu-west"
	r, c := newReconciler(t, env)
	r.ClusterName = "us-east"
	reconcileKey(t, r, "pr-1")
	got := getEnv(t, c, "pr-1")
	if len(got.Finalizers) != 0 || got.Status.Phase != "" {
		t.Fatalf("environment for another cluster was reconciled: %+v", got.Status)
	}
}

func TestMissingEnvironmentIsNoop(t *testing.T) {
	r, _ := newReconciler(t)
	reconcileKey(t, r, "does-not-exist")
}

func TestTemplateFSFixture(t *testing.T) {
	// renderBaseline must work with any fs.FS, not only the embedded one.
	fsys := fstest.MapFS{
		"baseline/dev/configmap.yaml.tmpl": &fstest.MapFile{Data: []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: only
  namespace: "{{ .Namespace }}"
data:
  T: "{{ .Tenant }}"
`)},
	}
	objs, err := renderBaseline(fsys, sampleEnv("dev"), "team-a-pr-1", "u")
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 || !strings.Contains(objs[0].GetName(), "only") {
		t.Fatalf("objs = %v", objs)
	}
}

// spec.tenant must match the tenant label of the Environment's namespace.
func TestTenantMismatchIsRejected(t *testing.T) {
	ctx := context.Background()
	env := sampleEnv("dev")
	env.Spec.Tenant = "team-b" // namespace platform-envs is labelled team-a
	r, c := newReconciler(t, env)
	reconcileKey(t, r, "pr-1")

	got := getEnv(t, c, "pr-1")
	if got.Status.Phase != platformv1alpha1.PhaseFailed {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if cond := condition(t, got, platformv1alpha1.ConditionTenantVerified); cond.Status != metav1.ConditionFalse || cond.Reason != ReasonTenantMismatch {
		t.Fatalf("TenantVerified = %+v", cond)
	}
	for _, name := range []string{"team-a-pr-1", "team-b-pr-1"} {
		if err := c.Get(ctx, client.ObjectKey{Name: name}, &corev1.Namespace{}); !apierrors.IsNotFound(err) {
			t.Fatalf("namespace %q must not be created on mismatch, got %v", name, err)
		}
	}
}

// Without a tenant label on the Environment's namespace the tenant cannot be verified.
func TestUnverifiableTenantIsRejected(t *testing.T) {
	env := sampleEnv("dev")
	unlabelled := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
	r, c := newReconciler(t, env, unlabelled)
	reconcileKey(t, r, "pr-1")

	got := getEnv(t, c, "pr-1")
	if cond := condition(t, got, platformv1alpha1.ConditionTenantVerified); cond.Reason != ReasonTenantMismatch {
		t.Fatalf("TenantVerified = %+v", cond)
	}
}
