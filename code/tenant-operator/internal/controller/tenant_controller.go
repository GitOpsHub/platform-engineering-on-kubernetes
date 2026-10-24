// Package controller contains the Tenant reconciler.
//
// Ownership rules (see README "Security"):
//   - A namespace is adopted only if it carries managed-by=tenant-operator and
//     tenant=<name>, or already has a controller reference to the Tenant.
//   - Every object the operator creates is owned through a controller reference
//     so garbage collection can clean it up.
//   - Every Delete is preceded by a controller-reference check; objects that are
//     not owned by the Tenant are never deleted, even when they carry our labels.
package controller

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

// Reasons reported in status conditions for spec or ownership problems.
const (
	ReasonInvalidNamespace  = "InvalidNamespace"
	ReasonNamespaceConflict = "NamespaceConflict"
	ReasonObjectConflict    = "ObjectConflict"
	ReasonInvalidQuota      = "InvalidQuota"
)

// specError marks a problem that more retries cannot fix (an invalid spec or an
// object that belongs to someone else). It is reported in status and the
// Tenant is only reconciled again when it, or the objects it conflicts with, change.
type specError struct {
	reason string
	err    error
}

func (e specError) Error() string { return e.err.Error() }
func (e specError) Unwrap() error { return e.err }

// TenantReconciler reconciles a Tenant object.
type TenantReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// OperatorNamespace is the namespace the operator runs in. It is never used
	// (or deleted) as a tenant namespace.
	OperatorNamespace string
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

	var se specError
	if errors.As(reconcileErr, &se) {
		// Spec or ownership problem: reported in status, wait for a change.
		logger.Info("tenant spec cannot be satisfied", "reason", se.reason, "message", se.Error())
		return ctrl.Result{}, nil
	}
	if reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}
	return result, nil
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
	fail := func(cond string, err error) (ctrl.Result, error) {
		reason := "ReconcileError"
		var se specError
		if errors.As(err, &se) {
			reason = se.reason
		}
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
		return fail(platformv1alpha1.ConditionQuotaReady, specError{ReasonInvalidQuota, err})
	}

	// 1. Validate every requested namespace before touching the cluster.
	for _, name := range tenant.Spec.Namespaces {
		if err := platformv1alpha1.ValidateNamespaceName(name, r.OperatorNamespace); err != nil {
			return fail(platformv1alpha1.ConditionNamespacesReady, specError{ReasonInvalidNamespace, err})
		}
	}

	// 2. Namespaces (create or adopt), then prune namespaces we own that were removed.
	for _, name := range tenant.Spec.Namespaces {
		if err := r.ensureNamespace(ctx, tenant, name); err != nil {
			return fail(platformv1alpha1.ConditionNamespacesReady, err)
		}
	}
	if err := r.pruneNamespaces(ctx, tenant); err != nil {
		return fail(platformv1alpha1.ConditionNamespacesReady, err)
	}
	// Status is informational only; ownership is decided by controller references.
	tenant.Status.Namespaces = slices.Clone(tenant.Spec.Namespaces)
	slices.Sort(tenant.Status.Namespaces)
	setCond(platformv1alpha1.ConditionNamespacesReady, true, "Provisioned",
		fmt.Sprintf("%d namespace(s) provisioned", len(tenant.Spec.Namespaces)))

	// 3. Per-namespace policy objects.
	for _, ns := range tenant.Spec.Namespaces {
		if err := r.ensureQuota(ctx, tenant, ns, hard); err != nil {
			return fail(platformv1alpha1.ConditionQuotaReady, err)
		}
		if err := r.ensureNetworkPolicies(ctx, tenant, ns); err != nil {
			return fail(platformv1alpha1.ConditionNetworkReady, err)
		}
		if err := r.ensureOwnerBinding(ctx, tenant, ns); err != nil {
			return fail(platformv1alpha1.ConditionRBACReady, err)
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
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// operatorLabels returns the labels the operator always sets on tenant-owned objects.
func operatorLabels(tenant *platformv1alpha1.Tenant) map[string]string {
	return map[string]string{
		platformv1alpha1.LabelTenant:    tenant.Name,
		platformv1alpha1.LabelManagedBy: platformv1alpha1.ManagedByValue,
	}
}

// applyNamespaceMetadata sets user labels/annotations first (reserved keys are
// filtered), then the operator labels, so users can never override ownership labels.
func applyNamespaceMetadata(tenant *platformv1alpha1.Tenant, ns *corev1.Namespace) {
	labels := map[string]string{}
	for k, v := range ns.Labels {
		labels[k] = v
	}
	for k, v := range platformv1alpha1.FilterUserMetadata(tenant.Labels) {
		labels[k] = v
	}
	for k, v := range operatorLabels(tenant) {
		labels[k] = v
	}
	ns.Labels = labels

	annotations := map[string]string{}
	for k, v := range ns.Annotations {
		annotations[k] = v
	}
	for k, v := range platformv1alpha1.FilterUserMetadata(tenant.Annotations) {
		annotations[k] = v
	}
	ns.Annotations = annotations
}

// mergeOperatorLabels sets the operator labels on a child object, keeping its other labels.
func mergeOperatorLabels(tenant *platformv1alpha1.Tenant, obj client.Object) {
	l := obj.GetLabels()
	if l == nil {
		l = map[string]string{}
	}
	for k, v := range operatorLabels(tenant) {
		l[k] = v
	}
	obj.SetLabels(l)
}

// ensureNamespace creates the namespace, or adopts it when CanAdoptNamespace allows.
func (r *TenantReconciler) ensureNamespace(ctx context.Context, tenant *platformv1alpha1.Tenant, name string) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: name}, ns)
	if apierrors.IsNotFound(err) {
		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		applyNamespaceMetadata(tenant, ns)
		if err := controllerutil.SetControllerReference(tenant, ns, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, ns)
	}
	if err != nil {
		return err
	}

	if !platformv1alpha1.CanAdoptNamespace(ns, tenant) {
		return specError{ReasonNamespaceConflict, fmt.Errorf(
			"namespace %q already exists and is not managed by tenant %q", name, tenant.Name)}
	}
	if !ns.DeletionTimestamp.IsZero() {
		return fmt.Errorf("namespace %q is terminating; will retry", name)
	}
	if err := controllerutil.SetControllerReference(tenant, ns, r.Scheme); err != nil {
		// Another controller already owns it.
		return specError{ReasonNamespaceConflict, fmt.Errorf("namespace %q: %w", name, err)}
	}
	applyNamespaceMetadata(tenant, ns)
	return r.Update(ctx, ns)
}

