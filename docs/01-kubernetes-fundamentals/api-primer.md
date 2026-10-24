# Kubernetes API Primer for Platform Engineers

## Learning Objectives
- Understand Kubernetes API structure: Groups, Versions, Resources
- Learn API verbs and their semantics
- Master resource representation: metadata, spec, status
- Work with API discovery and client libraries

## Prerequisites
- Basic Kubernetes concepts (pods, services, deployments)
- YAML/JSON familiarity
- kubectl basics

---

## API Structure

### Group/Version/Resource (GVR)
```
Group/Version/Resource (GVR)
├── Group: apps, batch, networking.k8s.io, platform.mycompany.io
├── Version: v1, v1beta1, v1alpha1
└── Resource: deployments, jobs, ingresses, tenants
```

### API Groups
| Group | Purpose | Stability |
|-------|---------|-----------|
| core (empty) | Pods, Services, ConfigMaps, Secrets | GA |
| apps | Deployments, ReplicaSets, StatefulSets | GA |
| batch | Jobs, CronJobs | GA |
| networking.k8s.io | Ingress, NetworkPolicy, IngressClass | GA |
| storage.k8s.io | StorageClass, VolumeAttachment | GA |
| rbac.authorization.k8s.io | Roles, RoleBindings, ClusterRoles | GA |
| apiextensions.k8s.io | CustomResourceDefinitions | GA |
| admissionregistration.k8s.io | Validating/Mutating Webhooks | GA |

### Version Lifecycle
```
v1alpha1 → v1beta1 → v1 (GA) → deprecated → removed
   ^           ^         ^           ^          ^
   |           |         |           |          |
Experimental  Beta    Stable    Deprecation  Removal
```

---

## API Verbs

### Read Operations
| Verb | Scope | Description |
|------|-------|-------------|
| get | Namespaced/Cluster | Retrieve single resource |
| list | Namespaced/Cluster | Retrieve multiple resources |
| watch | Namespaced/Cluster | Stream changes (WebSocket) |

### Write Operations
| Verb | Scope | Description |
|------|-------|-------------|
| create | Namespaced/Cluster | Create new resource |
| update | Namespaced/Cluster | Replace entire resource |
| patch | Namespaced/Cluster | Partial update (strategic/JSON/merge) |
| delete | Namespaced/Cluster | Delete single resource |
| deletecollection | Namespaced | Delete multiple resources |

### Special Operations
| Verb | Purpose |
|------|---------|
| bind | Bind pods to nodes (scheduler) |
| escalate | Escalate RBAC privileges |
| impersonate | Act as another user |
| proxy | Proxy requests to services |

---

## Resource Representation

### Metadata
```yaml
metadata:
  name: my-deployment          # Unique within namespace
  namespace: production        # Empty for cluster-scoped
  uid: a1b2c3d4-e5f6-...       # System-generated UUID
  resourceVersion: "12345"     # Optimistic locking
  generation: 3                # Spec mutation counter
  creationTimestamp: "2024-01-15T10:30:00Z"
  labels:
    app: myapp
    version: v1.2.0
    environment: production
  annotations:
    deployment.kubernetes.io/revision: "3"
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"apps/v1",...}
  finalizers:
    - foregroundDeletion
  ownerReferences:
    - apiVersion: apps/v1
      kind: ReplicaSet
      name: my-deployment-abc123
      uid: x7y8z9...
      controller: true
      blockOwnerDeletion: true
```

### Spec (Desired State)
```yaml
spec:
  replicas: 3
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      containers:
      - name: app
        image: myapp:v1.2.0
        ports:
        - containerPort: 8080
```

### Status (Observed State)
```yaml
status:
  observedGeneration: 3
  replicas: 3
  updatedReplicas: 3
  readyReplicas: 3
  availableReplicas: 3
  conditions:
  - type: Available
    status: "True"
    lastUpdateTime: "2024-01-15T10:35:00Z"
    reason: MinimumReplicasAvailable
    message: Deployment has minimum availability
```

---

## API Discovery

### Discover API Groups
```bash
# List all API groups
kubectl api-resources --api-group=apps

# List all resources (wide output)
kubectl api-resources -o wide

# Check specific resource
kubectl api-resources --namespaced=true --resource=deployments
```

### OpenAPI/Swagger
```bash
# Get OpenAPI spec
kubectl get --raw=/openapi/v2 | jq '.paths | keys[]' | head -20

# Get specific resource schema
kubectl get --raw=/openapi/v2 | jq '.paths."/apis/apps/v1/namespaces/{namespace}/deployments".get'
```

---

## Client Libraries

### Go Client (client-go)
```go
import (
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
    "k8s.io/client-go/tools/clientcmd"
)

// In-cluster config
config, err := rest.InClusterConfig()

// Or kubeconfig
config, err := clientcmd.BuildConfigFromFlags("", "/path/to/kubeconfig")

// Create clientset
clientset, err := kubernetes.NewForConfig(config)

// Use typed clients
deployments := clientset.AppsV1().Deployments("namespace")
pods := clientset.CoreV1().Pods("namespace")
```

