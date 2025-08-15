package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"go.uber.org/zap"

	"github.com/max/api-gateway/internal/config"
	"github.com/max/api-gateway/pkg/loadbalancer"
	"github.com/max/api-gateway/pkg/metrics"
)

// ReverseProxy handles reverse proxy functionality
type ReverseProxy struct {
	loadBalancer loadbalancer.LoadBalancer
	timeout      time.Duration
	retries      int
	logger       *zap.Logger
	metrics      *metrics.Manager
	serviceName  string
}

// NewReverseProxy creates a new reverse proxy
func NewReverseProxy(serviceName string, cfg *config.ServiceConfig, metricsMgr *metrics.Manager, logger *zap.Logger) (*ReverseProxy, error) {
	// Parse URLs
	targets := make([]*url.URL, 0, len(cfg.URLs))
	for _, urlStr := range cfg.URLs {
		target, err := url.Parse(urlStr)
		if err != nil {
			return nil, fmt.Errorf("invalid target URL %s: %w", urlStr, err)
		}
		targets = append(targets, target)
	}

	// Create load balancer
	var lb loadbalancer.LoadBalancer
	switch cfg.LoadBalancer {
	case "round_robin":
		lb = loadbalancer.NewRoundRobin(targets)
	case "weighted_round_robin":
		lb = loadbalancer.NewWeightedRoundRobin(targets, nil) // Equal weights by default
	case "least_connections":
		lb = loadbalancer.NewLeastConnections(targets)
	case "random":
		lb = loadbalancer.NewRandom(targets)
	default:
		lb = loadbalancer.NewRoundRobin(targets) // Default to round robin
	}

	return &ReverseProxy{
		loadBalancer: lb,
		timeout:      cfg.Timeout,
		retries:      cfg.Retries,
		logger:       logger,
		metrics:      metricsMgr,
		serviceName:  serviceName,
	}, nil
}

// ServeHTTP handles the HTTP request
func (rp *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Get target from load balancer
	target := rp.loadBalancer.NextTarget()
	if target == nil {
		rp.logger.Error("No available targets")
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Create reverse proxy for the target
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize the proxy
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		rp.modifyRequest(req, target)
	}

	// Set up error handling
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		rp.handleProxyError(w, r, err, target)
	}

	// Set up response modification
	proxy.ModifyResponse = func(resp *http.Response) error {
		return rp.modifyResponse(resp)
	}

	// Set timeout and capture response
	cw := &captureResponseWriter{ResponseWriter: w, status: http.StatusOK}
	if rp.timeout > 0 {
		http.TimeoutHandler(proxy, rp.timeout, "Gateway Timeout").ServeHTTP(cw, r)
	} else {
		proxy.ServeHTTP(cw, r)
	}

	// Log the request
	duration := time.Since(start)
	rp.logger.Info("Proxy request completed",
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.String("target", target.String()),
		zap.Duration("duration", duration),
		zap.Int("status", cw.status))

	// Record upstream metrics
	if rp.metrics != nil {
		rp.metrics.RecordUpstreamRequest(rp.serviceName, r.Method, cw.status, duration)
	}
}

// modifyRequest modifies the outgoing request
func (rp *ReverseProxy) modifyRequest(req *http.Request, target *url.URL) {
	// Set the Host header to the target host
	req.Host = target.Host

	// Add X-Forwarded headers
	if req.Header.Get("X-Forwarded-For") == "" {
		req.Header.Set("X-Forwarded-For", req.RemoteAddr)
	}
	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Header.Set("X-Forwarded-Proto", "http")
	if req.TLS != nil {
		req.Header.Set("X-Forwarded-Proto", "https")
	}

	// Add gateway identification
	req.Header.Set("X-Gateway", "api-gateway")
	req.Header.Set("X-Gateway-Time", time.Now().Format(time.RFC3339))

	rp.logger.Debug("Request modified",
		zap.String("target", target.String()),
		zap.String("host", req.Host),
		zap.String("x_forwarded_for", req.Header.Get("X-Forwarded-For")))
}

