// Command policy-engine runs the platform ValidatingWebhook. It evaluates
// Rego policies against Tenant, Environment and Workload objects and serves
// Prometheus metrics on a separate port.
package main

import (
	"context"
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

	engine "github.com/GitOpsHub/platform-engineering-on-kubernetes/code/policy-engine"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/policy-engine/admission"
	"github.com/GitOpsHub/platform-engineering-on-kubernetes/code/policy-engine/metrics"
)

// Injected at build time with -ldflags "-X main.version=...".
var version = "dev"

type options struct {
	addr, metricsAddr, certFile, keyFile string
	allowedRegistries                    string
	maxReplicas                          int
	logLevel                             string
}

func parseFlags(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("policy-engine", flag.ContinueOnError)
	fs.StringVar(&o.addr, "addr", ":8443", "HTTPS listen address for /validate")
	fs.StringVar(&o.metricsAddr, "metrics-addr", ":8080", "Plain HTTP address for /metrics and /healthz")
	fs.StringVar(&o.certFile, "tls-cert", "/tls/tls.crt", "TLS certificate (PEM)")
	fs.StringVar(&o.keyFile, "tls-key", "/tls/tls.key", "TLS private key (PEM)")
	fs.StringVar(&o.allowedRegistries, "allowed-registries", "ghcr.io/acme/", "Comma-separated image prefixes Workloads may use")
	fs.IntVar(&o.maxReplicas, "max-replicas", 10, "Replica ceiling for Workloads")
	fs.StringVar(&o.logLevel, "log-level", "info", "debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	return o, nil
}

func main() {
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "policy-engine:", err)
		os.Exit(1)
	}
}

func run(o options) error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(o.logLevel)); err != nil {
		return fmt.Errorf("invalid --log-level: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	logger.Info("starting policy-engine", "version", version)

	var registries []string
	for _, r := range strings.Split(o.allowedRegistries, ",") {
		if r = strings.TrimSpace(r); r != "" {
			registries = append(registries, r)
		}
	}
	eng, err := engine.NewEmbedded(engine.Config{AllowedRegistries: registries, MaxReplicas: o.maxReplicas})
	if err != nil {
		return fmt.Errorf("building policy engine: %w", err)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	m := metrics.New(reg)

	certs, err := admission.NewCertReloader(o.certFile, o.keyFile)
	if err != nil {
		return fmt.Errorf("TLS: %w", err)
	}

	webhookMux := http.NewServeMux()
	webhookMux.Handle("POST /validate", &admission.Handler{Engine: eng, Metrics: m, Logger: logger})
	webhookMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	webhook := &http.Server{
		Addr:              o.addr,
		Handler:           webhookMux,
		TLSConfig:         certs.TLSConfig(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	opsMux := http.NewServeMux()
	opsMux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	opsMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	ops := &http.Server{Addr: o.metricsAddr, Handler: opsMux, ReadHeaderTimeout: 5 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 2)
	go func() {
		logger.Info("webhook listening", "addr", o.addr, "registries", registries, "maxReplicas", o.maxReplicas)
		errCh <- webhook.ListenAndServeTLS("", "") // certificates come from TLSConfig
	}()
	go func() {
		logger.Info("ops listening", "addr", o.metricsAddr)
		errCh <- ops.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := webhook.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return ops.Shutdown(shutdownCtx)
}
