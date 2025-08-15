package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/max/api-gateway/internal/auth"
	"github.com/max/api-gateway/internal/config"
	"github.com/max/api-gateway/internal/ratelimit"
	"github.com/max/api-gateway/pkg/metrics"
)

// ContextKey represents a context key type
type ContextKey string

const (
	// UserContextKey is the context key for user information
	UserContextKey ContextKey = "user"
	// RequestIDKey is the context key for request ID
	RequestIDKey ContextKey = "request_id"
	// StartTimeKey is the context key for request start time
	StartTimeKey ContextKey = "start_time"
)

// Chain represents a middleware chain
type Chain struct {
	middlewares []gin.HandlerFunc
	logger      *zap.Logger
}

// NewChain creates a new middleware chain
func NewChain(logger *zap.Logger) *Chain {
	return &Chain{
		middlewares: make([]gin.HandlerFunc, 0),
		logger:      logger,
	}
}

// Use adds a middleware to the chain
func (c *Chain) Use(middleware gin.HandlerFunc) *Chain {
	c.middlewares = append(c.middlewares, middleware)
	return c
}

// Build returns the middleware chain as a slice
func (c *Chain) Build() []gin.HandlerFunc {
	return c.middlewares
}

// Manager manages middleware configuration and creation
type Manager struct {
	config      *config.Config
	jwtAuth     *auth.JWTAuth
	rateLimiter *ratelimit.Manager
	metrics     *metrics.Manager
	logger      *zap.Logger
}

// NewManager creates a new middleware manager
func NewManager(cfg *config.Config, jwtAuth *auth.JWTAuth, rateLimiter *ratelimit.Manager, metrics *metrics.Manager, logger *zap.Logger) *Manager {
	return &Manager{
		config:      cfg,
		jwtAuth:     jwtAuth,
		rateLimiter: rateLimiter,
		metrics:     metrics,
		logger:      logger,
	}
}

// CreateDefaultChain creates the default middleware chain
func (m *Manager) CreateDefaultChain() *Chain {
	chain := NewChain(m.logger)

	// Core middlewares (always applied)
	chain.Use(m.RequestID())
	chain.Use(m.Logger())
	chain.Use(m.Recovery())
	chain.Use(m.Metrics())

	// CORS middleware if enabled
	if m.config.Server.CORS.Enabled {
		chain.Use(m.CORS())
	}

	// Rate limiting middleware if enabled
	if m.config.RateLimit.Enabled {
		chain.Use(m.RateLimit())
	}

	// Authentication middleware (applied to protected routes)
	// This is typically applied selectively in routing

	return chain
}

// CreateAuthChain creates a chain with authentication
func (m *Manager) CreateAuthChain() *Chain {
	chain := m.CreateDefaultChain()
	chain.Use(m.JWTAuth())
	return chain
}

// CreateAdminChain creates a chain for admin endpoints
func (m *Manager) CreateAdminChain() *Chain {
	chain := m.CreateDefaultChain()
	chain.Use(m.JWTAuth())
	chain.Use(m.RequireRole("admin"))
	return chain
}

// RequestID middleware generates a unique request ID
func (m *Manager) RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := generateRequestID()
		c.Set(string(RequestIDKey), requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// Logger middleware logs HTTP requests
func (m *Manager) Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Set(string(StartTimeKey), start)

		// Process request
		c.Next()

		// Log request
		duration := time.Since(start)
		m.logger.Info("HTTP Request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", duration),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.String("request_id", c.GetString(string(RequestIDKey))))
	}
}

// Recovery middleware recovers from panics
func (m *Manager) Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				m.logger.Error("Panic recovered",
					zap.Any("error", err),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
					zap.String("request_id", c.GetString(string(RequestIDKey))))

				c.JSON(http.StatusInternalServerError, gin.H{
					"error":      "Internal Server Error",
					"request_id": c.GetString(string(RequestIDKey)),
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}

// Metrics middleware collects request metrics
func (m *Manager) Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Process request
		c.Next()

		// Record metrics
		duration := time.Since(start)
		if m.metrics != nil {
			m.metrics.RecordHTTPRequest(
				c.Request.Method,
				c.Request.URL.Path,
				c.Writer.Status(),
				duration,
			)
		}
	}
}

// CORS middleware handles Cross-Origin Resource Sharing
func (m *Manager) CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		corsConfig := m.config.Server.CORS

		// Set CORS headers
		if len(corsConfig.AllowedOrigins) > 0 {
			origin := c.Request.Header.Get("Origin")
			if origin != "" && contains(corsConfig.AllowedOrigins, origin) {
				c.Header("Access-Control-Allow-Origin", origin)
			} else if contains(corsConfig.AllowedOrigins, "*") {
				c.Header("Access-Control-Allow-Origin", "*")
			}
		}

		if len(corsConfig.AllowedMethods) > 0 {
			c.Header("Access-Control-Allow-Methods", joinStrings(corsConfig.AllowedMethods, ", "))
		}

		if len(corsConfig.AllowedHeaders) > 0 {
			c.Header("Access-Control-Allow-Headers", joinStrings(corsConfig.AllowedHeaders, ", "))
		}

		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// RateLimit middleware applies rate limiting
func (m *Manager) RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Determine rate limit key
		var key string

		// Try to get user ID from JWT token
		if claims, exists := c.Get("user"); exists {
			if userClaims, ok := claims.(*auth.Claims); ok {
				key = userClaims.UserID
			}
		}

		// Fall back to IP address
		if key == "" {
			key = c.ClientIP()
		}

		// Check rate limit
		allowed, err := m.rateLimiter.CheckLimit(key)
		if err != nil {
			m.logger.Error("Rate limit check failed",
				zap.Error(err),
				zap.String("key", key))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Rate limit check failed",
			})
			c.Abort()
			return
		}

		if !allowed {
			m.logger.Warn("Rate limit exceeded",
				zap.String("key", key),
				zap.String("ip", c.ClientIP()))

			c.Header("X-RateLimit-Limit", "100") // This should come from config
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", "60")

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"retry_after": 60,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// JWTAuth middleware validates JWT tokens
func (m *Manager) JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header required",
			})
			c.Abort()
			return
		}

		token, err := m.jwtAuth.ExtractTokenFromHeader(authHeader)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authorization header",
			})
			c.Abort()
			return
		}

		claims, err := m.jwtAuth.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid token",
			})
			c.Abort()
			return
		}

		// Store user claims in context
		c.Set("user", claims)
		c.Set(string(UserContextKey), claims)

		c.Next()
	}
}

// RequireRole middleware checks for required roles
func (m *Manager) RequireRole(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "User not authenticated",
			})
			c.Abort()
			return
		}

		userClaims, ok := claims.(*auth.Claims)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Invalid user claims",
			})
			c.Abort()
			return
		}

		if !m.jwtAuth.HasRole(userClaims, requiredRole) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Insufficient permissions",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAnyRole middleware checks for any of the required roles
func (m *Manager) RequireAnyRole(requiredRoles []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "User not authenticated",
			})
			c.Abort()
			return
		}

		userClaims, ok := claims.(*auth.Claims)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Invalid user claims",
			})
			c.Abort()
			return
		}

		if !m.jwtAuth.HasAnyRole(userClaims, requiredRoles) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Insufficient permissions",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// Helper functions

func generateRequestID() string {
	// Simple implementation - in production, use a proper UUID library
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func joinStrings(slice []string, separator string) string {
	if len(slice) == 0 {
		return ""
	}

	result := slice[0]
	for i := 1; i < len(slice); i++ {
		result += separator + slice[i]
	}
	return result
}

