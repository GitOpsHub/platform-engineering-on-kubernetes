# Lab 03: Build a Platform API

## Objectives
- Build a Kubernetes-native Platform API using controller-runtime
- Define Custom Resource Definitions (CRDs) for platform resources
- Implement controllers for Service, Environment, Tenant, and Deployment
- Deploy the Platform API to a cluster
- Test with the platform CLI

## Prerequisites
- Completed Lab 01 (Cluster Setup) and Lab 02 (GitOps Bootstrap)
- Go 1.21+ installed
- kubebuilder CLI installed
- Docker installed

## Estimated Time: 60-90 minutes

---

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Platform CLI   │────▶│  Platform API    │────▶│  Kubernetes     │
│  (kubectl-like) │     │  (CRDs + Ctrlrs) │     │  Resources      │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                              ▲
                              │
                     ┌────────┴────────┐
                     │  GitOps         │
                     │  (ArgoCD/Flux)  │
                     └─────────────────┘
```

---

## Step 1: Initialize Project with Kubebuilder

```bash
# Create project directory
mkdir -p platform-api
cd platform-api

# Initialize kubebuilder project
kubebuilder init --domain mycompany.io --repo github.com/myorg/platform-api --plugins=go/v4

# Create API types
kubebuilder create api --group platform --version v1 --kind Service --resource --controller
kubebuilder create api --group platform --version v1 --kind Environment --resource --controller
kubebuilder create api --group platform --version v1 --kind Tenant --resource --controller
kubebuilder create api --group platform --version v1 --kind Deployment --resource --controller
```

---

## Step 2: Define API Types

### Service Type
```go
// api/v1/service_types.go
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ServiceSpec struct {
	Description string            `json:"description,omitempty"`
	Owner       string            `json:"owner"`
	Repository  string            `json:"repository,omitempty"`
	Language    string            `json:"language,omitempty"`
	Framework   string            `json:"framework,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

type ServiceStatus struct {
	Phase       string            `json:"phase,omitempty"`
	Conditions  []metav1.Condition `json:"conditions,omitempty"`
	Environments []string         `json:"environments,omitempty"`
	LastDeployed *metav1.Time      `json:"lastDeployed,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Owner",type=string,JSONPath=.spec.owner
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=.status.phase
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=.metadata.creationTimestamp
type Service struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   ServiceSpec   `json:"spec,omitempty"`
	Status ServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Service `json:"items"`
}
```

### Environment Type
```go
// api/v1/environment_types.go
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EnvironmentSpec struct {
	DisplayName string            `json:"displayName,omitempty"`
	Description string            `json:"description,omitempty"`
	Cluster     string            `json:"cluster,omitempty"`
	Namespace   string            `json:"namespace"`
	Type        EnvironmentType   `json:"type,omitempty"` // development, staging, production
	Tags        map[string]string `json:"tags,omitempty"`
}

type EnvironmentType string

const (
	EnvironmentTypeDevelopment EnvironmentType = "development"
	EnvironmentTypeStaging     EnvironmentType = "staging"
	EnvironmentTypeProduction  EnvironmentType = "production"
)

type EnvironmentStatus struct {
	Phase      string            `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	Services   []string          `json:"services,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=.spec.type
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=.spec.cluster
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=.spec.namespace
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=.status.phase
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=.metadata.creationTimestamp
type Environment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   EnvironmentSpec   `json:"spec,omitempty"`
	Status EnvironmentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type EnvironmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Environment `json:"items"`
}
```

### Deployment Type
```go
// api/v1/deployment_types.go
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DeploymentSpec struct {
	Service     string            `json:"service"`
	Environment string            `json:"environment"`
	Version     string            `json:"version"`
	Config      map[string]string `json:"config,omitempty"`
	Replicas    *int32            `json:"replicas,omitempty"`
	Resources   ResourceRequirements `json:"resources,omitempty"`
}

type ResourceRequirements struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

