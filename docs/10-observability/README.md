# 10 - Observability

How to build observability as a self-service platform capability: collect metrics, logs, traces, and profiles, correlate them in Grafana, and turn them into actionable alerts.

## Documents

| Document | What it covers |
|----------|----------------|
| [Metrics Stack](./metrics-stack.md) | Prometheus data model, kube-prometheus-stack, Operator CRDs, recording rules, and scaling with Thanos, Mimir, or VictoriaMetrics |
| [Logging Stack](./logging-stack.md) | Loki for aggregation, Vector/Fluent Bit for collection, parsing, enrichment, and log-based alerts |
| [Tracing Stack](./tracing-stack.md) | OpenTelemetry Operator and Collectors, auto-instrumentation, tail sampling, Tempo/TraceQL, and signal correlation |
| [Profiling](./profiling.md) | Continuous profiling with Pyroscope and Alloy (eBPF and SDKs), flame-graph diffs, and span profiles |
| [Alerting](./alerting.md) | Symptom-based alert design, Alertmanager routing, inhibition, silences, and tenant `AlertmanagerConfig` |

## Reading Order

Read the pages in this order: Metrics Stack, Logging Stack, Tracing Stack, Profiling, then Alerting.
The diagram shows how the signals feed alerting and SLOs.

```mermaid
graph LR
    M[Metrics] --> A[Alerting]
    L[Logs] --> A
    T[Traces] --> M
    T --> P[Profiles]
    A --> SLO["11 - Reliability: SLOs"]
```

## Related Labs

- [Lab 06: Observability Stack](../../labs/lab-06-observability-stack/): Loki, Tempo, Prometheus,
  OpenTelemetry Collector, and Grafana Alloy. It does not install Alertmanager.

## Where This Leads

- Next section: [11 - Reliability](../11-reliability/README.md)
- Security of privileged agents: [Pod Security](../12-security-compliance/pod-security.md)
