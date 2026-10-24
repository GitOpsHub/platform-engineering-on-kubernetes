// Package webhooks contains the validating admission webhook for Tenants.
package webhooks

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	platformv1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/tenant-operator/api/v1alpha1"
)

// +kubebuilder:webhook:path=/validate-platform-mycompany-io-v1alpha1-tenant,mutating=false,failurePolicy=fail,sideEffects=None,groups=platform.mycompany.io,resources=tenants,verbs=create;update,versions=v1alpha1,name=vtenant.platform.mycompany.io,admissionReviewVersions=v1

// TenantValidator rejects Tenants whose quota cannot be parsed or whose
// namespaces collide with another tenant or with an unmanaged namespace.
//
// The webhook gives fast feedback at `kubectl apply` time; the controller
// re-checks the same invariants because webhooks can be bypassed (e.g. when
// the webhook is down and failurePolicy=Ignore, or objects created before the
// webhook was installed).
type TenantValidator struct {
	Reader client.Reader
}

var _ admission.Validator[*platformv1alpha1.Tenant] = &TenantValidator{}

// SetupWithManager registers the webhook with the manager's webhook server.
func (v *TenantValidator) SetupWithManager(mgr ctrl.Manager) error {
	if v.Reader == nil {
		v.Reader = mgr.GetClient()
	}
	return builder.WebhookManagedBy(mgr, &platformv1alpha1.Tenant{}).
		WithValidator(v).
		Complete()
}

// ValidateCreate implements admission.Validator.
func (v *TenantValidator) ValidateCreate(ctx context.Context, t *platformv1alpha1.Tenant) (admission.Warnings, error) {
	return v.validate(ctx, t)
}

// ValidateUpdate implements admission.Validator.
func (v *TenantValidator) ValidateUpdate(ctx context.Context, _, t *platformv1alpha1.Tenant) (admission.Warnings, error) {
	return v.validate(ctx, t)
}

// ValidateDelete implements admission.Validator. Deletion is always allowed.
func (v *TenantValidator) ValidateDelete(context.Context, *platformv1alpha1.Tenant) (admission.Warnings, error) {
	return nil, nil
}

func (v *TenantValidator) validate(ctx context.Context, t *platformv1alpha1.Tenant) (admission.Warnings, error) {
	var (
		errs     field.ErrorList
		warnings admission.Warnings
		spec     = field.NewPath("spec")
	)

	if _, err := t.Spec.Quota.ResourceList(); err != nil {
		errs = append(errs, field.Invalid(spec.Child("quota"), t.Spec.Quota, err.Error()))
	}

	for i, o := range t.Spec.Owners {
		if strings.TrimSpace(strings.TrimPrefix(o, "group:")) == "" {
			errs = append(errs, field.Invalid(spec.Child("owners").Index(i), o, "owner must not be empty"))
		}
	}
	if len(t.Spec.Owners) == 0 {
		warnings = append(warnings, "spec.owners is empty: nobody will be granted admin in the tenant namespaces")
	}

	claimed, err := v.claimedNamespaces(ctx, t.Name)
	if err != nil {
		return warnings, apierrors.NewInternalError(err)
	}

	seen := map[string]bool{}
	nsPath := spec.Child("namespaces")
	for i, ns := range t.Spec.Namespaces {
		p := nsPath.Index(i)
		for _, msg := range validation.IsDNS1123Label(ns) {
			errs = append(errs, field.Invalid(p, ns, msg))
		}
		if seen[ns] {
			errs = append(errs, field.Duplicate(p, ns))
		}
		seen[ns] = true

		if owner, ok := claimed[ns]; ok {
			errs = append(errs, field.Forbidden(p, fmt.Sprintf("namespace %q is already claimed by tenant %q", ns, owner)))
			continue
		}

		existing := &corev1.Namespace{}
		err := v.Reader.Get(ctx, client.ObjectKey{Name: ns}, existing)
		switch {
		case apierrors.IsNotFound(err):
		case err != nil:
			return warnings, apierrors.NewInternalError(err)
		default:
			if owner := existing.Labels[platformv1alpha1.LabelTenant]; owner != t.Name {
				if owner == "" {
					errs = append(errs, field.Forbidden(p, fmt.Sprintf("namespace %q already exists and is not managed by any tenant", ns)))
				} else {
					errs = append(errs, field.Forbidden(p, fmt.Sprintf("namespace %q belongs to tenant %q", ns, owner)))
				}
			}
		}
	}

	if len(errs) == 0 {
		return warnings, nil
	}
	return warnings, apierrors.NewInvalid(
		schema.GroupKind{Group: platformv1alpha1.GroupVersion.Group, Kind: "Tenant"}, t.Name, errs)
}

// claimedNamespaces maps namespace -> tenant for every Tenant except self.
func (v *TenantValidator) claimedNamespaces(ctx context.Context, self string) (map[string]string, error) {
	list := &platformv1alpha1.TenantList{}
	if err := v.Reader.List(ctx, list); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, other := range list.Items {
		if other.Name == self {
			continue
		}
		for _, ns := range other.Spec.Namespaces {
			out[ns] = other.Name
		}
	}
	return out, nil
}
