package controller

import (
	"context"
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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

	v1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/workload-operator/api/v1alpha1"
)

const testNS = "team-payments-prod"

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func newReconciler(t *testing.T, objs ...client.Object) (*WorkloadReconciler, client.Client) {
	t.Helper()
	s := newScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&v1alpha1.Workload{}).
		Build()
	return &WorkloadReconciler{Client: c, Scheme: s}, c
}

func sampleWorkload() *v1alpha1.Workload {
	replicas := int32(2)
	return &v1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "checkout", Namespace: testNS, UID: "uid-checkout", Generation: 1,
			Labels: map[string]string{
				v1alpha1.LabelTenant:      "team-payments",
				v1alpha1.LabelEnvironment: "prod",
			},
		},
		Spec: v1alpha1.WorkloadSpec{
			Image:    "ghcr.io/example/checkout:1.0",
			Replicas: &replicas,
			Port:     8080,
			Env:      []corev1.EnvVar{{Name: "LOG_LEVEL", Value: "info"}},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: mustQuantity("100m")},
			},
		},
	}
}

func mustQuantity(s string) resource.Quantity { return resource.MustParse(s) }

func config(t *testing.T, v any) *apiextensionsv1.JSON {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return &apiextensionsv1.JSON{Raw: raw}
}

func reconcile(t *testing.T, r *WorkloadReconciler, name string) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: testNS, Name: name}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	return res
}

func getWorkload(t *testing.T, c client.Client, name string) *v1alpha1.Workload {
	t.Helper()
	w := &v1alpha1.Workload{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: name}, w); err != nil {
		t.Fatalf("get workload: %v", err)
	}
	return w
}

func condition(t *testing.T, w *v1alpha1.Workload, typ string) metav1.Condition {
	t.Helper()
	c := meta.FindStatusCondition(w.Status.Conditions, typ)
	if c == nil {
		t.Fatalf("condition %s missing: %+v", typ, w.Status.Conditions)
	}
	return *c
}

// markDeploymentAvailable simulates the Deployment controller reporting ready replicas.
func markDeploymentAvailable(t *testing.T, c client.Client, name string, available int32) {
	t.Helper()
	dep := &appsv1.Deployment{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: name}, dep); err != nil {
		t.Fatal(err)
	}
	dep.Status.AvailableReplicas = available
	dep.Status.Replicas = available
	dep.Status.ReadyReplicas = available
	if err := c.Status().Update(context.Background(), dep); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileCreatesDeploymentAndService(t *testing.T) {
	ctx := context.Background()
	r, c := newReconciler(t, sampleWorkload())
	reconcile(t, r, "checkout")

	dep := &appsv1.Deployment{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: testNS, Name: "checkout"}, dep); err != nil {
		t.Fatal(err)
	}
	if *dep.Spec.Replicas != 2 {
		t.Fatalf("replicas = %d", *dep.Spec.Replicas)
	}
	ctr := dep.Spec.Template.Spec.Containers[0]
	if ctr.Image != "ghcr.io/example/checkout:1.0" || ctr.Ports[0].ContainerPort != 8080 {
		t.Fatalf("container = %+v", ctr)
	}
	if ctr.Resources.Requests.Cpu().String() != "100m" || len(ctr.Env) != 1 {
		t.Fatalf("container spec not carried over: %+v", ctr)
	}
	if !metav1.IsControlledBy(dep, getWorkload(t, c, "checkout")) {
		t.Fatalf("deployment has no controller reference to the workload")
	}
	if dep.Spec.Template.Labels[v1alpha1.WorkloadLabel] != "checkout" {
		t.Fatalf("pod labels = %v", dep.Spec.Template.Labels)
	}

	svc := &corev1.Service{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: testNS, Name: "checkout"}, svc); err != nil {
		t.Fatal(err)
	}
	if svc.Spec.Ports[0].Port != 8080 || svc.Spec.Selector[v1alpha1.WorkloadLabel] != "checkout" {
		t.Fatalf("service = %+v", svc.Spec)
	}

	w := getWorkload(t, c, "checkout")
	if w.Status.Phase != v1alpha1.PhaseProgressing {
		t.Fatalf("phase = %q (no replicas available yet)", w.Status.Phase)
	}
	if st := condition(t, w, v1alpha1.ConditionDeploymentReady).Status; st != metav1.ConditionFalse {
		t.Fatalf("DeploymentReady = %v", st)
	}
}

