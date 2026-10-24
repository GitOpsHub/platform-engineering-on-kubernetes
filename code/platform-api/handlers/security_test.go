package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/dynamic"

	mw "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/middleware"
)

// recordingFactory records every principal it is asked to build a client for.
type recordingFactory struct {
	dyn       dynamic.Interface
	principal []*mw.Principal
	err       error
}

func (f *recordingFactory) factory(p *mw.Principal) (dynamic.Interface, error) {
	f.principal = append(f.principal, p)
	if f.err != nil {
		return nil, f.err
	}
	return f.dyn, nil
}

const (
	tenantBody   = `{"metadata":{"name":"team-c"},"spec":{"displayName":"Team C","owners":["carol"]}}`
	workloadBody = `{"metadata":{"name":"cart"},"spec":{"image":"ghcr.io/acme/cart:2"}}`
)

func TestMutationsAndReadsRejectNilPrincipal(t *testing.T) {
	tests := []struct {
		name, method, path, body string
	}{
		{"create tenant", http.MethodPost, "/api/v1alpha1/tenants", tenantBody},
		{"delete tenant", http.MethodDelete, "/api/v1alpha1/tenants/team-a", ""},
		{"create workload", http.MethodPost, "/api/v1alpha1/namespaces/team-a/workloads", workloadBody},
		{"delete workload", http.MethodDelete, "/api/v1alpha1/namespaces/team-a/workloads/checkout", ""},
		{"create environment", http.MethodPost, "/api/v1alpha1/namespaces/team-a/environments",
			`{"metadata":{"name":"pr-1"},"spec":{"tenant":"team-a","type":"preview"}}`},
		{"delete environment", http.MethodDelete, "/api/v1alpha1/namespaces/team-a/environments/dev", ""},
		{"list tenants", http.MethodGet, "/api/v1alpha1/tenants", ""},
		{"get workload", http.MethodGet, "/api/v1alpha1/namespaces/team-a/workloads/checkout", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingFactory{dyn: newFakeDynamic(seed()...)}
			api := New(Options{ClientFor: rec.factory})
			h := mw.Chain(api.Routes(), withPrincipal(nil))
			rr := do(t, h, tc.method, tc.path, tc.body)
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401: %s", rr.Code, rr.Body)
			}
			if len(rec.principal) != 0 {
				t.Fatalf("client factory must not run without a principal, got %d calls", len(rec.principal))
			}
		})
	}
}

func TestPrincipalIsPassedToClientFactory(t *testing.T) {
	rec := &recordingFactory{dyn: newFakeDynamic(seed()...)}
	api := New(Options{ClientFor: rec.factory})
	caller := &mw.Principal{Subject: "alice", Groups: []string{"team-a-devs"}}
	h := mw.Chain(api.Routes(), withPrincipal(caller))

	if rr := do(t, h, http.MethodPost, "/api/v1alpha1/namespaces/team-a/workloads", workloadBody); rr.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body)
	}
	if len(rec.principal) != 1 {
		t.Fatalf("factory called %d times, want 1", len(rec.principal))
	}
	got := rec.principal[0]
	if got.Subject != "alice" || len(got.Groups) != 1 || got.Groups[0] != "team-a-devs" {
		t.Fatalf("factory received %+v, want the authenticated caller", got)
	}
}

func TestNonAdminCannotCreateOrDeleteTenants(t *testing.T) {
	tests := []struct {
		name   string
		groups []string
		method string
		path   string
		body   string
		want   int
	}{
		{"non-admin create", []string{"devs"}, http.MethodPost, "/api/v1alpha1/tenants", tenantBody, http.StatusForbidden},
		{"no groups create", nil, http.MethodPost, "/api/v1alpha1/tenants", tenantBody, http.StatusForbidden},
		{"non-admin delete", []string{"devs"}, http.MethodDelete, "/api/v1alpha1/tenants/team-a", "", http.StatusForbidden},
		{"admin create", []string{"platform-admins"}, http.MethodPost, "/api/v1alpha1/tenants", tenantBody, http.StatusCreated},
		{"admin delete", []string{"platform-admins"}, http.MethodDelete, "/api/v1alpha1/tenants/team-a", "", http.StatusNoContent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dyn := newFakeDynamic(seed()...)
			api := New(Options{ClientFor: fakeFactory(dyn)})
			h := mw.Chain(api.Routes(), withPrincipal(&mw.Principal{Subject: "u", Groups: tc.groups}))
			rr := do(t, h, tc.method, tc.path, tc.body)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body)
			}
		})
	}
}

