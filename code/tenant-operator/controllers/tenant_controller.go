// Package controllers contains the Tenant reconciler.
package controllers

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/tenant-operator/api/v1alpha1"
)

// Names of the per-namespace objects managed by the operator.
const (
	ResourceQuotaName       = "tenant-quota"
	LimitRangeName          = "tenant-limits"
	OwnersRoleBindingName   = "tenant-owners"
	DefaultDenyPolicyName   = "tenant-default-deny"
	AllowTenantPolicyName   = "tenant-allow-same-tenant"
	AllowDNSPolicyName      = "tenant-allow-dns"
	ownerGroupPrefix        = "group:"
	requeueWhileTerminating = 5 * time.Second
)

// errNamespaceConflict is returned when a requested namespace already exists
// and is not owned by this tenant. It is a terminal (spec) error: retrying
// will not help until the user changes the Tenant.
var errNamespaceConflict = errors.New("namespace conflict")

// TenantReconciler reconciles a Tenant object.
type TenantReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=platform.mycompany.io,resources=tenants,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=platform.mycompany.io,resources=tenants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.mycompany.io,resources=tenants/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas;limitranges,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=bind,resourceNames=admin

// Reconcile drives the cluster towards the state described by a Tenant.
func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("tenant", req.Name)

	tenant := &platformv1alpha1.Tenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !tenant.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, tenant)
	}

	if controllerutil.AddFinalizer(tenant, platformv1alpha1.TenantFinalizer) {
		if err := r.Update(ctx, tenant); err != nil {
			return ctrl.Result{}, err
		}
	}

	original := tenant.DeepCopy()
	result, reconcileErr := r.reconcileTenant(ctx, tenant)

	tenant.Status.ObservedGeneration = tenant.Generation
	if err := r.Status().Patch(ctx, tenant, client.MergeFrom(original)); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Join(reconcileErr, err)
	}

	if reconcileErr != nil {
		if errors.Is(reconcileErr, errNamespaceConflict) || isQuotaError(reconcileErr) {
			// Spec problem: surfaced in status, wait for the user to fix the Tenant.
			logger.Info("tenant spec cannot be satisfied", "reason", reconcileErr.Error())
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, reconcileErr
	}
	return result, nil
}

type quotaError struct{ error }

func isQuotaError(err error) bool {
	var qe quotaError
	return errors.As(err, &qe)
}

// reconcileTenant creates/updates all child objects and records conditions.
func (r *TenantReconciler) reconcileTenant(ctx context.Context, tenant *platformv1alpha1.Tenant) (ctrl.Result, error) {
	gen := tenant.Generation
	setCond := func(t string, ok bool, reason, msg string) {
		status := metav1.ConditionTrue
		if !ok {
			status = metav1.ConditionFalse
		}
		meta.SetStatusCondition(&tenant.Status.Conditions, metav1.Condition{
			Type: t, Status: status, Reason: reason, Message: msg, ObservedGeneration: gen,
		})
	}
	fail := func(cond, reason string, err error) (ctrl.Result, error) {
		setCond(cond, false, reason, err.Error())
		setCond(platformv1alpha1.ConditionReady, false, reason, err.Error())
		tenant.Status.Phase = platformv1alpha1.PhaseFailed
		return ctrl.Result{}, err
	}

	if tenant.Status.Phase == "" {
		tenant.Status.Phase = platformv1alpha1.PhasePending
	}

	hard, err := tenant.Spec.Quota.ResourceList()
	if err != nil {
		return fail(platformv1alpha1.ConditionQuotaReady, "InvalidQuota", quotaError{err})
	}

	// 1. Namespaces.
	for _, name := range tenant.Spec.Namespaces {
		if err := r.ensureNamespace(ctx, tenant, name); err != nil {
			return fail(platformv1alpha1.ConditionNamespacesReady, reasonFor(err, "NamespaceError"), err)
		}
	}
	if err := r.pruneNamespaces(ctx, tenant); err != nil {
		return fail(platformv1alpha1.ConditionNamespacesReady, "PruneFailed", err)
	}
	tenant.Status.Namespaces = slices.Clone(tenant.Spec.Namespaces)
	slices.Sort(tenant.Status.Namespaces)
	setCond(platformv1alpha1.ConditionNamespacesReady, true, "Provisioned",
		fmt.Sprintf("%d namespace(s) provisioned", len(tenant.Spec.Namespaces)))

	// 2-4. Per-namespace policy objects.
	for _, ns := range tenant.Spec.Namespaces {
		if err := r.ensureQuota(ctx, tenant, ns, hard); err != nil {
			return fail(platformv1alpha1.ConditionQuotaReady, "QuotaFailed", err)
		}
		if err := r.ensureNetworkPolicies(ctx, tenant, ns); err != nil {
			return fail(platformv1alpha1.ConditionNetworkReady, "NetworkPolicyFailed", err)
		}
		if err := r.ensureOwnerBinding(ctx, tenant, ns); err != nil {
			return fail(platformv1alpha1.ConditionRBACReady, "RBACFailed", err)
		}
	}
	setCond(platformv1alpha1.ConditionQuotaReady, true, "Applied", "ResourceQuota and LimitRange applied")
	if tenant.Spec.NetworkIsolation {
		setCond(platformv1alpha1.ConditionNetworkReady, true, "Isolated", "default-deny, same-tenant and DNS policies applied")
	} else {
		setCond(platformv1alpha1.ConditionNetworkReady, true, "Disabled", "network isolation disabled")
	}
	setCond(platformv1alpha1.ConditionRBACReady, true, "Bound",
		fmt.Sprintf("%d owner(s) bound to ClusterRole admin", len(tenant.Spec.Owners)))
	setCond(platformv1alpha1.ConditionReady, true, "Reconciled", "tenant is ready")
	tenant.Status.Phase = platformv1alpha1.PhaseReady
	return ctrl.Result{}, nil
}

