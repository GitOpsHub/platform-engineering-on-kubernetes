// Package controller contains the Environment reconciler.
//
// Ownership rules:
//   - The generated namespace "<tenant>-<name>" is adopted only if it carries the
//     managed-by=environment-operator, tenant and environment labels (and no
//     owner-uid annotation for a different Environment).
//   - Namespaces and child objects are marked with the owner-uid annotation. A
//     controller reference is not possible: a namespaced Environment cannot own
//     cluster-scoped namespaces or objects in another namespace.
//   - Every Delete is preceded by an ownership check (IsOwnedBy). Objects that
//     are not owned by the Environment are never modified or deleted.
package controller

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	platformv1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/environment-operator/api/v1alpha1"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/environment-operator/templates"
)

const requeueWhileTerminating = 5 * time.Second

// Reasons reported in status conditions.
const (
	ReasonInvalidNamespace  = "InvalidNamespace"
	ReasonNamespaceConflict = "NamespaceConflict"
	ReasonObjectConflict    = "ObjectConflict"
	ReasonUnknownTemplate   = "UnknownTemplate"
	ReasonRenderFailed      = "RenderFailed"
	ReasonTenantMismatch    = "TenantMismatch"
)

// specError marks a problem more retries cannot fix. It is reported in status.
type specError struct {
	reason string
	err    error
}

func (e specError) Error() string { return e.err.Error() }
func (e specError) Unwrap() error { return e.err }

// EnvironmentReconciler reconciles Environment objects.
type EnvironmentReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// OperatorNamespace is never used as an environment namespace.
	OperatorNamespace string
	// ClusterName, when set, makes the operator skip Environments whose spec.cluster differs.
	ClusterName string
	// BaseDomain is the suffix used for status.url.
	BaseDomain string
	// Templates holds the template sets. Defaults to the embedded templates.FS.
	Templates fs.FS
	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time
}

// +kubebuilder:rbac:groups=platform.mycompany.io,resources=environments,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=platform.mycompany.io,resources=environments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.mycompany.io,resources=environments/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile drives an Environment towards its desired state.
func (r *EnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("environment", req.NamespacedName)

	env := &platformv1alpha1.Environment{}
	if err := r.Get(ctx, req.NamespacedName, env); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !env.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, env)
	}

	if r.ClusterName != "" && env.Spec.Cluster != "" && env.Spec.Cluster != r.ClusterName {
		logger.V(1).Info("environment targets another cluster", "cluster", env.Spec.Cluster)
		return ctrl.Result{}, nil
	}

	if controllerutil.AddFinalizer(env, platformv1alpha1.EnvironmentFinalizer) {
		if err := r.Update(ctx, env); err != nil {
			return ctrl.Result{}, err
		}
	}

	original := env.DeepCopy()
	result, reconcileErr := r.reconcileEnvironment(ctx, env)

	env.Status.ObservedGeneration = env.Generation
	if err := r.Status().Patch(ctx, env, client.MergeFrom(original)); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Join(reconcileErr, err)
	}

	var se specError
	if errors.As(reconcileErr, &se) {
		logger.Info("environment spec cannot be satisfied", "reason", se.reason, "message", se.Error())
		return ctrl.Result{}, nil
	}
	if reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}
	return result, nil
}

func (r *EnvironmentReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *EnvironmentReconciler) templateFS() fs.FS {
	if r.Templates != nil {
		return r.Templates
	}
	return templates.FS
}

