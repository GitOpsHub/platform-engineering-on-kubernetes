package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TenantSpec defines the desired state of Tenant
type TenantSpec struct {
	// DisplayName is the human-readable name of the tenant
	DisplayName string `json:"displayName"`

	// Owner is the team or individual responsible for this tenant
	Owner string `json:"owner"`

	// Email is the contact email for the tenant
	Email string `json:"email"`

	// Description is an optional description of the tenant
	Description string `json:"description,omitempty"`

	// Quota defines resource quotas for this tenant
	Quota TenantQuotaSpec `json:"quota,omitempty"`

	// Namespaces defines the namespace template for this tenant
	Namespaces TenantNamespacesSpec `json:"namespaces,omitempty"`

	// Labels to apply to tenant resources
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to apply to tenant resources
	Annotations map[string]string `json:"annotations,omitempty"`
}

// TenantQuotaSpec defines resource quotas for a tenant
type TenantQuotaSpec struct {
	// CPU limit (e.g., "10", "1000m")
	CPU string `json:"cpu,omitempty"`

	// Memory limit (e.g., "10Gi", "100Mi")
	Memory string `json:"memory,omitempty"`

	// Storage limit (e.g., "100Gi")
	Storage string `json:"storage,omitempty"`

	// Maximum number of services
	Services int `json:"services,omitempty"`

	// Maximum number of environments
	Environments int `json:"environments,omitempty"`

	// Maximum number of deployments
	Deployments int `json:"deployments,omitempty"`

	// Maximum number of pods
	Pods int `json:"pods,omitempty"`

	// Maximum number of secrets
	Secrets int `json:"secrets,omitempty"`

	// Maximum number of configmaps
	ConfigMaps int `json:"configmaps,omitempty"`

	// Maximum number of persistent volume claims
	PVCs int `json:"pvcs,omitempty"`

	// Maximum number of load balancer services
	LoadBalancers int `json:"loadBalancers,omitempty"`

	// Maximum number of node ports
	NodePorts int `json:"nodePorts,omitempty"`
}

// TenantNamespacesSpec defines namespace configuration for a tenant
type TenantNamespacesSpec struct {
	// Prefix for tenant namespaces (default: tenant name)
	Prefix string `json:"prefix,omitempty"`

	// Environment suffixes to create (e.g., ["dev", "staging", "prod"])
	Environments []string `json:"environments,omitempty"`

	// Default environment (used if Environments not specified)
	DefaultEnvironment string `json:"defaultEnvironment,omitempty"`

	// Labels to apply to all tenant namespaces
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to apply to all tenant namespaces
	Annotations map[string]string `json:"annotations,omitempty"`

	// NetworkPolicy to apply to tenant namespaces
	NetworkPolicy *NetworkPolicySpec `json:"networkPolicy,omitempty"`

	// ResourceQuota to apply to tenant namespaces
	ResourceQuota *ResourceQuotaSpec `json:"resourceQuota,omitempty"`

	// LimitRange to apply to tenant namespaces
	LimitRange *LimitRangeSpec `json:"limitRange,omitempty"`
}

// NetworkPolicySpec defines default network policy for tenant namespaces
type NetworkPolicySpec struct {
	// Ingress rules
	Ingress []NetworkPolicyIngressRule `json:"ingress,omitempty"`

	// Egress rules
	Egress []NetworkPolicyEgressRule `json:"egress,omitempty"`

	// Policy types
	PolicyTypes []string `json:"policyTypes,omitempty"`
}

// NetworkPolicyIngressRule defines an ingress rule
type NetworkPolicyIngressRule struct {
	// From defines the source
	From []NetworkPolicyPeer `json:"from,omitempty"`

	// Ports defines allowed ports
	Ports []NetworkPolicyPort `json:"ports,omitempty"`
}

// NetworkPolicyEgressRule defines an egress rule
type NetworkPolicyEgressRule struct {
	// To defines the destination
	To []NetworkPolicyPeer `json:"to,omitempty"`

	// Ports defines allowed ports
	Ports []NetworkPolicyPort `json:"ports,omitempty"`
}

