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
		logger.Warn("authentication DISABLED: no --tokens-file configured")
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	opts := handlers.Options{
		Dynamic:         dyn,
		Discovery:       disc,
		Build:           handlers.BuildInfo{Version: Version, Commit: Commit, BuildTime: BuildTime},
		EnableProfiling: cfg.EnableProfiling,
		OpenAPISpec:     openAPISpec,
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
