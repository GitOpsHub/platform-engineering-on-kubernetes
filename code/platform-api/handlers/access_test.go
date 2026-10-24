package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	mw "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/middleware"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/types"
)

func TestSystemIdentitiesAreForbidden(t *testing.T) {
	tests := []struct {
		name   string
		p      *mw.Principal
		method string
		path   string
		body   string
	}{
		{"system group reads", &mw.Principal{Subject: "alice", Groups: []string{"system:masters"}}, http.MethodGet, "/api/v1alpha1/tenants", ""},
		{"system subject writes", &mw.Principal{Subject: "system:anonymous", Groups: []string{"platform-admins"}}, http.MethodPost, "/api/v1alpha1/tenants", tenantBody},
		{"system group on admin route", &mw.Principal{Subject: "alice", Groups: []string{"platform-admins", "system:nodes"}}, http.MethodDelete, "/api/v1alpha1/tenants/team-a", ""},
		{"system group on access review", &mw.Principal{Subject: "alice", Groups: []string{"system:masters"}}, http.MethodPost, "/api/v1alpha1/access-reviews", `{"verb":"get","resource":"workloads"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingFactory{dyn: newFakeDynamic(seed()...)}
			h := mw.Chain(New(Options{ClientFor: rec.factory}).Routes(), withPrincipal(tc.p))
			if rr := do(t, h, tc.method, tc.path, tc.body); rr.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403: %s", rr.Code, rr.Body)
			}
			if len(rec.principal) != 0 {
				t.Fatal("no client may be built for a reserved identity")
			}
		})
	}
}

func TestAccessReview(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCode    int
		wantAllowed bool
	}{
		{"allowed", `{"verb":"create","group":"platform.mycompany.io","resource":"workloads","namespace":"team-a"}`, 200, true},
		{"missing verb", `{"resource":"workloads"}`, 422, false},
		{"bad namespace", `{"verb":"get","resource":"workloads","namespace":"Bad_NS"}`, 422, false},
		{"unknown field", `{"verb":"get","resource":"workloads","bogus":1}`, 400, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dyn := newFakeDynamic()
			// The fake API server answers the review: allow only the "create" verb.
			dyn.PrependReactor("create", "selfsubjectaccessreviews", func(a k8stesting.Action) (bool, runtime.Object, error) {
				obj := a.(k8stesting.CreateAction).GetObject().(*unstructured.Unstructured)
				verb, _, _ := unstructured.NestedString(obj.Object, "spec", "resourceAttributes", "verb")
				obj.Object["status"] = map[string]any{"allowed": verb == "create"}
				return true, obj, nil
			})
			rec := &recordingFactory{dyn: dyn}
			h := mw.Chain(New(Options{ClientFor: rec.factory}).Routes(), withPrincipal(&mw.Principal{Subject: "alice", Groups: []string{"team-a"}}))
			rr := do(t, h, http.MethodPost, "/api/v1alpha1/access-reviews", tc.body)
			if rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.wantCode, rr.Body)
			}
			if tc.wantCode != 200 {
				return
			}
			var res types.AccessReviewResult
			if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
				t.Fatal(err)
			}
			if res.Allowed != tc.wantAllowed {
				t.Fatalf("allowed = %v, want %v", res.Allowed, tc.wantAllowed)
			}
			if len(rec.principal) != 1 || rec.principal[0].Subject != "alice" {
				t.Fatal("the review must be created with the caller's client")
			}
		})
	}
}

func TestAccessReviewUnauthenticated(t *testing.T) {
	h := mw.Chain(New(Options{ClientFor: fakeFactory(newFakeDynamic())}).Routes(), withPrincipal(nil))
	if rr := do(t, h, http.MethodPost, "/api/v1alpha1/access-reviews", `{"verb":"get","resource":"workloads"}`); rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestWatchStreamsEventsAsTheCaller(t *testing.T) {
	dyn := newFakeDynamic(seed()...)
	rec := &recordingFactory{dyn: dyn}
	h := mw.Chain(New(Options{ClientFor: rec.factory}).Routes(), withPrincipal(adminPrincipal))
	srv := httptest.NewServer(h)
	defer srv.Close()

	// The response headers are written only after the watch is registered, so
	// once Get returns it is safe to create objects.
	resp, err := http.Get(srv.URL + "/api/v1alpha1/namespaces/team-a/workloads?watch=true")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "ndjson") {
		t.Fatalf("status = %d, content-type = %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	if _, err := dyn.Resource(WorkloadGVR).Namespace("team-a").Create(context.Background(),
		cr("Workload", "team-a", "api", nil, map[string]any{"image": "ghcr.io/acme/api:1"}), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	// A watch without resourceVersion first replays existing objects as ADDED,
	// so read until the object created above shows up.
	reader := bufio.NewReader(resp.Body)
	var found bool
	for !found {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatalf("stream ended before the new object arrived: %v", err)
		}
		var ev struct {
			Type   string         `json:"type"`
			Object types.Workload `json:"object"`
		}
		if err := json.Unmarshal(line, &ev); err != nil {
			t.Fatalf("bad event line %q: %v", line, err)
		}
		if ev.Object.Metadata.Name == "api" {
			found = true
			if ev.Type != "ADDED" {
				t.Fatalf("event type = %q, want ADDED", ev.Type)
			}
		}
	}
	if len(rec.principal) == 0 || rec.principal[0].Subject != "admin" {
		t.Fatal("the watch must be opened as the caller")
	}
}

func TestWatchRequiresPrincipal(t *testing.T) {
	h := mw.Chain(New(Options{ClientFor: fakeFactory(newFakeDynamic())}).Routes(), withPrincipal(nil))
	if rr := do(t, h, http.MethodGet, "/api/v1alpha1/namespaces/team-a/workloads?watch=true", ""); rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}