// ownedNamespaces lists namespaces whose controller reference points at tenant.
func (r *TenantReconciler) ownedNamespaces(ctx context.Context, tenant *platformv1alpha1.Tenant) ([]corev1.Namespace, error) {
	list := &corev1.NamespaceList{}
	if err := r.List(ctx, list); err != nil {
		return nil, err
	}
	owned := make([]corev1.Namespace, 0, len(list.Items))
	for _, ns := range list.Items {
		if metav1.IsControlledBy(&ns, tenant) {
			owned = append(owned, ns)
		}
	}
	return owned, nil
}

// pruneNamespaces deletes namespaces owned by the tenant that are no longer in
// spec.namespaces. Only controller-owned namespaces are candidates.
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
		if platformv1alpha1.IsReservedNamespace(ns.Name, r.OperatorNamespace) {
			continue
		}
		// Ownership re-checked immediately before the delete.
		if !metav1.IsControlledBy(ns, tenant) {
			continue
		}
		if err := r.Delete(ctx, ns); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

// ensureChild creates or updates a namespaced object owned by tenant. An existing
// object without a controller reference to the tenant is a conflict; it is never adopted.
// mutate must modify the same obj pointer it closes over.
func (r *TenantReconciler) ensureChild(ctx context.Context, tenant *platformv1alpha1.Tenant, obj client.Object, mutate func()) error {
	err := r.Get(ctx, client.ObjectKeyFromObject(obj), obj)
	if apierrors.IsNotFound(err) {
		mutate()
		if err := controllerutil.SetControllerReference(tenant, obj, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, obj)
	}
	if err != nil {
		return err
	}
	if !metav1.IsControlledBy(obj, tenant) {
		return specError{ReasonObjectConflict, fmt.Errorf(
			"%T %s/%s exists and is not owned by tenant %q", obj, obj.GetNamespace(), obj.GetName(), tenant.Name)}
	}
	mutate()
	return r.Update(ctx, obj)
}

