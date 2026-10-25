# Logging Stack - Loki, Vector, Fluent Bit

## Learning Objectives
- Deploy and configure Loki for log aggregation
- Set up Vector/Fluent Bit for log collection
- Configure log parsing, labeling, and enrichment
- Implement log-based alerting and dashboards

## Prerequisites
- [Metrics Stack](./metrics-stack.md): Grafana installed, and the label model that Loki reuses
- A Kubernetes cluster with a default StorageClass (kind or k3d works for the exercises)
- Helm 3.x and `kubectl`
- Familiarity with container logging (`kubectl logs`, stdout/stderr)

---

## Architecture

```mermaid
graph TB
    Apps[Applications] --> Vector["Vector/Fluent Bit"]
    System[System Logs] --> Vector
    Vector --> Loki[Loki]
    Loki --> Querier[Querier]
    Querier --> Grafana[Grafana]
    Querier --> Alertmanager[Alertmanager]
    Loki --> Compactor[Compactor]
    Compactor --> Storage[(Object Storage)]
```

---

## Loki Stack Deployment

### Install Loki via Helm

This page uses the `grafana/loki` chart (the chart and version Lab 06 pins) in single-binary
mode. The older `grafana/loki-stack` chart is deprecated and bundles Promtail, which is
end-of-life, so it is not used here. Grafana comes from the [Metrics Stack](./metrics-stack.md)
install.

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update

helm upgrade --install loki grafana/loki \
  --namespace monitoring --create-namespace \
  --version 7.3.0 \
  --wait \
  --values - <<EOF
deploymentMode: SingleBinary

loki:
  auth_enabled: false
  commonConfig:
    replication_factor: 1
  storage:
    type: filesystem
  schemaConfig:
    configs:
      - from: "2024-04-01"
        store: tsdb
        object_store: filesystem
        schema: v13
        index:
          prefix: index_
          period: 24h
  limits_config:
    reject_old_samples: true
    reject_old_samples_max_age: 168h
    ingestion_rate_mb: 10
    ingestion_burst_size_mb: 20
    per_stream_rate_limit: 3MB
    per_stream_rate_limit_burst: 10MB
    max_entries_limit_per_query: 5000
    max_query_parallelism: 32
    retention_period: 720h
  compactor:
    retention_enabled: true
    delete_request_store: filesystem

singleBinary:
  replicas: 1
  persistence:
    enabled: true
    size: 5Gi            # use 100Gi and a fast StorageClass outside a lab
  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      memory: 512Mi

# Simple-scalable components are unused in SingleBinary mode.
read:
  replicas: 0
write:
  replicas: 0
backend:
  replicas: 0

# Laptop trims: no gateway, caches, canary or helm test pod.
gateway:
  enabled: false
chunksCache:
  enabled: false
resultsCache:
  enabled: false
lokiCanary:
  enabled: false
test:
  enabled: false
EOF
```

The release is named `loki`, so the in-cluster push URL is
`http://loki.monitoring.svc:3100/loki/api/v1/push`. Lab 06 installs the same chart in the
`platform-monitoring` namespace; see `labs/lab-06-observability-stack/loki-stack.yaml`.

### Install Grafana Alloy for Pod Logs

Grafana Alloy (chart `grafana/alloy` 1.12.1, Alloy v1.19.2, the pins used by Lab 06) replaces
Promtail. It reads pod logs through the Kubernetes API and pushes them to Loki, labelled with
`namespace`, `pod`, `container`, and `app`.

```bash
helm upgrade --install alloy grafana/alloy \
  --namespace monitoring \
  --version 1.12.1 \
  --wait \
  --values - <<'EOF'
controller:
  type: deployment
  replicas: 1

alloy:
  enableReporting: false
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      memory: 256Mi
  configMap:
    create: true
    content: |
      logging {
        level  = "info"
        format = "logfmt"
      }

      discovery.kubernetes "pods" {
        role = "pod"
      }

      discovery.relabel "pods" {
        targets = discovery.kubernetes.pods.targets

        rule {
          source_labels = ["__meta_kubernetes_namespace"]
          target_label  = "namespace"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_name"]
          target_label  = "pod"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_container_name"]
          target_label  = "container"
        }
        rule {
          source_labels = ["__meta_kubernetes_pod_label_app"]
          target_label  = "app"
        }
      }

      loki.source.kubernetes "pods" {
        targets    = discovery.relabel.pods.output
        forward_to = [loki.write.default.receiver]
      }

      loki.write "default" {
        endpoint {
          url = "http://loki.monitoring.svc:3100/loki/api/v1/push"
        }
      }
EOF
```

