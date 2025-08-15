package ratelimit

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/max/api-gateway/internal/config"
)

func TestTokenBucket_Allow(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	tb := NewTokenBucket(10, 5, logger) // 10 requests per second, burst of 5

	// Test basic functionality
	for i := 0; i < 5; i++ {
		allowed, err := tb.Allow("test-key")
		if err != nil {
			t.Fatalf("Token bucket error: %v", err)
		}
		if !allowed {
			t.Errorf("Expected request %d to be allowed", i)
		}
	}

	// Test burst limit
	allowed, err := tb.Allow("test-key")
	if err != nil {
		t.Fatalf("Token bucket error: %v", err)
	}
	if allowed {
		t.Error("Expected request to be denied after burst limit")
	}
}

func TestSlidingWindow_Allow(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	sw := NewSlidingWindow(5, 1*time.Second, logger) // 5 requests per second

	// Test basic functionality
	for i := 0; i < 5; i++ {
		allowed, err := sw.Allow("test-key")
		if err != nil {
			t.Fatalf("Sliding window error: %v", err)
		}
		if !allowed {
			t.Errorf("Expected request %d to be allowed", i)
		}
	}

	// Test limit exceeded
	allowed, err := sw.Allow("test-key")
	if err != nil {
		t.Fatalf("Sliding window error: %v", err)
	}
	if allowed {
		t.Error("Expected request to be denied after limit")
	}
}

func TestFixedWindow_Allow(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	fw := NewFixedWindow(3, 1*time.Second, logger) // 3 requests per second

	// Test basic functionality
	for i := 0; i < 3; i++ {
		allowed, err := fw.Allow("test-key")
		if err != nil {
			t.Fatalf("Fixed window error: %v", err)
		}
		if !allowed {
			t.Errorf("Expected request %d to be allowed", i)
		}
	}

	// Test limit exceeded
	allowed, err := fw.Allow("test-key")
	if err != nil {
		t.Fatalf("Fixed window error: %v", err)
	}
	if allowed {
		t.Error("Expected request to be denied after limit")
	}
}

func TestRateLimitManager_CheckLimit(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create a simple config for testing
	config := &config.RateLimitConfig{
		Enabled:   true,
		Algorithm: "token_bucket",
		Default: config.RateLimitRule{
			Requests: 5,
			Window:   1 * time.Second,
			Burst:    5, // burst must allow the immediate 5 requests
		},
	}

	manager := NewManager(config, nil, logger)

	// Test default rate limiting
	for i := 0; i < 5; i++ {
		allowed, err := manager.CheckLimit("test-key")
		if err != nil {
			t.Fatalf("Rate limit check error: %v", err)
		}
		if !allowed {
			t.Errorf("Expected request %d to be allowed", i)
		}
	}

	// Test limit exceeded
	allowed, err := manager.CheckLimit("test-key")
	if err != nil {
		t.Fatalf("Rate limit check error: %v", err)
	}
	if allowed {
		t.Error("Expected request to be denied after limit")
	}
}