// deleteChild deletes a namespaced object only if the tenant owns it.
func (r *TenantReconciler) deleteChild(ctx context.Context, tenant *platformv1alpha1.Tenant, obj client.Object) error {
	if err := r.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !metav1.IsControlledBy(obj, tenant) {
		return nil
	}
	return client.IgnoreNotFound(r.Delete(ctx, obj))
}

func (r *TenantReconciler) ensureQuota(ctx context.Context, tenant *platformv1alpha1.Tenant, ns string, hard corev1.ResourceList) error {
	rq := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: ResourceQuotaName, Namespace: ns}}
	if len(hard) == 0 {
		if err := r.deleteChild(ctx, tenant, rq); err != nil {
			return err
		}
	} else if err := r.ensureChild(ctx, tenant, rq, func() {
		mergeOperatorLabels(tenant, rq)
		rq.Spec.Hard = hard
	}); err != nil {
		return err
	}

	// A LimitRange gives containers sane defaults so that pods are admitted
	// once a quota on requests/limits exists.
	lr := &corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: LimitRangeName, Namespace: ns}}
	return r.ensureChild(ctx, tenant, lr, func() {
		mergeOperatorLabels(tenant, lr)
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
	})
}

func (r *TenantReconciler) ensureNetworkPolicies(ctx context.Context, tenant *platformv1alpha1.Tenant, ns string) error {
	desired := desiredNetworkPolicies(tenant.Name, ns)
	if !tenant.Spec.NetworkIsolation {
		for i := range desired {
			np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: desired[i].Name, Namespace: ns}}
			if err := r.deleteChild(ctx, tenant, np); err != nil {
				return err
			}
		}
		return nil
	}

	for i := range desired {
		want := desired[i]
		np := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: want.Name, Namespace: ns}}
		if err := r.ensureChild(ctx, tenant, np, func() {
			mergeOperatorLabels(tenant, np)
			np.Spec = want.Spec
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
		return r.deleteChild(ctx, tenant, rb)
	}
	return r.ensureChild(ctx, tenant, rb, func() {
		mergeOperatorLabels(tenant, rb)
		// RoleRef is immutable; it is always the same value so updates are safe.
		rb.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "admin"}
		rb.Subjects = ownerSubjects(tenant.Spec.Owners)
	})
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

// finalize deletes every namespace owned by the tenant, then releases the
// finalizer. Namespace deletion is asynchronous; we wait until every delete
// has been accepted. Garbage collection covers anything left behind.
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
		ns := &owned[i]
		if !ns.DeletionTimestamp.IsZero() || platformv1alpha1.IsReservedNamespace(ns.Name, r.OperatorNamespace) {
			continue
		}
		if !metav1.IsControlledBy(ns, tenant) {
			continue
		}
		if err := r.Delete(ctx, ns); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
	}

	// Re-list to confirm every owned namespace has been asked to terminate.
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
		Named("tenant").
		For(&platformv1alpha1.Tenant{}).
		Owns(&corev1.Namespace{}).
		Owns(&corev1.ResourceQuota{}).
		Owns(&corev1.LimitRange{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&rbacv1.RoleBinding{}).
		Complete(r)
}
