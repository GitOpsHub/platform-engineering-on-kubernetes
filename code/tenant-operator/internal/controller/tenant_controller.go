package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	platformv1 "github.com/myorg/platform-engineering/api/v1"
)

// TenantReconciler reconciles a Tenant object
type TenantReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// +kubebuilder:rbac:groups=platform.mycompany.io,resources=tenants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.mycompany.io,resources=tenants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.mycompany.io,resources=tenantquotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.mycompany.io,resources=tenantquotas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("tenant", req.NamespacedName)

	// Fetch the Tenant
	tenant := &platformv1.Tenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Tenant")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !tenant.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, tenant)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(tenant, "platform.mycompany.io/tenant-finalizer") {
		controllerutil.AddFinalizer(tenant, "platform.mycompany.io/tenant-finalizer")
		if err := r.Update(ctx, tenant); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Update status phase
	if tenant.Status.Phase == "" {
		tenant.Status.Phase = platformv1.TenantPhasePending
		if err := r.Status().Update(ctx, tenant); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile tenant namespaces
	if err := r.reconcileNamespaces(ctx, tenant); err != nil {
		r.updateCondition(tenant, "NamespacesReady", metav1.ConditionFalse, "ReconcileError", err.Error())
		r.updateStatus(ctx, tenant, platformv1.TenantPhaseFailed)
		return ctrl.Result{}, err
	}

	// Reconcile tenant quota
	if err := r.reconcileQuota(ctx, tenant); err != nil {
		r.updateCondition(tenant, "QuotaReady", metav1.ConditionFalse, "ReconcileError", err.Error())
		r.updateStatus(ctx, tenant, platformv1.TenantPhaseFailed)
		return ctrl.Result{}, err
	}

	// Update status
	tenant.Status.ObservedGeneration = tenant.Generation
	tenant.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}
	r.updateCondition(tenant, "Ready", metav1.ConditionTrue, "Reconciled", "Tenant reconciled successfully")
	r.updateStatus(ctx, tenant, platformv1.TenantPhaseReady)

	log.Info("Tenant reconciled successfully")
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *TenantReconciler) handleDeletion(ctx context.Context, tenant *platformv1.Tenant) (ctrl.Result, error) {
	log := r.Log.WithValues("tenant", tenant.Name)
	log.Info("Deleting tenant")

	// Delete tenant namespaces
	for _, nsName := range tenant.Status.Namespaces {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
		if err := r.Delete(ctx, ns); err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete namespace", "namespace", nsName)
			return ctrl.Result{}, err
		}
	}

	// Delete tenant quota
	quota := &platformv1.TenantQuota{ObjectMeta: metav1.ObjectMeta{Name: tenant.Name}}
	if err := r.Delete(ctx, quota); err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete tenant quota")
		return ctrl.Result{}, err
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(tenant, "platform.mycompany.io/tenant-finalizer")
	if err := r.Update(ctx, tenant); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Tenant deleted successfully")
	return ctrl.Result{}, nil
}

func (r *TenantReconciler) reconcileNamespaces(ctx context.Context, tenant *platformv1.Tenant) error {
	log := r.Log.WithValues("tenant", tenant.Name)

	prefix := tenant.Spec.Namespaces.Prefix
	if prefix == "" {
		prefix = tenant.Name
	}

	environments := tenant.Spec.Namespaces.Environments
	if len(environments) == 0 {
		defaultEnv := tenant.Spec.Namespaces.DefaultEnvironment
		if defaultEnv == "" {
			defaultEnv = "dev"
		}
		environments = []string{defaultEnv}
	}

	var createdNamespaces []string

	for _, env := range environments {
		nsName := fmt.Sprintf("%s-%s", prefix, env)

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
				Labels: map[string]string{
					"platform.mycompany.io/tenant":     tenant.Name,
					"platform.mycompany.io/environment": env,
					"platform.mycompany.io/managed-by": "tenant-operator",
				},
				Annotations: tenant.Spec.Namespaces.Annotations,
			},
		}

		// Merge tenant labels
		for k, v := range tenant.Spec.Namespaces.Labels {
			ns.Labels[k] = v
		}

		// Set owner reference
		if err := ctrl.SetControllerReference(tenant, ns, r.Scheme); err != nil {
			return err
		}

		// Create or update namespace
		existing := &corev1.Namespace{}
		if err := r.Get(ctx, types.NamespacedName{Name: nsName}, existing); err != nil {
			if errors.IsNotFound(err) {
				if err := r.Create(ctx, ns); err != nil {
					return err
				}
				log.Info("Created namespace", "namespace", nsName)
			} else {
				return err
			}
		} else {
			// Update labels/annotations
			existing.Labels = ns.Labels
			existing.Annotations = ns.Annotations
			if err := r.Update(ctx, existing); err != nil {
				return err
			}
		}

		createdNamespaces = append(createdNamespaces, nsName)

		// Apply network policy if specified
		if tenant.Spec.Namespaces.NetworkPolicy != nil {
			if err := r.applyNetworkPolicy(ctx, nsName, tenant.Spec.Namespaces.NetworkPolicy); err != nil {
				return err
			}
		}

		// Apply resource quota if specified
		if tenant.Spec.Namespaces.ResourceQuota != nil {
			if err := r.applyResourceQuota(ctx, nsName, tenant.Spec.Namespaces.ResourceQuota); err != nil {
				return err
			}
		}

		// Apply limit range if specified
		if tenant.Spec.Namespaces.LimitRange != nil {
			if err := r.applyLimitRange(ctx, nsName, tenant.Spec.Namespaces.LimitRange); err != nil {
				return err
			}
		}
	}

	// Clean up old namespaces
	for _, existingNs := range tenant.Status.Namespaces {
		found := false
		for _, newNs := range createdNamespaces {
			if existingNs == newNs {
				found = true
				break
			}
		}
		if !found {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: existingNs}}
			if err := r.Delete(ctx, ns); err != nil && !errors.IsNotFound(err) {
				return err
			}
			log.Info("Deleted old namespace", "namespace", existingNs)
		}
	}

	tenant.Status.Namespaces = createdNamespaces
	return nil
}