Expected output: each command ends with `STATUS: deployed`, and
`kubectl -n monitoring get pod loki-0` shows `Running`. A single Alloy Deployment is enough for a
lab because `loki.source.kubernetes` reads every pod through the API. For production, run a
node-local collector (Alloy as a DaemonSet, Vector, or Fluent Bit as described below).

Add Loki to Grafana as a data source with URL `http://loki.monitoring.svc:3100`.

---

## Vector (Modern Alternative to Fluent Bit)

### Install Vector
```bash
helm repo add vector https://vectordotdev.github.io/helm-charts
helm repo update

helm upgrade --install vector vector/vector \
  --namespace monitoring --create-namespace \
  --version 0.35.0 \
  --values - <<EOF
vector:
  config:
    api:
      enabled: true
      address: 0.0.0.0:8686
    sources:
      kubernetes_logs:
        type: kubernetes_logs
        include_annotations: true
        include_labels: true
    transforms:
      parse_json:
        type: remap
        inputs: ["kubernetes_logs"]
        source: |
          .message = parse_json!(.message) ?? .message
          .level = .level ?? "info"
          .timestamp = .timestamp ?? now()
      add_kubernetes_metadata:
        type: remap
        inputs: ["parse_json"]
        source: |
          .kubernetes_namespace = .kubernetes.pod_labels["app.kubernetes.io/name"] ?? .kubernetes.namespace
          .kubernetes_pod = .kubernetes.pod_name
          .kubernetes_container = .kubernetes.container_name
    sinks:
      loki:
        type: loki
        inputs: ["add_kubernetes_metadata"]
        endpoint: "http://loki:3100"
        labels:
          job: "kubernetes"
          cluster: "production"
        encoding:
          codec: json
      console:
        type: console
        inputs: ["add_kubernetes_metadata"]
        target: stdout
        encoding:
          codec: json
EOF
```

### Vector Configuration (Advanced)
```yaml
# vector-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: vector-config
  namespace: monitoring
data:
  vector.yaml: |
    api:
      enabled: true
      address: 0.0.0.0:8686
    
    sources:
      kubernetes_logs:
        type: kubernetes_logs
        include_annotations: true
        include_labels: true
        # Filter namespaces
        # namespace_selector:
        #   match_expressions:
        #     - key: name
        #       operator: NotIn
        #       values: [kube-system, flux-system]
      
      # Also collect container runtime logs
      container_logs:
        type: file
        include:
          - /var/log/containers/*.log
        exclude:
          - /var/log/containers/*_fluent-bit_*.log
        ignore_older_secs: 86400
    
    transforms:
      # Parse structured logs (JSON)
      parse_structured:
        type: remap
        inputs: ["kubernetes_logs", "container_logs"]
        source: |
          # Try to parse JSON
          parsed, err = parse_json(.message)
          if err == null {
            . = merge(., parsed)
          }
          # Normalize level
          .level = .level ?? .severity ?? .log_level ?? "info"
          .level = upcase(string(.level))
      
      # Enrich with Kubernetes metadata
      enrich_k8s:
        type: remap
        inputs: ["parse_structured"]
        source: |
          .k8s_namespace = .kubernetes.namespace
          .k8s_pod = .kubernetes.pod_name
          .k8s_container = .kubernetes.container_name
          .k8s_labels = .kubernetes.pod_labels
          .k8s_annotations = .kubernetes.pod_annotations
          # Add service name from labels
          .service = .k8s_labels["app.kubernetes.io/name"] ?? .k8s_labels["app"] ?? "unknown"
          .version = .k8s_labels["app.kubernetes.io/version"] ?? "unknown"
      
      # Filter sensitive data
      redact_secrets:
        type: remap
        inputs: ["enrich_k8s"]
        source: |
          # Redact common secret patterns
          .message = replace(.message, r'(?i)(password|token|secret|key|api[_-]?key)\s*[:=]\s*\S+', '${1}=[REDACTED]')
      
      # Add cluster metadata
      add_cluster_info:
        type: remap
        inputs: ["redact_secrets"]
        source: |
          .cluster = "production"
          .environment = "production"
          .region = "us-east-1"
    
    sinks:
      # Send to Loki
      loki:
        type: loki
        inputs: ["add_cluster_info"]
        endpoint: "http://loki.monitoring.svc:3100"
        labels:
          job: "kubernetes"
          cluster: "production"
        encoding:
          codec: json
        batch:
          max_bytes: 1048576
          timeout_secs: 10
        healthcheck:
          enabled: true
      
      # Also send to Elasticsearch (optional)
      # elasticsearch:
      #   type: elasticsearch
      #   inputs: ["add_cluster_info"]
      #   endpoints: ["https://elasticsearch:9200"]
      #   index: "logs-kubernetes-%Y-%m-%d"
      #   auth:
      #     strategy: basic
      #     user: elastic
      #     password: ${ELASTIC_PASSWORD}
      
      # Console for debugging
      console:
        type: console
        inputs: ["add_cluster_info"]
        target: stdout
        encoding:
          codec: json
EOF
```

