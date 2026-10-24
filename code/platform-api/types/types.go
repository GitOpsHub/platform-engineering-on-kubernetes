// Package types holds the wire types shared by the platform-api server
// (handlers) and its Go client SDK (client).
//
// The shapes mirror the platform.mycompany.io/v1alpha1 CRDs (Tenant,
// Environment, Workload) implemented by the operators in this repository, but
// this package deliberately has no Kubernetes dependencies so the SDK stays
// lightweight for consumers.
package types

// API group/version served by the CRDs this API fronts.
const (
	Group      = "platform.mycompany.io"
	Version    = "v1alpha1"
	APIVersion = Group + "/" + Version
)

// TypeMeta mirrors metav1.TypeMeta.
type TypeMeta struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

// ObjectMeta is the subset of metav1.ObjectMeta exposed by the API.
// Field names match Kubernetes so objects round-trip through JSON unchanged.
type ObjectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	UID               string            `json:"uid,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	Generation        int64             `json:"generation,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
}

// Condition mirrors metav1.Condition (timestamps kept as RFC3339 strings).
type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

// Object is implemented by every top-level resource type.
type Object interface {
	GetMeta() *ObjectMeta
	Validator
}

// ---------------------------------------------------------------------------
// Tenant (cluster-scoped)
// ---------------------------------------------------------------------------

// TenantQuota is the aggregate resource quota for a tenant.
type TenantQuota struct {
	CPU     string `json:"cpu,omitempty"`
	Memory  string `json:"memory,omitempty"`
	Storage string `json:"storage,omitempty"`
	Pods    int    `json:"pods,omitempty"`
}

// TenantSpec is the desired state of a Tenant.
type TenantSpec struct {
	DisplayName      string      `json:"displayName"`
	Owners           []string    `json:"owners,omitempty"`
	Namespaces       []string    `json:"namespaces,omitempty"`
	Quota            TenantQuota `json:"quota,omitempty"`
	NetworkIsolation bool        `json:"networkIsolation,omitempty"`
}

// TenantStatus is the observed state of a Tenant.
type TenantStatus struct {
	Phase      string      `json:"phase,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
	Namespaces []string    `json:"namespaces,omitempty"`
}

// Tenant is a team or business unit that owns namespaces on the platform.
type Tenant struct {
	TypeMeta `json:",inline"`
	Metadata ObjectMeta   `json:"metadata"`
	Spec     TenantSpec   `json:"spec"`
	Status   TenantStatus `json:"status,omitempty"`
}

// GetMeta implements Object.
func (t *Tenant) GetMeta() *ObjectMeta { return &t.Metadata }

// ---------------------------------------------------------------------------
// Environment (namespaced)
// ---------------------------------------------------------------------------

// Environment types accepted by the platform.
const (
	EnvironmentDev     = "dev"
	EnvironmentStaging = "staging"
	EnvironmentProd    = "prod"
	EnvironmentPreview = "preview"
)

// EnvironmentSpec is the desired state of an Environment.
type EnvironmentSpec struct {
	Tenant   string `json:"tenant"`
	Type     string `json:"type"`
	TTL      string `json:"ttl,omitempty"`
	Template string `json:"template,omitempty"`
	Cluster  string `json:"cluster,omitempty"`
}

// EnvironmentStatus is the observed state of an Environment.
type EnvironmentStatus struct {
	Phase      string      `json:"phase,omitempty"`
	Namespace  string      `json:"namespace,omitempty"`
	URL        string      `json:"url,omitempty"`
	ExpiresAt  string      `json:"expiresAt,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
}

// Environment is a deployable target (dev/staging/prod/preview) for a tenant.
type Environment struct {
	TypeMeta `json:",inline"`
	Metadata ObjectMeta        `json:"metadata"`
	Spec     EnvironmentSpec   `json:"spec"`
	Status   EnvironmentStatus `json:"status,omitempty"`
}

// GetMeta implements Object.
func (e *Environment) GetMeta() *ObjectMeta { return &e.Metadata }

// ---------------------------------------------------------------------------
// Workload (namespaced)
// ---------------------------------------------------------------------------

// Trait types understood by the workload-operator.
const (
	TraitIngress     = "ingress"
	TraitAutoscaling = "autoscaling"
	TraitPDB         = "pdb"
)

// EnvVar is a single environment variable for the workload container.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

// ResourceRequirements mirrors corev1.ResourceRequirements using plain
// quantity strings (e.g. {"cpu": "250m", "memory": "256Mi"}).
type ResourceRequirements struct {
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
}

// Trait attaches an operational capability to a Workload.
type Trait struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

// WorkloadSpec is the desired state of a Workload.
type WorkloadSpec struct {
	Image     string               `json:"image"`
	Replicas  *int32               `json:"replicas,omitempty"`
	Port      int32                `json:"port,omitempty"`
	Env       []EnvVar             `json:"env,omitempty"`
	Resources ResourceRequirements `json:"resources,omitempty"`
	Traits    []Trait              `json:"traits,omitempty"`
}

// WorkloadStatus is the observed state of a Workload.
type WorkloadStatus struct {
	Phase             string      `json:"phase,omitempty"`
	AvailableReplicas int32       `json:"availableReplicas,omitempty"`
	URL               string      `json:"url,omitempty"`
	Conditions        []Condition `json:"conditions,omitempty"`
}

// Workload is an application component (formerly "service" in v1 of this API).
type Workload struct {
	TypeMeta `json:",inline"`
	Metadata ObjectMeta     `json:"metadata"`
	Spec     WorkloadSpec   `json:"spec"`
	Status   WorkloadStatus `json:"status,omitempty"`
}

// GetMeta implements Object.
func (w *Workload) GetMeta() *ObjectMeta { return &w.Metadata }

// ---------------------------------------------------------------------------
// Envelopes
// ---------------------------------------------------------------------------

// List is the response envelope for list endpoints.
type List[T any] struct {
	Items    []T    `json:"items"`
	Continue string `json:"continue,omitempty"`
}

// ErrorBody is the JSON error envelope returned for every non-2xx response.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail describes an API error.
type ErrorDetail struct {
	Code      int      `json:"code"`
	Reason    string   `json:"reason"`
	Message   string   `json:"message"`
	Details   []string `json:"details,omitempty"`
	RequestID string   `json:"requestId,omitempty"`
}

// VersionInfo is returned by GET /version.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// PlatformInfo is returned by GET /api/v1alpha1/platform.
type PlatformInfo struct {
	Name         string    `json:"name"`
	Version      string    `json:"version"`
	Description  string    `json:"description"`
	APIVersion   string    `json:"apiVersion"`
	Kubernetes   *KubeInfo `json:"kubernetes,omitempty"`
	Capabilities []string  `json:"capabilities"`
}

// KubeInfo describes the backing cluster. Error is set (and the other fields
// empty) when the cluster version could not be determined.
type KubeInfo struct {
	Version  string `json:"version,omitempty"`
	Platform string `json:"platform,omitempty"`
	Error    string `json:"error,omitempty"`
}