func TestStatusIsDerivedFromDeployment(t *testing.T) {
	r, c := newReconciler(t, sampleWorkload())
	reconcile(t, r, "checkout")
	markDeploymentAvailable(t, c, "checkout", 2)
	reconcile(t, r, "checkout")

	w := getWorkload(t, c, "checkout")
	if w.Status.Phase != v1alpha1.PhaseReady || w.Status.AvailableReplicas != 2 {
		t.Fatalf("status = %+v", w.Status)
	}
	if !meta.IsStatusConditionTrue(w.Status.Conditions, v1alpha1.ConditionReady) {
		t.Fatalf("Ready not true")
	}
	if w.Status.ObservedGeneration != 1 {
		t.Fatalf("observedGeneration = %d", w.Status.ObservedGeneration)
	}
}

func TestProgressDeadlineMarksFailed(t *testing.T) {
	r, c := newReconciler(t, sampleWorkload())
	reconcile(t, r, "checkout")

	dep := &appsv1.Deployment{}
	_ = c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: "checkout"}, dep)
	dep.Status.Conditions = []appsv1.DeploymentCondition{{
		Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: ReasonProgressDeadline,
	}}
	if err := c.Status().Update(context.Background(), dep); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, "checkout")
	if w := getWorkload(t, c, "checkout"); w.Status.Phase != v1alpha1.PhaseFailed {
		t.Fatalf("phase = %q", w.Status.Phase)
	}
}

func TestIngressTraitCreatesIngressAndURL(t *testing.T) {
	ctx := context.Background()
	w := sampleWorkload()
	w.Spec.Traits = []v1alpha1.Trait{{Type: v1alpha1.TraitIngress, Config: config(t, map[string]any{"host": "checkout.example.com"})}}
	r, c := newReconciler(t, w)
	reconcile(t, r, "checkout")

	ing := &networkingv1.Ingress{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: testNS, Name: "checkout"}, ing); err != nil {
		t.Fatal(err)
	}
	if ing.Spec.Rules[0].Host != "checkout.example.com" {
		t.Fatalf("ingress = %+v", ing.Spec)
	}
	if got := getWorkload(t, c, "checkout").Status.URL; got != "https://checkout.example.com" {
		t.Fatalf("url = %q", got)
	}
}

func TestRemovedTraitIsDeleted(t *testing.T) {
	ctx := context.Background()
	w := sampleWorkload()
	w.Spec.Traits = []v1alpha1.Trait{{Type: v1alpha1.TraitPodDisruption}}
	r, c := newReconciler(t, w)
	reconcile(t, r, "checkout")
	if err := c.Get(ctx, types.NamespacedName{Namespace: testNS, Name: "checkout"}, &policyv1.PodDisruptionBudget{}); err != nil {
		t.Fatalf("pdb not created: %v", err)
	}

	cur := getWorkload(t, c, "checkout")
	cur.Spec.Traits = nil
	if err := c.Update(ctx, cur); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, "checkout")
	if err := c.Get(ctx, types.NamespacedName{Namespace: testNS, Name: "checkout"}, &policyv1.PodDisruptionBudget{}); !apierrors.IsNotFound(err) {
		t.Fatalf("pdb should be deleted after trait removal, got %v", err)
	}
}

func TestAutoscalingTraitOwnsReplicas(t *testing.T) {
	ctx := context.Background()
	w := sampleWorkload()
	w.Spec.Traits = []v1alpha1.Trait{{Type: v1alpha1.TraitAutoscaling, Config: config(t, map[string]any{"maxReplicas": 6})}}
	r, c := newReconciler(t, w)
	reconcile(t, r, "checkout")

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: testNS, Name: "checkout"}, hpa); err != nil {
		t.Fatal(err)
	}
	if hpa.Spec.MaxReplicas != 6 || hpa.Spec.ScaleTargetRef.Name != "checkout" {
		t.Fatalf("hpa = %+v", hpa.Spec)
	}

	// The HPA changes the live replica count; the reconciler must not reset it.
	dep := &appsv1.Deployment{}
	_ = c.Get(ctx, types.NamespacedName{Namespace: testNS, Name: "checkout"}, dep)
	five := int32(5)
	dep.Spec.Replicas = &five
	if err := c.Update(ctx, dep); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, "checkout")
	_ = c.Get(ctx, types.NamespacedName{Namespace: testNS, Name: "checkout"}, dep)
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 5 {
		t.Fatalf("reconcile reset HPA-owned replicas: %v", dep.Spec.Replicas)
	}
}

