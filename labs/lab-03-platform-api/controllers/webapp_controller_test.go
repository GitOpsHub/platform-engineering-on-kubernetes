package controllers

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/labs/lab-03-platform-api/api/v1alpha1"
)

var appKey = types.NamespacedName{Name: "hello", Namespace: "developer-platform"}

func newWebApp() *v1alpha1.WebApp {
	return &v1alpha1.WebApp{
		ObjectMeta: metav1.ObjectMeta{Name: appKey.Name, Namespace: appKey.Namespace},
		Spec: v1alpha1.WebAppSpec{
			Image:    "nginxinc/nginx-unprivileged:1.29-alpine",
			Replicas: ptr.To[int32](2),
			Port:     ptr.To[int32](8080),
		},
	}
}

// newTestReconciler returns a reconciler backed by the in-memory fake client.
// No cluster is needed, so `go test ./...` runs anywhere.
func newTestReconciler(t *testing.T, objs ...client.Object) (*WebAppReconciler, client.Client) {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := v1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		// Status subresources must be registered so Status().Update behaves
		// as it does against a real API server.
		WithStatusSubresource(&v1alpha1.WebApp{}, &appsv1.Deployment{}).
		Build()
	return &WebAppReconciler{Client: c, Scheme: s}, c
}

func reconcileOnce(t *testing.T, r *WebAppReconciler) {
	t.Helper()
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: appKey}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
}

func TestReconcileCreatesDeploymentAndService(t *testing.T) {
	r, c := newTestReconciler(t, newWebApp())
	ctx := context.Background()

	reconcileOnce(t, r)

	var app v1alpha1.WebApp
	if err := c.Get(ctx, appKey, &app); err != nil {
		t.Fatal(err)
	}
	if !controllerHasFinalizer(&app) {
		t.Errorf("finalizer %q not added; finalizers = %v", FinalizerName, app.Finalizers)
	}
	if app.Status.URL != "http://hello.developer-platform.svc.cluster.local" {
		t.Errorf("status.url = %q", app.Status.URL)
	}

	var dep appsv1.Deployment
	if err := c.Get(ctx, appKey, &dep); err != nil {
		t.Fatalf("deployment not created: %v", err)
	}
	if got := *dep.Spec.Replicas; got != 2 {
		t.Errorf("deployment replicas = %d, want 2", got)
	}
	if got := dep.Spec.Template.Spec.Containers[0].Image; got != app.Spec.Image {
		t.Errorf("container image = %q, want %q", got, app.Spec.Image)
	}
	if got := dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort; got != 8080 {
		t.Errorf("container port = %d, want 8080", got)
	}
	assertOwnedBy(t, dep.OwnerReferences, app.Name)

	var svc corev1.Service
	if err := c.Get(ctx, appKey, &svc); err != nil {
		t.Fatalf("service not created: %v", err)
	}
	if got := svc.Spec.Ports[0].Port; got != 80 {
		t.Errorf("service port = %d, want 80", got)
	}
	if svc.Spec.Ports[0].TargetPort != intstr.FromString("http") {
		t.Errorf("service targetPort = %v, want named port http", svc.Spec.Ports[0].TargetPort)
	}
	assertOwnedBy(t, svc.OwnerReferences, app.Name)

	// No pods are ready yet, so Ready must be False with WaitingForPods.
	cond := meta.FindStatusCondition(app.Status.Conditions, v1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != v1alpha1.ReasonWaitingForPods {
		t.Errorf("Ready condition = %+v, want False/WaitingForPods", cond)
	}
}

func TestReadyConditionFollowsDeploymentStatus(t *testing.T) {
	r, c := newTestReconciler(t, newWebApp())
	ctx := context.Background()
	reconcileOnce(t, r)

	// Pretend the kubelet reports both replicas ready.
	var dep appsv1.Deployment
	if err := c.Get(ctx, appKey, &dep); err != nil {
		t.Fatal(err)
	}
	dep.Status.ReadyReplicas = 2
	if err := c.Status().Update(ctx, &dep); err != nil {
		t.Fatal(err)
	}

	reconcileOnce(t, r)

	var app v1alpha1.WebApp
	if err := c.Get(ctx, appKey, &app); err != nil {
		t.Fatal(err)
	}
	cond := meta.FindStatusCondition(app.Status.Conditions, v1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != v1alpha1.ReasonReplicasReady {
		t.Errorf("Ready condition = %+v, want True/ReplicasReady", cond)
	}
	if app.Status.ReadyReplicas != 2 {
		t.Errorf("status.readyReplicas = %d, want 2", app.Status.ReadyReplicas)
	}
	if app.Status.ObservedGeneration != app.Generation {
		t.Errorf("observedGeneration = %d, want %d", app.Status.ObservedGeneration, app.Generation)
	}
}

func TestDeletionRunsFinalizerAndRemovesChildren(t *testing.T) {
	r, c := newTestReconciler(t, newWebApp())
	ctx := context.Background()
	reconcileOnce(t, r)

	var app v1alpha1.WebApp
	if err := c.Get(ctx, appKey, &app); err != nil {
		t.Fatal(err)
	}
	// With our finalizer present, Delete only sets deletionTimestamp.
	if err := c.Delete(ctx, &app); err != nil {
		t.Fatal(err)
	}
	reconcileOnce(t, r)

	// The finalizer has been released, so the WebApp is gone...
	if err := c.Get(ctx, appKey, &app); !apierrors.IsNotFound(err) {
		t.Errorf("WebApp still present after finalizer ran: err = %v", err)
	}
	// ...and so are the children.
	if err := c.Get(ctx, appKey, &appsv1.Deployment{}); !apierrors.IsNotFound(err) {
		t.Errorf("Deployment still present: err = %v", err)
	}
	if err := c.Get(ctx, appKey, &corev1.Service{}); !apierrors.IsNotFound(err) {
		t.Errorf("Service still present: err = %v", err)
	}
}

func TestDefaultsWhenOptionalFieldsUnset(t *testing.T) {
	app := newWebApp()
	app.Spec.Replicas = nil
	app.Spec.Port = nil
	if got := replicas(app); got != 1 {
		t.Errorf("replicas() = %d, want 1", got)
	}
	if got := port(app); got != 8080 {
		t.Errorf("port() = %d, want 8080", got)
	}
}

func controllerHasFinalizer(app *v1alpha1.WebApp) bool {
	for _, f := range app.Finalizers {
		if f == FinalizerName {
			return true
		}
	}
	return false
}

func assertOwnedBy(t *testing.T, refs []metav1.OwnerReference, owner string) {
	t.Helper()
	for _, ref := range refs {
		if ref.Kind == "WebApp" && ref.Name == owner && ref.Controller != nil && *ref.Controller {
			return
		}
	}
	t.Errorf("no controller owner reference to WebApp %q in %+v", owner, refs)
}
