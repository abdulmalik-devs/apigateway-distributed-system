package gateway

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/max/api-gateway/internal/auth"
	"github.com/max/api-gateway/internal/circuit"
	"github.com/max/api-gateway/internal/config"
	"github.com/max/api-gateway/internal/middleware"
	"github.com/max/api-gateway/internal/proxy"
	"github.com/max/api-gateway/internal/ratelimit"
	"github.com/max/api-gateway/pkg/metrics"
)

// Gateway represents the main API gateway
type Gateway struct {
	config            *config.Config
	configManager     *config.Manager
	router            *gin.Engine
	jwtAuth           *auth.JWTAuth
	rateLimiter       *ratelimit.Manager
	circuitManager    *circuit.Manager
	proxyManager      *proxy.ProxyManager
	middlewareManager *middleware.Manager
	metricsManager    *metrics.Manager
	logger            *zap.Logger
}

// NewGateway creates a new API gateway instance
func NewGateway(
	cfg *config.Config,
	configManager *config.Manager,
	jwtAuth *auth.JWTAuth,
	rateLimiter *ratelimit.Manager,
	circuitManager *circuit.Manager,
	proxyManager *proxy.ProxyManager,
	middlewareManager *middleware.Manager,
	metricsManager *metrics.Manager,
	logger *zap.Logger,
) *Gateway {
	// Set Gin mode based on config
	if cfg.Logging.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	return &Gateway{
		config:            cfg,
		configManager:     configManager,
		router:            router,
		jwtAuth:           jwtAuth,
		rateLimiter:       rateLimiter,
		circuitManager:    circuitManager,
		proxyManager:      proxyManager,
		middlewareManager: middlewareManager,
		metricsManager:    metricsManager,
		logger:            logger,
	}
}

// SetupRoutes sets up all the routes for the gateway
func (g *Gateway) SetupRoutes() error {
	// Apply default middleware chain
	defaultChain := g.middlewareManager.CreateDefaultChain()
	g.router.Use(defaultChain.Build()...)

	// Public routes (no authentication required)
	g.setupPublicRoutes()

	// Auth routes
	g.setupAuthRoutes()

	// Admin routes (authentication + admin role required)
	g.setupAdminRoutes()

	// Protected API routes (authentication required)
	g.setupProtectedRoutes()

	// Catch-all route for proxying to services
	g.setupProxyRoutes()

	g.logger.Info("Routes setup completed")
	return nil
}

// setupPublicRoutes sets up public routes
func (g *Gateway) setupPublicRoutes() {
	public := g.router.Group("/")

	// Health check endpoint
	public.GET("/health", g.healthCheck)

	// Metrics endpoint (if enabled and public)
	if g.config.Monitoring.Prometheus.Enabled {
		public.GET("/metrics", g.metricsManager.GinHandler())
	}

	// Gateway info endpoint
	public.GET("/info", g.gatewayInfo)
}

// setupAuthRoutes sets up authentication routes
func (g *Gateway) setupAuthRoutes() {
	auth := g.router.Group("/auth")

	// Login endpoint (would typically integrate with external auth service)
	auth.POST("/login", g.login)

	// Token refresh endpoint
	authChain := g.middlewareManager.CreateAuthChain()
	auth.POST("/refresh", authChain.Build()[len(authChain.Build())-1], g.refreshToken)

	// Logout endpoint
	auth.POST("/logout", authChain.Build()[len(authChain.Build())-1], g.logout)
}

// setupAdminRoutes sets up admin routes
func (g *Gateway) setupAdminRoutes() {
	adminChain := g.middlewareManager.CreateAdminChain()
	admin := g.router.Group("/admin", adminChain.Build()...)

	// Configuration management
	admin.GET("/config", g.getConfig)
	admin.POST("/config/reload", g.reloadConfig)

	// Service management
	admin.GET("/services", g.getServices)
	admin.POST("/services/:name", g.updateService)
	admin.DELETE("/services/:name", g.deleteService)

	// Statistics and monitoring
	admin.GET("/stats", g.getStats)
	admin.GET("/metrics/detailed", g.getDetailedMetrics)

	// Circuit breaker management
	admin.GET("/circuit-breakers", g.getCircuitBreakers)
	admin.POST("/circuit-breakers/:name/reset", g.resetCircuitBreaker)

	// Rate limiting management
	admin.GET("/rate-limits", g.getRateLimits)
	admin.POST("/rate-limits/:key/reset", g.resetRateLimit)
}