---

## Log Parsing & Enrichment

### Common Log Formats

#### JSON Logs (Recommended)
```json
{
  "timestamp": "2024-01-15T10:30:00.123Z",
  "level": "info",
  "message": "User login successful",
  "service": "auth-service",
  "version": "1.2.3",
  "trace_id": "abc123",
  "span_id": "def456",
  "user_id": "user-789",
  "duration_ms": 45
}
```

#### Structured Logging in Applications
```go
// Go structured logging with zerolog
import "github.com/rs/zerolog/log"

log.Info().
    Str("service", "auth-service").
    Str("version", "1.2.3").
    Str("trace_id", traceID).
    Str("user_id", userID).
    Int("duration_ms", 45).
    Msg("User login successful")
```

```python
# Python structured logging
import structlog

logger = structlog.get_logger()

logger.info(
    "user_login_successful",
    service="auth-service",
    version="1.2.3",
    trace_id=trace_id,
    user_id=user_id,
    duration_ms=45
)
```

### LogQL Queries (Loki Query Language)

#### Basic Queries
```logql
# All logs for a namespace
{namespace="production"}

# Logs for a specific service
{namespace="production", service="auth-service"}

# Error logs only
{namespace="production", service="auth-service"} |= "error"

# Logs with specific label
{namespace="production"} |~ "level=(error|warn)"
```

#### Parsing & Filtering
```logql
# Parse JSON and filter
{namespace="production"} | json | level="error"

# Extract fields
{namespace="production"} | json | trace_id="abc123"

# Filter by duration
{namespace="production"} | json | duration_ms > 1000

# Aggregate
sum by (service) (count_over_time({namespace="production"}[5m]))
```

#### Advanced Queries
```logql
# Rate of errors by service
sum by (service) (rate({namespace="production"} | json | level="error" [5m]))

# Quantile of response times
quantile_over_time(0.95, {namespace="production"} | json | __line__=~ "duration_ms:(\\d+)" [5m])

# Top errors
topk(10, sum by (error_message) (count_over_time({namespace="production"} | json | level="error" [1h])))
```

---

## Grafana Dashboards

