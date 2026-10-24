# Lab 02: GitOps Bootstrap with ArgoCD

## Objectives
- Install ArgoCD in a Kubernetes cluster
- Configure App of Apps pattern for platform bootstrapping
- Set up projects, repositories, and RBAC
- Bootstrap platform components via GitOps

## Prerequisites
- Completed Lab 01 (Cluster Setup)
- kubectl, argocd CLI installed
- GitHub/GitLab repository for platform config
- Helm 3.x

## Estimated Time: 45-60 minutes

---

## Architecture

```
Git Repository (platform-config)
├── clusters/
│   └── production/
│       ├── root/                    # App of Apps
│       │   ├── root-app.yaml
│       │   ├── platform-core.yaml
│       │   ├── monitoring.yaml
│       │   ├── networking.yaml
│       │   └── security.yaml
│       ├── platform-core/
│       │   ├── cert-manager.yaml
│       │   ├── external-secrets.yaml
│       │   └── sealed-secrets.yaml
│       ├── monitoring/
│       │   ├── prometheus.yaml
│       │   ├── loki.yaml
│       │   └── tempo.yaml
│       └── services/
│           └── (service applications)
```

---

## Step 1: Prepare Platform Configuration Repository

### Create Repository Structure
```bash
# Create local directory structure
mkdir -p platform-config/clusters/production/root
mkdir -p platform-config/clusters/production/platform-core
mkdir -p platform-config/clusters/production/monitoring
mkdir -p platform-config/clusters/production/networking
mkdir -p platform-config/clusters/production/security
mkdir -p platform-config/clusters/production/services

cd platform-config
git init
git remote add origin https://github.com/YOUR_ORG/platform-config.git
```

### Create Root Application (App of Apps)
```yaml
# clusters/production/root/root-app.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: platform-root
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: platform
  source:
    repoURL: https://github.com/YOUR_ORG/platform-config
    targetRevision: main
    path: clusters/production/root
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

### Create Platform Core Application
```yaml
# clusters/production/root/platform-core.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: platform-core
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: platform
  source:
    repoURL: https://github.com/YOUR_ORG/platform-config
    targetRevision: main
    path: clusters/production/platform-core
  destination:
    server: https://kubernetes.default.svc
    namespace: platform-system
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
      - PrunePropagationPolicy=foreground
```

### Create Monitoring Application
```yaml
# clusters/production/root/monitoring.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: platform-monitoring
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: platform
  source:
    repoURL: https://github.com/YOUR_ORG/platform-config
    targetRevision: main
    path: clusters/production/monitoring
  destination:
    server: https://kubernetes.default.svc
    namespace: platform-monitoring
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

### Create Networking Application
```yaml
# clusters/production/root/networking.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: platform-networking
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: platform
  source:
    repoURL: https://github.com/YOUR_ORG/platform-config
    targetRevision: main
    path: clusters/production/networking
  destination:
    server: https://kubernetes.default.svc
    namespace: platform-networking
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

### Create Security Application
```yaml
# clusters/production/root/security.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: platform-security
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: platform
  source:
    repoURL: https://github.com/YOUR_ORG/platform-config
    targetRevision: main
    path: clusters/production/security
  destination:
    server: https://kubernetes.default.svc
    namespace: platform-security
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

---

## Step 2: Create Platform Components

### cert-manager (platform-core)
```yaml
# clusters/production/platform-core/cert-manager.yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: jetstack
  namespace: flux-system
spec:
  interval: 10m
  url: https://charts.jetstack.io
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: cert-manager
  namespace: cert-manager
spec:
  interval: 10m
  chart:
    spec:
      chart: cert-manager
      version: v1.13.0
      sourceRef:
        kind: HelmRepository
        name: jetstack
  values:
    installCRDs: true
    prometheus:
      enabled: true
    webhook:
      timeoutSeconds: 4
```

### External Secrets Operator (platform-core)
```yaml
# clusters/production/platform-core/external-secrets.yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: external-secrets
  namespace: flux-system
spec:
  interval: 10m
  url: https://charts.external-secrets.io
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: external-secrets
  namespace: external-secrets
spec:
  interval: 10m
  chart:
    spec:
      chart: external-secrets
      version: 0.9.0
      sourceRef:
        kind: HelmRepository
        name: external-secrets
  values:
    installCRDs: true
    webhook:
      port: 9443
```

### Sealed Secrets (platform-core)
```yaml
# clusters/production/platform-core/sealed-secrets.yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: sealed-secrets
  namespace: flux-system
spec:
  interval: 10m
  url: https://bitnami-labs.github.io/sealed-secrets
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: sealed-secrets
  namespace: sealed-secrets
spec:
  interval: 10m
  chart:
    spec:
      chart: sealed-secrets
      version: 2.10.0
      sourceRef:
        kind: HelmRepository
        name: sealed-secrets
```