// setupProtectedRoutes sets up protected API routes
func (g *Gateway) setupProtectedRoutes() {
	authChain := g.middlewareManager.CreateAuthChain()
	api := g.router.Group("/api", authChain.Build()...)

	// User profile endpoints
	api.GET("/profile", g.getUserProfile)
	api.PUT("/profile", g.updateUserProfile)

	// Token validation endpoint
	api.GET("/validate", g.validateToken)
}

// setupProxyRoutes sets up proxy routes for services
func (g *Gateway) setupProxyRoutes() {
	// Catch-all proxy route
	g.router.NoRoute(g.proxyRequest)
}

// Router returns the Gin router
func (g *Gateway) Router() *gin.Engine {
	return g.router
}

// Route handlers

// healthCheck handles health check requests
func (g *Gateway) healthCheck(c *gin.Context) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": "2024-01-01T00:00:00Z", // Would use actual timestamp
		"version":   "1.0.0",
		"services":  g.getServiceHealth(),
	}

	c.JSON(http.StatusOK, health)
}

// gatewayInfo returns gateway information
func (g *Gateway) gatewayInfo(c *gin.Context) {
	info := map[string]interface{}{
		"name":        "API Gateway",
		"version":     "1.0.0",
		"description": "Production-grade API Gateway",
		"build_date":  "2024-01-01",
		"go_version":  "1.21",
		"features": []string{
			"JWT Authentication",
			"Rate Limiting",
			"Load Balancing",
			"Circuit Breaker",
			"Caching",
			"Metrics",
		},
	}

	c.JSON(http.StatusOK, info)
}

// login handles login requests
func (g *Gateway) login(c *gin.Context) {
	var loginReq struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&loginReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Integrate with actual authentication service
	// For now, accept any credentials for demo purposes
	if loginReq.Username == "admin" && loginReq.Password == "password" {
		token, err := g.jwtAuth.GenerateToken(
			"user123",
			loginReq.Username,
			"admin@example.com",
			[]string{"admin", "user"},
			map[string]string{"department": "engineering"},
		)
		if err != nil {
			g.logger.Error("Failed to generate token", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token":   token,
			"type":    "Bearer",
			"expires": g.config.Auth.JWT.ExpirationTime.String(),
		})
		return
	}

	c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
}

// refreshToken handles token refresh requests
func (g *Gateway) refreshToken(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	token, err := g.jwtAuth.ExtractTokenFromHeader(authHeader)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header"})
		return
	}

	newToken, err := g.jwtAuth.RefreshToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token refresh failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":   newToken,
		"type":    "Bearer",
		"expires": g.config.Auth.JWT.ExpirationTime.String(),
	})
}

