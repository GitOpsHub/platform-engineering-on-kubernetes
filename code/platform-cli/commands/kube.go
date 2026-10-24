package commands

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-cli/config"
)

// Group and version of the platform CRDs (the shared API contract).
const (
	Group   = "platform.mycompany.io"
	Version = "v1alpha1"
	// APIVersion is the apiVersion string written into created objects.
	APIVersion = Group + "/" + Version
)

// GVRs of the platform resources.
var (
	TenantGVR      = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "tenants"}
	EnvironmentGVR = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "environments"}
	WorkloadGVR    = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "workloads"}
)

// newDynamicClient loads kubeconfig with the usual rules (explicit path, then
// $KUBECONFIG, then ~/.kube/config) and applies the context override.
func newDynamicClient(c config.Config) (dynamic.Interface, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if c.Kubeconfig != "" {
		rules.ExplicitPath = c.Kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: c.Context}
	rc, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(rc)
}