### Loki Dashboard (JSON)
```json
{
  "dashboard": {
    "title": "Kubernetes Logging",
    "panels": [
      {
        "title": "Log Volume by Service",
        "type": "timeseries",
        "targets": [
          {
            "expr": "sum by (service) (rate({namespace=~\"production|staging\"}[5m]))",
            "legendFormat": "{{service}}"
          }
        ]
      },
      {
        "title": "Error Rate by Service",
        "type": "timeseries",
        "targets": [
          {
            "expr": "sum by (service) (rate({namespace=~\"production|staging\"} | json | level=\"error\" [5m]))",
            "legendFormat": "{{service}}"
          }
        ]
      },
      {
        "title": "Log Levels Distribution",
        "type": "piechart",
        "targets": [
          {
            "expr": "sum by (level) (count_over_time({namespace=~\"production|staging\"} | json [1h]))",
            "legendFormat": "{{level}}"
          }
        ]
      },
      {
        "title": "Recent Error Logs",
        "type": "logs",
        "targets": [
          {
            "expr": "{namespace=~\"production|staging\"} | json | level=\"error\"",
            "legendFormat": "{{service}} - {{pod}}"
          }
        ]
      }
    ]
  }
}
```

---

## Log-Based Alerting

### Prometheus Rules from Loki
```yaml
# loki-alerts.yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: loki-alerts
  namespace: monitoring
spec:
  groups:
    - name: logging
      rules:
        - alert: HighErrorRate
          expr: |
            sum by (service, namespace) (
              rate({namespace=~"production|staging"} | json | level="error" [5m])
            ) > 0.1
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "High error rate for {{ $labels.service }}"
            description: "Error rate is {{ $value }} errors/sec for service {{ $labels.service }} in {{ $labels.namespace }}"
        
        - alert: LogVolumeSpike
          expr: |
            sum by (service, namespace) (
              rate({namespace=~"production|staging"}[5m])
            ) > 1000
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "Log volume spike for {{ $labels.service }}"
            description: "Log volume is {{ $value }} logs/sec for service {{ $labels.service }}"
        
        - alert: CriticalErrors
          expr: |
            count_over_time({namespace=~"production|staging"} | json | level="critical" [5m]) > 0
          for: 1m
          labels:
            severity: critical
          annotations:
            summary: "Critical errors detected"
            description: "Critical errors found in {{ $labels.service }}"
        
        - alert: LokiIngestionErrors
          expr: |
            rate(loki_write_requests_total{status=~"5.."}[5m]) > 0
          for: 2m
          labels:
            severity: critical
          annotations:
            summary: "Loki ingestion errors"
            description: "Loki is returning 5xx errors for write requests"
```

---

## Retention & Storage

### S3/GCS Backend (Production)
```yaml
loki:
  config: |
    common:
      storage:
        s3:
          bucketnames: loki-logs
          endpoint: s3.us-east-1.amazonaws.com
          region: us-east-1
          access_key_id: ${AWS_ACCESS_KEY_ID}
          secret_access_key: ${AWS_SECRET_ACCESS_KEY}
          s3forcepathstyle: false
          http_config:
            idle_conn_timeout: 90s
    schema_config:
      configs:
        - from: 2024-01-01
          store: boltdb-shipper
          object_store: s3
          schema: v12
          index:
            prefix: index_
            period: 24h
    limits_config:
      retention_deletes_enabled: true
      retention_period: 90d
    compactor:
      working_directory: /loki/compactor
      shared_store: s3
      compaction_interval: 10m
      retention_enabled: true
      retention_delete_delay: 2h
```

---

## Hands-On Exercises