### Dynamic Client (for CRDs)
```go
import (
    "k8s.io/client-go/dynamic"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "k8s.io/apimachinery/pkg/runtime/schema"
)

dynamicClient, _ := dynamic.NewForConfig(config)

gvr := schema.GroupVersionResource{
    Group:    "platform.mycompany.io",
    Version:  "v1",
    Resource: "tenants",
}

// List
list, _ := dynamicClient.Resource(gvr).Namespace("ns").List(ctx, metav1.ListOptions{})

// Create
obj := &unstructured.Unstructured{
    Object: map[string]interface{}{
        "apiVersion": "platform.mycompany.io/v1",
        "kind":       "Tenant",
        "metadata": map[string]interface{}{
            "name": "my-tenant",
        },
        "spec": map[string]interface{}{
            "displayName": "My Tenant",
        },
    },
}
created, _ := dynamicClient.Resource(gvr).Namespace("ns").Create(ctx, obj, metav1.CreateOptions{})
```

### Controller Runtime Client
```go
import (
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/config"
)

cfg := config.GetConfigOrDie()
cl, _ := client.New(cfg, client.Options{Scheme: scheme})

// List with field selector
var tenants TenantList
cl.List(ctx, &tenants, client.InNamespace("ns"), client.MatchingFields{"spec.owner": "team-a"})

// Create
tenant := &Tenant{...}
cl.Create(ctx, tenant)

// Update with retry
retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
    existing := &Tenant{}
    cl.Get(ctx, key, existing)
    existing.Spec = newSpec
    return cl.Update(ctx, existing)
})
```

---

## API Patterns for Platform Engineers

### Label Selectors
```bash
# Equality-based
kubectl get pods -l app=myapp,version=v1.2.0

# Set-based
kubectl get pods -l 'app in (myapp,otherapp),version notin (v1.0.0)'

# In code
labels.Set{"app": "myapp", "version": "v1.2.0"}.AsSelector()
labels.SelectorFromSet(labels.Set{"environment": "production"})
```

### Field Selectors
```bash
# Only supports metadata fields
kubectl get pods --field-selector=status.phase=Running
kubectl get pods --field-selector=spec.nodeName=node-1
```

### Pagination (Large Clusters)
```go
listOpts := metav1.ListOptions{
    Limit:        500,
    Continue:     "",
}

for {
    list, err := clientset.CoreV1().Pods("").List(ctx, listOpts)
    if err != nil { break }
    
    for _, pod := range list.Items {
        // process pod
    }
    
    if list.Continue == "" { break }
    listOpts.Continue = list.Continue
}
```

### Watch for Changes
```go
watch, _ := clientset.CoreV1().Pods("ns").Watch(ctx, metav1.ListOptions{
    FieldSelector: "status.phase=Pending",
})

for event := range watch.ResultChan() {
    switch event.Type {
    case watch.Added:
        // New pod
    case watch.Modified:
        // Pod updated
    case watch.Deleted:
        // Pod deleted
    case watch.Error:
        // Handle error
    }
}
```

---

## Hands-On Exercises

### Exercise 1: Explore API Resources
```bash
# 1. List all API groups
kubectl api-versions | sort

# 2. Find all namespaced resources
kubectl api-resources --namespaced=true -o name | sort

# 3. Find all cluster-scoped resources
kubectl api-resources --namespaced=false -o name | sort

# 4. Get resource details
kubectl explain deployment.spec.template.spec.containers
```

### Exercise 2: Work with Dynamic Client
```go
// Create a simple program to list all CRDs
package main

import (
    "context"
    "fmt"
    "k8s.io/client-go/dynamic"
    "k8s.io/client-go/tools/clientcmd"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
    config, _ := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
    dynClient, _ := dynamic.NewForConfig(config)
    
    gvr := schema.GroupVersionResource{
        Group:    "apiextensions.k8s.io",
        Version:  "v1",
        Resource: "customresourcedefinitions",
    }
    
    list, _ := dynClient.Resource(gvr).List(context.Background(), metav1.ListOptions{})
    for _, item := range list.Items {
        name := item.GetName()
        group := item.Object["spec"].(map[string]interface{})["group"]
        fmt.Printf("CRD: %s (group: %s)\n", name, group)
    }
}
```

### Exercise 3: Patch Operations
```bash
# Strategic merge patch (default)
kubectl patch deployment myapp -p '{"spec":{"replicas":5}}'

# JSON patch (RFC 6902)
kubectl patch deployment myapp --type=json -p='[{"op":"replace","path":"/spec/replicas","value":5}]'

# Merge patch
kubectl patch deployment myapp --type=merge -p='{"spec":{"replicas":5}}'
```

---

## Further Reading

- [Kubernetes API Concepts](https://kubernetes.io/docs/concepts/overview/kubernetes-api/)
- [API Reference](https://kubernetes.io/docs/reference/kubernetes-api/)
- [Client-go Examples](https://github.com/kubernetes/client-go/tree/master/examples)
- [Controller Runtime Client](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/client)
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
