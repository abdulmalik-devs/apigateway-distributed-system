package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/max/api-gateway/internal/auth"
	"github.com/max/api-gateway/internal/circuit"
	"github.com/max/api-gateway/internal/config"
	"github.com/max/api-gateway/internal/gateway"
	"github.com/max/api-gateway/internal/middleware"
	"github.com/max/api-gateway/internal/proxy"
	"github.com/max/api-gateway/internal/ratelimit"
	"github.com/max/api-gateway/pkg/metrics"
)

const (
	defaultConfigPath = "configs/config.yaml"
	shutdownTimeout   = 30 * time.Second
)

func main() {
	// Initialize logger
	logger, err := initLogger()
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting API Gateway")

	// Load configuration
	configPath := getConfigPath()
	configManager := config.NewManager(logger)
	if err := configManager.Load(configPath); err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	cfg := configManager.Get()
	logger.Info("Configuration loaded", zap.String("config_path", configPath))

	// Initialize Redis client
	redisClient := initRedis(cfg.Redis, logger)
	if redisClient != nil {
		defer redisClient.Close()
		logger.Info("Redis client initialized")
	}

	// Initialize components
	metricsManager := metrics.NewManager(logger)
	jwtAuth := auth.NewJWTAuth(
		cfg.Auth.JWT.Secret,
		cfg.Auth.JWT.ExpirationTime,
		cfg.Auth.JWT.RefreshTime,
		cfg.Auth.JWT.Issuer,
		cfg.Auth.JWT.Audience,
		cfg.Auth.JWT.Algorithm,
		logger,
	)
	rateLimiter := ratelimit.NewManager(&cfg.RateLimit, redisClient, logger)
	circuitManager := circuit.NewManager(logger)
	proxyManager := proxy.NewProxyManager(logger, metricsManager)
	middlewareManager := middleware.NewManager(cfg, jwtAuth, rateLimiter, metricsManager, logger)

	// Initialize gateway
	gw := gateway.NewGateway(
		cfg,
		configManager,
		jwtAuth,
		rateLimiter,
		circuitManager,
		proxyManager,
		middlewareManager,
		metricsManager,
		logger,
	)

	// Setup routes
	if err := gw.SetupRoutes(); err != nil {
		logger.Fatal("Failed to setup routes", zap.Error(err))
	}

	// Initialize services from configuration
	if err := initializeServices(cfg, proxyManager, circuitManager, logger, metricsManager); err != nil {
		logger.Fatal("Failed to initialize services", zap.Error(err))
	}

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      gw.Router(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start metrics server if enabled
	var metricsServer *http.Server
	if cfg.Monitoring.Prometheus.Enabled {
		metricsServer = startMetricsServer(cfg.Monitoring.Prometheus, metricsManager, logger)
	}

	// Start configuration watcher
	go configManager.Watch()

	// Start server
	go func() {
		logger.Info("Starting HTTP server",
			zap.String("address", server.Addr),
			zap.Bool("tls_enabled", cfg.Server.TLS.Enabled))

		var err error
		if cfg.Server.TLS.Enabled {
			err = server.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
		} else {
			err = server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server startup failed", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Shutdown main server
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	// Shutdown metrics server
	if metricsServer != nil {
		if err := metricsServer.Shutdown(ctx); err != nil {
			logger.Error("Metrics server forced to shutdown", zap.Error(err))
		}
	}

	logger.Info("Server shutdown complete")
}

// initLogger initializes the logger
func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}

// getConfigPath returns the configuration file path
func getConfigPath() string {
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		return path
	}
	return defaultConfigPath
}

// initRedis initializes Redis client
func initRedis(cfg config.RedisConfig, logger *zap.Logger) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		logger.Warn("Failed to connect to Redis", zap.Error(err))
		client.Close()
		return nil
	}

	return client
}

// initializeServices initializes services from configuration
func initializeServices(cfg *config.Config, proxyManager *proxy.ProxyManager, circuitManager *circuit.Manager, logger *zap.Logger, metricsMgr *metrics.Manager) error {
	for serviceName, serviceConfig := range cfg.Routing.Services {
		// Create circuit breaker for service
		if serviceConfig.CircuitBreaker.Enabled {
			circuitManager.CreateBreaker(serviceName, serviceConfig.CircuitBreaker)
		}

		// Add service to proxy manager
		if err := proxyManager.AddService(serviceName, &serviceConfig); err != nil {
			return fmt.Errorf("failed to add service %s: %w", serviceName, err)
		}

		logger.Info("Service initialized",
			zap.String("service", serviceName),
			zap.Strings("urls", serviceConfig.URLs),
			zap.String("load_balancer", serviceConfig.LoadBalancer))
	}

	return nil
}

// startMetricsServer starts the Prometheus metrics server
func startMetricsServer(cfg config.PrometheusConfig, metricsManager *metrics.Manager, logger *zap.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.Handle(cfg.Path, metricsManager.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	go func() {
		logger.Info("Starting metrics server",
			zap.String("address", server.Addr),
			zap.String("path", cfg.Path))

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Metrics server startup failed", zap.Error(err))
		}
	}()

	return server
}
