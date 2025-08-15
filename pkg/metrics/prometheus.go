package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Manager manages Prometheus metrics
type Manager struct {
	// HTTP metrics
	httpRequests     *prometheus.CounterVec
	httpDuration     *prometheus.HistogramVec
	httpRequestSize  *prometheus.HistogramVec
	httpResponseSize *prometheus.HistogramVec

	// Rate limiting metrics
	rateLimitHits   *prometheus.CounterVec
	rateLimitMisses *prometheus.CounterVec

	// Circuit breaker metrics
	circuitBreakerState *prometheus.GaugeVec
	circuitBreakerReqs  *prometheus.CounterVec

	// Gateway metrics
	upstreamRequests *prometheus.CounterVec
	upstreamDuration *prometheus.HistogramVec
	upstreamErrors   *prometheus.CounterVec

	// Cache metrics
	cacheHits   *prometheus.CounterVec
	cacheMisses *prometheus.CounterVec

	// System metrics
	gatewayInfo       *prometheus.GaugeVec
	gatewayUptime     prometheus.Gauge
	activeConnections prometheus.Gauge

	registry  *prometheus.Registry
	logger    *zap.Logger
	startTime time.Time
}

// NewManager creates a new metrics manager
func NewManager(logger *zap.Logger) *Manager {
	registry := prometheus.NewRegistry()

	// HTTP metrics
	httpRequests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_http_requests_total",
			Help: "Total number of HTTP requests processed by the gateway",
		},
		[]string{"method", "path", "status_code"},
	)

	httpDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status_code"},
	)

	httpRequestSize := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_http_request_size_bytes",
			Help:    "HTTP request size in bytes",
			Buckets: []float64{100, 1000, 10000, 100000, 1000000},
		},
		[]string{"method", "path"},
	)

	httpResponseSize := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_http_response_size_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: []float64{100, 1000, 10000, 100000, 1000000},
		},
		[]string{"method", "path", "status_code"},
	)

	// Rate limiting metrics
	rateLimitHits := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_rate_limit_hits_total",
			Help: "Total number of rate limit hits",
		},
		[]string{"algorithm", "key_type"},
	)

	rateLimitMisses := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_rate_limit_misses_total",
			Help: "Total number of rate limit misses (requests allowed)",
		},
		[]string{"algorithm", "key_type"},
	)

	// Circuit breaker metrics
	circuitBreakerState := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_circuit_breaker_state",
			Help: "Circuit breaker state (0=closed, 1=half-open, 2=open)",
		},
		[]string{"name"},
	)

	circuitBreakerReqs := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_circuit_breaker_requests_total",
			Help: "Total number of requests through circuit breaker",
		},
		[]string{"name", "state", "result"},
	)

	// Upstream metrics
	upstreamRequests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_upstream_requests_total",
			Help: "Total number of upstream requests",
		},
		[]string{"service", "method", "status_code"},
	)

	upstreamDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_upstream_request_duration_seconds",
			Help:    "Upstream request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "method"},
	)

	upstreamErrors := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_upstream_errors_total",
			Help: "Total number of upstream errors",
		},
		[]string{"service", "error_type"},
	)

	// Cache metrics
	cacheHits := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"cache_type"},
	)

	cacheMisses := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"cache_type"},
	)

	// System metrics
	gatewayInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_info",
			Help: "Gateway information",
		},
		[]string{"version", "build_date"},
	)

	gatewayUptime := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gateway_uptime_seconds",
			Help: "Gateway uptime in seconds",
		},
	)

	activeConnections := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gateway_active_connections",
			Help: "Number of active connections",
		},
	)

	// Register metrics
	registry.MustRegister(
		httpRequests,
		httpDuration,
		httpRequestSize,
		httpResponseSize,
		rateLimitHits,
		rateLimitMisses,
		circuitBreakerState,
		circuitBreakerReqs,
		upstreamRequests,
		upstreamDuration,
		upstreamErrors,
		cacheHits,
		cacheMisses,
		gatewayInfo,
		gatewayUptime,
		activeConnections,
	)

	manager := &Manager{
		httpRequests:        httpRequests,
		httpDuration:        httpDuration,
		httpRequestSize:     httpRequestSize,
		httpResponseSize:    httpResponseSize,
		rateLimitHits:       rateLimitHits,
		rateLimitMisses:     rateLimitMisses,
		circuitBreakerState: circuitBreakerState,
		circuitBreakerReqs:  circuitBreakerReqs,
		upstreamRequests:    upstreamRequests,
		upstreamDuration:    upstreamDuration,
		upstreamErrors:      upstreamErrors,
		cacheHits:           cacheHits,
		cacheMisses:         cacheMisses,
		gatewayInfo:         gatewayInfo,
		gatewayUptime:       gatewayUptime,
		activeConnections:   activeConnections,
		registry:            registry,
		logger:              logger,
		startTime:           time.Now(),
	}

	// Set gateway info
	gatewayInfo.WithLabelValues("1.0.0", time.Now().Format("2006-01-02")).Set(1)

	// Start uptime updater
	go manager.updateUptime()

	return manager
}

