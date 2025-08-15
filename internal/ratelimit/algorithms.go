package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// Algorithm represents a rate limiting algorithm
type Algorithm interface {
	Allow(key string) (bool, error)
	Reset(key string) error
}

// TokenBucket implements token bucket rate limiting
type TokenBucket struct {
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
	mu       sync.RWMutex
	logger   *zap.Logger
}

// NewTokenBucket creates a new token bucket rate limiter
func NewTokenBucket(rps int, burst int, logger *zap.Logger) *TokenBucket {
	return &TokenBucket{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(rps),
		burst:    burst,
		logger:   logger,
	}
}

// Allow checks if a request is allowed
func (tb *TokenBucket) Allow(key string) (bool, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	limiter, exists := tb.limiters[key]
	if !exists {
		limiter = rate.NewLimiter(tb.rate, tb.burst)
		tb.limiters[key] = limiter
	}

	allowed := limiter.Allow()
	tb.logger.Debug("Token bucket check",
		zap.String("key", key),
		zap.Bool("allowed", allowed),
		zap.Float64("tokens", limiter.Tokens()))

	return allowed, nil
}

// Reset resets the rate limiter for a key
func (tb *TokenBucket) Reset(key string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	delete(tb.limiters, key)
	return nil
}

// SlidingWindow implements sliding window rate limiting
type SlidingWindow struct {
	windows map[string]*Window
	limit   int
	window  time.Duration
	mu      sync.RWMutex
	logger  *zap.Logger
}

// Window represents a sliding window
type Window struct {
	requests []time.Time
	mu       sync.RWMutex
}

// NewSlidingWindow creates a new sliding window rate limiter
func NewSlidingWindow(limit int, window time.Duration, logger *zap.Logger) *SlidingWindow {
	return &SlidingWindow{
		windows: make(map[string]*Window),
		limit:   limit,
		window:  window,
		logger:  logger,
	}
}

// Allow checks if a request is allowed
func (sw *SlidingWindow) Allow(key string) (bool, error) {
	sw.mu.Lock()
	window, exists := sw.windows[key]
	if !exists {
		window = &Window{requests: make([]time.Time, 0)}
		sw.windows[key] = window
	}
	sw.mu.Unlock()

	window.mu.Lock()
	defer window.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-sw.window)

	// Remove old requests
	i := 0
	for i < len(window.requests) && window.requests[i].Before(cutoff) {
		i++
	}
	window.requests = window.requests[i:]

	// Check if we can allow this request
	if len(window.requests) >= sw.limit {
		sw.logger.Debug("Sliding window limit exceeded",
			zap.String("key", key),
			zap.Int("requests", len(window.requests)),
			zap.Int("limit", sw.limit))
		return false, nil
	}

	// Add current request
	window.requests = append(window.requests, now)
	sw.logger.Debug("Sliding window allowed",
		zap.String("key", key),
		zap.Int("requests", len(window.requests)))

	return true, nil
}

// Reset resets the rate limiter for a key
func (sw *SlidingWindow) Reset(key string) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	delete(sw.windows, key)
	return nil
}

// FixedWindow implements fixed window rate limiting
type FixedWindow struct {
	counters map[string]*Counter
	limit    int
	window   time.Duration
	mu       sync.RWMutex
	logger   *zap.Logger
}

// Counter represents a fixed window counter
type Counter struct {
	count  int
	window time.Time
	mu     sync.RWMutex
}

// NewFixedWindow creates a new fixed window rate limiter
func NewFixedWindow(limit int, window time.Duration, logger *zap.Logger) *FixedWindow {
	return &FixedWindow{
		counters: make(map[string]*Counter),
		limit:    limit,
		window:   window,
		logger:   logger,
	}
}

// Allow checks if a request is allowed
func (fw *FixedWindow) Allow(key string) (bool, error) {
	fw.mu.Lock()
	counter, exists := fw.counters[key]
	if !exists {
		counter = &Counter{
			count:  0,
			window: time.Now().Truncate(fw.window),
		}
		fw.counters[key] = counter
	}
	fw.mu.Unlock()

	counter.mu.Lock()
	defer counter.mu.Unlock()

	now := time.Now()
	currentWindow := now.Truncate(fw.window)

	// Reset counter if window has passed
	if currentWindow.After(counter.window) {
		counter.count = 0
		counter.window = currentWindow
	}

	// Check if we can allow this request
	if counter.count >= fw.limit {
		fw.logger.Debug("Fixed window limit exceeded",
			zap.String("key", key),
			zap.Int("count", counter.count),
			zap.Int("limit", fw.limit))
		return false, nil
	}

	counter.count++
	fw.logger.Debug("Fixed window allowed",
		zap.String("key", key),
		zap.Int("count", counter.count))

	return true, nil
}

// Reset resets the rate limiter for a key
func (fw *FixedWindow) Reset(key string) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	delete(fw.counters, key)
	return nil
}

// DistributedRateLimit implements distributed rate limiting using Redis
type DistributedRateLimit struct {
	client *redis.Client
	limit  int
	window time.Duration
	script *redis.Script
	logger *zap.Logger
}

// NewDistributedRateLimit creates a new distributed rate limiter
func NewDistributedRateLimit(client *redis.Client, limit int, window time.Duration, logger *zap.Logger) *DistributedRateLimit {
	// Lua script for atomic rate limiting
	script := redis.NewScript(`
		local key = KEYS[1]
		local window = tonumber(ARGV[1])
		local limit = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		
		-- Remove old entries
		redis.call('zremrangebyscore', key, '-inf', now - window * 1000)
		
		-- Get current count
		local current = redis.call('zcard', key)
		
		if current < limit then
			-- Add current request
			redis.call('zadd', key, now, now)
			redis.call('expire', key, window)
			return {1, limit - current - 1}
		else
			return {0, 0}
		end
	`)

	return &DistributedRateLimit{
		client: client,
		limit:  limit,
		window: window,
		script: script,
		logger: logger,
	}
}

// Allow checks if a request is allowed
func (drl *DistributedRateLimit) Allow(key string) (bool, error) {
	now := time.Now().UnixMilli()
	windowMs := int64(drl.window / time.Millisecond)

	result, err := drl.script.Run(
		context.Background(),
		drl.client,
		[]string{key},
		windowMs,
		drl.limit,
		now,
	).Result()

	if err != nil {
		drl.logger.Error("Distributed rate limit error", zap.Error(err))
		return false, fmt.Errorf("rate limit error: %w", err)
	}

	values := result.([]interface{})
	allowed := values[0].(int64) == 1
	remaining := values[1].(int64)

	drl.logger.Debug("Distributed rate limit check",
		zap.String("key", key),
		zap.Bool("allowed", allowed),
		zap.Int64("remaining", remaining))

	return allowed, nil
}

// Reset resets the rate limiter for a key
func (drl *DistributedRateLimit) Reset(key string) error {
	return drl.client.Del(context.Background(), key).Err()
}
