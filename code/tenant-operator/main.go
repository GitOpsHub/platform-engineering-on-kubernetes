package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	platformv1 "github.com/myorg/platform-engineering/api/v1"
	"github.com/myorg/platform-engineering/internal/controller"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var logLevel string
	var webhookPort int

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true, "Enable leader election for controller manager.")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "Webhook server port")
	flag.Parse()

	// Setup logger
	logger := setupLogger(logLevel)
	ctrl.SetLogger(logger)

	logger.Info("Starting Tenant Operator",
		"version", version,
		"commit", commit,
		"buildTime", buildTime,
	)

	// Get config
	config := ctrl.GetConfigOrDie()
	config.QPS = 100
	config.Burst = 200

	// Create manager
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                 platformv1.Scheme,
		Metrics:                server.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "tenant-operator-leader-election",
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				"platform-system": {},
			},
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: webhookPort,
		}),
	})
	if err != nil {
		logger.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	// Setup controllers
	if err := setupControllers(mgr, logger); err != nil {
		logger.Error(err, "Unable to setup controllers")
		os.Exit(1)
	}

	// Setup webhooks
	if err := setupWebhooks(mgr); err != nil {
		logger.Error(err, "Unable to setup webhooks")
		os.Exit(1)
	}

	// Add health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logger.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logger.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	// Start manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("Received shutdown signal")
		cancel()
	}()

	logger.Info("Starting manager")
	if err := mgr.Start(ctx); err != nil {
		logger.Error(err, "Manager exited with error")
		os.Exit(1)
	}
}

func setupLogger(level string) logr.Logger {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	opts := zap.Options{
		Development: false,
		Level:       zapLevel,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	return zap.New(zap.UseFlagOptions(&opts))
}

func setupControllers(mgr ctrl.Manager, logger logr.Logger) error {
	// Tenant Controller
	tenantReconciler := &controller.TenantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    logger.WithName("controllers").WithName("Tenant"),
	}
	if err := tenantReconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to create Tenant controller: %w", err)
	}

	// TenantQuota Controller
	quotaReconciler := &controller.TenantQuotaReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    logger.WithName("controllers").WithName("TenantQuota"),
	}
	if err := quotaReconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to create TenantQuota controller: %w", err)
	}

	// Namespace Controller (for tenant namespace management)
	nsReconciler := &controller.NamespaceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    logger.WithName("controllers").WithName("Namespace"),
	}
	if err := nsReconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to create Namespace controller: %w", err)
	}

	return nil
}

func setupWebhooks(mgr ctrl.Manager) error {
	// Tenant validation webhook
	if err := (&platformv1.Tenant{}).SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("failed to setup Tenant webhook: %w", err)
	}

	// TenantQuota validation webhook
	if err := (&platformv1.TenantQuota{}).SetupWebhookWithManager(mgr); err != nil {
		return fmt.Errorf("failed to setup TenantQuota webhook: %w", err)
	}

	return nil
}
