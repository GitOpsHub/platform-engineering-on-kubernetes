// Package handlers implements the platform-api REST endpoints on top of the
// Kubernetes dynamic client. Every resource endpoint is a thin translation
// layer over the platform.mycompany.io/v1alpha1 CRDs; the operators do the
// actual reconciliation.
package handlers

import (
	"context"
	"net/http"
	"net/http/pprof"
	"runtime"
	"time"

	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/dynamic"

	mw "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/middleware"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/types"
)

// BuildInfo is injected at build time via -ldflags.
type BuildInfo struct {
	Version   string
	Commit    string
	BuildTime string
}

// ServerVersioner is the subset of discovery.DiscoveryInterface we need.
type ServerVersioner interface {
	ServerVersion() (*version.Info, error)
}

// Options configures the API handler.
type Options struct {
	Dynamic         dynamic.Interface
	Discovery       ServerVersioner
	Build           BuildInfo
	MetricsHandler  http.Handler // mounted at /metrics when non-nil
	EnableProfiling bool         // mounts net/http/pprof under /debug/pprof/
	OpenAPISpec     []byte       // served at /openapi.yaml when non-nil
}

// API holds handler dependencies.
type API struct {
	opts         Options
	tenants      *resource[types.Tenant, *types.Tenant]
	environments *resource[types.Environment, *types.Environment]
	workloads    *resource[types.Workload, *types.Workload]
}

// New returns the API.
func New(opts Options) *API {
	return &API{
		opts:         opts,
		tenants:      &resource[types.Tenant, *types.Tenant]{client: opts.Dynamic, gvr: TenantGVR, kind: "Tenant"},
		environments: &resource[types.Environment, *types.Environment]{client: opts.Dynamic, gvr: EnvironmentGVR, kind: "Environment", namespaced: true},
		workloads:    &resource[types.Workload, *types.Workload]{client: opts.Dynamic, gvr: WorkloadGVR, kind: "Workload", namespaced: true},
	}
}

// Routes returns a mux with every endpoint registered (no middleware).
// Uses Go 1.22+ method- and wildcard-aware ServeMux patterns.
func (a *API) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.healthz)
	mux.HandleFunc("GET /readyz", a.readyz)
	mux.HandleFunc("GET /version", a.version)
	if a.opts.OpenAPISpec != nil {
		mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/yaml")
			_, _ = w.Write(a.opts.OpenAPISpec)
		})
	}
	if a.opts.MetricsHandler != nil {
		mux.Handle("GET /metrics", a.opts.MetricsHandler)
	}
	if a.opts.EnableProfiling {
		// Register pprof handlers explicitly on our own mux instead of relying
		// on the side-effect registration on http.DefaultServeMux.
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	const p = "/api/" + types.Version
	mux.HandleFunc("GET "+p+"/platform", a.platformInfo)

	// Tenants (cluster-scoped)
	mux.HandleFunc("GET "+p+"/tenants", a.tenants.list)
	mux.HandleFunc("POST "+p+"/tenants", a.tenants.create)
	mux.HandleFunc("GET "+p+"/tenants/{name}", a.tenants.get)
	mux.HandleFunc("DELETE "+p+"/tenants/{name}", a.tenants.delete)

	// Environments & Workloads (namespaced) plus all-namespace list views.
	for _, rs := range []struct {
		name                   string
		list, get, create, del http.HandlerFunc
	}{
		{"environments", a.environments.list, a.environments.get, a.environments.create, a.environments.delete},
		{"workloads", a.workloads.list, a.workloads.get, a.workloads.create, a.workloads.delete},
	} {
		mux.HandleFunc("GET "+p+"/"+rs.name, rs.list)
		mux.HandleFunc("GET "+p+"/namespaces/{namespace}/"+rs.name, rs.list)
		mux.HandleFunc("POST "+p+"/namespaces/{namespace}/"+rs.name, rs.create)
		mux.HandleFunc("GET "+p+"/namespaces/{namespace}/"+rs.name+"/{name}", rs.get)
		mux.HandleFunc("DELETE "+p+"/namespaces/{namespace}/"+rs.name+"/{name}", rs.del)
	}
	return mux
}

func (a *API) healthz(w http.ResponseWriter, _ *http.Request) {
	mw.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) readyz(w http.ResponseWriter, r *http.Request) {
	if a.opts.Discovery == nil {
		mw.WriteError(w, r, http.StatusServiceUnavailable, "NotReady", "no Kubernetes discovery client configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() { _, err := a.opts.Discovery.ServerVersion(); errCh <- err }()
	select {
	case err := <-errCh:
		if err != nil {
			mw.WriteError(w, r, http.StatusServiceUnavailable, "NotReady", "kubernetes API unreachable: "+err.Error())
			return
		}
	case <-ctx.Done():
		mw.WriteError(w, r, http.StatusServiceUnavailable, "NotReady", "kubernetes API check timed out")
		return
	}
	mw.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *API) version(w http.ResponseWriter, _ *http.Request) {
	mw.WriteJSON(w, http.StatusOK, types.VersionInfo{
		Version:   a.opts.Build.Version,
		Commit:    a.opts.Build.Commit,
		BuildTime: a.opts.Build.BuildTime,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	})
}

func (a *API) platformInfo(w http.ResponseWriter, _ *http.Request) {
	info := types.PlatformInfo{
		Name:        "platform-engineering",
		Version:     a.opts.Build.Version,
		Description: "Internal Developer Platform API",
		APIVersion:  types.APIVersion,
		Capabilities: []string{
			"tenant-management",
			"environment-management",
			"workload-management",
		},
	}
	// Degrade gracefully: never dereference a nil version on error.
	switch {
	case a.opts.Discovery == nil:
		info.Kubernetes = &types.KubeInfo{Error: "no Kubernetes discovery client configured"}
	default:
		if v, err := a.opts.Discovery.ServerVersion(); err != nil || v == nil {
			msg := "unknown"
			if err != nil {
				msg = err.Error()
			}
			info.Kubernetes = &types.KubeInfo{Error: msg}
		} else {
			info.Kubernetes = &types.KubeInfo{Version: v.GitVersion, Platform: v.Platform}
		}
	}
	mw.WriteJSON(w, http.StatusOK, info)
}