type DeploymentStatus struct {
	Phase       string            `json:"phase,omitempty"`
	Conditions  []metav1.Condition `json:"conditions,omitempty"`
	Replicas    int32             `json:"replicas,omitempty"`
	ReadyReplicas int32           `json:"readyReplicas,omitempty"`
	Image       string            `json:"image,omitempty"`
	Endpoint    string            `json:"endpoint,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Service",type=string,JSONPath=.spec.service
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=.spec.environment
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=.spec.version
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=.status.phase
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=.status.readyReplicas
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=.metadata.creationTimestamp
type Deployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   DeploymentSpec   `json:"spec,omitempty"`
	Status DeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type DeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Deployment `json:"items"`
}
```

---

## Step 3: Implement Controllers

### Service Controller
```go
// internal/controller/service_controller.go
package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1 "github.com/myorg/platform-api/api/v1"
)

type ServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx).WithValues("service", req.NamespacedName)

	service := &platformv1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !service.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, service)
	}

	// Initialize status
	if service.Status.Phase == "" {
		service.Status.Phase = "Pending"
		if err := r.Status().Update(ctx, service); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Validate service
	if err := r.validateService(service); err != nil {
		log.Error(err, "Service validation failed")
		r.updateCondition(service, "Valid", metav1.ConditionFalse, "ValidationFailed", err.Error())
		r.updatePhase(ctx, service, "Failed")
		return ctrl.Result{}, err
	}

	// Update status
	r.updateCondition(service, "Valid", metav1.ConditionTrue, "Validated", "Service is valid")
	r.updatePhase(ctx, service, "Ready")

	log.Info("Service reconciled successfully")
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *ServiceReconciler) handleDeletion(ctx context.Context, service *platformv1.Service) (ctrl.Result, error) {
	// Cleanup logic here
	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) validateService(service *platformv1.Service) error {
	if service.Spec.Owner == "" {
		return fmt.Errorf("owner is required")
	}
	return nil
}

func (r *ServiceReconciler) updateCondition(service *platformv1.Service, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
	// ... update logic
}

func (r *ServiceReconciler) updatePhase(ctx context.Context, service *platformv1.Service, phase string) {
	service.Status.Phase = phase
	r.Status().Update(ctx, service)
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1.Service{}).
		Complete(r)
}
```

---

## Step 4: Build and Deploy

### Build Docker Image
```bash
# Build
make docker-build IMG=ghcr.io/myorg/platform-api:v1.0.0

# Push
make docker-push IMG=ghcr.io/myorg/platform-api:v1.0.0
```

### Deploy with Kustomize
```bash
# Deploy to cluster
make deploy IMG=ghcr.io/myorg/platform-api:v1.0.0

# Or use kustomize directly
kubectl apply -k config/default
```

### Verify Deployment
```bash
# Check pods
kubectl get pods -n platform-system -l control-plane=controller-manager

# Check CRDs
kubectl get crd | grep platform.mycompany.io

# Check API
kubectl api-resources --api-group=platform.mycompany.io
```

---

## Step 5: Test the Platform API

### Create a Service
```yaml
# service.yaml
apiVersion: platform.mycompany.io/v1
kind: Service
metadata:
  name: user-service
  namespace: platform-system
spec:
  description: "User management service"
  owner: "team-platform"
  repository: "https://github.com/myorg/user-service"
  language: "go"
  framework: "gin"
  tags:
    team: "platform"
    criticality: "high"
```

```bash
kubectl apply -f service.yaml
```

### Create an Environment
```yaml
# environment.yaml
apiVersion: platform.mycompany.io/v1
kind: Environment
metadata:
  name: production
  namespace: platform-system
spec:
  displayName: "Production Environment"
  description: "Production environment for customer-facing services"
  cluster: "production-east"
  namespace: "production"
  type: "production"
```

```bash
kubectl apply -f environment.yaml
```

### Create a Deployment
```yaml
# deployment.yaml
apiVersion: platform.mycompany.io/v1
kind: Deployment
metadata:
  name: user-service-prod
  namespace: platform-system
spec:
  service: "user-service"
  environment: "production"
  version: "v1.2.3"
  replicas: 3
  resources:
    cpu: "500m"
    memory: "512Mi"
  config:
    LOG_LEVEL: "info"
    DB_POOL_SIZE: "10"
```

```bash
kubectl apply -f deployment.yaml
```

### List Resources via CLI
```bash
# Using kubectl
kubectl get services.platform.mycompany.io -n platform-system
kubectl get environments.platform.mycompany.io -n platform-system
kubectl get deployments.platform.mycompany.io -n platform-system

# Using platform-cli (from Lab 04)
platform-cli service list
platform-cli env list
platform-cli deployment list
```

---

## Step 6: Add Validation Webhooks

```go
// api/v1/service_webhook.go
package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (r *Service) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

var _ webhook.Defaulter = &Service{}

func (r *Service) Default() {
	if r.Spec.Language == "" {
		r.Spec.Language = "go"
	}
	if r.Spec.Framework == "" {
		r.Spec.Framework = "standard"
	}
}

var _ webhook.Validator = &Service{}

func (r *Service) ValidateCreate() error {
	if r.Spec.Owner == "" {
		return field.Invalid(field.NewPath("spec").Child("owner"), r.Spec.Owner, "owner is required")
	}
	if r.Spec.Repository != "" && !strings.HasPrefix(r.Spec.Repository, "https://") {
		return field.Invalid(field.NewPath("spec").Child("repository"), r.Spec.Repository, "repository must be HTTPS URL")
	}
	return nil
}

func (r *Service) ValidateUpdate(old runtime.Object) error {
	return r.ValidateCreate()
}

func (r *Service) ValidateDelete() error {
	return nil
}
```

---

## Troubleshooting

### Controller Not Starting
```bash
# Check controller logs
kubectl logs -n platform-system -l control-plane=controller-manager -f

# Check RBAC
kubectl auth can-i create services.platform.mycompany.io --as=system:serviceaccount:platform-system:controller-manager
```

### CRD Not Found
```bash
# Reinstall CRDs
make install

# Or manually
kubectl apply -f config/crd/bases/
```

### Webhook Failures
```bash
# Check webhook service
kubectl get svc -n platform-system

# Check cert-manager
kubectl get certificaterequest -n platform-system
```

---

## Next Steps

1. **Lab 04**: [Build Service Catalog with Backstage](../lab-04-service-catalog/README.md)
2. **Read**: [Platform API Design](../docs/09-platform-apis/platform-api-design.md)
3. **Explore**: [Controller Runtime Documentation](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
4. **Practice**: Add more CRDs (Database, Cache, Queue, etc.)

---

## Resources

- [Kubebuilder Book](https://kubebuilder.io/)
- [Controller Runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
