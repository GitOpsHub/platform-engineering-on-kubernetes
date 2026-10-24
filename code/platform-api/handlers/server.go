// Package handlers implements the platform-api REST endpoints on top of the
// Kubernetes dynamic client. Every resource endpoint is a thin translation
// layer over the platform.mycompany.io/v1alpha1 CRDs; the operators do the
// actual reconciliation.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"runtime"
	"slices"
	"strings"
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

// DefaultPlatformAdminGroup is the group whose members may create and delete
// Tenants when Options.PlatformAdminGroup is left empty.
const DefaultPlatformAdminGroup = "platform-admins"

// ClientFactory returns a Kubernetes dynamic client that acts AS the given
// principal (for example via rest.ImpersonationConfig), so that cluster RBAC
// applies to the real caller rather than to platform-api's own identity.
type ClientFactory func(p *mw.Principal) (dynamic.Interface, error)

// Options configures the API handler.
type Options struct {
	// ClientFor builds the per-request, caller-scoped client. Every resource
	// request goes through it with the authenticated principal. Required.
	ClientFor ClientFactory
	// Dynamic is platform-api's OWN client. It is used only for requests that
	// carry no principal when InsecureSkipAuthz is set (local development).
	Dynamic dynamic.Interface

	Discovery       ServerVersioner
	Build           BuildInfo
	MetricsHandler  http.Handler // mounted at /metrics when non-nil
	EnableProfiling bool         // mounts net/http/pprof under /debug/pprof/
	OpenAPISpec     []byte       // served at /openapi.yaml when non-nil

	// PlatformAdminGroup is the group required to create or delete Tenants.
	// Empty means DefaultPlatformAdminGroup.
	PlatformAdminGroup string

	// InsecureSkipAuthz disables the nil-principal 401 and the admin-group check
	// and lets unauthenticated requests use Dynamic. Dev/lab use ONLY.
	InsecureSkipAuthz bool
}

// API holds handler dependencies.
type API struct {
	opts         Options
	tenants      *resource[types.Tenant, *types.Tenant]
	environments *resource[types.Environment, *types.Environment]
	workloads    *resource[types.Workload, *types.Workload]
}

// errUnauthenticated is returned when a request has no principal.
var errUnauthenticated = errors.New("authentication required")

// ErrForbiddenPrincipal is returned for a principal that names a reserved
// system identity (a system:* subject or group). Such a caller is refused
// before any impersonation happens.
var ErrForbiddenPrincipal = errors.New("principal names a reserved system identity")

// ValidatePrincipal rejects subjects and groups in the reserved system: namespace,
// such as system:masters or system:nodes. RBAC cannot express "anything except
// system:*", so the refusal is made here, before any impersonation.
func ValidatePrincipal(p *mw.Principal) error {
	if p == nil {
		return errUnauthenticated
	}
	if strings.HasPrefix(p.Subject, "system:") {
		return ErrForbiddenPrincipal
	}
	for _, g := range p.Groups {
		if strings.HasPrefix(g, "system:") {
			return ErrForbiddenPrincipal
		}
	}
	return nil
}

// New returns the API.
func New(opts Options) *API {
	if opts.PlatformAdminGroup == "" {
		opts.PlatformAdminGroup = DefaultPlatformAdminGroup
	}
	a := &API{opts: opts}
	a.tenants = &resource[types.Tenant, *types.Tenant]{clientFor: a.clientFor, gvr: TenantGVR, kind: "Tenant"}
	a.environments = &resource[types.Environment, *types.Environment]{clientFor: a.clientFor, gvr: EnvironmentGVR, kind: "Environment", namespaced: true}
	a.workloads = &resource[types.Workload, *types.Workload]{clientFor: a.clientFor, gvr: WorkloadGVR, kind: "Workload", namespaced: true}
	return a
}

// clientFor returns the client that must serve r: one acting as the caller.
// platform-api's own identity is never used for a principal-bearing request.
func (a *API) clientFor(r *http.Request) (dynamic.Interface, error) {
	p := mw.PrincipalFrom(r.Context())
	if p == nil {
		if a.opts.InsecureSkipAuthz && a.opts.Dynamic != nil {
			return a.opts.Dynamic, nil
		}
		return nil, errUnauthenticated
	}
	if a.opts.ClientFor == nil {
		return nil, errors.New("no per-request client factory configured")
	}
	return a.opts.ClientFor(p)
}

// guard enforces authentication on a route and, when admin is set, the
// platform-admin group. Requests without a principal get 401 before any
// Kubernetes call is made.
func (a *API) guard(admin bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.opts.InsecureSkipAuthz {
			next(w, r)
			return
		}
		p := mw.PrincipalFrom(r.Context())
		if p == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="platform-api"`)
			mw.WriteError(w, r, http.StatusUnauthorized, "Unauthorized", errUnauthenticated.Error())
			return
		}
		if err := ValidatePrincipal(p); err != nil {
			mw.WriteError(w, r, http.StatusForbidden, "Forbidden", err.Error())
			return
		}
		if admin && !slices.Contains(p.Groups, a.opts.PlatformAdminGroup) {
			mw.WriteError(w, r, http.StatusForbidden, "Forbidden",
				fmt.Sprintf("this operation requires membership of group %q", a.opts.PlatformAdminGroup))
			return
		}
		next(w, r)
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
	// Every resource route is guarded: no principal => 401. Tenant create and
	// delete additionally require the platform-admin group.
	mux.HandleFunc("GET "+p+"/tenants", a.guard(false, a.tenants.list))
	mux.HandleFunc("POST "+p+"/tenants", a.guard(true, a.tenants.create))
	mux.HandleFunc("GET "+p+"/tenants/{name}", a.guard(false, a.tenants.get))
	mux.HandleFunc("DELETE "+p+"/tenants/{name}", a.guard(true, a.tenants.delete))

	// Per-user permission check ("Can I?"). Evaluated as the caller.
	mux.HandleFunc("POST "+p+"/access-reviews", a.guard(false, a.accessReview))

	// Environments & Workloads (namespaced) plus all-namespace list views.
	for _, rs := range []struct {
		name                   string
		list, get, create, del http.HandlerFunc
	}{
		{"environments", a.environments.list, a.environments.get, a.environments.create, a.environments.delete},
		{"workloads", a.workloads.list, a.workloads.get, a.workloads.create, a.workloads.delete},
	} {
		mux.HandleFunc("GET "+p+"/"+rs.name, a.guard(false, rs.list))
		mux.HandleFunc("GET "+p+"/namespaces/{namespace}/"+rs.name, a.guard(false, rs.list))
		mux.HandleFunc("POST "+p+"/namespaces/{namespace}/"+rs.name, a.guard(false, rs.create))
		mux.HandleFunc("GET "+p+"/namespaces/{namespace}/"+rs.name+"/{name}", a.guard(false, rs.get))
		mux.HandleFunc("DELETE "+p+"/namespaces/{namespace}/"+rs.name+"/{name}", a.guard(false, rs.del))
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
