// Package controller contains the Workload reconciler.
//
// A Workload is turned into a Deployment and a Service with the same name, plus
// one child object per requested trait (Ingress, HorizontalPodAutoscaler,
// PodDisruptionBudget). Every child carries a controller reference to the
// Workload, so garbage collection removes them with it. Existing objects that
// the Workload does not control are reported as conflicts and never modified
// or deleted.
package controller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/workload-operator/api/v1alpha1"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/workload-operator/traits"
)

// Reasons reported in status conditions.
const (
	ReasonInvalidSpec      = "InvalidSpec"
	ReasonInvalidTrait     = "InvalidTrait"
	ReasonObjectConflict   = "ObjectConflict"
	ReasonProgressDeadline = "ProgressDeadlineExceeded"

	containerName = "app"
)

// specError marks a problem more retries cannot fix. It is reported in status.
type specError struct {
	reason string
	err    error
}

func (e specError) Error() string { return e.err.Error() }
func (e specError) Unwrap() error { return e.err }

// WorkloadReconciler reconciles Workload objects.
type WorkloadReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=platform.mycompany.io,resources=workloads,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=platform.mycompany.io,resources=workloads/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives a Workload towards its desired child objects.
func (r *WorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("workload", req.NamespacedName)

	w := &v1alpha1.Workload{}
	if err := r.Get(ctx, req.NamespacedName, w); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !w.DeletionTimestamp.IsZero() {
		// Children are owned by the Workload and removed by garbage collection.
		return ctrl.Result{}, nil
	}

	original := w.DeepCopy()
	reconcileErr := r.reconcileWorkload(ctx, w)

	w.Status.ObservedGeneration = w.Generation
	if err := r.Status().Patch(ctx, w, client.MergeFrom(original)); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Join(reconcileErr, err)
	}

	var se specError
	if errors.As(reconcileErr, &se) {
		logger.Info("workload spec cannot be satisfied", "reason", se.reason, "message", se.Error())
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, reconcileErr
}

