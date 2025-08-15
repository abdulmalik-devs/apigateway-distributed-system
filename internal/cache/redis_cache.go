package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/max/api-gateway/internal/config"
)

// Cache interface defines caching operations
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Clear(ctx context.Context) error
	GetTTL(ctx context.Context, key string) (time.Duration, error)
}

// RedisCache implements Redis-based caching
type RedisCache struct {
	client     *redis.Client
	prefix     string
	defaultTTL time.Duration
	logger     *zap.Logger
}

// NewRedisCache creates a new Redis cache
func NewRedisCache(client *redis.Client, prefix string, defaultTTL time.Duration, logger *zap.Logger) *RedisCache {
	return &RedisCache{
		client:     client,
		prefix:     prefix,
		defaultTTL: defaultTTL,
		logger:     logger,
	}
}

// Get retrieves a value from cache
func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	fullKey := r.buildKey(key)

	value, err := r.client.Get(ctx, fullKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			r.logger.Debug("Cache miss", zap.String("key", key))
			return nil, ErrCacheMiss
		}
		r.logger.Error("Cache get error", zap.String("key", key), zap.Error(err))
		return nil, err
	}

	r.logger.Debug("Cache hit", zap.String("key", key))
	return value, nil
}

// Set stores a value in cache
func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	fullKey := r.buildKey(key)

	if ttl == 0 {
		ttl = r.defaultTTL
	}

	err := r.client.Set(ctx, fullKey, value, ttl).Err()
	if err != nil {
		r.logger.Error("Cache set error", zap.String("key", key), zap.Error(err))
		return err
	}

	r.logger.Debug("Cache set", zap.String("key", key), zap.Duration("ttl", ttl))
	return nil
}

// Delete removes a value from cache
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	fullKey := r.buildKey(key)

	err := r.client.Del(ctx, fullKey).Err()
	if err != nil {
		r.logger.Error("Cache delete error", zap.String("key", key), zap.Error(err))
		return err
	}

	r.logger.Debug("Cache delete", zap.String("key", key))
	return nil
}

// Exists checks if a key exists in cache
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	fullKey := r.buildKey(key)

	exists, err := r.client.Exists(ctx, fullKey).Result()
	if err != nil {
		r.logger.Error("Cache exists error", zap.String("key", key), zap.Error(err))
		return false, err
	}

	return exists > 0, nil
}

// Clear removes all cached items with the prefix
func (r *RedisCache) Clear(ctx context.Context) error {
	pattern := r.buildKey("*")

	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		r.logger.Error("Cache clear error", zap.Error(err))
		return err
	}

	if len(keys) > 0 {
		err = r.client.Del(ctx, keys...).Err()
		if err != nil {
			r.logger.Error("Cache clear delete error", zap.Error(err))
			return err
		}
	}

	r.logger.Info("Cache cleared", zap.Int("keys_deleted", len(keys)))
	return nil
}

// GetTTL returns the TTL of a key
func (r *RedisCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	fullKey := r.buildKey(key)

	ttl, err := r.client.TTL(ctx, fullKey).Result()
	if err != nil {
		r.logger.Error("Cache TTL error", zap.String("key", key), zap.Error(err))
		return 0, err
	}

	return ttl, nil
}

// buildKey builds the full cache key with prefix
func (r *RedisCache) buildKey(key string) string {
	if r.prefix == "" {
		return key
	}
	return fmt.Sprintf("%s:%s", r.prefix, key)
}

// Manager manages multiple cache instances
type Manager struct {
	caches     map[string]Cache
	defaultTTL time.Duration
	logger     *zap.Logger
}

// NewManager creates a new cache manager
func NewManager(cfg *config.CacheConfig, redisClient *redis.Client, logger *zap.Logger) *Manager {
	manager := &Manager{
		caches:     make(map[string]Cache),
		defaultTTL: cfg.TTL,
		logger:     logger,
	}

	if cfg.Enabled && redisClient != nil {
		// Create default cache
		manager.caches["default"] = NewRedisCache(redisClient, "gateway", cfg.TTL, logger)

		// Create specialized caches
		manager.caches["responses"] = NewRedisCache(redisClient, "gateway:responses", cfg.TTL, logger)
		manager.caches["auth"] = NewRedisCache(redisClient, "gateway:auth", 1*time.Hour, logger)
		manager.caches["ratelimit"] = NewRedisCache(redisClient, "gateway:ratelimit", 1*time.Minute, logger)

		logger.Info("Cache manager initialized with Redis")
	} else {
		// Use in-memory cache as fallback
		memCache := NewMemoryCache(cfg.MaxSize, cfg.TTL, logger)
		manager.caches["default"] = memCache
		manager.caches["responses"] = memCache
		manager.caches["auth"] = memCache
		manager.caches["ratelimit"] = memCache

		logger.Info("Cache manager initialized with in-memory cache")
	}

	return manager
}

// GetCache returns a cache instance by name
func (m *Manager) GetCache(name string) Cache {
	if cache, exists := m.caches[name]; exists {
		return cache
	}
	return m.caches["default"]
}

// GetResponseCache returns the response cache
func (m *Manager) GetResponseCache() Cache {
	return m.GetCache("responses")
}

// GetAuthCache returns the auth cache
func (m *Manager) GetAuthCache() Cache {
	return m.GetCache("auth")
}

// GetRateLimitCache returns the rate limit cache
func (m *Manager) GetRateLimitCache() Cache {
	return m.GetCache("ratelimit")
}

// CacheResponse caches an HTTP response
func (m *Manager) CacheResponse(ctx context.Context, key string, response *CachedResponse, ttl time.Duration) error {
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	return m.GetResponseCache().Set(ctx, key, data, ttl)
}

// GetCachedResponse retrieves a cached HTTP response
func (m *Manager) GetCachedResponse(ctx context.Context, key string) (*CachedResponse, error) {
	data, err := m.GetResponseCache().Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var response CachedResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &response, nil
}

// CacheJSON caches a JSON-serializable object
func (m *Manager) CacheJSON(ctx context.Context, cacheName, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return m.GetCache(cacheName).Set(ctx, key, data, ttl)
}

// GetJSON retrieves and unmarshals a JSON object from cache
func (m *Manager) GetJSON(ctx context.Context, cacheName, key string, target interface{}) error {
	data, err := m.GetCache(cacheName).Get(ctx, key)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

// CachedResponse represents a cached HTTP response
type CachedResponse struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers"`
	Body       []byte              `json:"body"`
	Timestamp  time.Time           `json:"timestamp"`
}

// GetStats returns cache statistics
func (m *Manager) GetStats() map[string]interface{} {
	// This would return actual stats from Redis or memory cache
	return map[string]interface{}{
		"enabled":     len(m.caches) > 0,
		"caches":      len(m.caches),
		"default_ttl": m.defaultTTL.String(),
	}
}

// Common cache errors
var (
	ErrCacheMiss     = fmt.Errorf("cache miss")
	ErrCacheNotFound = fmt.Errorf("cache not found")
)

