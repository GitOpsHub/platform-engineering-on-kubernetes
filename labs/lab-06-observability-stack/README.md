# Lab 06: Observability Stack - Logs, Metrics, Traces, and Correlation

## Objectives
- Install a laptop-sized observability stack on the Lab 01 cluster: **Loki** (logs), **Tempo** (traces),
  **Prometheus** with a **Thanos sidecar** (metrics), an **OpenTelemetry Collector** (OTLP fan-out) and **Grafana Alloy** (pod logs)
- Instrument a sample Python service (`checkout-api`) with Prometheus histograms that carry **trace_id exemplars**,
  JSON logs with `trace_id`/`span_id`, and OTLP traces
- Wire Grafana datasources so you can jump **metrics -> traces -> logs** and back
- Understand what the Thanos sidecar does, and what it does not do without object storage

Background reading: [Logging stack](../../docs/10-observability/logging-stack.md),
[Metrics stack](../../docs/10-observability/metrics-stack.md),
[Tracing stack](../../docs/10-observability/tracing-stack.md),
[Alerting](../../docs/10-observability/alerting.md),
[SLOs and SLIs](../../docs/11-reliability/slos-slis.md).

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Docker Desktop / Docker Engine | 25+ | **8 GB RAM minimum, 10 GB comfortable**, 4 CPUs (see [Laptop sizing](#laptop-sizing)) |
| kubectl | 1.33+ | Within one minor version of the cluster (1.35) |
| Helm | 3.12+ (Helm 4 works too) | Charts are pulled with `--repo`, no `helm repo add` needed |
| kind | 0.33+ | Used to load the sample app image into the cluster |
| Python | 3.12 | Only if you want to run `app.py` outside the cluster (optional) |
| [Lab 01 done](../lab-01-cluster-setup/README.md) | - | kind cluster `platform-engineering` (context `kind-platform-engineering`), Cilium, Traefik, cert-manager, and the `platform-monitoring` namespace |

## Estimated Time: 45-60 minutes
(Most of it is image pulls and Prometheus Operator CRD installation on the first run.)

---

## Files in this lab

| File | Purpose |
|------|---------|
| [`loki-stack.yaml`](loki-stack.yaml) | Loki values: single binary, filesystem storage on a 5 Gi PVC |
| [`tempo-stack.yaml`](tempo-stack.yaml) | Tempo values: single binary, OTLP on 4317/4318, span metrics to Prometheus |
| [`otel-collector.yaml`](otel-collector.yaml) | Collector values: OTLP in; traces to Tempo, logs to Loki, metrics to Prometheus |
| [`alloy-logs.yaml`](alloy-logs.yaml) | Alloy values: ships pod logs from the Kubernetes API to Loki |
| [`prometheus-stack.yaml`](prometheus-stack.yaml) | kube-prometheus-stack values: Prometheus, Thanos sidecar, Grafana |
| [`apps/checkout-api/`](apps/checkout-api/) | Sample instrumented service, load generator, Dockerfile, Kubernetes manifests |
| [`grafana/datasources.yaml`](grafana/datasources.yaml) | Grafana datasource provisioning (Prometheus, Loki, Tempo) with correlation settings |
| [`grafana/datasources-configmap.yaml`](grafana/datasources-configmap.yaml) | Same datasources as a ConfigMap for the Grafana sidecar |
| [`setup.sh`](setup.sh) | Installs everything in order (idempotent) |
| [`teardown.sh`](teardown.sh) | Removes the app, datasources and all releases. Keeps the `platform-monitoring` namespace (Lab 01) |

### Pinned versions

| Component | Version | Installed by |
|-----------|---------|--------------|
| Loki chart 7.3.0 (Loki 3.6.12) | single binary | Helm, `loki-stack.yaml` |
| Grafana Alloy chart 1.12.1 (Alloy v1.19.2) | Deployment | Helm, `alloy-logs.yaml` |
| Tempo chart 3.2.1 from `grafana-community` (Tempo 3.1.1) | single binary | Helm, `tempo-stack.yaml` |
| OpenTelemetry Collector chart 0.173.1 (collector 0.160.0) | deployment mode | Helm, `otel-collector.yaml` |
| kube-prometheus-stack chart 91.4.1 | Prometheus, Operator, Grafana | Helm, `prometheus-stack.yaml` |
| Thanos v0.42.4 | sidecar image | `prometheus-stack.yaml` |
| checkout-api Python deps | prometheus-client 0.26.0, opentelemetry 1.45.1 | `apps/checkout-api/requirements.txt` |
| uv 0.13.5 | builds the app image | `apps/checkout-api/Dockerfile` |

Chart versions live at the top of `setup.sh`; change them there, and the values files if a chart's schema changes.

> **Tempo chart migration.** The earlier `grafana/tempo` 1.24.4 chart is marked `deprecated: true` upstream, so
> this lab uses its successor, `grafana-community/tempo` 3.2.1 (Tempo 3.1.1). The successor has a different
> values schema. `tempo-stack.yaml` was rewritten for it: receivers, retention and the metrics generator live under
> `tempo:`, and the chart's own config template builds the Tempo config from them. The values render cleanly with
> `helm template`. Live behaviour (span metrics reaching Prometheus, exemplars) is still a step to check on your cluster.

---

## Architecture

```
checkout-api (checkout ns)
  |- /metrics  --scrape (ServiceMonitor)-------------> Prometheus (+ Thanos sidecar)
  |- OTLP traces -> otel-collector -> Tempo ----------+
  |- stdout JSON logs -> Alloy -> Loki                |
                                                      v
                              Grafana: metrics -> exemplar trace_id -> Tempo
                                       logs -> derived field trace_id -> Tempo
                                       traces -> tracesToLogsV2 -> Loki, tracesToMetrics -> Prometheus
```

Logs take the Alloy path. The collector has a logs pipeline too, for apps that send OTLP logs natively.

---

## Steps

### Step 1 - Check the cluster

```bash
kubectl config current-context    # kind-platform-engineering
kubectl get nodes                 # 3 nodes, Ready
kubectl get ns platform-monitoring  # created in Lab 01 (privileged PSA)
```

### Step 2 - Install everything

```bash
cd labs/lab-06-observability-stack
./setup.sh                  # full install
# or ./setup.sh --skip-app  # observability stack only
```

The script installs in this order: Loki, Tempo, OpenTelemetry Collector, Alloy, kube-prometheus-stack, then
builds `checkout-api:0.1.0`, loads it into kind, applies the app, and applies the Grafana datasource ConfigMap.

The rest of this section walks through what the script does so you can run each step by hand.

### Step 3 - Loki and Tempo

```bash
helm upgrade --install loki loki --repo https://grafana.github.io/helm-charts --version 7.3.0 \
  --namespace platform-monitoring -f loki-stack.yaml --wait

helm upgrade --install tempo tempo --repo https://grafana-community.github.io/helm-charts --version 3.2.1 \
  --namespace platform-monitoring -f tempo-stack.yaml --wait
```

Loki runs in `SingleBinary` mode and keeps its chunks on a PVC (`loki-stack.yaml`). The gateway, caches and canary
are off, which keeps it small. Tempo listens for OTLP on `4317` (gRPC) and `4318` (HTTP).

### Step 4 - OpenTelemetry Collector

```bash
helm upgrade --install otel-collector opentelemetry-collector \
  --repo https://open-telemetry.github.io/opentelemetry-helm-charts --version 0.173.1 \
  --namespace platform-monitoring -f otel-collector.yaml --wait
```

[`otel-collector.yaml`](otel-collector.yaml) defines one receiver (`otlp`) and three pipelines, each with
`memory_limiter` and `batch` processors in front:

| Pipeline | Exporter | Destination |
|----------|----------|-------------|
| traces | `otlp_grpc/tempo` | Tempo `:4317` |
| logs | `otlp_http/loki` | Loki `/otlp` (native OTLP ingestion) |
| metrics | `prometheusremotewrite` | Prometheus `/api/v1/write` (remote write receiver enabled) |

### Step 5 - Grafana Alloy (pod logs)

```bash
helm upgrade --install alloy alloy --repo https://grafana.github.io/helm-charts --version 1.12.1 \
  --namespace platform-monitoring -f alloy-logs.yaml --wait
```

Alloy reads pod logs through the Kubernetes API (`loki.source.kubernetes`) and pushes them to Loki, labelled
with `namespace`, `pod`, `container` and `app`. It runs as one Deployment, so it needs no host log mounts.

### Step 6 - kube-prometheus-stack, Thanos sidecar and Grafana

```bash
helm upgrade --install kps kube-prometheus-stack \
  --repo https://prometheus-community.github.io/helm-charts --version 91.4.1 \
  --namespace platform-monitoring -f prometheus-stack.yaml --wait
```

- **Grafana is enabled** and is the UI for all signals. It loads datasources from
  `grafana/datasources-configmap.yaml` via the sidecar (label `grafana_datasource: "1"`). The chart's built-in
  Prometheus datasource is disabled so the uid `prometheus` is not defined twice.
- **Alertmanager is disabled.** Alert routing is covered in [Alerting](../../docs/10-observability/alerting.md).
- **Retention is 12h** and the Prometheus volume is 5 Gi.
- **Thanos sidecar (`prometheusSpec.thanos`) is sidecar-only.** It runs next to Prometheus and serves the Thanos
  StoreAPI. It does **not** ship blocks to object storage in this lab. Uploading needs a bucket and a Secret,
  which are not deployed here. The commented `objectStorageConfig` block in the values file shows where the
  Secret would be referenced. Without it, long-term retention and cross-cluster queries are not illustrated.
- `kubeEtcd`, `kubeControllerManager`, `kubeScheduler` and `kubeProxy` scrapers are off because kind does not
  expose them, and Cilium replaces kube-proxy.

### Step 7 - Datasource ConfigMap

```bash
kubectl apply -f grafana/datasources-configmap.yaml
```

[`grafana/datasources.yaml`](grafana/datasources.yaml) is the source of truth. The ConfigMap embeds the same content
for the sidecar. It sets up:

| Datasource (uid) | Correlation setting |
|------------------|---------------------|
| Prometheus (`prometheus`, default) | `exemplarTraceIdDestinations` `trace_id` -> Tempo (**metrics -> traces**) |
| Loki (`loki`) | `derivedFields` matching `"trace_id":"..."` in the JSON line -> Tempo (**logs -> traces**) |
| Tempo (`tempo`) | `tracesToLogsV2` -> Loki, `tracesToMetrics` -> Prometheus, `serviceMap` and `nodeGraph` on (**traces -> logs and metrics**) |

### Step 8 - Sample app

```bash
docker build -t checkout-api:0.1.0 apps/checkout-api
kind load docker-image checkout-api:0.1.0 --name platform-engineering
kubectl apply -f apps/checkout-api/k8s/00-namespace.yaml
kubectl apply -f apps/checkout-api/k8s/
```

[`app.py`](apps/checkout-api/app.py) serves:

| Route | Behaviour |
|-------|-----------|
| `GET /checkout` | Opens a `checkout` span with a `charge-payment` child, takes 10-300 ms, fails about 5% of the time (HTTP 500) |
| `GET /metrics` | OpenMetrics output. `http_request_duration_seconds` histogram with `trace_id` exemplars |
| `GET /healthz` | Liveness and readiness |

Every log line is JSON with `trace_id` and `span_id` from the active span. The ServiceMonitor
([`30-servicemonitor.yaml`](apps/checkout-api/k8s/30-servicemonitor.yaml)) makes Prometheus scrape the app. The
load generator Job ([`40-loadgen-job.yaml`](apps/checkout-api/k8s/40-loadgen-job.yaml)) sends about 5 requests per
second for 5 minutes.

---

## Verification

```bash
kubectl -n platform-monitoring get pods       # all Running/Ready (loki-0, tempo-0, alloy, otel-collector, kps-*)
kubectl -n checkout get pods                  # checkout-api Running, checkout-loadgen Completed or Running
```

**Grafana**

```bash
kubectl -n platform-monitoring port-forward svc/kps-grafana 3000:80
kubectl -n platform-monitoring get secret kps-grafana -o jsonpath='{.data.admin-password}' | base64 -d; echo
# http://localhost:3000, user admin, password from the command above
```

Open **Connections > Data sources**, then **Save & test** on Prometheus, Loki and Tempo. All three must pass.

**Metrics and exemplars**

```bash
kubectl -n platform-monitoring port-forward svc/kps-kube-prometheus-stack-prometheus 9090:9090 &
curl -s 'http://localhost:9090/api/v1/query?query=sum(rate(http_request_duration_seconds_count[5m]))'
curl -s 'http://localhost:9090/api/v1/query_exemplars?query=http_request_duration_seconds_bucket' | head -c 600
```

The exemplar query should return `trace_id` labels. Graph the histogram in Grafana Explore, and the
exemplar dots link to traces.

**Logs -> traces**

In Grafana Explore, pick Loki and run `{namespace="checkout"} | json`. Each line with a `trace_id` shows a
**View trace** link (derived field).

**Traces -> logs and metrics**

Explore, pick Tempo, search `service.name = checkout-api`. Open a trace, then use **Logs for this span**
(Loki) and the metrics links (Prometheus).

**Thanos sidecar**

```bash
kubectl -n platform-monitoring logs -l app.kubernetes.io/name=prometheus -c thanos-sidecar --tail=20
```

Expect startup messages and no object storage errors, because no `objstore` config is set. The sidecar
only serves local data.

---

## Cleanup

```bash
./teardown.sh
```

This removes the `checkout` namespace, the Grafana ConfigMap, and the kps, alloy, otel-collector, tempo and loki
releases. Helm keeps StatefulSet PVCs, so the script also deletes the Loki, Tempo and Prometheus volumes by name
(`storage-loki-0`, `storage-tempo-0`, `prometheus-kps-kube-prometheus-stack-prometheus-db-*`).

> **The `platform-monitoring` namespace is kept on purpose.** Lab 01 creates it, and other labs use it. `teardown.sh`
> prints this at the start and at the end. It never deletes that namespace, and it never deletes the kind cluster.
> To remove the namespace as well, run `kubectl delete namespace platform-monitoring` yourself, and only after you
> have confirmed nothing else in it is needed.

Helm does not remove CRDs, so the Prometheus Operator CRDs stay. Remove them only if you do not need them:
`kubectl get crd | grep monitoring.coreos.com`, then delete the listed CRDs.

---

## Troubleshooting

### Helm fails with `metadata.annotations: Too long` on kube-prometheus-stack
The Prometheus Operator CRDs are large. Apply them with server-side apply from the chart's `crds` directory
(`helm pull prometheus-community/kube-prometheus-stack --version 91.4.1 --untar`) and retry the install.

### Pods `Pending` or `OOMKilled`
Docker has too little memory. Raise it in Docker Desktop > Settings > Resources to 8 GB or more, then
`kubectl get events -n platform-monitoring --sort-by=.lastTimestamp | tail -20`.

### Grafana Save & test fails for Tempo or Loki
The datasource URL is a Service DNS name. Check it exists:
```bash
kubectl -n platform-monitoring get svc loki tempo prometheus-operated
```

### No traces in Tempo
```bash
kubectl -n platform-monitoring logs deploy/otel-collector | tail -30    # exporter errors?
kubectl -n checkout logs deploy/checkout-api | tail -5                 # "listening on :8080, exporting traces..."
```
The app sends to `otel-collector.platform-monitoring.svc:4317`, which is set in `10-deployment.yaml`.

### No logs in Loki
```bash
kubectl -n platform-monitoring logs deploy/alloy | tail -30            # RBAC or push errors?
```
Alloy needs `pods/log` access. The chart creates it.

### Metrics have no exemplars
Confirm the app emits them, then confirm Prometheus stores them:
```bash
kubectl -n checkout port-forward svc/checkout-api 8081:8080 &
curl -s localhost:8081/metrics | grep 'trace_id' | head -2
```
If the app shows exemplars and Prometheus does not, check the Prometheus version the operator runs. Exemplar
storage behaviour differs between Prometheus 2.x and 3.x, so compare against the
[feature flags page](https://prometheus.io/docs/prometheus/latest/feature_flags/#exemplars-storage) for that version.

### Thanos sidecar in `CrashLoopBackOff`
Check the Thanos version against the Prometheus version the operator deploys:
`kubectl -n platform-monitoring logs <prometheus-pod> -c thanos-sidecar`. Adjust `thanos.version` in
`prometheus-stack.yaml`.

### `kind load docker-image` fails
The cluster name must be `platform-engineering`. Check with `kind get clusters`.

### Load generator Job already finished
Jobs are immutable. Delete and re-apply:
```bash
kubectl -n checkout delete job checkout-loadgen --ignore-not-found
kubectl apply -f apps/checkout-api/k8s/40-loadgen-job.yaml
```

---

## Next Steps

1. **Read**: [SLOs and SLIs](../../docs/11-reliability/slos-slis.md). Build an availability SLO from the
   `http_request_duration_seconds` histogram.
2. **Read**: [Alerting](../../docs/10-observability/alerting.md). Add Alertmanager and a burn-rate rule.
3. **Expose Grafana** through Traefik with an Ingress and a `*.localtest.me` host, as in Lab 01.
4. **Object storage**: add MinIO or a cloud bucket, then enable `objectStorageConfig` on the Thanos sidecar
   (or Loki and Tempo storage) to see real long-term retention.
5. **Lab 07**: [Multi-Tenancy](../lab-07-multi-tenancy/) (see the lab directory)

---

## Laptop sizing

Approximate memory limits on the cluster (requests are about half of these):

| Workload | Memory limit |
|----------|--------------|
| Prometheus (+ Thanos sidecar) | 1 GiB + 256 MiB |
| Loki | 512 MiB |
| Tempo | 512 MiB |
| Grafana | 256 MiB |
| OpenTelemetry Collector | 256 MiB |
| Alloy | 256 MiB |
| Prometheus Operator, kube-state-metrics, node-exporter | about 350 MiB |
| checkout-api + load generator | about 200 MiB |

That is about 3.6 GiB of limits on top of the Lab 01 platform and the kind nodes. Plan on **8 GB of Docker
memory minimum, 10 GB comfortable**, and 4 CPUs. Keep the Lab 01 cluster and this lab only. Run
`./teardown.sh` before starting heavier labs.

---

## Resources

- [Grafana Loki Helm chart](https://github.com/grafana/helm-charts/tree/main/charts/loki)
- [Grafana Tempo documentation](https://grafana.com/docs/tempo/latest/)
- [Grafana Alloy](https://grafana.com/docs/alloy/latest/)
- [OpenTelemetry Collector Helm chart](https://github.com/open-telemetry/opentelemetry-helm-charts/tree/main/charts/opentelemetry-collector)
- [kube-prometheus-stack chart](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
- [Prometheus Operator: ThanosSpec](https://prometheus-operator.dev/docs/api-reference/api/)
- [Thanos sidecar](https://thanos.io/tip/components/sidecar.md/)
- [Grafana datasource provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/#data-sources)
- [Prometheus exemplars](https://prometheus.io/docs/prometheus/latest/feature_flags/#exemplars-storage)