func (r *WorkloadReconciler) reconcileWorkload(ctx context.Context, w *v1alpha1.Workload) error {
	gen := w.Generation
	setCond := func(t string, ok bool, reason, msg string) {
		status := metav1.ConditionTrue
		if !ok {
			status = metav1.ConditionFalse
		}
		meta.SetStatusCondition(&w.Status.Conditions, metav1.Condition{
			Type: t, Status: status, Reason: reason, Message: msg, ObservedGeneration: gen,
		})
	}
	fail := func(cond string, err error) error {
		reason := "ReconcileError"
		var se specError
		if errors.As(err, &se) {
			reason = se.reason
		}
		setCond(cond, false, reason, err.Error())
		setCond(v1alpha1.ConditionReady, false, reason, err.Error())
		w.Status.Phase = v1alpha1.PhaseFailed
		return err
	}
	if w.Status.Phase == "" {
		w.Status.Phase = v1alpha1.PhasePending
	}

	// 1. Validate everything before writing anything.
	requested, err := parseTraits(w)
	if err != nil {
		return fail(v1alpha1.ConditionTraitsApplied, err)
	}
	if w.Spec.Port < 1 || w.Spec.Port > 65535 {
		return fail(v1alpha1.ConditionDeploymentReady, specError{ReasonInvalidSpec,
			fmt.Errorf("spec.port %d is out of range", w.Spec.Port)})
	}

	_, hasAutoscaling := requested[v1alpha1.TraitAutoscaling]
	deployment := buildDeployment(w, hasAutoscaling)
	service := buildService(w)

	built := map[v1alpha1.TraitType]client.Object{}
	for _, tr := range traits.All() {
		req, ok := requested[tr.Type()]
		if !ok {
			continue
		}
		obj, err := tr.Build(w, req)
		if err != nil {
			if errors.Is(err, traits.ErrInvalidConfig) {
				return fail(v1alpha1.ConditionTraitsApplied, specError{ReasonInvalidTrait, err})
			}
			return fail(v1alpha1.ConditionTraitsApplied, err)
		}
		built[tr.Type()] = obj
	}

	// 2. Deployment and Service.
	if err := r.applyOwned(ctx, w, deployment); err != nil {
		return fail(v1alpha1.ConditionDeploymentReady, err)
	}
	if err := r.applyOwned(ctx, w, service); err != nil {
		return fail(v1alpha1.ConditionDeploymentReady, err)
	}

	// 3. Traits: apply the requested ones and delete the ones that were removed.
	for _, tr := range traits.All() {
		if obj, ok := built[tr.Type()]; ok {
			if err := r.applyOwned(ctx, w, obj); err != nil {
				return fail(v1alpha1.ConditionTraitsApplied, err)
			}
			continue
		}
		if err := r.deleteOwned(ctx, w, tr.Placeholder(w)); err != nil {
			return fail(v1alpha1.ConditionTraitsApplied, err)
		}
	}
	names := []string{}
	for _, tr := range traits.All() {
		if _, ok := requested[tr.Type()]; ok {
			names = append(names, string(tr.Type()))
		}
	}
	setCond(v1alpha1.ConditionTraitsApplied, true, "Applied", fmt.Sprintf("traits: [%s]", strings.Join(names, ", ")))

	// 4. Status is derived from the live Deployment and the ingress trait.
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(deployment), dep); err != nil {
		return fail(v1alpha1.ConditionDeploymentReady, err)
	}
	w.Status.URL = ""
	if ing, ok := built[v1alpha1.TraitIngress].(*networkingv1.Ingress); ok && len(ing.Spec.Rules) > 0 {
		w.Status.URL = "https://" + ing.Spec.Rules[0].Host
	}
	w.Status.AvailableReplicas = dep.Status.AvailableReplicas
	want := desiredReplicas(w, dep)
	available := want == 0 || dep.Status.AvailableReplicas >= want

	switch {
	case deploymentDeadlineExceeded(dep):
		w.Status.Phase = v1alpha1.PhaseFailed
		setCond(v1alpha1.ConditionDeploymentReady, false, ReasonProgressDeadline, "deployment exceeded its progress deadline")
	case available:
		w.Status.Phase = v1alpha1.PhaseReady
		setCond(v1alpha1.ConditionDeploymentReady, true, "Available",
			fmt.Sprintf("%d/%d replicas available", dep.Status.AvailableReplicas, want))
	default:
		w.Status.Phase = v1alpha1.PhaseProgressing
		setCond(v1alpha1.ConditionDeploymentReady, false, "Progressing",
			fmt.Sprintf("%d/%d replicas available", dep.Status.AvailableReplicas, want))
	}
	setCond(v1alpha1.ConditionReady, w.Status.Phase == v1alpha1.PhaseReady, string(w.Status.Phase),
		"workload status derived from deployment and traits")
	return nil
}

// parseTraits checks trait types and duplicates and returns each requested
// trait's raw config keyed by type.
func parseTraits(w *v1alpha1.Workload) (map[v1alpha1.TraitType][]byte, error) {
	out := map[v1alpha1.TraitType][]byte{}
	for i, t := range w.Spec.Traits {
		if _, err := traits.Lookup(t.Type); err != nil {
			return nil, specError{ReasonInvalidTrait, fmt.Errorf("spec.traits[%d]: %w", i, err)}
		}
		if _, dup := out[t.Type]; dup {
			return nil, specError{ReasonInvalidTrait, fmt.Errorf("spec.traits[%d]: duplicate trait %q", i, t.Type)}
		}
		var raw []byte
		if t.Config != nil {
			raw = t.Config.Raw
		}
		out[t.Type] = raw
	}
	return out, nil
}

// desiredReplicas is the live Deployment replica count when an HPA owns it,
// otherwise spec.replicas (default 1).
func desiredReplicas(w *v1alpha1.Workload, dep *appsv1.Deployment) int32 {
	if dep.Spec.Replicas != nil {
		return *dep.Spec.Replicas
	}
	if w.Spec.Replicas != nil {
		return *w.Spec.Replicas
	}
	return 1
}

