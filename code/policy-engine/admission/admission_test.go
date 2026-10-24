package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/prometheus/client_golang/prometheus"

	engine "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/policy-engine"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/policy-engine/metrics"
)

// fakeEngine returns canned violations or an error and records the request it saw.
type fakeEngine struct {
	violations []engine.Violation
	err        error
	seen       engine.Request
}

func (f *fakeEngine) Evaluate(_ context.Context, req engine.Request) ([]engine.Violation, error) {
	f.seen = req
	return f.violations, f.err
}

func admissionRequest(op admissionv1.Operation, kind string, raw string) *admissionv1.AdmissionRequest {
	return &admissionv1.AdmissionRequest{
		UID:       "uid-1",
		Kind:      metav1.GroupVersionKind{Group: "platform.mycompany.io", Version: "v1alpha1", Kind: kind},
		Namespace: "payments",
		Name:      "checkout",
		Operation: op,
		Object:    runtime.RawExtension{Raw: []byte(raw)},
	}
}

const workloadJSON = `{"apiVersion":"platform.mycompany.io/v1alpha1","kind":"Workload","metadata":{"name":"checkout"},"spec":{"image":"ghcr.io/acme/checkout:1"}}`

func TestReview(t *testing.T) {
	tests := []struct {
		name        string
		eng         *fakeEngine
		req         *admissionv1.AdmissionRequest
		wantAllowed bool
		wantCode    int32
		wantMsg     string
		wantEngine  bool // whether the engine should have been called
	}{
		{"compliant object allowed", &fakeEngine{}, admissionRequest(admissionv1.Create, "Workload", workloadJSON), true, 0, "", true},
		{"violations denied with messages", &fakeEngine{violations: []engine.Violation{
			{Policy: "registry-allowlist", Message: "bad image"}, {Policy: "no-privileged", Message: "privileged"},
		}}, admissionRequest(admissionv1.Create, "Workload", workloadJSON), false, 403, "bad image; privileged", true},
		{"engine error fails closed", &fakeEngine{err: errors.New("boom")},
			admissionRequest(admissionv1.Create, "Workload", workloadJSON), false, 500, "request denied", true},
		{"undecodable object denied", &fakeEngine{}, admissionRequest(admissionv1.Create, "Workload", `{not json`), false, 400, "decoding object", false},
		{"empty object denied", &fakeEngine{}, admissionRequest(admissionv1.Create, "Workload", ``), false, 400, "no object", false},
		{"delete always allowed", &fakeEngine{violations: []engine.Violation{{Policy: "x", Message: "y"}}},
			admissionRequest(admissionv1.Delete, "Workload", ``), true, 0, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{Engine: tc.eng, Metrics: metrics.New(prometheus.NewRegistry())}
			resp := h.Review(context.Background(), tc.req)
			if resp.UID != "uid-1" {
				t.Fatalf("response must echo the request UID, got %q", resp.UID)
			}
			if resp.Allowed != tc.wantAllowed {
				t.Fatalf("allowed = %v, want %v (result %+v)", resp.Allowed, tc.wantAllowed, resp.Result)
			}
			if tc.wantCode != 0 && (resp.Result == nil || resp.Result.Code != tc.wantCode) {
				t.Fatalf("result = %+v, want code %d", resp.Result, tc.wantCode)
			}
			if tc.wantMsg != "" && (resp.Result == nil || !strings.Contains(resp.Result.Message, tc.wantMsg)) {
				t.Fatalf("message = %+v, want containing %q", resp.Result, tc.wantMsg)
			}
			if tc.wantEngine && tc.eng.seen.Kind != "Workload" {
				t.Fatalf("engine not called with the object kind: %+v", tc.eng.seen)
			}
			if tc.wantEngine && tc.eng.seen.Name != "checkout" {
				t.Fatalf("engine not called with the object name: %+v", tc.eng.seen)
			}
		})
	}
}

func TestServeHTTPRoundTrip(t *testing.T) {
	review := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "admission.k8s.io/v1", Kind: "AdmissionReview"},
		Request:  admissionRequest(admissionv1.Create, "Workload", workloadJSON),
	}
	body, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{Engine: &fakeEngine{violations: []engine.Violation{{Policy: "p", Message: "nope"}}}}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body)
	}
	var out admissionv1.AdmissionReview
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Response == nil || out.Response.UID != "uid-1" || out.Response.Allowed {
		t.Fatalf("unexpected response: %+v", out.Response)
	}
	if out.APIVersion != "admission.k8s.io/v1" || out.Kind != "AdmissionReview" {
		t.Fatalf("response must be a v1 AdmissionReview, got %s %s", out.APIVersion, out.Kind)
	}
}

func TestServeHTTPRejectsBadInput(t *testing.T) {
	h := &Handler{Engine: &fakeEngine{}}
	tests := []struct {
		name   string
		method string
		body   string
		want   int
	}{
		{"get not allowed", http.MethodGet, "", http.StatusMethodNotAllowed},
		{"bad json", http.MethodPost, "{", http.StatusBadRequest},
		{"no request", http.MethodPost, `{"apiVersion":"admission.k8s.io/v1","kind":"AdmissionReview"}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest(tc.method, "/validate", strings.NewReader(tc.body)))
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d", rr.Code, tc.want)
			}
		})
	}
}
