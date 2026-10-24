package traits

import (
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/workload-operator/api/v1alpha1"
)

// AutoscalingConfig is the config of the autoscaling trait.
type AutoscalingConfig struct {
	// MinReplicas defaults to 1.
	MinReplicas int32 `json:"minReplicas,omitempty"`
	// MaxReplicas is required and must be >= MinReplicas.
	MaxReplicas int32 `json:"maxReplicas"`
	// TargetCPUUtilizationPercentage defaults to 80.
	TargetCPUUtilizationPercentage int32 `json:"targetCPUUtilizationPercentage,omitempty"`
}

type autoscalingTrait struct{}

func (autoscalingTrait) Type() v1alpha1.TraitType { return v1alpha1.TraitAutoscaling }

func (autoscalingTrait) Placeholder(w *v1alpha1.Workload) client.Object {
	return &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: w.Name, Namespace: w.Namespace}}
}

// Build renders an autoscaling/v2 HorizontalPodAutoscaler targeting the Workload's Deployment.
func (autoscalingTrait) Build(w *v1alpha1.Workload, raw []byte) (client.Object, error) {
	cfg := AutoscalingConfig{MinReplicas: 1, TargetCPUUtilizationPercentage: 80}
	if err := decodeConfig(raw, &cfg); err != nil {
		return nil, err
	}
	if cfg.MinReplicas < 1 {
		return nil, fmt.Errorf("%w: minReplicas must be >= 1", ErrInvalidConfig)
	}
	if cfg.MaxReplicas < cfg.MinReplicas {
		return nil, fmt.Errorf("%w: maxReplicas (%d) must be >= minReplicas (%d)", ErrInvalidConfig, cfg.MaxReplicas, cfg.MinReplicas)
	}
	if cfg.TargetCPUUtilizationPercentage < 1 || cfg.TargetCPUUtilizationPercentage > 100 {
		return nil, fmt.Errorf("%w: targetCPUUtilizationPercentage must be between 1 and 100", ErrInvalidConfig)
	}

	minReplicas := cfg.MinReplicas
	target := cfg.TargetCPUUtilizationPercentage
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: w.Name, Namespace: w.Namespace, Labels: ObjectLabels(w)},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       w.Name,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: cfg.MaxReplicas,
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: &target,
					},
				},
			}},
		},
	}, nil
}