func deploymentDeadlineExceeded(dep *appsv1.Deployment) bool {
	for _, c := range dep.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Status == corev1.ConditionFalse && c.Reason == ReasonProgressDeadline {
			return true
		}
	}
	return false
}

func objectMeta(w *v1alpha1.Workload) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: w.Name, Namespace: w.Namespace, Labels: traits.ObjectLabels(w)}
}

// buildDeployment renders the Deployment. With an autoscaling trait the replica
// count is left unset so the HPA owns it.
func buildDeployment(w *v1alpha1.Workload, hasAutoscaling bool) *appsv1.Deployment {
	var replicas *int32
	if !hasAutoscaling {
		n := int32(1)
		if w.Spec.Replicas != nil {
			n = *w.Spec.Replicas
		}
		replicas = &n
	}
	return &appsv1.Deployment{
		ObjectMeta: objectMeta(w),
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{MatchLabels: traits.SelectorLabels(w)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: traits.ObjectLabels(w)},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:      containerName,
						Image:     w.Spec.Image,
						Env:       w.Spec.Env,
						Resources: w.Spec.Resources,
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: w.Spec.Port,
							Protocol:      corev1.ProtocolTCP,
						}},
					}},
				},
			},
		},
	}
}

// buildService renders a ClusterIP Service exposing the Workload port.
func buildService(w *v1alpha1.Workload) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: objectMeta(w),
		Spec: corev1.ServiceSpec{
			Selector: traits.SelectorLabels(w),
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       w.Spec.Port,
				TargetPort: intstr.FromInt32(w.Spec.Port),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

// applyOwned creates desired, or updates the existing object when the Workload
// controls it. An object without a controller reference to the Workload is a conflict.
func (r *WorkloadReconciler) applyOwned(ctx context.Context, w *v1alpha1.Workload, desired client.Object) error {
	if err := controllerutil.SetControllerReference(w, desired, r.Scheme); err != nil {
		return err
	}
	existing, ok := reflect.New(reflect.TypeOf(desired).Elem()).Interface().(client.Object)
	if !ok {
		return fmt.Errorf("cannot allocate %T", desired)
	}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(existing, w) {
		return specError{ReasonObjectConflict, fmt.Errorf(
			"%T %s/%s exists and is not controlled by workload %q", desired, desired.GetNamespace(), desired.GetName(), w.Name)}
	}

	desired.SetResourceVersion(existing.GetResourceVersion())
	// Keep fields the API server assigns or defaults, otherwise updates churn or fail.
	switch d := desired.(type) {
	case *corev1.Service:
		e := existing.(*corev1.Service)
		if d.Spec.ClusterIP == "" {
			d.Spec.ClusterIP = e.Spec.ClusterIP
			d.Spec.ClusterIPs = e.Spec.ClusterIPs
		}
		if len(d.Spec.IPFamilies) == 0 {
			d.Spec.IPFamilies = e.Spec.IPFamilies
			d.Spec.IPFamilyPolicy = e.Spec.IPFamilyPolicy
		}
	case *appsv1.Deployment:
		// With an HPA the replica count belongs to the autoscaler; keep its value.
		if d.Spec.Replicas == nil {
			d.Spec.Replicas = existing.(*appsv1.Deployment).Spec.Replicas
		}
	}
	return r.Update(ctx, desired)
}

// deleteOwned deletes the child object only when the Workload controls it.
func (r *WorkloadReconciler) deleteOwned(ctx context.Context, w *v1alpha1.Workload, obj client.Object) error {
	if err := r.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !metav1.IsControlledBy(obj, w) {
		return nil
	}
	return client.IgnoreNotFound(r.Delete(ctx, obj))
}

// SetupWithManager registers the controller. Children are owned, so Owns()
// wakes the Workload whenever the Deployment, Service or a trait object changes.
func (r *WorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("workload").
		For(&v1alpha1.Workload{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Complete(r)
}
