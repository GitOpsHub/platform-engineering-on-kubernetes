package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/dynamic"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	mw "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/middleware"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/types"
)

func newFakeDynamic(objs ...runtime.Object) *dynfake.FakeDynamicClient {
	return dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			TenantGVR:      "TenantList",
			EnvironmentGVR: "EnvironmentList",
			WorkloadGVR:    "WorkloadList",
		}, objs...)
}

func cr(kind, ns, name string, labels map[string]string, spec map[string]any) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{"spec": spec}}
	u.SetAPIVersion(types.APIVersion)
	u.SetKind(kind)
	u.SetNamespace(ns)
	u.SetName(name)
	u.SetLabels(labels)
	return u
}

type fakeDiscovery struct {
	info *version.Info
	err  error
}

func (f fakeDiscovery) ServerVersion() (*version.Info, error) { return f.info, f.err }

// adminPrincipal is an authenticated platform admin used by most tests.
var adminPrincipal = &mw.Principal{Subject: "admin", Groups: []string{"platform-admins"}}

// fakeFactory returns a ClientFactory that always hands out dyn.
func fakeFactory(dyn dynamic.Interface) ClientFactory {
	return func(*mw.Principal) (dynamic.Interface, error) { return dyn, nil }
}

// withPrincipal injects p as the authenticated caller (nil = anonymous).
func withPrincipal(p *mw.Principal) mw.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p != nil {
				r = r.WithContext(mw.WithPrincipal(r.Context(), p))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func newHandler(dyn *dynfake.FakeDynamicClient, disc ServerVersioner) http.Handler {
	return newHandlerFor(dyn, disc, adminPrincipal)
}

func newHandlerFor(dyn *dynfake.FakeDynamicClient, disc ServerVersioner, p *mw.Principal) http.Handler {
	api := New(Options{ClientFor: fakeFactory(dyn), Dynamic: dyn, Discovery: disc,
		Build: BuildInfo{Version: "1.2.3"}, OpenAPISpec: []byte("openapi: 3.0.3")})
	return mw.Chain(api.Routes(), withPrincipal(p), mw.RequestID())
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func seed() []runtime.Object {
	return []runtime.Object{
		cr("Tenant", "", "team-a", map[string]string{"tier": "gold"}, map[string]any{"displayName": "Team A", "owners": []any{"alice"}}),
		cr("Tenant", "", "team-b", nil, map[string]any{"displayName": "Team B", "owners": []any{"bob"}}),
		cr("Environment", "team-a", "dev", nil, map[string]any{"tenant": "team-a", "type": "dev"}),
		cr("Environment", "team-b", "prod", nil, map[string]any{"tenant": "team-b", "type": "prod"}),
		cr("Workload", "team-a", "checkout", nil, map[string]any{"image": "ghcr.io/acme/checkout:1", "replicas": int64(2), "port": int64(8080)}),
	}
}

func TestListEndpoints(t *testing.T) {
	h := newHandler(newFakeDynamic(seed()...), fakeDiscovery{info: &version.Info{GitVersion: "v1.30.0"}})
	tests := []struct {
		name      string
		path      string
		wantCode  int
		wantNames []string
	}{
		{"tenants", "/api/v1alpha1/tenants", 200, []string{"team-a", "team-b"}},
		{"tenants by label", "/api/v1alpha1/tenants?labelSelector=tier%3Dgold", 200, []string{"team-a"}},
		{"environments all namespaces", "/api/v1alpha1/environments", 200, []string{"dev", "prod"}},
		{"environments in namespace", "/api/v1alpha1/namespaces/team-b/environments", 200, []string{"prod"}},
		{"workloads in namespace", "/api/v1alpha1/namespaces/team-a/workloads", 200, []string{"checkout"}},
		{"workloads empty namespace", "/api/v1alpha1/namespaces/team-b/workloads", 200, []string{}},
		{"bad limit", "/api/v1alpha1/tenants?limit=0", 400, nil},
		{"bad namespace", "/api/v1alpha1/namespaces/Bad_NS/workloads", 400, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := do(t, h, http.MethodGet, tc.path, "")
			if rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.wantCode, rr.Body)
			}
			if tc.wantNames == nil {
				return
			}
			var list types.List[struct {
				Metadata types.ObjectMeta `json:"metadata"`
			}]
			if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
				t.Fatal(err)
			}
			got := map[string]bool{}
			for _, it := range list.Items {
				got[it.Metadata.Name] = true
			}
			if len(got) != len(tc.wantNames) {
				t.Fatalf("got %v, want %v", got, tc.wantNames)
			}
			for _, n := range tc.wantNames {
				if !got[n] {
					t.Fatalf("missing %q in %v", n, got)
				}
			}
		})
	}
}