### Prometheus Stack (monitoring)
```yaml
# clusters/production/monitoring/prometheus.yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: prometheus-community
  namespace: flux-system
spec:
  interval: 10m
  url: https://prometheus-community.github.io/helm-charts
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: kube-prometheus-stack
  namespace: platform-monitoring
spec:
  interval: 10m
  chart:
    spec:
      chart: kube-prometheus-stack
      version: 45.0.0
      sourceRef:
        kind: HelmRepository
        name: prometheus-community
  values:
    prometheus:
      prometheusSpec:
        retention: 30d
        storageSpec:
          volumeClaimTemplate:
            spec:
              storageClassName: fast-ssd
              resources:
                requests:
                  storage: 50Gi
    grafana:
      enabled: true
      persistence:
        enabled: true
        size: 10Gi
      dashboardProviders:
        dashboardproviders.yaml:
          apiVersion: 1
          providers:
            - name: 'default'
              orgId: 1
              folder: ''
              type: file
              disableDeletion: false
              updateIntervalMinutes: 10
              allowUiUpdates: true
              options:
                path: /var/lib/grafana/dashboards/default
```

### Loki Stack (monitoring)
```yaml
# clusters/production/monitoring/loki.yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: grafana
  namespace: flux-system
spec:
  interval: 10m
  url: https://grafana.github.io/helm-charts
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: loki
  namespace: platform-monitoring
spec:
  interval: 10m
  chart:
    spec:
      chart: loki
      version: 5.0.0
      sourceRef:
        kind: HelmRepository
        name: grafana
  values:
    loki:
      auth_enabled: false
      ingester:
        chunk_idle_period: 5m
        chunk_block_size: 256KB
        chunk_retain_period: 30s
      schema_config:
        configs:
          - from: 2024-01-01
            store: boltdb-shipper
            object_store: filesystem
            schema: v11
            index:
              prefix: index_
              period: 24h
      limits_config:
        reject_old_samples: true
        reject_old_samples_max_age: 168h
    promtail:
      enabled: true
```

### Tempo (monitoring)
```yaml
# clusters/production/monitoring/tempo.yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: grafana
  namespace: flux-system
spec:
  interval: 10m
  url: https://grafana.github.io/helm-charts
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: tempo
  namespace: platform-monitoring
spec:
  interval: 10m
  chart:
    spec:
      chart: tempo
      version: 1.0.0
      sourceRef:
        kind: HelmRepository
        name: grafana
  values:
    tempo:
      storage:
        trace:
          backend: local
          local:
            path: /var/tempo
      ingester:
        trace_idle_period: 10s
```

### NGINX Ingress (networking)
```yaml
# clusters/production/networking/ingress-nginx.yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: ingress-nginx
  namespace: flux-system
spec:
  interval: 10m
  url: https://kubernetes.github.io/ingress-nginx
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: ingress-nginx
  namespace: ingress-nginx
spec:
  interval: 10m
  chart:
    spec:
      chart: ingress-nginx
      version: 4.7.0
      sourceRef:
        kind: HelmRepository
        name: ingress-nginx
  values:
    controller:
      service:
        type: LoadBalancer
      metrics:
        enabled: true
        serviceMonitor:
          enabled: true
      admissionWebhooks:
        enabled: true
```

---

## Step 3: Install ArgoCD

### Install ArgoCD
```bash
# Create namespace
kubectl create namespace argocd

# Install ArgoCD
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Wait for pods
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=argocd-server -n argocd --timeout=300s
```

### Configure ArgoCD
```bash
# Get initial admin password
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d

# Port forward for initial setup
kubectl port-forward -n argocd svc/argocd-server 8080:443 &

# Login via CLI
argocd login localhost:8080 --username admin --password <password> --insecure
```

### Create Platform Project
```bash
# Create project via CLI
argocd proj create platform \
  --description "Platform team managed applications" \
  --src https://github.com/YOUR_ORG/platform-config \
  --dest https://kubernetes.default.svc,platform-system \
  --dest https://kubernetes.default.svc,platform-monitoring \
  --dest https://kubernetes.default.svc,platform-networking \
  --dest https://kubernetes.default.svc,platform-security

# Allow cluster-scoped resources
argocd proj allow-cluster-resource platform '*' '*'
argocd proj allow-namespace-resource platform '*' '*'
```

### Add Repository
```bash
# Add Git repository (HTTPS with token)
argocd repo add https://github.com/YOUR_ORG/platform-config \
  --username YOUR_USERNAME \
  --password YOUR_GITHUB_TOKEN \
  --name platform-config

# Or SSH
argocd repo add git@github.com:YOUR_ORG/platform-config.git \
  --ssh-private-key-path ~/.ssh/id_rsa \
  --name platform-config
```