func reasonFor(err error, fallback string) string {
	if errors.Is(err, errNamespaceConflict) {
		return "NamespaceConflict"
	}
	return fallback
}

// labelsFor returns the standard labels for objects generated for a tenant.
func labelsFor(tenant *platformv1alpha1.Tenant) map[string]string {
	return map[string]string{
		platformv1alpha1.LabelTenant:    tenant.Name,
		platformv1alpha1.LabelManagedBy: platformv1alpha1.ManagedByValue,
	}
}

func mergeLabels(obj client.Object, want map[string]string) {
	l := obj.GetLabels()
	if l == nil {
		l = map[string]string{}
	}
	for k, v := range want {
		l[k] = v
	}
	obj.SetLabels(l)
}

func (r *TenantReconciler) ensureNamespace(ctx context.Context, tenant *platformv1alpha1.Tenant, name string) error {
	existing := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: name}, existing)
	switch {
	case apierrors.IsNotFound(err):
		// fall through to CreateOrUpdate
	case err != nil:
		return err
	default:
		if owner := existing.Labels[platformv1alpha1.LabelTenant]; owner != tenant.Name {
			if owner == "" {
				return fmt.Errorf("%w: namespace %q already exists and is not managed by a tenant", errNamespaceConflict, name)
			}
			return fmt.Errorf("%w: namespace %q belongs to tenant %q", errNamespaceConflict, name, owner)
		}
	}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, ns, func() error {
		mergeLabels(ns, labelsFor(tenant))
		return controllerutil.SetControllerReference(tenant, ns, r.Scheme)
	})
	return err
}

// pruneNamespaces deletes namespaces previously created for the tenant that
// were removed from spec.namespaces.
func (r *TenantReconciler) pruneNamespaces(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	owned, err := r.ownedNamespaces(ctx, tenant)
	if err != nil {
		return err
	}
	for i := range owned {
		ns := &owned[i]
		if slices.Contains(tenant.Spec.Namespaces, ns.Name) || !ns.DeletionTimestamp.IsZero() {
			continue
		}
		if err := r.Delete(ctx, ns); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

func (r *TenantReconciler) ownedNamespaces(ctx context.Context, tenant *platformv1alpha1.Tenant) ([]corev1.Namespace, error) {
	list := &corev1.NamespaceList{}
	if err := r.List(ctx, list, client.MatchingLabels(labelsFor(tenant))); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *TenantReconciler) ensureQuota(ctx context.Context, tenant *platformv1alpha1.Tenant, ns string, hard corev1.ResourceList) error {
	rq := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: ResourceQuotaName, Namespace: ns}}
	if len(hard) == 0 {
		if err := r.Delete(ctx, rq); client.IgnoreNotFound(err) != nil {
			return err
		}
	} else {
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, rq, func() error {
			mergeLabels(rq, labelsFor(tenant))
			rq.Spec.Hard = hard
			return controllerutil.SetControllerReference(tenant, rq, r.Scheme)
		}); err != nil {
			return err
		}
	}

	// A LimitRange gives containers sane defaults so that pods are admitted
	// once a quota on requests/limits exists.
	lr := &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: LimitRangeName, Namespace: ns}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, lr, func() error {
		mergeLabels(lr, labelsFor(tenant))
		lr.Spec.Limits = []corev1.LimitRangeItem{{
			Type: corev1.LimitTypeContainer,
			Default: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			DefaultRequest: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		}}
		return controllerutil.SetControllerReference(tenant, lr, r.Scheme)
	})
	return err
}

func (r *TenantReconciler) ensureNetworkPolicies(ctx context.Context, tenant *platformv1alpha1.Tenant, ns string) error {
	names := []string{DefaultDenyPolicyName, AllowTenantPolicyName, AllowDNSPolicyName}
	if !tenant.Spec.NetworkIsolation {
		for _, n := range names {
			np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: ns}}
			if err := r.Delete(ctx, np); client.IgnoreNotFound(err) != nil {
				return err
			}
		}
		return nil
	}

	for _, desired := range desiredNetworkPolicies(tenant.Name, ns) {
		np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: desired.Name, Namespace: ns}}
		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
			mergeLabels(np, labelsFor(tenant))
			np.Spec = desired.Spec
			return controllerutil.SetControllerReference(tenant, np, r.Scheme)
		}); err != nil {
			return err
		}
	}
	return nil
}

