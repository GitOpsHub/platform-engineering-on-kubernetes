package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCountersAndHistogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := New(reg)

	m.ObserveDecision("Workload", DecisionDeny, 2*time.Millisecond)
	m.ObserveDecision("Workload", DecisionDeny, 3*time.Millisecond)
	m.ObserveDecision("Workload", DecisionAllow, time.Millisecond)
	m.ObserveViolation("registry-allowlist")

	tests := []struct {
		name string
		got  func() float64
		want float64
	}{
		{"deny count", func() float64 { return testutil.ToFloat64(m.decisions.WithLabelValues("Workload", DecisionDeny)) }, 2},
		{"allow count", func() float64 { return testutil.ToFloat64(m.decisions.WithLabelValues("Workload", DecisionAllow)) }, 1},
		{"violation count", func() float64 { return testutil.ToFloat64(m.violations.WithLabelValues("registry-allowlist")) }, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.got(); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}

	n, err := testutil.GatherAndCount(reg, "policy_engine_evaluation_duration_seconds")
	if err != nil || n != 1 {
		t.Fatalf("histogram series = %d, %v", n, err)
	}
}
