package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/client"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/handlers"
	mw "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/middleware"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/types"
)

// newServer runs the real handler stack against a fake dynamic client so the
// SDK is tested end-to-end over HTTP.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		handlers.TenantGVR: "TenantList", handlers.EnvironmentGVR: "EnvironmentList", handlers.WorkloadGVR: "WorkloadList",
	})
	h := mw.Chain(handlers.New(handlers.Options{Dynamic: dyn, Build: handlers.BuildInfo{Version: "test"}}).Routes(),
		mw.RequestID(),
		mw.Auth(mw.AuthConfig{Verifier: mw.StaticTokens{"good": {Subject: "sdk"}}, PublicPaths: []string{"/version"}}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func int32p(v int32) *int32 { return &v }

func TestResourceLifecycle(t *testing.T) {
	srv := newServer(t)
	c, err := client.New(srv.URL, client.WithToken("good"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	tenant, err := c.Tenants().Create(ctx, &types.Tenant{
		Metadata: types.ObjectMeta{Name: "team-a"},
		Spec:     types.TenantSpec{DisplayName: "Team A", Owners: []string{"alice"}},
	})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if tenant.Metadata.Annotations[handlers.CreatedByAnnotation] != "sdk" {
		t.Fatalf("created-by not set: %+v", tenant.Metadata)
	}
	if _, err := c.Tenants().Create(ctx, tenant); !client.IsConflict(err) {
		t.Fatalf("duplicate create: want conflict, got %v", err)
	}

	if _, err := c.Environments("team-a").Create(ctx, &types.Environment{
		Metadata: types.ObjectMeta{Name: "dev"},
		Spec:     types.EnvironmentSpec{Tenant: "team-a", Type: types.EnvironmentDev},
	}); err != nil {
		t.Fatalf("create env: %v", err)
	}

	wl, err := c.Workloads("team-a").Create(ctx, &types.Workload{
		Metadata: types.ObjectMeta{Name: "checkout", Labels: map[string]string{"app": "checkout"}},
		Spec: types.WorkloadSpec{Image: "ghcr.io/acme/checkout:1", Replicas: int32p(2), Port: 8080,
			Traits: []types.Trait{{Type: types.TraitIngress, Config: map[string]any{"host": "checkout.example.com"}}}},
	})
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}
	if wl.Metadata.Namespace != "team-a" || *wl.Spec.Replicas != 2 || wl.Spec.Traits[0].Config["host"] != "checkout.example.com" {
		t.Fatalf("unexpected workload: %+v", wl)
	}

	all, err := c.Workloads("").List(ctx, client.ListOptions{LabelSelector: "app=checkout"})
	if err != nil || len(all.Items) != 1 {
		t.Fatalf("list workloads: %v %+v", err, all)
	}
	got, err := c.Workloads("team-a").Get(ctx, "checkout")
	if err != nil || got.Spec.Image != "ghcr.io/acme/checkout:1" {
		t.Fatalf("get workload: %v %+v", err, got)
	}
	if err := c.Workloads("team-a").Delete(ctx, "checkout"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := c.Workloads("team-a").Get(ctx, "checkout"); !client.IsNotFound(err) {
		t.Fatalf("get after delete: want not found, got %v", err)
	}
}

func TestErrors(t *testing.T) {
	srv := newServer(t)
	ctx := context.Background()
	tests := []struct {
		name  string
		token string
		call  func(*client.Client) error
		check func(error) bool
		want  string
	}{
		{"unauthorized", "bad", func(c *client.Client) error { _, err := c.Tenants().List(ctx, client.ListOptions{}); return err },
			client.IsUnauthorized, "401"},
		{"validation details surfaced", "good", func(c *client.Client) error {
			_, err := c.Tenants().Create(ctx, &types.Tenant{Metadata: types.ObjectMeta{Name: "x"}})
			return err
		}, func(err error) bool { return err != nil }, "spec.owners"},
		{"public endpoint needs no token", "", func(c *client.Client) error {
			v, err := c.Version(ctx)
			if err == nil && v.Version != "test" {
				t.Errorf("version = %q", v.Version)
			}
			return err
		}, func(err error) bool { return err == nil }, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := client.New(srv.URL, client.WithToken(tc.token))
			err := tc.call(c)
			if !tc.check(err) {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q missing %q", err, tc.want)
			}
			if err != nil && !strings.Contains(err.Error(), "request-id") {
				t.Fatalf("error should include request id: %v", err)
			}
		})
	}
}

func TestNonJSONErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream exploded", http.StatusBadGateway)
	}))
	defer srv.Close()
	c, _ := client.New(srv.URL)
	_, err := c.Tenants().Get(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "upstream exploded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRejectsBadURL(t *testing.T) {
	for _, u := range []string{"ftp://x", "::bad", "localhost:8080"} {
		if _, err := client.New(u); err == nil {
			t.Errorf("New(%q) should fail", u)
		}
	}
}
