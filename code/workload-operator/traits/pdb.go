package traits

import (
	"fmt"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/workload-operator/api/v1alpha1"
)

// PodDisruptionConfig is the config of the pdb trait. Set at most one of the two fields.
// When neither is set, minAvailable defaults to 1.
type PodDisruptionConfig struct {
	MinAvailable   *intstr.IntOrString `json:"minAvailable,omitempty"`
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

type podDisruptionTrait struct{}

func (podDisruptionTrait) Type() v1alpha1.TraitType { return v1alpha1.TraitPodDisruption }

func (podDisruptionTrait) Placeholder(w *v1alpha1.Workload) client.Object {
	return &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: w.Name, Namespace: w.Namespace}}
}

// Build renders a policy/v1 PodDisruptionBudget selecting the Workload's pods.
func (podDisruptionTrait) Build(w *v1alpha1.Workload, raw []byte) (client.Object, error) {
	var cfg PodDisruptionConfig
	if err := decodeConfig(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.MinAvailable != nil && cfg.MaxUnavailable != nil {
		return nil, fmt.Errorf("%w: set only one of minAvailable and maxUnavailable", ErrInvalidConfig)
	}
	if cfg.MinAvailable == nil && cfg.MaxUnavailable == nil {
		one := intstr.FromInt32(1)
		cfg.MinAvailable = &one
	}

	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: w.Name, Namespace: w.Namespace, Labels: ObjectLabels(w)},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable:   cfg.MinAvailable,
			MaxUnavailable: cfg.MaxUnavailable,
			Selector:       &metav1.LabelSelector{MatchLabels: SelectorLabels(w)},
		},
	}, nil
}