// modifyResponse modifies the incoming response
func (rp *ReverseProxy) modifyResponse(resp *http.Response) error {
	// Add CORS headers if needed
	if resp.Header.Get("Access-Control-Allow-Origin") == "" {
		resp.Header.Set("Access-Control-Allow-Origin", "*")
	}

	// Add gateway headers
	resp.Header.Set("X-Gateway", "api-gateway")
	resp.Header.Set("X-Gateway-Time", time.Now().Format(time.RFC3339))

	// Remove sensitive headers
	resp.Header.Del("Server")

	rp.logger.Debug("Response modified",
		zap.Int("status", resp.StatusCode),
		zap.String("content_type", resp.Header.Get("Content-Type")))

	return nil
}

// handleProxyError handles proxy errors
func (rp *ReverseProxy) handleProxyError(w http.ResponseWriter, r *http.Request, err error, target *url.URL) {
	rp.logger.Error("Proxy error",
		zap.Error(err),
		zap.String("target", target.String()),
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path))

	// Mark target as unhealthy in load balancer
	if healthChecker, ok := rp.loadBalancer.(loadbalancer.HealthChecker); ok {
		healthChecker.MarkUnhealthy(target)
	}

	// Return appropriate error response
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		http.Error(w, "Gateway Timeout", http.StatusGatewayTimeout)
		if rp.metrics != nil {
			rp.metrics.RecordUpstreamError(rp.serviceName, "timeout")
		}
		return
	}
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
	if rp.metrics != nil {
		rp.metrics.RecordUpstreamError(rp.serviceName, "bad_gateway")
	}
}

// captureResponseWriter wraps ResponseWriter to capture status and size
type captureResponseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (c *captureResponseWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureResponseWriter) Write(b []byte) (int, error) {
	n, err := c.ResponseWriter.Write(b)
	c.size += n
	return n, err
}

// ProxyManager manages multiple reverse proxies
type ProxyManager struct {
	proxies map[string]*ReverseProxy
	logger  *zap.Logger
	metrics *metrics.Manager
}

// NewProxyManager creates a new proxy manager
func NewProxyManager(logger *zap.Logger, metricsMgr *metrics.Manager) *ProxyManager {
	return &ProxyManager{
		proxies: make(map[string]*ReverseProxy),
		logger:  logger,
		metrics: metricsMgr,
	}
}

// AddService adds a service proxy
func (pm *ProxyManager) AddService(name string, cfg *config.ServiceConfig) error {
	proxy, err := NewReverseProxy(name, cfg, pm.metrics, pm.logger)
	if err != nil {
		return fmt.Errorf("failed to create proxy for service %s: %w", name, err)
	}

	pm.proxies[name] = proxy
	pm.logger.Info("Service proxy added", zap.String("service", name))
	return nil
}

// GetProxy returns a proxy for a service
func (pm *ProxyManager) GetProxy(service string) *ReverseProxy {
	return pm.proxies[service]
}

// RemoveService removes a service proxy
func (pm *ProxyManager) RemoveService(name string) {
	delete(pm.proxies, name)
	pm.logger.Info("Service proxy removed", zap.String("service", name))
}

// UpdateService updates a service proxy configuration
func (pm *ProxyManager) UpdateService(name string, cfg *config.ServiceConfig) error {
	proxy, err := NewReverseProxy(name, cfg, pm.metrics, pm.logger)
	if err != nil {
		return fmt.Errorf("failed to update proxy for service %s: %w", name, err)
	}

	pm.proxies[name] = proxy
	pm.logger.Info("Service proxy updated", zap.String("service", name))
	return nil
}

// ListServices returns all registered services
func (pm *ProxyManager) ListServices() []string {
	services := make([]string, 0, len(pm.proxies))
	for name := range pm.proxies {
		services = append(services, name)
	}
	return services
}

// GetStats returns proxy statistics
func (pm *ProxyManager) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"services_count": len(pm.proxies),
		"services":       pm.ListServices(),
	}

	return stats
}