func TestInvalidTraitConfigFailsWithoutWriting(t *testing.T) {
	ctx := context.Background()
	w := sampleWorkload()
	w.Spec.Traits = []v1alpha1.Trait{{Type: v1alpha1.TraitAutoscaling, Config: config(t, map[string]any{"maxReplicas": 1, "minReplicas": 3})}}
	r, c := newReconciler(t, w)
	reconcile(t, r, "checkout")

	got := getWorkload(t, c, "checkout")
	if got.Status.Phase != v1alpha1.PhaseFailed {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if cond := condition(t, got, v1alpha1.ConditionTraitsApplied); cond.Reason != ReasonInvalidTrait {
		t.Fatalf("reason = %q", cond.Reason)
	}
	// Validation happens before any write, so no Deployment exists.
	if err := c.Get(ctx, types.NamespacedName{Namespace: testNS, Name: "checkout"}, &appsv1.Deployment{}); !apierrors.IsNotFound(err) {
		t.Fatalf("deployment created despite invalid trait: %v", err)
	}
}

func TestDuplicateAndUnknownTraitsFail(t *testing.T) {
	cases := map[string][]v1alpha1.Trait{
		"duplicate": {{Type: v1alpha1.TraitPodDisruption}, {Type: v1alpha1.TraitPodDisruption}},
		"unknown":   {{Type: "cron"}},
	}
	for name, traits := range cases {
		t.Run(name, func(t *testing.T) {
			w := sampleWorkload()
			w.Spec.Traits = traits
			r, c := newReconciler(t, w)
			reconcile(t, r, "checkout")
			got := getWorkload(t, c, "checkout")
			if got.Status.Phase != v1alpha1.PhaseFailed {
				t.Fatalf("phase = %q", got.Status.Phase)
			}
		})
	}
}

func TestForeignDeploymentIsNotOverwritten(t *testing.T) {
	ctx := context.Background()
	foreign := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: testNS},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "theirs", Image: "other:1"}},
		}}}}
	r, c := newReconciler(t, sampleWorkload(), foreign)
	reconcile(t, r, "checkout")

	got := getWorkload(t, c, "checkout")
	if cond := condition(t, got, v1alpha1.ConditionDeploymentReady); cond.Reason != ReasonObjectConflict {
		t.Fatalf("reason = %q", cond.Reason)
	}
	dep := &appsv1.Deployment{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: testNS, Name: "checkout"}, dep); err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Template.Spec.Containers[0].Image != "other:1" || len(dep.OwnerReferences) != 0 {
		t.Fatalf("foreign deployment modified: %+v", dep.Spec.Template.Spec.Containers)
	}
}

func TestForeignIngressIsNotDeleted(t *testing.T) {
	ctx := context.Background()
	// A trait is removed, but the Ingress with the same name belongs to someone else.
	foreign := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "checkout", Namespace: testNS}}
	w := sampleWorkload()
	r, c := newReconciler(t, w, foreign)
	reconcile(t, r, "checkout")
	if err := c.Get(ctx, types.NamespacedName{Namespace: testNS, Name: "checkout"}, &networkingv1.Ingress{}); err != nil {
		t.Fatalf("foreign ingress deleted: %v", err)
	}
}

func TestMissingWorkloadIsNoop(t *testing.T) {
	r, _ := newReconciler(t)
	reconcile(t, r, "does-not-exist")
}

func TestUnknownFieldInTraitConfigIsRejected(t *testing.T) {
	w := sampleWorkload()
	w.Spec.Traits = []v1alpha1.Trait{{Type: v1alpha1.TraitIngress, Config: &apiextensionsv1.JSON{Raw: []byte(`{"host":"a.example.com","hots":"typo"}`)}}}
	r, c := newReconciler(t, w)
	reconcile(t, r, "checkout")
	if got := getWorkload(t, c, "checkout"); got.Status.Phase != v1alpha1.PhaseFailed {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}
