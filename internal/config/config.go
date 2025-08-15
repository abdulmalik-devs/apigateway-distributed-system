package config

import (
	"fmt"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Config represents the main configuration structure
type Config struct {
	Server          ServerConfig          `mapstructure:"server"`
	Auth            AuthConfig            `mapstructure:"auth"`
	RateLimit       RateLimitConfig       `mapstructure:"rate_limit"`
	Routing         RoutingConfig         `mapstructure:"routing"`
	Cache           CacheConfig           `mapstructure:"cache"`
	Database        DatabaseConfig        `mapstructure:"database"`
	Redis           RedisConfig           `mapstructure:"redis"`
	Monitoring      MonitoringConfig      `mapstructure:"monitoring"`
	Logging         LoggingConfig         `mapstructure:"logging"`
	EventProcessing EventProcessingConfig `mapstructure:"event_processing"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	Host         string        `mapstructure:"host"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
	TLS          TLSConfig     `mapstructure:"tls"`
	CORS         CORSConfig    `mapstructure:"cors"`
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	AllowedMethods []string `mapstructure:"allowed_methods"`
	AllowedHeaders []string `mapstructure:"allowed_headers"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWT JWTConfig    `mapstructure:"jwt"`
	API APIKeyConfig `mapstructure:"api_key"`
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret         string        `mapstructure:"secret"`
	ExpirationTime time.Duration `mapstructure:"expiration_time"`
	RefreshTime    time.Duration `mapstructure:"refresh_time"`
	Issuer         string        `mapstructure:"issuer"`
	Audience       string        `mapstructure:"audience"`
	Algorithm      string        `mapstructure:"algorithm"`
}

// APIKeyConfig holds API key configuration
type APIKeyConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Header  string `mapstructure:"header"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled    bool                     `mapstructure:"enabled"`
	Algorithm  string                   `mapstructure:"algorithm"`
	Default    RateLimitRule            `mapstructure:"default"`
	PerUser    map[string]RateLimitRule `mapstructure:"per_user"`
	PerService map[string]RateLimitRule `mapstructure:"per_service"`
}

// RateLimitRule defines rate limiting rules
type RateLimitRule struct {
	Requests int           `mapstructure:"requests"`
	Window   time.Duration `mapstructure:"window"`
	Burst    int           `mapstructure:"burst"`
}

// RoutingConfig holds routing configuration
type RoutingConfig struct {
	Services map[string]ServiceConfig `mapstructure:"services"`
	Default  ServiceConfig            `mapstructure:"default"`
}

// ServiceConfig holds service configuration
type ServiceConfig struct {
	URLs           []string             `mapstructure:"urls"`
	LoadBalancer   string               `mapstructure:"load_balancer"`
	Timeout        time.Duration        `mapstructure:"timeout"`
	Retries        int                  `mapstructure:"retries"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	FailureThreshold int           `mapstructure:"failure_threshold"`
	RecoveryTimeout  time.Duration `mapstructure:"recovery_timeout"`
	HalfOpenRequests int           `mapstructure:"half_open_requests"`
}

// CacheConfig holds caching configuration
type CacheConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	TTL     time.Duration `mapstructure:"ttl"`
	MaxSize int           `mapstructure:"max_size"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

// MonitoringConfig holds monitoring configuration
type MonitoringConfig struct {
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Tracing    TracingConfig    `mapstructure:"tracing"`
}

// PrometheusConfig holds Prometheus configuration
type PrometheusConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
	Port    int    `mapstructure:"port"`
}

// TracingConfig holds tracing configuration
type TracingConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Jaeger  string `mapstructure:"jaeger"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// EventProcessingConfig holds event processing configuration
type EventProcessingConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Provider string `mapstructure:"provider"` // "kafka" or "rabbitmq"
	Kafka    KafkaConfig
	RabbitMQ RabbitMQConfig
}

// KafkaConfig holds Kafka-specific configuration
type KafkaConfig struct {
	Brokers        []string          `mapstructure:"brokers"`
	Topics         map[string]string `mapstructure:"topics"`
	ConsumerGroup  string            `mapstructure:"consumer_group"`
	ProducerConfig ProducerConfig    `mapstructure:"producer_config"`
}

// RabbitMQConfig holds RabbitMQ-specific configuration
type RabbitMQConfig struct {
	URL       string            `mapstructure:"url"`
	Exchanges map[string]string `mapstructure:"exchanges"`
	Queues    map[string]string `mapstructure:"queues"`
}

// ProducerConfig holds producer-specific settings
type ProducerConfig struct {
	Acks        string `mapstructure:"acks"`
	Compression string `mapstructure:"compression"`
	BatchSize   int    `mapstructure:"batch_size"`
	LingerMs    int    `mapstructure:"linger_ms"`
}

// Manager handles configuration loading and reloading
type Manager struct {
	config *Config
	viper  *viper.Viper
	logger *zap.Logger
	mu     sync.RWMutex
}

// NewManager creates a new configuration manager
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		viper:  viper.New(),
		logger: logger,
	}
}

// Load loads configuration from file
func (m *Manager) Load(configPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set default values
	m.setDefaults()

	// Configure viper
	m.viper.SetConfigFile(configPath)
	m.viper.SetConfigType("yaml")
	m.viper.AutomaticEnv()

	// Read config file
	if err := m.viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Unmarshal config
	var config Config
	if err := m.viper.Unmarshal(&config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate config
	if err := m.validateConfig(&config); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	m.config = &config
	m.logger.Info("Configuration loaded successfully", zap.String("file", configPath))
	return nil
}

// Get returns the current configuration
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// Reload reloads the configuration from file
func (m *Manager) Reload() error {
	return m.Load(m.viper.ConfigFileUsed())
}

// Watch watches for configuration file changes
func (m *Manager) Watch() {
	m.viper.WatchConfig()
	m.viper.OnConfigChange(func(e fsnotify.Event) {
		m.logger.Info("Configuration file changed, reloading", zap.String("file", e.Name))
		if err := m.Reload(); err != nil {
			m.logger.Error("Failed to reload configuration", zap.Error(err))
		}
	})
}

// setDefaults sets default configuration values
func (m *Manager) setDefaults() {
	// Server defaults
	m.viper.SetDefault("server.port", 8080)
	m.viper.SetDefault("server.host", "0.0.0.0")
	m.viper.SetDefault("server.read_timeout", "30s")
	m.viper.SetDefault("server.write_timeout", "30s")
	m.viper.SetDefault("server.idle_timeout", "60s")
	m.viper.SetDefault("server.tls.enabled", false)
	m.viper.SetDefault("server.cors.enabled", true)
	m.viper.SetDefault("server.cors.allowed_origins", []string{"*"})
	m.viper.SetDefault("server.cors.allowed_methods", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	m.viper.SetDefault("server.cors.allowed_headers", []string{"*"})

	// Auth defaults
	m.viper.SetDefault("auth.jwt.expiration_time", "1h")
	m.viper.SetDefault("auth.jwt.refresh_time", "24h")
	m.viper.SetDefault("auth.jwt.algorithm", "HS256")
	m.viper.SetDefault("auth.api_key.enabled", true)
	m.viper.SetDefault("auth.api_key.header", "X-API-Key")

	// Rate limiting defaults
	m.viper.SetDefault("rate_limit.enabled", true)
	m.viper.SetDefault("rate_limit.algorithm", "token_bucket")
	m.viper.SetDefault("rate_limit.default.requests", 100)
	m.viper.SetDefault("rate_limit.default.window", "1m")
	m.viper.SetDefault("rate_limit.default.burst", 10)

	// Cache defaults
	m.viper.SetDefault("cache.enabled", true)
	m.viper.SetDefault("cache.ttl", "5m")
	m.viper.SetDefault("cache.max_size", 1000)

	// Redis defaults
	m.viper.SetDefault("redis.host", "localhost")
	m.viper.SetDefault("redis.port", 6379)
	m.viper.SetDefault("redis.db", 0)
	m.viper.SetDefault("redis.pool_size", 10)

	// Database defaults
	m.viper.SetDefault("database.host", "localhost")
	m.viper.SetDefault("database.port", 5432)
	m.viper.SetDefault("database.sslmode", "disable")

	// Monitoring defaults
	m.viper.SetDefault("monitoring.prometheus.enabled", true)
	m.viper.SetDefault("monitoring.prometheus.path", "/metrics")
	m.viper.SetDefault("monitoring.prometheus.port", 9090)
	m.viper.SetDefault("monitoring.tracing.enabled", false)

	// Logging defaults
	m.viper.SetDefault("logging.level", "info")
	m.viper.SetDefault("logging.format", "json")
	m.viper.SetDefault("logging.output", "stdout")
}

// validateConfig validates the configuration
func (m *Manager) validateConfig(config *Config) error {
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}

	if config.Auth.JWT.Secret == "" {
		return fmt.Errorf("JWT secret is required")
	}

	if config.RateLimit.Enabled && config.RateLimit.Default.Requests <= 0 {
		return fmt.Errorf("rate limit requests must be positive")
	}

	return nil
}
