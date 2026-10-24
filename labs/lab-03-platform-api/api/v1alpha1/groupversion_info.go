// Package v1alpha1 contains the API types for the platform.mycompany.io group.
//
// +kubebuilder:object:generate=true
// +groupName=platform.mycompany.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is the group/version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "platform.mycompany.io", Version: "v1alpha1"}

	// SchemeBuilder registers the types of this group with a runtime.Scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds WebApp and WebAppList to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&WebApp{}, &WebAppList{})
}
