package ratelimit

import (
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/max/api-gateway/internal/config"
)

// Manager manages rate limiting for the gateway
type Manager struct {
	algorithms map[string]Algorithm
	config     *config.RateLimitConfig
	logger     *zap.Logger
}

// NewManager creates a new rate limit manager
func NewManager(cfg *config.RateLimitConfig, redisClient *redis.Client, logger *zap.Logger) *Manager {
	manager := &Manager{
		algorithms: make(map[string]Algorithm),
		config:     cfg,
		logger:     logger,
	}

	// Initialize algorithms based on configuration
	if cfg.Enabled {
		manager.initializeAlgorithms(redisClient)
	}

	return manager
}

// initializeAlgorithms initializes rate limiting algorithms
func (m *Manager) initializeAlgorithms(redisClient *redis.Client) {
	switch m.config.Algorithm {
	case "token_bucket":
		m.algorithms["default"] = NewTokenBucket(
			m.config.Default.Requests,
			m.config.Default.Burst,
			m.logger,
		)
	case "sliding_window":
		m.algorithms["default"] = NewSlidingWindow(
			m.config.Default.Requests,
			m.config.Default.Window,
			m.logger,
		)
	case "fixed_window":
		m.algorithms["default"] = NewFixedWindow(
			m.config.Default.Requests,
			m.config.Default.Window,
			m.logger,
		)
	case "distributed":
		if redisClient != nil {
			m.algorithms["default"] = NewDistributedRateLimit(
				redisClient,
				m.config.Default.Requests,
				m.config.Default.Window,
				m.logger,
			)
		} else {
			// Fallback to token bucket if Redis is not available
			m.algorithms["default"] = NewTokenBucket(
				m.config.Default.Requests,
				m.config.Default.Burst,
				m.logger,
			)
		}
	default:
		// Default to token bucket
		m.algorithms["default"] = NewTokenBucket(
			m.config.Default.Requests,
			m.config.Default.Burst,
			m.logger,
		)
	}

	// Initialize per-user rate limiters
	for userID, rule := range m.config.PerUser {
		key := fmt.Sprintf("user:%s", userID)
		switch m.config.Algorithm {
		case "token_bucket":
			m.algorithms[key] = NewTokenBucket(rule.Requests, rule.Burst, m.logger)
		case "sliding_window":
			m.algorithms[key] = NewSlidingWindow(rule.Requests, rule.Window, m.logger)
		case "fixed_window":
			m.algorithms[key] = NewFixedWindow(rule.Requests, rule.Window, m.logger)
		case "distributed":
			if redisClient != nil {
				m.algorithms[key] = NewDistributedRateLimit(redisClient, rule.Requests, rule.Window, m.logger)
			}
		}
	}

	// Initialize per-service rate limiters
	for serviceID, rule := range m.config.PerService {
		key := fmt.Sprintf("service:%s", serviceID)
		switch m.config.Algorithm {
		case "token_bucket":
			m.algorithms[key] = NewTokenBucket(rule.Requests, rule.Burst, m.logger)
		case "sliding_window":
			m.algorithms[key] = NewSlidingWindow(rule.Requests, rule.Window, m.logger)
		case "fixed_window":
			m.algorithms[key] = NewFixedWindow(rule.Requests, rule.Window, m.logger)
		case "distributed":
			if redisClient != nil {
				m.algorithms[key] = NewDistributedRateLimit(redisClient, rule.Requests, rule.Window, m.logger)
			}
		}
	}

	m.logger.Info("Rate limiting algorithms initialized",
		zap.String("algorithm", m.config.Algorithm),
		zap.Int("algorithms_count", len(m.algorithms)))
}

// CheckLimit checks if a request is allowed for the given key
func (m *Manager) CheckLimit(key string) (bool, error) {
	if !m.config.Enabled {
		return true, nil
	}

	// Try to find specific algorithm for the key
	algorithm, exists := m.algorithms[key]
	if !exists {
		// Fall back to default algorithm
		algorithm = m.algorithms["default"]
	}

	if algorithm == nil {
		m.logger.Warn("No rate limiting algorithm available", zap.String("key", key))
		return true, nil
	}

	return algorithm.Allow(key)
}

// CheckUserLimit checks rate limit for a specific user
func (m *Manager) CheckUserLimit(userID string) (bool, error) {
	key := fmt.Sprintf("user:%s", userID)
	return m.CheckLimit(key)
}

// CheckServiceLimit checks rate limit for a specific service
func (m *Manager) CheckServiceLimit(serviceID string) (bool, error) {
	key := fmt.Sprintf("service:%s", serviceID)
	return m.CheckLimit(key)
}

// CheckIPLimit checks rate limit for an IP address
func (m *Manager) CheckIPLimit(ip string) (bool, error) {
	key := fmt.Sprintf("ip:%s", ip)
	return m.CheckLimit(key)
}

// CheckAPIKeyLimit checks rate limit for an API key
func (m *Manager) CheckAPIKeyLimit(apiKey string) (bool, error) {
	key := fmt.Sprintf("apikey:%s", apiKey)
	return m.CheckLimit(key)
}

// Reset resets the rate limiter for a key
func (m *Manager) Reset(key string) error {
	algorithm, exists := m.algorithms[key]
	if !exists {
		algorithm = m.algorithms["default"]
	}

	if algorithm == nil {
		return fmt.Errorf("no algorithm found for key: %s", key)
	}

	return algorithm.Reset(key)
}

// GetLimitInfo returns rate limit information for a key
func (m *Manager) GetLimitInfo(key string) (*LimitInfo, error) {
	if !m.config.Enabled {
		return &LimitInfo{
			Limit:     -1,
			Remaining: -1,
			ResetTime: time.Time{},
		}, nil
	}

	// For now, return basic info based on configuration
	// In a more advanced implementation, this could query the actual state
	rule := m.config.Default

	// Check for specific user or service rules
	if userRule, exists := m.config.PerUser[key]; exists {
		rule = userRule
	} else if serviceRule, exists := m.config.PerService[key]; exists {
		rule = serviceRule
	}

	return &LimitInfo{
		Limit:     rule.Requests,
		Remaining: rule.Requests, // This would need to be calculated from actual state
		ResetTime: time.Now().Add(rule.Window),
		Window:    rule.Window,
	}, nil
}

// LimitInfo contains rate limit information
type LimitInfo struct {
	Limit     int           `json:"limit"`
	Remaining int           `json:"remaining"`
	ResetTime time.Time     `json:"reset_time"`
	Window    time.Duration `json:"window"`
}

// UpdateConfig updates the rate limiting configuration
func (m *Manager) UpdateConfig(cfg *config.RateLimitConfig, redisClient *redis.Client) {
	m.config = cfg

	// Clear existing algorithms
	m.algorithms = make(map[string]Algorithm)

	// Reinitialize with new config
	if cfg.Enabled {
		m.initializeAlgorithms(redisClient)
	}

	m.logger.Info("Rate limiting configuration updated")
}

// IsEnabled returns whether rate limiting is enabled
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
}

// GetStats returns rate limiting statistics
func (m *Manager) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"enabled":           m.config.Enabled,
		"algorithm":         m.config.Algorithm,
		"algorithms_count":  len(m.algorithms),
		"default_limit":     m.config.Default.Requests,
		"default_window":    m.config.Default.Window.String(),
		"per_user_rules":    len(m.config.PerUser),
		"per_service_rules": len(m.config.PerService),
	}

	return stats
}