// logout handles logout requests
func (g *Gateway) logout(c *gin.Context) {
	// In a production system, you might want to blacklist the token
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

// Admin handlers

// getConfig returns the current configuration
func (g *Gateway) getConfig(c *gin.Context) {
	c.JSON(http.StatusOK, g.config)
}

// reloadConfig reloads the configuration
func (g *Gateway) reloadConfig(c *gin.Context) {
	if err := g.configManager.Reload(); err != nil {
		g.logger.Error("Failed to reload configuration", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reload configuration"})
		return
	}

	g.config = g.configManager.Get()
	c.JSON(http.StatusOK, gin.H{"message": "Configuration reloaded successfully"})
}

// getServices returns all registered services
func (g *Gateway) getServices(c *gin.Context) {
	services := g.proxyManager.ListServices()
	c.JSON(http.StatusOK, gin.H{"services": services})
}

// updateService updates a service configuration
func (g *Gateway) updateService(c *gin.Context) {
	name := c.Param("name")

	var serviceConfig config.ServiceConfig
	if err := c.ShouldBindJSON(&serviceConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := g.proxyManager.UpdateService(name, &serviceConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Service updated successfully"})
}

// deleteService removes a service
func (g *Gateway) deleteService(c *gin.Context) {
	name := c.Param("name")
	g.proxyManager.RemoveService(name)
	c.JSON(http.StatusOK, gin.H{"message": "Service removed successfully"})
}

// getStats returns gateway statistics
func (g *Gateway) getStats(c *gin.Context) {
	stats := map[string]interface{}{
		"gateway":         g.metricsManager.GetStats(),
		"rate_limiter":    g.rateLimiter.GetStats(),
		"circuit_breaker": g.circuitManager.GetStats(),
		"proxy":           g.proxyManager.GetStats(),
	}

	c.JSON(http.StatusOK, stats)
}

// getDetailedMetrics returns detailed metrics
func (g *Gateway) getDetailedMetrics(c *gin.Context) {
	// This would return more detailed metrics in a production system
	c.JSON(http.StatusOK, gin.H{"message": "Detailed metrics endpoint"})
}

// getCircuitBreakers returns circuit breaker status
func (g *Gateway) getCircuitBreakers(c *gin.Context) {
	states := g.circuitManager.GetAllStates()
	c.JSON(http.StatusOK, gin.H{"circuit_breakers": states})
}

// resetCircuitBreaker resets a circuit breaker
func (g *Gateway) resetCircuitBreaker(c *gin.Context) {
	name := c.Param("name")
	if err := g.circuitManager.ResetBreaker(name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Circuit breaker reset successfully"})
}

// getRateLimits returns rate limiting information
func (g *Gateway) getRateLimits(c *gin.Context) {
	info := g.rateLimiter.GetStats()
	c.JSON(http.StatusOK, info)
}

// resetRateLimit resets rate limiting for a key
func (g *Gateway) resetRateLimit(c *gin.Context) {
	key := c.Param("key")
	if err := g.rateLimiter.Reset(key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Rate limit reset successfully"})
}

// Protected route handlers

// getUserProfile returns user profile information
func (g *Gateway) getUserProfile(c *gin.Context) {
	claims, _ := c.Get("user")
	userClaims := claims.(*auth.Claims)

	profile := map[string]interface{}{
		"user_id":  userClaims.UserID,
		"username": userClaims.Username,
		"email":    userClaims.Email,
		"roles":    userClaims.Roles,
		"metadata": userClaims.Metadata,
	}

	c.JSON(http.StatusOK, profile)
}

// updateUserProfile updates user profile
func (g *Gateway) updateUserProfile(c *gin.Context) {
	// Implementation would update user profile in database
	c.JSON(http.StatusOK, gin.H{"message": "Profile updated successfully"})
}

// validateToken validates the current token
func (g *Gateway) validateToken(c *gin.Context) {
	claims, _ := c.Get("user")
	userClaims := claims.(*auth.Claims)

	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"user_id": userClaims.UserID,
		"expires": userClaims.ExpiresAt.Time,
	})
}

// proxyRequest handles proxying requests to backend services
func (g *Gateway) proxyRequest(c *gin.Context) {
	// Extract service name from path
	path := c.Request.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if len(parts) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found"})
		return
	}

	serviceName := parts[0]

	// Get proxy for service
	serviceProxy := g.proxyManager.GetProxy(serviceName)
	if serviceProxy == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Service not found: " + serviceName})
		return
	}

	// Execute with circuit breaker if configured
	circuitBreaker := g.circuitManager.GetBreaker(serviceName)
	if circuitBreaker != nil {
		err := circuitBreaker.Call(func() error {
			serviceProxy.ServeHTTP(c.Writer, c.Request)
			return nil
		})

		if err != nil {
			g.logger.Error("Circuit breaker error", zap.Error(err), zap.String("service", serviceName))
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Service temporarily unavailable"})
			return
		}
	} else {
		serviceProxy.ServeHTTP(c.Writer, c.Request)
	}
}

// Helper methods

// getServiceHealth returns health status of all services
func (g *Gateway) getServiceHealth() map[string]string {
	services := g.proxyManager.ListServices()
	health := make(map[string]string)

	for _, service := range services {
		// Check circuit breaker state
		if breaker := g.circuitManager.GetBreaker(service); breaker != nil {
			if breaker.IsOpen() {
				health[service] = "unhealthy"
			} else {
				health[service] = "healthy"
			}
		} else {
			health[service] = "healthy"
		}
	}

	return health
}