---

## Step 4: Bootstrap Platform

### Apply Root Application
```bash
# Apply the root App of Apps
kubectl apply -f clusters/production/root/root-app.yaml

# Watch synchronization
argocd app get platform-root
argocd app sync platform-root
```

### Verify Bootstrap
```bash
# Check all applications
argocd app list

# Check sync status
argocd app get platform-core
argocd app get platform-monitoring
argocd app get platform-networking
argocd app get platform-security

# Verify components deployed
kubectl get pods -n cert-manager
kubectl get pods -n external-secrets
kubectl get pods -n sealed-secrets
kubectl get pods -n platform-monitoring
kubectl get pods -n ingress-nginx
```

---

## Step 5: Configure RBAC

### Create Teams and Roles
```bash
# Create developer role
argocd proj role create platform developer \
  --description "Developer access to services"

# Grant permissions
argocd proj role create platform developer \
  -p "p, proj:platform:developer, applications, get, platform/*, allow" \
  -p "p, proj:platform:developer, applications, sync, platform/developers/*, allow"

# Assign groups
argocd proj role add-group platform developer platform-developers
```

### Configure OIDC (Optional)
```bash
# Update argocd-cm ConfigMap for OIDC
kubectl patch configmap argocd-cm -n argocd --patch '
data:
  oidc.config: |
    name: OIDC
    issuer: https://dex.mycompany.com
    clientID: argocd
    clientSecret: '$OIDC_CLIENT_SECRET'
    requestedScopes: ["openid", "profile", "email", "groups"]
'
```

---

## Step 6: Verify End-to-End

### Test Application Deployment
```bash
# Create a test service directory
mkdir -p clusters/production/services/test-app

# Create test application
cat > clusters/production/services/test-app/application.yaml <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: test-app
  namespace: argocd
spec:
  project: platform
  source:
    repoURL: https://github.com/YOUR_ORG/platform-config
    targetRevision: main
    path: clusters/production/services/test-app
  destination:
    server: https://kubernetes.default.svc
    namespace: test-app
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
EOF

# Create simple deployment
cat > clusters/production/services/test-app/deployment.yaml <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: test-app
spec:
  replicas: 2
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: test-app
        image: nginx:alpine
        ports:
        - containerPort: 80
EOF

# Commit and push
git add clusters/production/services/test-app
git commit -m "Add test-app"
git push origin main

# Watch ArgoCD sync
argocd app get test-app
```

### Verify Drift Detection
```bash
# Make manual change
kubectl scale deployment test-app -n test-app --replicas=5

# Check ArgoCD detects drift
argocd app get test-app

# ArgoCD should auto-heal (selfHeal: true)
# Check replicas return to 2
kubectl get deployment test-app -n test-app
```

---

## Cleanup

```bash
# Delete root application (cascades to children)
kubectl delete -f clusters/production/root/root-app.yaml

# Or delete individually
argocd app delete platform-core --cascade
argocd app delete platform-monitoring --cascade
argocd app delete platform-networking --cascade
argocd app delete platform-security --cascade

# Uninstall ArgoCD
kubectl delete -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
kubectl delete namespace argocd
```

---

## Troubleshooting

### ArgoCD Not Syncing
```bash
# Check application controller logs
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-application-controller -f

# Check repo server logs
kubectl logs -n argocd -l app.kubernetes.io/name=argocd-repo-server -f

# Verify repository access
argocd repo list
argocd repo get platform-config
```

### Application Stuck in Progressing
```bash
# Get detailed status
argocd app get <app-name> -o json | jq '.status'

# Check resource health
argocd app get <app-name> --show-tree

# Force sync
argocd app sync <app-name> --force --prune
```

### Permission Denied
```bash
# Check project permissions
argocd proj get platform

# Check RBAC
kubectl get configmap argocd-rbac-cm -n argocd -o yaml
```

---

## Next Steps

1. **Read**: [Secrets Management](../docs/06-secrets-management/external-secrets.md)
2. **Lab 03**: [Build Platform API](../lab-03-platform-api/README.md)
3. **Explore**: [ArgoCD Deep Dive](../docs/05-gitops/argocd-deep-dive.md)
4. **Practice**: Add more services to the platform

---

## Resources

- [ArgoCD Getting Started](https://argo-cd.readthedocs.io/en/stable/getting_started/)
- [App of Apps Pattern](https://argo-cd.readthedocs.io/en/stable/operator-manual/cluster_bootstrapping/)
- [ApplicationSet](https://github.com/argoproj/applicationset)
