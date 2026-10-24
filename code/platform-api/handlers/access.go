package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"

	mw "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/middleware"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/types"
)

// selfSubjectAccessReviewGVR is evaluated by the API server as the impersonated caller.
var selfSubjectAccessReviewGVR = schema.GroupVersionResource{
	Group: "authorization.k8s.io", Version: "v1", Resource: "selfsubjectaccessreviews",
}

// accessReview answers "can the caller do this?" by creating a
// SelfSubjectAccessReview with the caller's impersonated client. The answer
// therefore comes from Kubernetes RBAC for that user, not from platform-api.
func (a *API) accessReview(w http.ResponseWriter, r *http.Request) {
	client, err := a.clientFor(r)
	if err != nil {
		writeClientError(w, r, err)
		return
	}
	var in types.AccessReviewRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, mw.MaxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		mw.WriteError(w, r, http.StatusBadRequest, "BadRequest", "invalid JSON: "+err.Error())
		return
	}
	if err := in.Validate(); err != nil {
		mw.WriteError(w, r, http.StatusUnprocessableEntity, "Invalid", err.Error())
		return
	}

	review := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{"resourceAttributes": map[string]any{
			"verb":      in.Verb,
			"group":     in.Group,
			"resource":  in.Resource,
			"namespace": in.Namespace,
			"name":      in.Name,
		}},
	}}
	review.SetAPIVersion("authorization.k8s.io/v1")
	review.SetKind("SelfSubjectAccessReview")

	created, err := client.Resource(selfSubjectAccessReviewGVR).Create(r.Context(), review, metav1.CreateOptions{})
	if err != nil {
		writeKubeError(w, r, err)
		return
	}
	allowed, _, _ := unstructured.NestedBool(created.Object, "status", "allowed")
	denied, _, _ := unstructured.NestedBool(created.Object, "status", "denied")
	reason, _, _ := unstructured.NestedString(created.Object, "status", "reason")
	mw.WriteJSON(w, http.StatusOK, types.AccessReviewResult{Allowed: allowed, Denied: denied, Reason: reason})
}

// watch streams changes to a list as newline-delimited JSON events. The stream
// is opened as the caller, so the user only receives objects RBAC lets them see.
// Each line is {"type": "ADDED|MODIFIED|DELETED", "object": {...}}.
func (rs *resource[T, PT]) watch(w http.ResponseWriter, r *http.Request, namespace string) {
	client, ok := rs.client(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	wi, err := rs.ri(client, namespace).Watch(r.Context(), metav1.ListOptions{LabelSelector: q.Get("labelSelector"), ResourceVersion: q.Get("resourceVersion")})
	if err != nil {
		writeKubeError(w, r, err)
		return
	}
	defer wi.Stop()

	rc := http.NewResponseController(w)
	// The server's WriteTimeout would cut long-lived streams; lift it for this response.
	_ = rc.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	_ = rc.Flush()

	enc := json.NewEncoder(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-wi.ResultChan():
			if !open || ev.Type == watch.Error {
				return // the client re-lists and watches again
			}
			u, ok := ev.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}
			obj, err := fromUnstructured[T](u)
			if err != nil {
				return
			}
			if err := enc.Encode(types.WatchEvent[T]{Type: string(ev.Type), Object: obj}); err != nil {
				return
			}
			_ = rc.Flush()
		}
	}
}

// writeClientError maps a clientFor failure to a status code.
func writeClientError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errUnauthenticated):
		mw.WriteError(w, r, http.StatusUnauthorized, "Unauthorized", err.Error())
	case errors.Is(err, ErrForbiddenPrincipal):
		mw.WriteError(w, r, http.StatusForbidden, "Forbidden", err.Error())
	default:
		mw.WriteError(w, r, http.StatusInternalServerError, "InternalError", fmt.Sprintf("%v", err))
	}
}
