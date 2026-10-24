// Package metrics exposes Prometheus metrics for policy evaluation.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Decision values for the decision label.
const (
	DecisionAllow = "allow"
	DecisionDeny  = "deny"
	DecisionError = "error"
)

// Metrics records admission decisions and rule violations.
type Metrics struct {
	decisions  *prometheus.CounterVec
	violations *prometheus.CounterVec
	latency    *prometheus.HistogramVec
}

// New registers the metrics with reg.
func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		decisions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "policy_engine_decisions_total",
			Help: "Admission decisions by Kubernetes kind and outcome.",
		}, []string{"kind", "decision"}),
		violations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "policy_engine_violations_total",
			Help: "Policy rule violations by policy name.",
		}, []string{"policy"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "policy_engine_evaluation_duration_seconds",
			Help:    "Time spent evaluating policies per admission request.",
			Buckets: []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		}, []string{"kind"}),
	}
	reg.MustRegister(m.decisions, m.violations, m.latency)
	return m
}

// ObserveDecision counts one decision and records how long it took.
func (m *Metrics) ObserveDecision(kind, decision string, d time.Duration) {
	m.decisions.WithLabelValues(kind, decision).Inc()
	m.latency.WithLabelValues(kind).Observe(d.Seconds())
}

// ObserveViolation counts one violation of the named policy.
func (m *Metrics) ObserveViolation(policy string) {
	m.violations.WithLabelValues(policy).Inc()
}
