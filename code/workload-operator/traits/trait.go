// Package traits contains the optional capabilities that can be attached to a
// Workload. Each trait builds one child object (Ingress, HorizontalPodAutoscaler
// or PodDisruptionBudget) that is owned by the Workload.
package traits

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/workload-operator/api/v1alpha1"
)

// ErrInvalidConfig marks trait configuration that cannot be satisfied. The
// controller reports it in status and does not retry.
var ErrInvalidConfig = errors.New("invalid trait config")

// Trait builds the child object for one trait type.
type Trait interface {
	// Type is the spec.traits[].type value this trait implements.
	Type() v1alpha1.TraitType

	// Build returns the desired child object for w. config is the raw
	// spec.traits[].config; nil or empty means all defaults.
	Build(w *v1alpha1.Workload, config []byte) (client.Object, error)

	// Placeholder returns an empty object of the kind this trait builds, named
	// after w, used to look up or delete the child when the trait is removed.
	Placeholder(w *v1alpha1.Workload) client.Object
}

// All returns every trait implementation, in a stable order.
func All() []Trait {
	return []Trait{ingressTrait{}, autoscalingTrait{}, podDisruptionTrait{}}
}

// Lookup returns the trait implementing t.
func Lookup(t v1alpha1.TraitType) (Trait, error) {
	for _, tr := range All() {
		if tr.Type() == t {
			return tr, nil
		}
	}
	return nil, fmt.Errorf("%w: unknown trait type %q", ErrInvalidConfig, t)
}

// decodeConfig strictly decodes raw into dst; unknown fields are an error.
func decodeConfig(raw []byte, dst any) error {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	return nil
}

// SelectorLabels are the labels that select the pods of w. They are part of the
// Deployment selector, so they must never change for a given Workload name.
func SelectorLabels(w *v1alpha1.Workload) map[string]string {
	return map[string]string{v1alpha1.WorkloadLabel: w.Name}
}

// ObjectLabels are the labels set on every object generated for w. Tenant and
// environment labels are copied from the Workload when present.
func ObjectLabels(w *v1alpha1.Workload) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name": w.Name,
		v1alpha1.WorkloadLabel:   w.Name,
		v1alpha1.LabelManagedBy:  v1alpha1.ManagedByValue,
	}
	for _, k := range []string{v1alpha1.LabelTenant, v1alpha1.LabelEnvironment} {
		if v, ok := w.Labels[k]; ok {
			labels[k] = v
		}
	}
	return labels
}
