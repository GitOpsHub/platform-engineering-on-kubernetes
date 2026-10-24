package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// IsEmpty reports whether no quota dimension is set.
func (q TenantQuota) IsEmpty() bool {
	return q.CPU == "" && q.Memory == "" && q.Storage == "" && q.Pods == 0
}

// ResourceList converts the quota into the "hard" section of a ResourceQuota.
// It is shared by the controller and the validating webhook so both agree on
// what a valid quota is.
func (q TenantQuota) ResourceList() (corev1.ResourceList, error) {
	hard := corev1.ResourceList{}
	parse := func(field, value string, names ...corev1.ResourceName) error {
		if value == "" {
			return nil
		}
		qty, err := resource.ParseQuantity(value)
		if err != nil {
			return fmt.Errorf("spec.quota.%s: invalid quantity %q: %w", field, value, err)
		}
		if qty.Sign() < 0 {
			return fmt.Errorf("spec.quota.%s: must not be negative, got %q", field, value)
		}
		for _, n := range names {
			hard[n] = qty.DeepCopy()
		}
		return nil
	}
	if err := parse("cpu", q.CPU, corev1.ResourceRequestsCPU, corev1.ResourceLimitsCPU); err != nil {
		return nil, err
	}
	if err := parse("memory", q.Memory, corev1.ResourceRequestsMemory, corev1.ResourceLimitsMemory); err != nil {
		return nil, err
	}
	if err := parse("storage", q.Storage, corev1.ResourceRequestsStorage); err != nil {
		return nil, err
	}
	if q.Pods < 0 {
		return nil, fmt.Errorf("spec.quota.pods: must not be negative, got %d", q.Pods)
	}
	if q.Pods > 0 {
		hard[corev1.ResourcePods] = *resource.NewQuantity(int64(q.Pods), resource.DecimalSI)
	}
	return hard, nil
}
