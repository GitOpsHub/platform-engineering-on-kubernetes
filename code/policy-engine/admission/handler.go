// Package admission serves a Kubernetes ValidatingWebhook that evaluates
// platform policies against objects before they are persisted.
package admission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	engine "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/policy-engine"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/policy-engine/metrics"
)

// MaxRequestBytes bounds an AdmissionReview body.
const MaxRequestBytes = 3 << 20 // 3 MiB; large enough for typical CRs

// Evaluator is the policy engine's evaluation contract.
type Evaluator interface {
	Evaluate(ctx context.Context, req engine.Request) ([]engine.Violation, error)
}

// Handler answers AdmissionReview requests. It fails closed: a policy error or
// an undecodable object is denied rather than allowed through.
type Handler struct {
	Engine  Evaluator
	Metrics *metrics.Metrics // optional
	Logger  *slog.Logger     // optional; slog.Default() when nil
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxRequestBytes))
	if err != nil {
		http.Error(w, "reading request body: "+err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	var review admissionv1.AdmissionReview
	if err := json.Unmarshal(body, &review); err != nil {
		http.Error(w, "decoding AdmissionReview: "+err.Error(), http.StatusBadRequest)
		return
	}
	if review.Request == nil {
		http.Error(w, "AdmissionReview has no request", http.StatusBadRequest)
		return
	}
	resp := h.Review(r.Context(), review.Request)
	out := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: admissionv1.SchemeGroupVersion.String(), Kind: "AdmissionReview"},
		Response: resp,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger().Error("writing AdmissionReview", "error", err)
	}
}

// Review decides one admission request. It is exported for direct testing.
func (h *Handler) Review(ctx context.Context, req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	start := time.Now()
	kind := req.Kind.Kind
	resp := &admissionv1.AdmissionResponse{UID: req.UID, Allowed: true}

	// DELETE carries the object in OldObject and has nothing new to validate.
	if req.Operation == admissionv1.Delete {
		h.observe(kind, metrics.DecisionAllow, start)
		return resp
	}

	obj, err := decodeObject(req.Object.Raw)
	if err != nil {
		h.observe(kind, metrics.DecisionError, start)
		return deny(resp, http.StatusBadRequest, "BadRequest", err.Error())
	}
	name := req.Name
	if name == "" {
		name, _ = obj["metadata"].(map[string]any)["name"].(string)
	}
	violations, err := h.Engine.Evaluate(ctx, engine.Request{
		Operation: string(req.Operation),
		Kind:      kind,
		Namespace: req.Namespace,
		Name:      name,
		Object:    obj,
	})
	if err != nil {
		h.logger().Error("policy evaluation failed", "kind", kind, "name", name, "error", err)
		h.observe(kind, metrics.DecisionError, start)
		return deny(resp, http.StatusInternalServerError, "InternalError", "policy evaluation failed; request denied")
	}
	if len(violations) == 0 {
		h.observe(kind, metrics.DecisionAllow, start)
		return resp
	}
	msgs := make([]string, 0, len(violations))
	for _, v := range violations {
		msgs = append(msgs, v.Message)
		if h.Metrics != nil {
			h.Metrics.ObserveViolation(v.Policy)
		}
	}
	h.logger().Info("admission denied", "kind", kind, "name", name, "violations", len(violations))
	h.observe(kind, metrics.DecisionDeny, start)
	return deny(resp, http.StatusForbidden, "Forbidden", strings.Join(msgs, "; "))
}

func (h *Handler) observe(kind, decision string, start time.Time) {
	if h.Metrics != nil {
		h.Metrics.ObserveDecision(kind, decision, time.Since(start))
	}
}

func (h *Handler) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

func deny(resp *admissionv1.AdmissionResponse, code int32, reason, msg string) *admissionv1.AdmissionResponse {
	resp.Allowed = false
	resp.Result = &metav1.Status{
		Status:  metav1.StatusFailure,
		Code:    code,
		Reason:  metav1.StatusReason(reason),
		Message: msg,
	}
	return resp
}

func decodeObject(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, errors.New("request carries no object")
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("decoding object: %w", err)
	}
	if obj == nil {
		return nil, errors.New("object is null")
	}
	return obj, nil
}