// desiredNetworkPolicies returns the isolation policy set for one namespace:
//   - default-deny all ingress and egress
//   - allow ingress/egress to any namespace labelled with the same tenant
//   - allow DNS egress to kube-system
func desiredNetworkPolicies(tenantName, ns string) []networkingv1.NetworkPolicy {
	sameTenant := []networkingv1.NetworkPolicyPeer{{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{platformv1alpha1.LabelTenant: tenantName},
		},
	}}
	udp, tcp := corev1.ProtocolUDP, corev1.ProtocolTCP
	dnsPort := intstr.FromInt32(53)
	both := []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}

	return []networkingv1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: DefaultDenyPolicyName, Namespace: ns},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				PolicyTypes: both,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: AllowTenantPolicyName, Namespace: ns},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				PolicyTypes: both,
				Ingress:     []networkingv1.NetworkPolicyIngressRule{{From: sameTenant}},
				Egress:      []networkingv1.NetworkPolicyEgressRule{{To: sameTenant}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: AllowDNSPolicyName, Namespace: ns},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
				Egress: []networkingv1.NetworkPolicyEgressRule{{
					To: []networkingv1.NetworkPolicyPeer{{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"kubernetes.io/metadata.name": "kube-system"},
						},
					}},
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &udp, Port: &dnsPort},
						{Protocol: &tcp, Port: &dnsPort},
					},
				}},
			},
		},
	}
}

func (r *TenantReconciler) ensureOwnerBinding(ctx context.Context, tenant *platformv1alpha1.Tenant, ns string) error {
	rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: OwnersRoleBindingName, Namespace: ns}}
	if len(tenant.Spec.Owners) == 0 {
		return client.IgnoreNotFound(r.Delete(ctx, rb))
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
		mergeLabels(rb, labelsFor(tenant))
		// RoleRef is immutable; it is always the same value so updates are safe.
		rb.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "admin"}
		rb.Subjects = ownerSubjects(tenant.Spec.Owners)
		return controllerutil.SetControllerReference(tenant, rb, r.Scheme)
	})
	return err
}

// ownerSubjects maps "alice@example.com" to a User and "group:team-a" to a Group.
func ownerSubjects(owners []string) []rbacv1.Subject {
	subjects := make([]rbacv1.Subject, 0, len(owners))
	for _, o := range owners {
		if g, ok := strings.CutPrefix(o, ownerGroupPrefix); ok {
			subjects = append(subjects, rbacv1.Subject{Kind: rbacv1.GroupKind, APIGroup: rbacv1.GroupName, Name: g})
			continue
		}
		subjects = append(subjects, rbacv1.Subject{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: o})
	}
	return subjects
}

// finalize deletes every namespace created for the tenant, then releases the
// finalizer. Namespace deletion itself is asynchronous (the namespace
// lifecycle controller drains contents); we only wait until every delete has
// been accepted.
func (r *TenantReconciler) finalize(ctx context.Context, tenant *platformv1alpha1.Tenant) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(tenant, platformv1alpha1.TenantFinalizer) {
		return ctrl.Result{}, nil
	}

	if tenant.Status.Phase != platformv1alpha1.PhaseTerminating {
		original := tenant.DeepCopy()
		tenant.Status.Phase = platformv1alpha1.PhaseTerminating
		meta.SetStatusCondition(&tenant.Status.Conditions, metav1.Condition{
			Type: platformv1alpha1.ConditionReady, Status: metav1.ConditionFalse,
			Reason: "Terminating", Message: "deleting tenant namespaces", ObservedGeneration: tenant.Generation,
		})
		if err := r.Status().Patch(ctx, tenant, client.MergeFrom(original)); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
	}

	owned, err := r.ownedNamespaces(ctx, tenant)
	if err != nil {
		return ctrl.Result{}, err
	}
	for i := range owned {
		if !owned[i].DeletionTimestamp.IsZero() {
			continue
		}
		if err := r.Delete(ctx, &owned[i]); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
	}

	// Re-list to confirm every namespace is gone or terminating.
	owned, err = r.ownedNamespaces(ctx, tenant)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, ns := range owned {
		if ns.DeletionTimestamp.IsZero() {
			return ctrl.Result{RequeueAfter: requeueWhileTerminating}, nil
		}
	}

	controllerutil.RemoveFinalizer(tenant, platformv1alpha1.TenantFinalizer)
	return ctrl.Result{}, client.IgnoreNotFound(r.Update(ctx, tenant))
}

// SetupWithManager sets up the controller with the Manager.
func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Tenant{}).
		Owns(&corev1.Namespace{}).
		Owns(&corev1.ResourceQuota{}).
		Owns(&corev1.LimitRange{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&rbacv1.RoleBinding{}).
		Named("tenant").
		Complete(r)
}