// RecordHTTPRequest records an HTTP request metric
func (m *Manager) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration) {
	statusStr := strconv.Itoa(statusCode)

	m.httpRequests.WithLabelValues(method, path, statusStr).Inc()
	m.httpDuration.WithLabelValues(method, path, statusStr).Observe(duration.Seconds())
}

// RecordHTTPRequestSize records HTTP request size
func (m *Manager) RecordHTTPRequestSize(method, path string, size int64) {
	m.httpRequestSize.WithLabelValues(method, path).Observe(float64(size))
}

// RecordHTTPResponseSize records HTTP response size
func (m *Manager) RecordHTTPResponseSize(method, path string, statusCode int, size int64) {
	statusStr := strconv.Itoa(statusCode)
	m.httpResponseSize.WithLabelValues(method, path, statusStr).Observe(float64(size))
}

// RecordRateLimitHit records a rate limit hit
func (m *Manager) RecordRateLimitHit(algorithm, keyType string) {
	m.rateLimitHits.WithLabelValues(algorithm, keyType).Inc()
}

// RecordRateLimitMiss records a rate limit miss (allowed request)
func (m *Manager) RecordRateLimitMiss(algorithm, keyType string) {
	m.rateLimitMisses.WithLabelValues(algorithm, keyType).Inc()
}

// SetCircuitBreakerState sets the circuit breaker state
func (m *Manager) SetCircuitBreakerState(name string, state int) {
	m.circuitBreakerState.WithLabelValues(name).Set(float64(state))
}

// RecordCircuitBreakerRequest records a circuit breaker request
func (m *Manager) RecordCircuitBreakerRequest(name, state, result string) {
	m.circuitBreakerReqs.WithLabelValues(name, state, result).Inc()
}

// RecordUpstreamRequest records an upstream request
func (m *Manager) RecordUpstreamRequest(service, method string, statusCode int, duration time.Duration) {
	statusStr := strconv.Itoa(statusCode)

	m.upstreamRequests.WithLabelValues(service, method, statusStr).Inc()
	m.upstreamDuration.WithLabelValues(service, method).Observe(duration.Seconds())
}

// RecordUpstreamError records an upstream error
func (m *Manager) RecordUpstreamError(service, errorType string) {
	m.upstreamErrors.WithLabelValues(service, errorType).Inc()
}

// RecordCacheHit records a cache hit
func (m *Manager) RecordCacheHit(cacheType string) {
	m.cacheHits.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss records a cache miss
func (m *Manager) RecordCacheMiss(cacheType string) {
	m.cacheMisses.WithLabelValues(cacheType).Inc()
}

// SetActiveConnections sets the number of active connections
func (m *Manager) SetActiveConnections(count int) {
	m.activeConnections.Set(float64(count))
}

// Handler returns the Prometheus HTTP handler
func (m *Manager) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// GinHandler returns a Gin handler for metrics endpoint
func (m *Manager) GinHandler() gin.HandlerFunc {
	handler := m.Handler()
	return func(c *gin.Context) {
		handler.ServeHTTP(c.Writer, c.Request)
	}
}

// updateUptime updates the uptime metric periodically
func (m *Manager) updateUptime() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		uptime := time.Since(m.startTime).Seconds()
		m.gatewayUptime.Set(uptime)
	}
}

// GetStats returns current metrics statistics
func (m *Manager) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"uptime_seconds":  time.Since(m.startTime).Seconds(),
		"start_time":      m.startTime.Format(time.RFC3339),
		"metrics_enabled": true,
	}
}

// Reset resets all metrics (useful for testing)
func (m *Manager) Reset() {
	m.httpRequests.Reset()
	m.httpDuration.Reset()
	m.httpRequestSize.Reset()
	m.httpResponseSize.Reset()
	m.rateLimitHits.Reset()
	m.rateLimitMisses.Reset()
	m.circuitBreakerState.Reset()
	m.circuitBreakerReqs.Reset()
	m.upstreamRequests.Reset()
	m.upstreamDuration.Reset()
	m.upstreamErrors.Reset()
	m.cacheHits.Reset()
	m.cacheMisses.Reset()
	m.gatewayUptime.Set(0)
	m.activeConnections.Set(0)
}

// Middleware creates a Gin middleware for automatic metrics collection
func (m *Manager) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Get request size
		requestSize := c.Request.ContentLength
		if requestSize > 0 {
			m.RecordHTTPRequestSize(c.Request.Method, c.Request.URL.Path, requestSize)
		}

		// Process request
		c.Next()

		// Record metrics
		duration := time.Since(start)
		m.RecordHTTPRequest(
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			duration,
		)

		// Record response size if available
		if responseSize := c.Writer.Size(); responseSize > 0 {
			m.RecordHTTPResponseSize(
				c.Request.Method,
				c.Request.URL.Path,
				c.Writer.Status(),
				int64(responseSize),
			)
		}
	}
}

