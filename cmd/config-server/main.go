package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/max/api-gateway/internal/config"
)

const (
	defaultPort       = 8090
	defaultConfigPath = "configs/config.yaml"
	shutdownTimeout   = 30 * time.Second
)

type ConfigServer struct {
	configManager *config.Manager
	router        *gin.Engine
	logger        *zap.Logger
}

func main() {
	// Initialize logger
	logger, err := initLogger()
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting Configuration Server")

	// Load configuration
	configPath := getConfigPath()
	configManager := config.NewManager(logger)
	if err := configManager.Load(configPath); err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Create config server
	server := &ConfigServer{
		configManager: configManager,
		router:        gin.New(),
		logger:        logger,
	}

	// Setup routes
	server.setupRoutes()

	// Create HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", defaultPort),
		Handler:      server.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		logger.Info("Starting Configuration Server", zap.String("address", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server startup failed", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down Configuration Server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Configuration Server shutdown complete")
}

func (cs *ConfigServer) setupRoutes() {
	// Middleware
	cs.router.Use(gin.Logger())
	cs.router.Use(gin.Recovery())

	// API routes
	api := cs.router.Group("/api/v1")

	// Configuration endpoints
	api.GET("/config", cs.getConfig)
	api.PUT("/config", cs.updateConfig)
	api.POST("/config/reload", cs.reloadConfig)
	api.GET("/config/validate", cs.validateConfig)

	// Health check
	cs.router.GET("/health", cs.healthCheck)

	// Metrics
	cs.router.GET("/metrics", cs.getMetrics)

	cs.logger.Info("Configuration Server routes setup completed")
}

// Route handlers

func (cs *ConfigServer) getConfig(c *gin.Context) {
	config := cs.configManager.Get()
	c.JSON(http.StatusOK, gin.H{
		"config":    config,
		"timestamp": time.Now().UTC(),
	})
}

func (cs *ConfigServer) updateConfig(c *gin.Context) {
	var newConfig config.Config
	if err := c.ShouldBindJSON(&newConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid configuration format",
			"details": err.Error(),
		})
		return
	}

	// TODO: Validate configuration
	// TODO: Save configuration to file
	// TODO: Notify gateways of configuration change

	c.JSON(http.StatusOK, gin.H{
		"message":   "Configuration updated successfully",
		"timestamp": time.Now().UTC(),
	})
}

func (cs *ConfigServer) reloadConfig(c *gin.Context) {
	if err := cs.configManager.Reload(); err != nil {
		cs.logger.Error("Failed to reload configuration", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to reload configuration",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Configuration reloaded successfully",
		"timestamp": time.Now().UTC(),
	})
}

func (cs *ConfigServer) validateConfig(c *gin.Context) {
	var configToValidate config.Config
	if err := c.ShouldBindJSON(&configToValidate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"valid":   false,
			"error":   "Invalid JSON format",
			"details": err.Error(),
		})
		return
	}

	// TODO: Implement proper validation logic
	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"message": "Configuration is valid",
	})
}

func (cs *ConfigServer) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"service":   "config-server",
		"version":   "1.0.0",
		"timestamp": time.Now().UTC(),
	})
}

func (cs *ConfigServer) getMetrics(c *gin.Context) {
	// TODO: Implement metrics collection
	c.JSON(http.StatusOK, gin.H{
		"metrics": gin.H{
			"config_reloads": 0,
			"config_updates": 0,
			"uptime_seconds": time.Since(time.Now()).Seconds(),
		},
	})
}

// Helper functions

func initLogger() (*zap.Logger, error) {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	return config.Build()
}

func getConfigPath() string {
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		return path
	}
	return defaultConfigPath
}