// reconcileEnvironment creates or updates the namespace and baseline objects and records conditions.
func (r *EnvironmentReconciler) reconcileEnvironment(ctx context.Context, env *platformv1alpha1.Environment) (ctrl.Result, error) {
	gen := env.Generation
	setCond := func(t string, ok bool, reason, msg string) {
		status := metav1.ConditionTrue
		if !ok {
			status = metav1.ConditionFalse
		}
		meta.SetStatusCondition(&env.Status.Conditions, metav1.Condition{
			Type: t, Status: status, Reason: reason, Message: msg, ObservedGeneration: gen,
		})
	}
	fail := func(cond string, err error) (ctrl.Result, error) {
		reason := "ReconcileError"
		var se specError
		if errors.As(err, &se) {
			reason = se.reason
		}
		setCond(cond, false, reason, err.Error())
		setCond(platformv1alpha1.ConditionReady, false, reason, err.Error())
		env.Status.Phase = platformv1alpha1.PhaseFailed
		return ctrl.Result{}, err
	}

	if env.Status.Phase == "" {
		env.Status.Phase = platformv1alpha1.PhasePending
	}

	// The tenant is never trusted from spec alone: it must match the tenant label
	// of the namespace the Environment lives in. Mismatches are rejected.
	tenant, err := r.serverSideTenant(ctx, env)
	if err != nil {
		return fail(platformv1alpha1.ConditionTenantVerified, err)
	}
	setCond(platformv1alpha1.ConditionTenantVerified, true, "Verified",
		fmt.Sprintf("spec.tenant %q matches namespace %q", tenant, env.Namespace))

	ns, err := platformv1alpha1.ValidateNamespace(tenant, env.Name, r.OperatorNamespace)
	if err != nil {
		return fail(platformv1alpha1.ConditionNamespaceReady, specError{ReasonInvalidNamespace, err})
	}
	env.Status.Namespace = ns

	now := r.now()
	if env.IsExpired(now) {
		env.Status.Phase = platformv1alpha1.PhaseExpired
		env.Status.ExpiresAt = env.ExpiresAt()
		setCond(platformv1alpha1.ConditionExpired, true, "TTLElapsed",
			fmt.Sprintf("ttl %s elapsed; deleting environment", env.Spec.TTL.Duration))
		if err := r.Delete(ctx, env); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if !templateSetExists(r.templateFS(), env.Spec.Template) {
		return fail(platformv1alpha1.ConditionTemplatesReady, specError{ReasonUnknownTemplate,
			fmt.Errorf("unknown template set %q", env.Spec.Template)})
	}

	if err := r.ensureNamespace(ctx, env, ns); err != nil {
		return fail(platformv1alpha1.ConditionNamespaceReady, err)
	}
	setCond(platformv1alpha1.ConditionNamespaceReady, true, "Provisioned", fmt.Sprintf("namespace %s ready", ns))

	url := platformv1alpha1.URLFor(env.Spec.Tenant, env.Name, r.BaseDomain)
	objs, err := renderBaseline(r.templateFS(), env, ns, url)
	if err != nil {
		return fail(platformv1alpha1.ConditionTemplatesReady, specError{ReasonRenderFailed, err})
	}
	for _, obj := range objs {
		if err := r.ensureOwned(ctx, env, obj); err != nil {
			return fail(platformv1alpha1.ConditionTemplatesReady, err)
		}
	}
	setCond(platformv1alpha1.ConditionTemplatesReady, true, "Applied",
		fmt.Sprintf("%d baseline object(s) applied", len(objs)))

	env.Status.URL = url
	env.Status.ExpiresAt = env.ExpiresAt()
	env.Status.Phase = platformv1alpha1.PhaseReady
	meta.RemoveStatusCondition(&env.Status.Conditions, platformv1alpha1.ConditionExpired)
	setCond(platformv1alpha1.ConditionReady, true, "Reconciled", "environment is ready")

	if exp := env.ExpiresAt(); exp != nil {
		// Wake up exactly at expiry; the expiry branch above deletes the Environment.
		return ctrl.Result{RequeueAfter: exp.Time.Sub(now)}, nil
	}
	return ctrl.Result{}, nil
}

// operatorMetadata sets the labels and owner-uid annotation on obj. Labels are
// set on top of existing ones so unrelated metadata is preserved.
func operatorMetadata(env *platformv1alpha1.Environment, obj metav1.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[platformv1alpha1.LabelTenant] = env.Spec.Tenant
	labels[platformv1alpha1.LabelEnvironment] = env.Name
	labels[platformv1alpha1.LabelManagedBy] = platformv1alpha1.ManagedByValue
	obj.SetLabels(labels)

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[platformv1alpha1.OwnerUIDAnnotation] = string(env.UID)
	obj.SetAnnotations(annotations)
}

// ensureNamespace creates the namespace, or adopts it when CanAdoptNamespace allows.
func (r *EnvironmentReconciler) ensureNamespace(ctx context.Context, env *platformv1alpha1.Environment, name string) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: name}, ns)
	if apierrors.IsNotFound(err) {
		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		operatorMetadata(env, ns)
		return r.Create(ctx, ns)
	}
	if err != nil {
		return err
	}
	if !platformv1alpha1.CanAdoptNamespace(ns, env) {
		return specError{ReasonNamespaceConflict, fmt.Errorf(
			"namespace %q already exists and is not managed by environment %q of tenant %q", name, env.Name, env.Spec.Tenant)}
	}
	if !ns.DeletionTimestamp.IsZero() {
		return fmt.Errorf("namespace %q is terminating; will retry", name)
	}
	operatorMetadata(env, ns)
	return r.Update(ctx, ns)
}

