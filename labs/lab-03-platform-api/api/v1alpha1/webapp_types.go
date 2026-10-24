package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types and reasons reported in WebAppStatus.Conditions.
const (
	// ConditionReady is True once every requested replica is ready.
	ConditionReady = "Ready"

	ReasonReplicasReady  = "ReplicasReady"
	ReasonWaitingForPods = "WaitingForPods"
)

// WebAppSpec is the desired state of a WebApp.
//
// The CEL rules run in the API server, so an invalid WebApp is rejected by
// kubectl before any controller sees it.
type WebAppSpec struct {
	// Image is the container image to run. It must include an explicit tag
	// or digest, and the :latest tag is rejected.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:XValidation:rule="!self.endsWith(':latest')",message="image must not use the :latest tag"
	// +kubebuilder:validation:XValidation:rule="self.contains(':') || self.contains('@')",message="image must include an explicit tag or digest"
	Image string `json:"image"`

	// Replicas is the number of pods to run.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Port is the port the container listens on. The Service maps port 80 to it.
	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port *int32 `json:"port,omitempty"`
}

// WebAppStatus is the observed state of a WebApp, written by the controller.
type WebAppStatus struct {
	// ObservedGeneration is the .metadata.generation the controller last acted on.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReadyReplicas is the number of pods that are ready.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// URL is the in-cluster address of the Service.
	// +optional
	URL string `json:"url,omitempty"`

	// Conditions describe the current state, for example Ready.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=wa,scope=Namespaced,categories=platform
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WebApp is a stateless web workload. The platform turns it into a Deployment
// and a Service, so application teams never write those objects directly.
type WebApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebAppSpec   `json:"spec,omitempty"`
	Status WebAppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WebAppList is a list of WebApp objects.
type WebAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebApp `json:"items"`
}
