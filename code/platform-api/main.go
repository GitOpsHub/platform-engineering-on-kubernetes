// Command platform-api is the REST front door of the Internal Developer
// Platform. It exposes Tenants, Environments and Workloads
// (platform.mycompany.io/v1alpha1 CRDs) over a small, stable JSON API so
// portals and CLIs don't need direct Kubernetes access.
package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/handlers"
	mw "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/platform-api/middleware"
)

// Version information (set via -ldflags "-X main.Version=...").
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

//go:embed openapi.yaml
var openAPISpec []byte

// Config holds command-line configuration.
type Config struct {
	Addr            string
	Kubeconfig      string
	LogLevel        string
	EnableMetrics   bool
	EnableProfiling bool
	TokensFile      string
	CORSOrigins     string
	CORSCredentials bool
	AdminGroup      string
	// ImpersonateGroups is the allowlist of groups platform-api may impersonate.
	// Groups outside it are dropped before impersonation. It mirrors the groups
	// listed in the chart's impersonator ClusterRole.
	ImpersonateGroups string
	InsecureSkipAuthz bool
}

func main() {
	cfg := parseFlags(os.Args[1:])
	logger := newLogger(cfg.LogLevel)
	if err := run(cfg, logger); err != nil {
		logger.Error("platform-api exited with error", "error", err)
		os.Exit(1)
	}
}

