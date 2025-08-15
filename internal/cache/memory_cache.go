package cache

import (
	"context"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"go.uber.org/zap"
)

// MemoryCache implements in-memory caching using go-cache
type MemoryCache struct {
	cache      *cache.Cache
	defaultTTL time.Duration
	logger     *zap.Logger
	mu         sync.RWMutex
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(maxSize int, defaultTTL time.Duration, logger *zap.Logger) *MemoryCache {
	// Create cache with default TTL and cleanup interval
	c := cache.New(defaultTTL, 10*time.Minute)

	return &MemoryCache{
		cache:      c,
		defaultTTL: defaultTTL,
		logger:     logger,
	}
}

// Get retrieves a value from cache
func (m *MemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	value, found := m.cache.Get(key)
	if !found {
		m.logger.Debug("Memory cache miss", zap.String("key", key))
		return nil, ErrCacheMiss
	}

	data, ok := value.([]byte)
	if !ok {
		m.logger.Error("Invalid cache value type", zap.String("key", key))
		return nil, ErrCacheMiss
	}

	m.logger.Debug("Memory cache hit", zap.String("key", key))
	return data, nil
}

// Set stores a value in cache
func (m *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl == 0 {
		ttl = m.defaultTTL
	}

	m.cache.Set(key, value, ttl)
	m.logger.Debug("Memory cache set", zap.String("key", key), zap.Duration("ttl", ttl))
	return nil
}

// Delete removes a value from cache
func (m *MemoryCache) Delete(ctx context.Context, key string) error {
	m.cache.Delete(key)
	m.logger.Debug("Memory cache delete", zap.String("key", key))
	return nil
}

// Exists checks if a key exists in cache
func (m *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	_, found := m.cache.Get(key)
	return found, nil
}

// Clear removes all cached items
func (m *MemoryCache) Clear(ctx context.Context) error {
	m.cache.Flush()
	m.logger.Info("Memory cache cleared")
	return nil
}

// GetTTL returns the TTL of a key
func (m *MemoryCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	// go-cache doesn't provide direct TTL access, so we estimate
	_, expiration, found := m.cache.GetWithExpiration(key)
	if !found {
		return 0, ErrCacheMiss
	}

	if expiration.IsZero() {
		return -1, nil // No expiration
	}

	ttl := time.Until(expiration)
	if ttl < 0 {
		return 0, nil // Already expired
	}

	return ttl, nil
}

// GetStats returns memory cache statistics
func (m *MemoryCache) GetStats() map[string]interface{} {
	items := m.cache.Items()

	stats := map[string]interface{}{
		"type":        "memory",
		"item_count":  len(items),
		"default_ttl": m.defaultTTL.String(),
	}

	return stats
}

// LRUCache implements a simple LRU cache
type LRUCache struct {
	capacity int
	items    map[string]*lruItem
	head     *lruItem
	tail     *lruItem
	mu       sync.RWMutex
	logger   *zap.Logger
}

type lruItem struct {
	key        string
	value      []byte
	expiration time.Time
	prev       *lruItem
	next       *lruItem
}

// NewLRUCache creates a new LRU cache
func NewLRUCache(capacity int, logger *zap.Logger) *LRUCache {
	lru := &LRUCache{
		capacity: capacity,
		items:    make(map[string]*lruItem),
		logger:   logger,
	}

	// Initialize head and tail sentinels
	lru.head = &lruItem{}
	lru.tail = &lruItem{}
	lru.head.next = lru.tail
	lru.tail.prev = lru.head

	return lru
}

// Get retrieves a value from LRU cache
func (l *LRUCache) Get(ctx context.Context, key string) ([]byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	item, exists := l.items[key]
	if !exists {
		l.logger.Debug("LRU cache miss", zap.String("key", key))
		return nil, ErrCacheMiss
	}

	// Check expiration
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
		l.removeItem(item)
		delete(l.items, key)
		l.logger.Debug("LRU cache expired", zap.String("key", key))
		return nil, ErrCacheMiss
	}

	// Move to front (most recently used)
	l.moveToFront(item)
	l.logger.Debug("LRU cache hit", zap.String("key", key))
	return item.value, nil
}

// Set stores a value in LRU cache
func (l *LRUCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var expiration time.Time
	if ttl > 0 {
		expiration = time.Now().Add(ttl)
	}

	if item, exists := l.items[key]; exists {
		// Update existing item
		item.value = value
		item.expiration = expiration
		l.moveToFront(item)
	} else {
		// Add new item
		item := &lruItem{
			key:        key,
			value:      value,
			expiration: expiration,
		}

		l.items[key] = item
		l.addToFront(item)

		// Check capacity
		if len(l.items) > l.capacity {
			l.evictLRU()
		}
	}

	l.logger.Debug("LRU cache set", zap.String("key", key), zap.Duration("ttl", ttl))
	return nil
}

// Delete removes a value from LRU cache
func (l *LRUCache) Delete(ctx context.Context, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if item, exists := l.items[key]; exists {
		l.removeItem(item)
		delete(l.items, key)
		l.logger.Debug("LRU cache delete", zap.String("key", key))
	}

	return nil
}

// Exists checks if a key exists in LRU cache
func (l *LRUCache) Exists(ctx context.Context, key string) (bool, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	item, exists := l.items[key]
	if !exists {
		return false, nil
	}

	// Check expiration
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
		return false, nil
	}

	return true, nil
}

// Clear removes all items from LRU cache
func (l *LRUCache) Clear(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.items = make(map[string]*lruItem)
	l.head.next = l.tail
	l.tail.prev = l.head

	l.logger.Info("LRU cache cleared")
	return nil
}

// GetTTL returns the TTL of a key in LRU cache
func (l *LRUCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	item, exists := l.items[key]
	if !exists {
		return 0, ErrCacheMiss
	}

	if item.expiration.IsZero() {
		return -1, nil // No expiration
	}

	ttl := time.Until(item.expiration)
	if ttl < 0 {
		return 0, nil // Already expired
	}

	return ttl, nil
}

// Helper methods for LRU cache

// addToFront adds an item to the front of the list
func (l *LRUCache) addToFront(item *lruItem) {
	item.prev = l.head
	item.next = l.head.next
	l.head.next.prev = item
	l.head.next = item
}

// removeItem removes an item from the list
func (l *LRUCache) removeItem(item *lruItem) {
	item.prev.next = item.next
	item.next.prev = item.prev
}

// moveToFront moves an item to the front
func (l *LRUCache) moveToFront(item *lruItem) {
	l.removeItem(item)
	l.addToFront(item)
}

// evictLRU removes the least recently used item
func (l *LRUCache) evictLRU() {
	last := l.tail.prev
	if last != l.head {
		l.removeItem(last)
		delete(l.items, last.key)
		l.logger.Debug("LRU cache eviction", zap.String("key", last.key))
	}
}

// GetStats returns LRU cache statistics
func (l *LRUCache) GetStats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	stats := map[string]interface{}{
		"type":       "lru",
		"capacity":   l.capacity,
		"item_count": len(l.items),
	}

	return stats
}