func TestCustomPlatformAdminGroup(t *testing.T) {
	dyn := newFakeDynamic()
	api := New(Options{ClientFor: fakeFactory(dyn), PlatformAdminGroup: "ops-admins"})
	h := mw.Chain(api.Routes(), withPrincipal(&mw.Principal{Subject: "u", Groups: []string{"platform-admins"}}))
	if rr := do(t, h, http.MethodPost, "/api/v1alpha1/tenants", tenantBody); rr.Code != http.StatusForbidden {
		t.Fatalf("default group must not grant access once a custom group is set: %d", rr.Code)
	}
	h = mw.Chain(api.Routes(), withPrincipal(&mw.Principal{Subject: "u", Groups: []string{"ops-admins"}}))
	if rr := do(t, h, http.MethodPost, "/api/v1alpha1/tenants", tenantBody); rr.Code != http.StatusCreated {
		t.Fatalf("custom admin group rejected: %d %s", rr.Code, rr.Body)
	}
}

func TestInsecureSkipAuthzAllowsAnonymousWithBaseClient(t *testing.T) {
	dyn := newFakeDynamic(seed()...)
	rec := &recordingFactory{dyn: newFakeDynamic()}
	api := New(Options{ClientFor: rec.factory, Dynamic: dyn, InsecureSkipAuthz: true})
	h := mw.Chain(api.Routes(), withPrincipal(nil))

	if rr := do(t, h, http.MethodGet, "/api/v1alpha1/tenants", ""); rr.Code != http.StatusOK {
		t.Fatalf("anonymous list with insecure mode: %d %s", rr.Code, rr.Body)
	}
	if rr := do(t, h, http.MethodPost, "/api/v1alpha1/tenants", tenantBody); rr.Code != http.StatusCreated {
		t.Fatalf("anonymous tenant create with insecure mode: %d %s", rr.Code, rr.Body)
	}
	if len(rec.principal) != 0 {
		t.Fatal("anonymous requests must use the base client, not the caller factory")
	}
}

func TestClientFactoryErrorIsReported(t *testing.T) {
	rec := &recordingFactory{err: errors.New("boom")}
	api := New(Options{ClientFor: rec.factory})
	h := mw.Chain(api.Routes(), withPrincipal(adminPrincipal))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1alpha1/tenants", nil))
	if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "boom") {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body)
	}
}

// The admin gate reads the caller's own groups, not the groups the client
// factory forwards. A factory that forwards no groups must not cost an admin
// the Tenant create right.
func TestAdminRecognisedEvenWhenFactoryForwardsNoGroups(t *testing.T) {
	dyn := newFakeDynamic()
	noGroups := func(p *mw.Principal) (dynamic.Interface, error) {
		// Simulate an allowlist that does not contain the admin group.
		if p == nil {
			return nil, errors.New("no principal")
		}
		return dyn, nil
	}
	api := New(Options{ClientFor: noGroups, PlatformAdminGroup: "platform-admins"})
	caller := &mw.Principal{Subject: "admin", Groups: []string{"platform-admins"}}
	h := mw.Chain(api.Routes(), withPrincipal(caller))
	if rr := do(t, h, http.MethodPost, "/api/v1alpha1/tenants", tenantBody); rr.Code != http.StatusCreated {
		t.Fatalf("admin must be recognised from the caller's groups: %d %s", rr.Code, rr.Body)
	}
}