func (r *TenantReconciler) applyNetworkPolicy(ctx context.Context, namespace string, spec *platformv1.NetworkPolicySpec) error {
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-default",
			Namespace: namespace,
		},
	}

	_, err := ctrl.CreateOrUpdate(ctx, r.Client, np, func() error {
		np.Spec.PodSelector = metav1.LabelSelector{}
		np.Spec.PolicyTypes = spec.PolicyTypes
		if len(spec.PolicyTypes) == 0 {
			np.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}
		}

		// Convert ingress rules
		np.Spec.Ingress = make([]networkingv1.NetworkPolicyIngressRule, len(spec.Ingress))
		for i, rule := range spec.Ingress {
			np.Spec.Ingress[i] = networkingv1.NetworkPolicyIngressRule{
				From:  convertNetworkPolicyPeers(rule.From),
				Ports: convertNetworkPolicyPorts(rule.Ports),
			}
		}

		// Convert egress rules
		np.Spec.Egress = make([]networkingv1.NetworkPolicyEgressRule, len(spec.Egress))
		for i, rule := range spec.Egress {
			np.Spec.Egress[i] = networkingv1.NetworkPolicyEgressRule{
				To:    convertNetworkPolicyPeers(rule.To),
				Ports: convertNetworkPolicyPorts(rule.Ports),
			}
		}
		return nil
	})

	return err
}

func convertNetworkPolicyPeers(peers []platformv1.NetworkPolicyPeer) []networkingv1.NetworkPolicyPeer {
	result := make([]networkingv1.NetworkPolicyPeer, len(peers))
	for i, p := range peers {
		result[i] = networkingv1.NetworkPolicyPeer{
			PodSelector:       p.PodSelector,
			NamespaceSelector: p.NamespaceSelector,
			IPBlock:           convertIPBlock(p.IPBlock),
		}
	}
	return result
}

func convertNetworkPolicyPorts(ports []platformv1.NetworkPolicyPort) []networkingv1.NetworkPolicyPort {
	result := make([]networkingv1.NetworkPolicyPort, len(ports))
	for i, p := range ports {
		result[i] = networkingv1.NetworkPolicyPort{
			Protocol: p.Protocol,
			Port:     p.Port,
			EndPort:  p.EndPort,
		}
	}
	return result
}

func convertIPBlock(block *platformv1.IPBlock) *networkingv1.IPBlock {
	if block == nil {
		return nil
	}
	return &networkingv1.IPBlock{
		CIDR:   block.CIDR,
		Except: block.Except,
	}
}

func (r *TenantReconciler) applyResourceQuota(ctx context.Context, namespace string, spec *platformv1.ResourceQuotaSpec) error {
	rq := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-quota",
			Namespace: namespace,
		},
	}

	_, err := ctrl.CreateOrUpdate(ctx, r.Client, rq, func() error {
		rq.Spec.Hard = spec.Hard
		rq.Spec.Scopes = spec.Scopes
		return nil
	})

	return err
}

func (r *TenantReconciler) applyLimitRange(ctx context.Context, namespace string, spec *platformv1.LimitRangeSpec) error {
	lr := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-limits",
			Namespace: namespace,
		},
	}

	_, err := ctrl.CreateOrUpdate(ctx, r.Client, lr, func() error {
		lr.Spec.Limits = make([]corev1.LimitRangeItem, len(spec.Limits))
		for i, item := range spec.Limits {
			lr.Spec.Limits[i] = corev1.LimitRangeItem{
				Type:                 corev1.LimitType(item.Type),
				Max:                  item.Max,
				Min:                  item.Min,
				Default:              item.Default,
				DefaultRequest:       item.DefaultRequest,
				MaxLimitRequestRatio: item.MaxLimitRequestRatio,
			}
		}
		return nil
	})

	return err
}

func (r *TenantReconciler) reconcileQuota(ctx context.Context, tenant *platformv1.Tenant) error {
	quota := &platformv1.TenantQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tenant.Name,
			Namespace: "platform-system",
		},
	}

	_, err := ctrl.CreateOrUpdate(ctx, r.Client, quota, func() error {
		quota.Spec.TenantRef = tenant.Name
		quota.Spec.Quota = tenant.Spec.Quota
		return nil
	})

	return err
}

func (r *TenantReconciler) updateCondition(tenant *platformv1.Tenant, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
	// Update or add condition
	found := false
	for i, c := range tenant.Status.Conditions {
		if c.Type == conditionType {
			tenant.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		tenant.Status.Conditions = append(tenant.Status.Conditions, condition)
	}
}

func (r *TenantReconciler) updateStatus(ctx context.Context, tenant *platformv1.Tenant, phase platformv1.TenantPhase) {
	tenant.Status.Phase = phase
	if err := r.Status().Update(ctx, tenant); err != nil {
		r.Log.Error(err, "Failed to update tenant status")
	}
}

func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.Tenant{}).
		Owns(&corev1.Namespace{}).
		Owns(&platformv1.TenantQuota{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