func parseFlags(args []string) Config {
	var c Config
	fs := flag.NewFlagSet("platform-api", flag.ExitOnError)
	fs.StringVar(&c.Addr, "addr", envOr("PLATFORM_API_ADDR", ":8080"), "HTTP listen address")
	fs.StringVar(&c.Kubeconfig, "kubeconfig", os.Getenv("KUBECONFIG"), "Path to kubeconfig (empty for in-cluster config)")
	fs.StringVar(&c.LogLevel, "log-level", envOr("PLATFORM_API_LOG_LEVEL", "info"), "Log level: debug, info, warn, error")
	fs.BoolVar(&c.EnableMetrics, "metrics", true, "Serve Prometheus metrics on /metrics")
	fs.BoolVar(&c.EnableProfiling, "profiling", false, "Serve pprof on /debug/pprof/ (do not expose publicly)")
	fs.StringVar(&c.TokensFile, "tokens-file", os.Getenv("PLATFORM_API_TOKENS_FILE"),
		"File of 'token,subject[,group1;group2]' lines for bearer auth; empty disables auth (dev only)")
	fs.StringVar(&c.CORSOrigins, "cors-origins", os.Getenv("PLATFORM_API_CORS_ORIGINS"), "Comma-separated allowed CORS origins")
	fs.BoolVar(&c.CORSCredentials, "cors-credentials", false, "Send Access-Control-Allow-Credentials for allowed (non-wildcard) origins")
	fs.StringVar(&c.AdminGroup, "platform-admin-group", envOr("PLATFORM_API_ADMIN_GROUP", handlers.DefaultPlatformAdminGroup),
		"Group whose members may create and delete Tenants")
	// Deliberately not read from the environment: it must be an explicit choice.
	fs.StringVar(&c.ImpersonateGroups, "impersonate-groups", envOr("PLATFORM_API_IMPERSONATE_GROUPS", handlers.DefaultPlatformAdminGroup),
		"Comma-separated allowlist of groups that may be impersonated. system:* groups are refused. The admin group is always included.")
	fs.BoolVar(&c.InsecureSkipAuthz, "insecure-skip-authz", false,
		"DEV ONLY: allow unauthenticated requests (served with platform-api's own credentials) and skip the admin-group check")
	_ = fs.Parse(args)
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

func run(cfg Config, logger *slog.Logger) error {
	logger.Info("starting platform-api", "version", Version, "commit", Commit, "buildTime", BuildTime)

	restCfg, err := restConfig(cfg.Kubeconfig)
	if err != nil {
		return fmt.Errorf("kubernetes config: %w", err)
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("dynamic client: %w", err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("discovery client: %w", err)
	}

	var verifier mw.TokenVerifier
	if cfg.TokensFile != "" {
		tokens, err := loadTokens(cfg.TokensFile)
		if err != nil {
			return err
		}
		verifier = tokens
	} else {
		logger.Warn("no --tokens-file configured: resource requests will be rejected with 401 " +
			"unless --insecure-skip-authz is set (development only)")
	}
	if cfg.InsecureSkipAuthz {
		logger.Warn("INSECURE MODE: --insecure-skip-authz is set. Unauthenticated requests use platform-api's own " +
			"ServiceAccount, and the platform-admin check is disabled. NEVER run this in a shared cluster.")
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	impersonateAllowlist, err := buildAllowlist(cfg.ImpersonateGroups)
	if err != nil {
		return err
	}
	opts := handlers.Options{
		ClientFor:          impersonatingClientFor(restCfg, impersonateAllowlist, logger),
		Dynamic:            dyn,
		Discovery:          disc,
		Build:              handlers.BuildInfo{Version: Version, Commit: Commit, BuildTime: BuildTime},
		EnableProfiling:    cfg.EnableProfiling,
		OpenAPISpec:        openAPISpec,
		PlatformAdminGroup: cfg.AdminGroup,
		InsecureSkipAuthz:  cfg.InsecureSkipAuthz,
	}
	if cfg.EnableMetrics {
		opts.MetricsHandler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
	}

	handler := mw.Chain(handlers.New(opts).Routes(),
		mw.RequestID(),
		mw.Logging(logger),
		mw.Recover(logger),
		mw.CORS(mw.CORSConfig{AllowedOrigins: splitCSV(cfg.CORSOrigins), AllowCredentials: cfg.CORSCredentials, MaxAge: 10 * time.Minute}),
		mw.Auth(mw.AuthConfig{Verifier: verifier, PublicPaths: []string{"/healthz", "/readyz", "/version", "/metrics", "/openapi.yaml"}}),
		mw.Metrics(reg), // innermost: needs r.Pattern set by the mux
	)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second, // pprof CPU profiles default to 30s
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

// buildAllowlist parses the impersonation allowlist. It refuses system:* entries.
// The list is exactly what the chart renders into impersonation.groups.
func buildAllowlist(raw string) ([]string, error) {
	allowed := splitCSV(raw)
	for _, g := range allowed {
		if strings.HasPrefix(g, "system:") {
			return nil, fmt.Errorf("--impersonate-groups: %q is a reserved system group and cannot be impersonated", g)
		}
	}
	return allowed, nil
}

// impersonatingClientFor returns the per-request client factory. Each call
// refuses reserved identities, drops groups that are not allowlisted, and
// builds a dynamic client that impersonates the authenticated caller. Every
// impersonation is written to the audit log.
func impersonatingClientFor(base *rest.Config, allowed []string, logger *slog.Logger) handlers.ClientFactory {
	return func(p *mw.Principal) (dynamic.Interface, error) {
		if p == nil || p.Subject == "" {
			return nil, errors.New("refusing to build a client without an authenticated subject")
		}
		if err := handlers.ValidatePrincipal(p); err != nil {
			logger.Warn("refused impersonation of a reserved identity", "audit", true,
				"subject", p.Subject, "groups", p.Groups)
			return nil, err
		}
		c, dropped := impersonationConfig(base, p, allowed)
		logger.Info("impersonating", "audit", true, "subject", p.Subject,
			"groups", c.Impersonate.Groups, "droppedGroups", dropped)
		return dynamic.NewForConfig(c)
	}
}

// impersonationConfig returns a copy of base that impersonates p with only the
// allowlisted groups. It also returns the groups that were dropped. base itself
// is never modified, so the server's own identity is untouched. Subjects are not
// filtered here: the users rule of the ClusterRole (resourceNames) limits them.
func impersonationConfig(base *rest.Config, p *mw.Principal, allowed []string) (*rest.Config, []string) {
	var kept, dropped []string
	for _, g := range p.Groups {
		if slices.Contains(allowed, g) && !strings.HasPrefix(g, "system:") {
			kept = append(kept, g)
		} else {
			dropped = append(dropped, g)
		}
	}
	c := rest.CopyConfig(base)
	c.Impersonate = rest.ImpersonationConfig{UserName: p.Subject, Groups: kept}
	return c, dropped
}

func restConfig(kubeconfig string) (*rest.Config, error) {
	var (
		c   *rest.Config
		err error
	)
	if kubeconfig != "" {
		c, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		c, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, err
	}
	c.QPS, c.Burst, c.Timeout = 50, 100, 30*time.Second
	c.UserAgent = "platform-api/" + Version
	return c, nil
}

// loadTokens parses "token,subject[,group1;group2]" lines; '#' starts a comment.
func loadTokens(path string) (mw.StaticTokens, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading tokens file: %w", err)
	}
	return parseTokens(string(raw))
}

func parseTokens(s string) (mw.StaticTokens, error) {
	tokens := mw.StaticTokens{}
	for i, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("tokens file line %d: want 'token,subject[,groups]'", i+1)
		}
		p := mw.Principal{Subject: strings.TrimSpace(parts[1])}
		if len(parts) > 2 {
			p.Groups = splitList(parts[2], ";")
		}
		tokens[strings.TrimSpace(parts[0])] = p
	}
	if len(tokens) == 0 {
		return nil, errors.New("tokens file contains no tokens")
	}
	return tokens, nil
}

func splitCSV(s string) []string { return splitList(s, ",") }

func splitList(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