// NetworkPolicyPeer defines a network policy peer
type NetworkPolicyPeer struct {
	// PodSelector selects pods
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`

	// NamespaceSelector selects namespaces
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// IPBlock defines IP CIDR
	IPBlock *IPBlock `json:"ipBlock,omitempty"`
}

// NetworkPolicyPort defines a network policy port
type NetworkPolicyPort struct {
	Protocol *string `json:"protocol,omitempty"`
	Port     *int32  `json:"port,omitempty"`
	EndPort  *int32  `json:"endPort,omitempty"`
}

// IPBlock defines an IP CIDR block
type IPBlock struct {
	CIDR   string   `json:"cidr"`
	Except []string `json:"except,omitempty"`
}

// ResourceQuotaSpec defines a resource quota
type ResourceQuotaSpec struct {
	// Hard limits
	Hard map[string]string `json:"hard,omitempty"`

	// Scopes
	Scopes []string `json:"scopes,omitempty"`
}

// LimitRangeSpec defines a limit range
type LimitRangeSpec struct {
	Limits []LimitRangeItem `json:"limits,omitempty"`
}

// LimitRangeItem defines a limit range item
type LimitRangeItem struct {
	Type                 string            `json:"type"`
	Max                  map[string]string `json:"max,omitempty"`
	Min                  map[string]string `json:"min,omitempty"`
	Default              map[string]string `json:"default,omitempty"`
	DefaultRequest       map[string]string `json:"defaultRequest,omitempty"`
	MaxLimitRequestRatio map[string]string `json:"maxLimitRequestRatio,omitempty"`
}

// TenantStatus defines the observed state of Tenant
type TenantStatus struct {
	// Phase represents the current phase of the tenant
	Phase TenantPhase `json:"phase,omitempty"`

	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Namespaces lists the namespaces created for this tenant
	Namespaces []string `json:"namespaces,omitempty"`

	// ObservedGeneration is the last generation observed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastReconcileTime is the last time the tenant was reconciled
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`
}

// TenantPhase represents the phase of a tenant
type TenantPhase string

const (
	// TenantPhasePending indicates the tenant is being created
	TenantPhasePending TenantPhase = "Pending"

	// TenantPhaseReady indicates the tenant is ready
	TenantPhaseReady TenantPhase = "Ready"

	// TenantPhaseDeleting indicates the tenant is being deleted
	TenantPhaseDeleting TenantPhase = "Deleting"

	// TenantPhaseFailed indicates the tenant reconciliation failed
	TenantPhaseFailed TenantPhase = "Failed"
)

// TenantQuotaSpec defines the desired state of TenantQuota
type TenantQuotaSpec struct {
	// TenantRef references the tenant this quota belongs to
	TenantRef string `json:"tenantRef"`

	// Quota defines the resource quota
	Quota TenantQuotaSpec `json:"quota"`
}

// TenantQuotaStatus defines the observed state of TenantQuota
type TenantQuotaStatus struct {
	// Used resources
	Used map[string]string `json:"used,omitempty"`

	// Hard limits
	Hard map[string]string `json:"hard,omitempty"`

	// Conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=.spec.displayName
// +kubebuilder:printcolumn:name="Owner",type=string,JSONPath=.spec.owner
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=.status.phase
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=.metadata.creationTimestamp

// Tenant is the Schema for the tenants API
type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantSpec   `json:"spec,omitempty"`
	Status TenantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TenantList contains a list of Tenant
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Tenant",type=string,JSONPath=.spec.tenantRef
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=.status.phase
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=.metadata.creationTimestamp

// TenantQuota is the Schema for the tenantquotas API
type TenantQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantQuotaSpec   `json:"spec,omitempty"`
	Status TenantQuotaStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TenantQuotaList contains a list of TenantQuota
type TenantQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TenantQuota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
	SchemeBuilder.Register(&TenantQuota{}, &TenantQuotaList{})
}
