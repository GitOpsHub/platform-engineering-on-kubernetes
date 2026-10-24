// Package controllers contains the reconciler for the WebApp custom resource.
//
// For every WebApp the reconciler maintains:
//   - a Deployment and a Service with the same name, both owned by the WebApp
//     (owner references make Kubernetes garbage-collect them if the WebApp is
//     deleted and the finalizer is bypassed);
//   - a finalizer that deletes those children in the foreground before the
//     WebApp is allowed to disappear;
//   - a Ready condition and a few status fields that reflect the Deployment.
package controllers

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/labs/lab-03-platform-api/api/v1alpha1"
)

const (
	// FinalizerName blocks deletion of a WebApp until its children are gone.
	FinalizerName = "platform.mycompany.io/webapp-cleanup"

	defaultReplicas int32 = 1
	defaultPort     int32 = 8080

	// requeueWhileDeleting is how long to wait before re-checking children
	// that are still being deleted in the foreground.
	requeueWhileDeleting = 2 * time.Second
)

// WebAppReconciler reconciles a WebApp object.
type WebAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=platform.mycompany.io,resources=webapps,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=platform.mycompany.io,resources=webapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.mycompany.io,resources=webapps/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the cluster towards the state described by one WebApp.
func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var app v1alpha1.WebApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		// Deleted before we got to it: nothing to do.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !app.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, &app)
	}

	// Add the finalizer before creating children, so deletion always goes
	// through finalize() and our cleanup order is respected.
	if controllerutil.AddFinalizer(&app, FinalizerName) {
		if err := r.Update(ctx, &app); err != nil {
			return ctrl.Result{}, err
		}
	}

	dep, err := r.reconcileDeployment(ctx, &app)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileService(ctx, &app); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, r.updateStatus(ctx, &app, dep)
}

// reconcileDeployment creates or updates the Deployment for app.
func (r *WebAppReconciler) reconcileDeployment(ctx context.Context, app *v1alpha1.WebApp) (*appsv1.Deployment, error) {
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dep, func() error {
		if err := controllerutil.SetControllerReference(app, dep, r.Scheme); err != nil {
			return err
		}
		dep.Labels = managedLabels(app)
		dep.Spec.Replicas = ptr.To(replicas(app))
		dep.Spec.Selector = &metav1.LabelSelector{MatchLabels: selectorLabels(app)}
		dep.Spec.Template.Labels = selectorLabels(app)
		dep.Spec.Template.Spec = corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot:   ptr.To(true),
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			Containers: []corev1.Container{{
				Name:  "app",
				Image: app.Spec.Image,
				Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: port(app)}},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromString("http")},
					},
					PeriodSeconds: 5,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("32Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: ptr.To(false),
					Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				},
			}},
		}
		return nil
	})
	return dep, err
}

// reconcileService creates or updates the Service that fronts the Deployment.
func (r *WebAppReconciler) reconcileService(ctx context.Context, app *v1alpha1.WebApp) error {
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: app.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(app, svc, r.Scheme); err != nil {
			return err
		}
		svc.Labels = managedLabels(app)
		svc.Spec.Selector = selectorLabels(app)
		svc.Spec.Ports = []corev1.ServicePort{{
			Name:       "http",
			Port:       80,
			TargetPort: intstr.FromString("http"),
		}}
		return nil
	})
	return err
}

// updateStatus records what the Deployment looks like right now.
func (r *WebAppReconciler) updateStatus(ctx context.Context, app *v1alpha1.WebApp, dep *appsv1.Deployment) error {
	desired := replicas(app)
	ready := dep.Status.ReadyReplicas

	app.Status.ObservedGeneration = app.Generation
	app.Status.ReadyReplicas = ready
	app.Status.URL = fmt.Sprintf("http://%s.%s.svc.cluster.local", app.Name, app.Namespace)

	cond := metav1.Condition{
		Type:               v1alpha1.ConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             v1alpha1.ReasonReplicasReady,
		Message:            fmt.Sprintf("%d/%d replicas ready", ready, desired),
		ObservedGeneration: app.Generation,
	}
	if ready < desired {
		cond.Status = metav1.ConditionFalse
		cond.Reason = v1alpha1.ReasonWaitingForPods
	}
	meta.SetStatusCondition(&app.Status.Conditions, cond)

	logf.FromContext(ctx).V(1).Info("updating status", "ready", ready, "desired", desired)
	return r.Status().Update(ctx, app)
}

// finalize runs when a WebApp has been deleted but still carries our finalizer.
//
// Children are deleted with foreground propagation, so Kubernetes removes their
// dependents (pods, ReplicaSets) before the children themselves. We wait until
// the children are gone, then remove the finalizer so the WebApp can go.
func (r *WebAppReconciler) finalize(ctx context.Context, app *v1alpha1.WebApp) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(app, FinalizerName) {
		return ctrl.Result{}, nil
	}

	stillThere := false
	for _, child := range children(app) {
		key := client.ObjectKeyFromObject(child)
		if err := r.Delete(ctx, child, client.PropagationPolicy(metav1.DeletePropagationForeground)); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		if err := r.Get(ctx, key, child); err == nil {
			stillThere = true
		} else if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	if stillThere {
		return ctrl.Result{RequeueAfter: requeueWhileDeleting}, nil
	}

	controllerutil.RemoveFinalizer(app, FinalizerName)
	return ctrl.Result{}, r.Update(ctx, app)
}

// SetupWithManager wires the reconciler to the manager. Owns() means a change to
// a child Deployment or Service re-triggers reconciliation of its WebApp.
func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.WebApp{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

// children returns empty objects that name the Deployment and Service of app.
func children(app *v1alpha1.WebApp) []client.Object {
	om := metav1.ObjectMeta{Name: app.Name, Namespace: app.Namespace}
	return []client.Object{
		&appsv1.Deployment{ObjectMeta: om},
		&corev1.Service{ObjectMeta: om},
	}
}

// selectorLabels identify the pods of one WebApp. Keep them stable: changing a
// Deployment selector is rejected by the API server.
func selectorLabels(app *v1alpha1.WebApp) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "webapp",
		"app.kubernetes.io/instance": app.Name,
	}
}

func managedLabels(app *v1alpha1.WebApp) map[string]string {
	labels := selectorLabels(app)
	labels["platform.mycompany.io/managed"] = "true"
	return labels
}

func replicas(app *v1alpha1.WebApp) int32 {
	if app.Spec.Replicas == nil {
		return defaultReplicas
	}
	return *app.Spec.Replicas
}

func port(app *v1alpha1.WebApp) int32 {
	if app.Spec.Port == nil {
		return defaultPort
	}
	return *app.Spec.Port
}