func TestGetWorkloadDecodesSpec(t *testing.T) {
	h := newHandler(newFakeDynamic(seed()...), nil)
	rr := do(t, h, http.MethodGet, "/api/v1alpha1/namespaces/team-a/workloads/checkout", "")
	if rr.Code != 200 {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body)
	}
	var wl types.Workload
	if err := json.Unmarshal(rr.Body.Bytes(), &wl); err != nil {
		t.Fatal(err)
	}
	if wl.Spec.Image != "ghcr.io/acme/checkout:1" || wl.Spec.Replicas == nil || *wl.Spec.Replicas != 2 || wl.Spec.Port != 8080 {
		t.Fatalf("unexpected workload: %+v", wl.Spec)
	}
	if wl.Kind != "Workload" || wl.APIVersion != types.APIVersion {
		t.Fatalf("unexpected type meta: %+v", wl.TypeMeta)
	}
}

func TestGetNotFound(t *testing.T) {
	h := newHandler(newFakeDynamic(), nil)
	rr := do(t, h, http.MethodGet, "/api/v1alpha1/tenants/missing", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
	var body types.ErrorBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Reason != "NotFound" || body.Error.RequestID == "" {
		t.Fatalf("unexpected error body: %+v", body)
	}
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		body     string
		wantCode int
		wantLoc  string
	}{
		{"tenant", "/api/v1alpha1/tenants",
			`{"metadata":{"name":"team-c"},"spec":{"displayName":"Team C","owners":["carol"],"quota":{"cpu":"4","pods":10}}}`,
			201, "/api/v1alpha1/tenants/team-c"},
		{"tenant duplicate", "/api/v1alpha1/tenants",
			`{"metadata":{"name":"team-a"},"spec":{"displayName":"A","owners":["alice"]}}`, 409, ""},
		{"tenant invalid", "/api/v1alpha1/tenants", `{"metadata":{"name":"team-c"},"spec":{"displayName":""}}`, 422, ""},
		{"tenant unknown field", "/api/v1alpha1/tenants", `{"metadata":{"name":"x"},"spec":{"displayName":"x","owners":["a"],"bogus":1}}`, 400, ""},
		{"environment", "/api/v1alpha1/namespaces/team-a/environments",
			`{"metadata":{"name":"pr-7"},"spec":{"tenant":"team-a","type":"preview","ttl":"24h"}}`,
			201, "/api/v1alpha1/namespaces/team-a/environments/pr-7"},
		{"environment namespace mismatch", "/api/v1alpha1/namespaces/team-a/environments",
			`{"metadata":{"name":"pr-8","namespace":"team-b"},"spec":{"tenant":"team-a","type":"dev"}}`, 400, ""},
		{"workload", "/api/v1alpha1/namespaces/team-a/workloads",
			`{"metadata":{"name":"cart","labels":{"app.kubernetes.io/name":"cart"}},"spec":{"image":"ghcr.io/acme/cart:2","replicas":3,"port":8080,"traits":[{"type":"autoscaling","config":{"maxReplicas":10}}]}}`,
			201, "/api/v1alpha1/namespaces/team-a/workloads/cart"},
		{"workload bad label", "/api/v1alpha1/namespaces/team-a/workloads",
			`{"metadata":{"name":"cart2","labels":{"bad key!":"x"}},"spec":{"image":"img"}}`, 422, ""},
		{"empty body", "/api/v1alpha1/tenants", ``, 400, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dyn := newFakeDynamic(seed()...)
			h := newHandler(dyn, nil)
			rr := do(t, h, http.MethodPost, tc.path, tc.body)
			if rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.wantCode, rr.Body)
			}
			if tc.wantLoc != "" && rr.Header().Get("Location") != tc.wantLoc {
				t.Fatalf("Location = %q, want %q", rr.Header().Get("Location"), tc.wantLoc)
			}
		})
	}
}

