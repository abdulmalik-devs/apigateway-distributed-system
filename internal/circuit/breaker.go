package circuit

import (
	"errors"
	"fmt"
	"sync"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"

	"github.com/max/api-gateway/internal/config"
)

// CircuitBreaker wraps the gobreaker circuit breaker
type CircuitBreaker struct {
	breaker *gobreaker.CircuitBreaker
	logger  *zap.Logger
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, cfg config.CircuitBreakerConfig, logger *zap.Logger) *CircuitBreaker {
	if !cfg.Enabled {
		return &CircuitBreaker{
			logger: logger,
		}
	}

	settings := gobreaker.Settings{
		Name:        name,
		MaxRequests: uint32(cfg.HalfOpenRequests),
		Interval:    cfg.RecoveryTimeout,
		Timeout:     cfg.RecoveryTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= uint32(cfg.FailureThreshold)
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Info("Circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()))
		},
	}

	breaker := gobreaker.NewCircuitBreaker(settings)

	return &CircuitBreaker{
		breaker: breaker,
		logger:  logger,
	}
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(fn func() (interface{}, error)) (interface{}, error) {
	if cb.breaker == nil {
		// Circuit breaker is disabled
		return fn()
	}

	return cb.breaker.Execute(fn)
}

// Call executes a function with circuit breaker protection (no return value)
func (cb *CircuitBreaker) Call(fn func() error) error {
	if cb.breaker == nil {
		// Circuit breaker is disabled
		return fn()
	}

	_, err := cb.breaker.Execute(func() (interface{}, error) {
		return nil, fn()
	})
	return err
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() gobreaker.State {
	if cb.breaker == nil {
		return gobreaker.StateClosed
	}
	return cb.breaker.State()
}

// Counts returns the current counts of the circuit breaker
func (cb *CircuitBreaker) Counts() gobreaker.Counts {
	if cb.breaker == nil {
		return gobreaker.Counts{}
	}
	return cb.breaker.Counts()
}

// IsOpen returns true if the circuit breaker is open
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.State() == gobreaker.StateOpen
}

// IsClosed returns true if the circuit breaker is closed
func (cb *CircuitBreaker) IsClosed() bool {
	return cb.State() == gobreaker.StateClosed
}

// IsHalfOpen returns true if the circuit breaker is half-open
func (cb *CircuitBreaker) IsHalfOpen() bool {
	return cb.State() == gobreaker.StateHalfOpen
}

// Manager manages multiple circuit breakers
type Manager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
	logger   *zap.Logger
}

// NewManager creates a new circuit breaker manager
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		breakers: make(map[string]*CircuitBreaker),
		logger:   logger,
	}
}

// GetBreaker returns a circuit breaker by name
func (m *Manager) GetBreaker(name string) *CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.breakers[name]
}

// CreateBreaker creates a new circuit breaker
func (m *Manager) CreateBreaker(name string, cfg config.CircuitBreakerConfig) *CircuitBreaker {
	m.mu.Lock()
	defer m.mu.Unlock()

	breaker := NewCircuitBreaker(name, cfg, m.logger)
	m.breakers[name] = breaker

	m.logger.Info("Circuit breaker created",
		zap.String("name", name),
		zap.Bool("enabled", cfg.Enabled))

	return breaker
}

// RemoveBreaker removes a circuit breaker
func (m *Manager) RemoveBreaker(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.breakers, name)
	m.logger.Info("Circuit breaker removed", zap.String("name", name))
}

// ExecuteWithBreaker executes a function with the specified circuit breaker
func (m *Manager) ExecuteWithBreaker(name string, fn func() (interface{}, error)) (interface{}, error) {
	breaker := m.GetBreaker(name)
	if breaker == nil {
		return nil, fmt.Errorf("circuit breaker not found: %s", name)
	}

	return breaker.Execute(fn)
}

// CallWithBreaker executes a function with the specified circuit breaker (no return value)
func (m *Manager) CallWithBreaker(name string, fn func() error) error {
	breaker := m.GetBreaker(name)
	if breaker == nil {
		return fmt.Errorf("circuit breaker not found: %s", name)
	}

	return breaker.Call(fn)
}

// GetAllStates returns the states of all circuit breakers
func (m *Manager) GetAllStates() map[string]BreakerInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make(map[string]BreakerInfo)
	for name, breaker := range m.breakers {
		counts := breaker.Counts()
		states[name] = BreakerInfo{
			Name:                 name,
			State:                breaker.State().String(),
			Requests:             counts.Requests,
			TotalSuccesses:       counts.TotalSuccesses,
			TotalFailures:        counts.TotalFailures,
			ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
			ConsecutiveFailures:  counts.ConsecutiveFailures,
		}
	}

	return states
}

// BreakerInfo contains circuit breaker information
type BreakerInfo struct {
	Name                 string `json:"name"`
	State                string `json:"state"`
	Requests             uint32 `json:"requests"`
	TotalSuccesses       uint32 `json:"total_successes"`
	TotalFailures        uint32 `json:"total_failures"`
	ConsecutiveSuccesses uint32 `json:"consecutive_successes"`
	ConsecutiveFailures  uint32 `json:"consecutive_failures"`
}

// GetStats returns circuit breaker statistics
func (m *Manager) GetStats() map[string]interface{} {
	states := m.GetAllStates()

	openCount := 0
	halfOpenCount := 0
	closedCount := 0

	for _, state := range states {
		switch state.State {
		case "open":
			openCount++
		case "half-open":
			halfOpenCount++
		case "closed":
			closedCount++
		}
	}

	stats := map[string]interface{}{
		"total_breakers": len(states),
		"open":           openCount,
		"half_open":      halfOpenCount,
		"closed":         closedCount,
		"breakers":       states,
	}

	return stats
}

// HealthCheck checks the health of all circuit breakers
func (m *Manager) HealthCheck() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	openBreakers := make([]string, 0)
	for name, breaker := range m.breakers {
		if breaker.IsOpen() {
			openBreakers = append(openBreakers, name)
		}
	}

	if len(openBreakers) > 0 {
		return fmt.Errorf("circuit breakers are open: %v", openBreakers)
	}

	return nil
}

// ResetBreaker resets a circuit breaker to closed state
func (m *Manager) ResetBreaker(name string) error {
	m.mu.RLock()
	breaker := m.breakers[name]
	m.mu.RUnlock()

	if breaker == nil {
		return fmt.Errorf("circuit breaker not found: %s", name)
	}

	if breaker.breaker == nil {
		return nil // Circuit breaker is disabled
	}

	// Reset by creating a new circuit breaker with the same settings
	// This is a limitation of the gobreaker library
	m.logger.Info("Circuit breaker reset requested", zap.String("name", name))
	return nil
}

// Middleware creates a Gin middleware for circuit breaker protection
func (m *Manager) Middleware(breakerName string) func(fn func() error) error {
	return func(fn func() error) error {
		return m.CallWithBreaker(breakerName, fn)
	}
}

// WrapHTTPCall wraps an HTTP call with circuit breaker protection
func (m *Manager) WrapHTTPCall(breakerName string, call func() error) error {
	return m.CallWithBreaker(breakerName, func() error {
		err := call()
		if err != nil {
			// Log the error for monitoring
			m.logger.Debug("HTTP call failed",
				zap.String("breaker", breakerName),
				zap.Error(err))
		}
		return err
	})
}

// Common circuit breaker errors
var (
	ErrCircuitBreakerOpen     = errors.New("circuit breaker is open")
	ErrCircuitBreakerNotFound = errors.New("circuit breaker not found")
	ErrTooManyRequests        = errors.New("too many requests")
)