// ensureOwned creates a rendered child object, or updates it when it is owned by
// this Environment. An existing object that is not owned is a conflict.
func (r *EnvironmentReconciler) ensureOwned(ctx context.Context, env *platformv1alpha1.Environment, desired client.Object) error {
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
	if !platformv1alpha1.IsOwnedBy(existing, env) {
		return specError{ReasonObjectConflict, fmt.Errorf(
			"%T %s/%s exists and is not owned by environment %q", existing,
			desired.GetNamespace(), desired.GetName(), env.Name)}
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, desired)
}

// finalize deletes the generated namespace if this Environment owns it, then releases the finalizer.
func (r *EnvironmentReconciler) finalize(ctx context.Context, env *platformv1alpha1.Environment) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(env, platformv1alpha1.EnvironmentFinalizer) {
		return ctrl.Result{}, nil
	}

	if env.Status.Phase != platformv1alpha1.PhaseTerminating {
		original := env.DeepCopy()
		env.Status.Phase = platformv1alpha1.PhaseTerminating
		meta.SetStatusCondition(&env.Status.Conditions, metav1.Condition{
			Type: platformv1alpha1.ConditionReady, Status: metav1.ConditionFalse,
			Reason: "Terminating", Message: "deleting environment namespace", ObservedGeneration: env.Generation,
		})
		if err := r.Status().Patch(ctx, env, client.MergeFrom(original)); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
	}

	nsName := env.Status.Namespace
	if nsName == "" {
		nsName = platformv1alpha1.NamespaceFor(env.Spec.Tenant, env.Name)
	}
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: nsName}, ns)
	switch {
	case apierrors.IsNotFound(err):
	case err != nil:
		return ctrl.Result{}, err
	case !platformv1alpha1.IsOwnedBy(ns, env):
		// Not ours: never delete it.
	default:
		if ns.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, ns); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: requeueWhileTerminating}, nil
	}

	controllerutil.RemoveFinalizer(env, platformv1alpha1.EnvironmentFinalizer)
	return ctrl.Result{}, client.IgnoreNotFound(r.Update(ctx, env))
}

// serverSideTenant reads the tenant label from the Environment's own namespace and
// returns it when it matches spec.tenant. A missing namespace, a missing label or
// a different tenant is a TenantMismatch spec error.
func (r *EnvironmentReconciler) serverSideTenant(ctx context.Context, env *platformv1alpha1.Environment) (string, error) {
	home := &corev1.Namespace{}
	if err := r.Get(ctx, client.ObjectKey{Name: env.Namespace}, home); err != nil {
		if apierrors.IsNotFound(err) {
			return "", specError{ReasonTenantMismatch, fmt.Errorf(
				"cannot verify tenant: namespace %q does not exist", env.Namespace)}
		}
		return "", err
	}
	tenant, ok := home.Labels[platformv1alpha1.LabelTenant]
	if !ok || tenant == "" {
		return "", specError{ReasonTenantMismatch, fmt.Errorf(
			"cannot verify tenant: namespace %q has no %s label", env.Namespace, platformv1alpha1.LabelTenant)}
	}
	if env.Spec.Tenant != tenant {
		return "", specError{ReasonTenantMismatch, fmt.Errorf(
			"spec.tenant %q does not match tenant %q of namespace %q", env.Spec.Tenant, tenant, env.Namespace)}
	}
	return tenant, nil
}

// mapToEnvironments maps any object labelled with tenant/environment to the matching Environments.
func (r *EnvironmentReconciler) mapToEnvironments(ctx context.Context, obj client.Object) []reconcile.Request {
	tenant := obj.GetLabels()[platformv1alpha1.LabelTenant]
	name := obj.GetLabels()[platformv1alpha1.LabelEnvironment]
	if tenant == "" || name == "" {
		return nil
	}
	list := &platformv1alpha1.EnvironmentList{}
	if err := r.List(ctx, list); err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for _, env := range list.Items {
		if env.Name == name && env.Spec.Tenant == tenant {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&env)})
		}
	}
	return reqs
}

// SetupWithManager sets up the controller with the Manager. Generated objects are
// not owned by the Environment (see package comment), so they are mapped back by labels.
func (r *EnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	labelled := handler.EnqueueRequestsFromMapFunc(r.mapToEnvironments)
	return ctrl.NewControllerManagedBy(mgr).
		Named("environment").
		For(&platformv1alpha1.Environment{}).
		Watches(&corev1.Namespace{}, labelled).
		Watches(&corev1.ResourceQuota{}, labelled).
		Watches(&corev1.ConfigMap{}, labelled).
		Complete(r)
}