func TestCreateStoresCRAndCreatedBy(t *testing.T) {
	dyn := newFakeDynamic()
	api := New(Options{ClientFor: fakeFactory(dyn)})
	h := mw.Chain(api.Routes(), mw.Auth(mw.AuthConfig{Verifier: mw.StaticTokens{"t": {Subject: "alice"}}}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1alpha1/namespaces/team-a/workloads",
		strings.NewReader(`{"metadata":{"name":"cart","uid":"client-supplied"},"spec":{"image":"img:1"},"status":{"phase":"Ready"}}`))
	req.Header.Set("Authorization", "Bearer t")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body)
	}
	u, err := dyn.Resource(WorkloadGVR).Namespace("team-a").Get(req.Context(), "cart", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if u.GetAnnotations()[CreatedByAnnotation] != "alice" {
		t.Fatalf("created-by annotation = %v", u.GetAnnotations())
	}
	if _, has := u.Object["status"]; has {
		t.Fatalf("status must not be persisted from request: %v", u.Object["status"])
	}
	if string(u.GetUID()) == "client-supplied" {
		t.Fatal("client-supplied uid must be dropped")
	}
	if u.GetKind() != "Workload" || u.GetAPIVersion() != types.APIVersion {
		t.Fatalf("bad type meta: %s %s", u.GetAPIVersion(), u.GetKind())
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name, path string
		wantCode   int
	}{
		{"tenant", "/api/v1alpha1/tenants/team-a", 204},
		{"environment", "/api/v1alpha1/namespaces/team-a/environments/dev", 204},
		{"workload", "/api/v1alpha1/namespaces/team-a/workloads/checkout", 204},
		{"missing", "/api/v1alpha1/namespaces/team-a/workloads/nope", 404},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHandler(newFakeDynamic(seed()...), nil)
			if rr := do(t, h, http.MethodDelete, tc.path, ""); rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.wantCode, rr.Body)
			}
		})
	}
}

func TestKubeErrorMapping(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"forbidden", apierrors.NewForbidden(TenantGVR.GroupResource(), "", errors.New("rbac")), 403},
		{"invalid", apierrors.NewInvalid(schema.GroupKind{Group: types.Group, Kind: "Tenant"}, "x", nil), 422},
		{"timeout", apierrors.NewTimeoutError("slow", 1), 504},
		{"plain error", errors.New("connection refused"), 500},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dyn := newFakeDynamic()
			dyn.PrependReactor("list", "tenants", func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, tc.err
			})
			if rr := do(t, newHandler(dyn, nil), http.MethodGet, "/api/v1alpha1/tenants", ""); rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.wantCode, rr.Body)
			}
		})
	}
}

func TestOperationalEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		disc       ServerVersioner
		path       string
		wantCode   int
		wantInBody string
	}{
		{"healthz", nil, "/healthz", 200, `"ok"`},
		{"readyz ok", fakeDiscovery{info: &version.Info{GitVersion: "v1.30.0"}}, "/readyz", 200, "ready"},
		{"readyz down", fakeDiscovery{err: errors.New("dial tcp: refused")}, "/readyz", 503, "refused"},
		{"readyz unconfigured", nil, "/readyz", 503, "NotReady"},
		{"version reports runtime go version", nil, "/version", 200, `"goVersion":"go`},
		{"platform ok", fakeDiscovery{info: &version.Info{GitVersion: "v1.30.0", Platform: "linux/amd64"}}, "/api/v1alpha1/platform", 200, "v1.30.0"},
		// Regression: the old implementation nil-dereferenced here.
		{"platform with discovery error", fakeDiscovery{err: errors.New("boom")}, "/api/v1alpha1/platform", 200, `"error":"boom"`},
		{"openapi", nil, "/openapi.yaml", 200, "openapi"},
		{"pprof disabled by default", nil, "/debug/pprof/", 404, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := do(t, newHandler(newFakeDynamic(), tc.disc), http.MethodGet, tc.path, "")
			if rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.wantCode, rr.Body)
			}
			if !strings.Contains(rr.Body.String(), tc.wantInBody) {
				t.Fatalf("body %q missing %q", rr.Body, tc.wantInBody)
			}
		})
	}
}

func TestProfilingEnabled(t *testing.T) {
	h := New(Options{ClientFor: fakeFactory(newFakeDynamic()), EnableProfiling: true}).Routes()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "goroutine") {
		t.Fatalf("pprof index not served: %d", rr.Code)
	}
}