These exercises assume Loki and Alloy from [Install Loki via Helm](#install-loki-via-helm) and
[Install Grafana Alloy for Pod Logs](#install-grafana-alloy-for-pod-logs) run in the `monitoring`
namespace. Lab 06 runs the same chart pins in `platform-monitoring`; if you use it, replace
`-n monitoring` and `loki.monitoring.svc` in the commands.

### Exercise 1: Ship logs from a workload

**Goal:** confirm that a pod's logs reach Loki with the labels you expect.

```bash
kubectl run log-demo -n default --image=busybox --restart=Never -- \
  sh -c 'while true; do echo "{\"level\":\"info\",\"msg\":\"hello\"}"; sleep 2; done'
kubectl port-forward -n monitoring svc/loki 3100:3100 &
sleep 5
curl -G -s http://localhost:3100/loki/api/v1/query_range \
  --data-urlencode 'query={pod="log-demo"}' | jq '.data.result[0].stream'
```

Expected output (label values vary by cluster):

```json
{
  "namespace": "default",
  "pod": "log-demo",
  "container": "log-demo"
}
```

**Check:**

- `curl -s http://localhost:3100/loki/api/v1/labels | jq -r '.data[]'` lists `pod` and `namespace`.
- If the Exercise 1 query returns `null`, wait 15 seconds and run it again.

### Exercise 2: Parse and filter with LogQL

**Goal:** extract JSON fields at query time and aggregate them. Run these in Grafana
(Explore, Loki data source) against the pod from Exercise 1.

```logql
{pod="log-demo"} | json | level=~"warn|error"
sum by (namespace) (count_over_time({namespace=~".+"} | json | level="info" [5m]))
```

Expected output: the first query returns no lines (the pod only writes `info`). The second
returns one series for `default` with a count that grows by about 30 every minute.

**Check:**

- Change `level="info"` to `level="error"` in the second query. The result is empty.

### Exercise 3: Break and fix ingestion

**Goal:** see how a new label value becomes a new stream and why cardinality matters.

```bash
kubectl delete pod log-demo
kubectl run log-demo-2 -n default --labels app=log-demo-2 --image=busybox --restart=Never -- \
  sh -c 'while true; do echo "{\"level\":\"warn\",\"msg\":\"again\"}"; sleep 2; done'
sleep 20
curl -G -s http://localhost:3100/loki/api/v1/query_range \
  --data-urlencode 'query={pod="log-demo-2"} | json | level="warn"' | jq '.data.result | length'
```

Expected output: `1`.

**Check:**

- The stream appears within one scrape interval (the query above prints `1`).
- You can explain why a high-cardinality label (for example, a request ID) would create one
  stream per request and hurt Loki, and how you would move that value into the log line instead.

Clean up with `kubectl delete pod log-demo-2; kill %1`.

---

## Troubleshooting

### Loki is not receiving logs

**Cause:** the collector cannot reach Loki, or Loki rejects the push.

**Fix:** check each hop in order.

```bash
# Check Alloy/Vector logs
kubectl logs -n monitoring -l app.kubernetes.io/name=alloy
kubectl logs -n monitoring -l app.kubernetes.io/name=vector

# Check Loki logs
kubectl logs -n monitoring -l app.kubernetes.io/name=loki

# Test Loki endpoint
curl -X POST http://loki.monitoring.svc:3100/loki/api/v1/push \
  -H "Content-Type: application/json" \
  -d '{"streams":[{"stream":{"job":"test"},"values":[["'$(date +%s%N)'","test message"]]}]}'
```

### Loki memory usage is high

**Cause:** too many streams (high-cardinality labels) or oversized chunks.

**Fix:** check usage, then tune limits.

```bash
# Check Loki memory
kubectl top pods -n monitoring -l app.kubernetes.io/name=loki

# Adjust limits
# Reduce ingestion_rate_mb, per_stream_rate_limit
# Increase chunk_target_size
```

### Queries are slow

**Cause:** wide time ranges, no caching, or unselective stream selectors.

**Fix:** tune as follows.

```bash
# Enable query caching
# In Loki config:
# query_cache:
#   enabled: true

# Use recording rules for common queries
# Reduce max_query_parallelism
```

## Further Reading

- [Loki Documentation](https://grafana.com/docs/loki/)
- [Vector Documentation](https://vector.dev/docs/)
- [LogQL Reference](https://grafana.com/docs/loki/latest/logql/)
- [Grafana Loki Dashboards](https://grafana.com/grafana/dashboards/?dataSource=loki)

---

## Next Steps

1. **Read**: [Tracing Stack](./tracing-stack.md). Link log lines to traces with `trace_id`.
2. **Read**: [Alerting](./alerting.md). Route Loki ruler alerts through Alertmanager.
3. **Practice**: [Lab 06: Observability Stack](../../labs/lab-06-observability-stack/)
