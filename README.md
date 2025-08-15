# Production-Grade API Gateway

A high-performance, enterprise-ready API Gateway built in Go that serves as the entry point for microservices, implementing authentication, rate limiting, request routing, and comprehensive observability.

## 🏗️ Architecture Overview

![API Gateway Architecture](docs/architecture.png)

*Complete system architecture showing the API Gateway with distributed event processing, designed to handle 100,000+ concurrent users with comprehensive monitoring, security, and scalability features.*

## Features

### Core Functionality
- **JWT-based Authentication & Authorization** - Secure token validation with role-based access control
- **Advanced Rate Limiting** - Multiple algorithms (Token Bucket, Sliding Window, Fixed Window, Distributed/Redis)
- **Dynamic Request Routing** - Intelligent load balancing with health checks
- **Request/Response Transformation** - Hook points for custom transformations
- **Caching Layer** - Redis-based caching with intelligent invalidation
- **Circuit Breaker** - Automatic failure detection and recovery
- **Real-time Monitoring** - Prometheus metrics and Grafana dashboards
- **Distributed Event Processing** - Kafka/RabbitMQ integration for high-throughput event streaming

### Technical Stack
- **Go 1.23+** with Gin framework
- **Redis** for rate limiting and caching
- **PostgreSQL** for configuration and analytics
- **JWT** for authentication
- **Prometheus + Grafana** for monitoring
- **Kafka/RabbitMQ** for distributed event processing
- **Docker & Kubernetes** ready

## Quick Start (Local)

### Prerequisites
- Go 1.23+
- Docker (for full stack)

### Run Gateway only
```bash
go mod download
go run cmd/gateway/main.go
# Health
curl -s http://localhost:8080/health | jq
```

## Run Full Stack (recommended)

We ship a full local stack via Docker Compose (Gateway + Redis + Postgres + Prometheus + Grafana + Jaeger + sample services).

```bash
# Build and start everything
docker-compose up -d --build

# Tail logs (optional)
docker-compose logs -f gateway
```

### Access URLs
- **Gateway**: `http://localhost:8081`
  - Health: `http://localhost:8081/health`
- **Gateway metrics (Prom exporters)**: scraped internally; Prometheus UI at `http://localhost:9091`
- **Grafana**: `http://localhost:3001` (login: `admin` / `admin`)
  - Dashboard: "API Gateway Overview" is pre-provisioned
- **Jaeger (Tracing UI)**: `http://localhost:16687` (spans appear after tracing is enabled)

Ports are mapped to avoid conflicts:
- Gateway: host 8081 -> container 8080
- Gateway metrics: host 9092 -> container 9090 (Prometheus scrapes via service DNS `gateway:9090`)
- Prometheus UI: host 9091 -> Prom 9090
- Grafana UI: host 3001 -> Grafana 3000
- Jaeger UI: host 16687 -> Jaeger 16686

### Generate Traffic (to populate Grafana)
Upstream latency graphs populate when the gateway proxies requests to downstream services. Hit the example services via the gateway:
```bash
# Single requests
curl -sS http://localhost:8081/user_service > /dev/null
curl -sS http://localhost:8081/order_service > /dev/null
curl -sS http://localhost:8081/payment_service > /dev/null

# Small load
for i in {1..200}; do curl -sS http://localhost:8081/user_service > /dev/null; sleep 0.1; done
```
Then refresh Grafana (Last 5–15 min) on the "API Gateway Overview" dashboard.

### Tracing (Jaeger)
Jaeger UI shows distributed traces/spans. Traces appear once the gateway emits spans and propagates context. Tracing hooks are planned; if you want them now, ask to "add tracing" and we'll wire OpenTelemetry with a Jaeger exporter and propagate `traceparent` headers to downstreams.

## Production Deployment for 100k+ Users

### 🚀 **Phase 1: Infrastructure Scaling**

#### 1.1 High-Availability Setup
```bash
# Deploy to Kubernetes with horizontal scaling
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/redis.yaml

# Scale gateway for high availability
kubectl scale deployment api-gateway --replicas=3 -n api-gateway
kubectl scale deployment redis --replicas=3 -n api-gateway
```

#### 1.2 Load Balancer Configuration
```yaml
# k8s/ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: api-gateway-ingress
  namespace: api-gateway
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/rate-limit: "1000"
    nginx.ingress.kubernetes.io/rate-limit-window: "1m"
spec:
  tls:
  - hosts:
    - api.yourdomain.com
    secretName: api-gateway-tls
  rules:
  - host: api.yourdomain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: api-gateway-service
            port:
              number: 8080
```

#### 1.3 Database Scaling
```bash
# PostgreSQL with read replicas
helm install postgresql bitnami/postgresql \
  --set global.postgresql.auth.postgresPassword="your-password" \
  --set readReplicas.persistence.size=100Gi \
  --set readReplicas.replicaCount=2

# Redis Cluster for high availability
helm install redis-cluster bitnami/redis-cluster \
  --set cluster.nodes=6 \
  --set cluster.replicas=1 \
  --set persistence.size=50Gi
```

### 🎯 **Phase 2: Distributed Event Processing Integration**

#### 2.1 Event Processing Infrastructure
```bash
# Deploy Kafka cluster
helm install kafka bitnami/kafka \
  --set replicaCount=3 \
  --set persistence.size=100Gi \
  --set zookeeper.persistence.size=20Gi

# Deploy RabbitMQ for alternative messaging
helm install rabbitmq bitnami/rabbitmq \
  --set replicaCount=3 \
  --set persistence.size=50Gi \
  --set auth.password="your-rabbitmq-password"
```

#### 2.2 Event Processing Configuration
```yaml
# configs/production-config.yaml
event_processing:
  enabled: true
  provider: "kafka"  # or "rabbitmq"
  kafka:
    brokers:
      - "kafka-0.kafka-headless.default.svc.cluster.local:9092"
      - "kafka-1.kafka-headless.default.svc.cluster.local:9092"
      - "kafka-2.kafka-headless.default.svc.cluster.local:9092"
    topics:
      api_events: "api-gateway-events"
      user_events: "user-events"
      audit_logs: "audit-logs"
    consumer_group: "api-gateway-consumer"
    producer_config:
      acks: "all"
      compression: "snappy"
      batch_size: 16384
      linger_ms: 5
  rabbitmq:
    url: "amqp://user:password@rabbitmq.default.svc.cluster.local:5672/"
    exchanges:
      api_events: "api-gateway-events"
      user_events: "user-events"
    queues:
      audit_logs: "audit-logs"
      metrics: "metrics-queue"
```

#### 2.3 Event Processing Implementation
```go
// internal/events/processor.go
package events

import (
    "context"
    "encoding/json"
    "time"
    
    "github.com/Shopify/sarama"
    "go.uber.org/zap"
)

type EventProcessor struct {
    producer sarama.SyncProducer
    consumer sarama.Consumer
    logger   *zap.Logger
}

type APIEvent struct {
    Timestamp   time.Time         `json:"timestamp"`
    EventType   string            `json:"event_type"`
    UserID      string            `json:"user_id"`
    Service     string            `json:"service"`
    Path        string            `json:"path"`
    Method      string            `json:"method"`
    StatusCode  int               `json:"status_code"`
    Latency     time.Duration     `json:"latency"`
    IPAddress   string            `json:"ip_address"`
    UserAgent   string            `json:"user_agent"`
    Metadata    map[string]string `json:"metadata"`
}

func (ep *EventProcessor) PublishEvent(event *APIEvent) error {
    data, err := json.Marshal(event)
    if err != nil {
        return err
    }
    
    msg := &sarama.ProducerMessage{
        Topic: "api-gateway-events",
        Key:   sarama.StringEncoder(event.UserID),
        Value: sarama.ByteEncoder(data),
    }
    
    _, _, err = ep.producer.SendMessage(msg)
    return err
}
```

### 🔧 **Phase 3: Performance Optimization**

#### 3.1 Gateway Scaling Configuration
```yaml
# k8s/deployment.yaml (updated)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-gateway
  namespace: api-gateway
spec:
  replicas: 5  # Scale based on load
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 2
      maxUnavailable: 1
  template:
    spec:
      containers:
      - name: api-gateway
        image: your-registry/api-gateway:latest
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2000m"
        env:
        - name: GOMAXPROCS
          value: "4"
        - name: GOGC
          value: "50"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        ports:
        - containerPort: 8080
        - containerPort: 9090
```

#### 3.2 Rate Limiting for High Load
```yaml
# configs/production-config.yaml
rate_limit:
  enabled: true
  algorithm: "distributed"
  default:
    requests: 1000  # Increased for production
    window: "1m"
    burst: 200
  per_user:
    premium:
      requests: 5000
      window: "1m"
      burst: 500
    enterprise:
      requests: 10000
      window: "1m"
      burst: 1000
  per_service:
    user_service:
      requests: 5000
      window: "1m"
      burst: 500
    payment_service:
      requests: 2000
      window: "1m"
      burst: 200
```

### 📊 **Phase 4: Monitoring & Observability**

#### 4.1 Advanced Monitoring Setup
```yaml
# k8s/monitoring.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: monitoring
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
      evaluation_interval: 15s
    
    rule_files:
      - "alert_rules.yml"
    
    scrape_configs:
      - job_name: 'api-gateway'
        static_configs:
          - targets: ['api-gateway-service.api-gateway.svc.cluster.local:9090']
        metrics_path: /metrics
        scrape_interval: 10s
        
      - job_name: 'kafka'
        static_configs:
          - targets: ['kafka-exporter.kafka.svc.cluster.local:9308']
          
      - job_name: 'redis'
        static_configs:
          - targets: ['redis-exporter.redis.svc.cluster.local:9121']
```

#### 4.2 Alerting Rules
```yaml
# configs/alert_rules.yml
groups:
  - name: api-gateway
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "High error rate detected"
          
      - alert: HighLatency
        expr: histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High latency detected"
          
      - alert: CircuitBreakerOpen
        expr: circuit_breaker_state{state="open"} > 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Circuit breaker is open"
```

### 🔐 **Phase 5: Security & Compliance**

#### 5.1 Security Configuration
```yaml
# configs/security-config.yaml
security:
  tls:
    enabled: true
    cert_file: "/etc/ssl/certs/api-gateway.crt"
    key_file: "/etc/ssl/private/api-gateway.key"
    min_version: "1.2"
    cipher_suites:
      - "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"
      - "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305"
  
  cors:
    enabled: true
    allowed_origins:
      - "https://yourdomain.com"
      - "https://app.yourdomain.com"
    allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
    allowed_headers: ["Authorization", "Content-Type", "X-API-Key"]
    max_age: 86400
  
  rate_limit:
    enabled: true
    ddos_protection:
      enabled: true
      max_requests_per_ip: 1000
      window: "1m"
      block_duration: "10m"
```

#### 5.2 Audit Logging
```go
// internal/audit/logger.go
package audit

import (
    "context"
    "encoding/json"
    "time"
    
    "github.com/max/api-gateway/internal/events"
)

type AuditLogger struct {
    eventProcessor *events.EventProcessor
}

type AuditEvent struct {
    Timestamp   time.Time         `json:"timestamp"`
    UserID      string            `json:"user_id"`
    Action      string            `json:"action"`
    Resource    string            `json:"resource"`
    IPAddress   string            `json:"ip_address"`
    UserAgent   string            `json:"user_agent"`
    Status      string            `json:"status"`
    Metadata    map[string]string `json:"metadata"`
}

func (al *AuditLogger) LogAccess(ctx context.Context, event *AuditEvent) error {
    return al.eventProcessor.PublishEvent(&events.APIEvent{
        Timestamp:  event.Timestamp,
        EventType:  "audit_access",
        UserID:     event.UserID,
        Path:       event.Resource,
        Method:     event.Action,
        IPAddress:  event.IPAddress,
        UserAgent:  event.UserAgent,
        Metadata:   event.Metadata,
    })
}
```

### 🚀 **Phase 6: Deployment Automation**

#### 6.1 CI/CD Pipeline
```yaml
# .github/workflows/deploy.yml
name: Deploy to Production

on:
  push:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.23'
      - run: go test ./...
      - run: go build -o bin/gateway cmd/gateway/main.go
  
  build:
    needs: test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Build Docker image
        run: |
          docker build -t your-registry/api-gateway:${{ github.sha }} .
          docker push your-registry/api-gateway:${{ github.sha }}
  
  deploy:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Deploy to Kubernetes
        run: |
          kubectl set image deployment/api-gateway \
            api-gateway=your-registry/api-gateway:${{ github.sha }} \
            -n api-gateway
          kubectl rollout status deployment/api-gateway -n api-gateway
```

#### 6.2 Infrastructure as Code
```hcl
# terraform/main.tf
terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.0"
    }
  }
}

resource "kubernetes_deployment" "api_gateway" {
  metadata {
    name      = "api-gateway"
    namespace = "api-gateway"
  }
  
  spec {
    replicas = var.gateway_replicas
    
    selector {
      match_labels = {
        app = "api-gateway"
      }
    }
    
    template {
      metadata {
        labels = {
          app = "api-gateway"
        }
      }
      
      spec {
        container {
          image = var.gateway_image
          name  = "api-gateway"
          
          resources {
            limits = {
              cpu    = "2000m"
              memory = "2Gi"
            }
            requests = {
              cpu    = "500m"
              memory = "512Mi"
            }
          }
        }
      }
    }
  }
}
```

## Integration Steps for Your Application

### Step 1: Connect Your Microservices
```bash
# Update your service configurations
kubectl apply -f k8s/your-service-config.yaml

# Configure service discovery
kubectl patch configmap gateway-config -n api-gateway --patch-file - <<EOF
data:
  config.yaml: |
    routing:
      services:
        your_service:
          urls:
            - "http://your-service.your-namespace.svc.cluster.local:8080"
          load_balancer: "round_robin"
          timeout: "30s"
          retries: 3
EOF
```

### Step 2: Configure Event Processing
```bash
# Create event processing topics
kubectl exec -it kafka-0 -n default -- kafka-topics.sh \
  --create --topic your-app-events \
  --bootstrap-server localhost:9092 \
  --partitions 6 --replication-factor 3

# Configure event consumers
kubectl apply -f k8s/event-consumers.yaml
```

### Step 3: Set Up Monitoring
```bash
# Deploy monitoring stack
helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace

# Import custom dashboards
kubectl apply -f k8s/grafana-dashboards.yaml
```

### Step 4: Configure Authentication
```bash
# Update JWT configuration for your auth provider
kubectl patch configmap gateway-config -n api-gateway --patch-file - <<EOF
data:
  config.yaml: |
    auth:
      jwt:
        secret: "your-production-jwt-secret"
        issuer: "your-auth-provider"
        audience: "your-app-users"
EOF
```

### Step 5: Load Testing
```bash
# Install k6 for load testing
helm install k6 grafana/k6 -n monitoring

# Run load test
kubectl apply -f k8s/load-test.yaml
```

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Client Apps   │───▶│   API Gateway   │───▶│  Microservices  │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                              │
                              ▼
                       ┌─────────────────┐
                       │   Redis Cache   │
                       └─────────────────┘
                              │
                              ▼
                       ┌─────────────────┐
                       │   PostgreSQL    │
                       └─────────────────┘
                              │
                              ▼
                       ┌─────────────────┐
                       │   Kafka/RabbitMQ│
                       │  Event Stream   │
                       └─────────────────┘
```

A more detailed architecture diagram is available in the PR description and via the Mermaid diagram included in the repository discussion.

## Configuration

The gateway supports dynamic configuration reloading. Key configuration sections:

- **Server**: Port, TLS, CORS settings
- **Authentication**: JWT settings, API keys
- **Rate Limiting**: Algorithms, limits per user/service
- **Routing**: Service discovery, load balancing
- **Caching**: Redis settings, TTL policies
- **Monitoring**: Prometheus, tracing settings
- **Event Processing**: Kafka/RabbitMQ configuration

Default config lives in `configs/config.yaml`. Override with `CONFIG_PATH=/path/to/config.yaml`.

## API Endpoints

### Public Endpoints
- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics (exposed by gateway; also scraped internally by Prometheus)
- `POST /auth/login` - Authentication (demo)
- `POST /auth/refresh` - Token refresh

### Admin Endpoints
- `GET /admin/config` - Current configuration
- `POST /admin/config/reload` - Reload configuration
- `GET /admin/stats` - Gateway statistics
- `GET /admin/circuit-breakers` - Circuit breaker status
- `GET /admin/events` - Event processing status

## Rate Limiting

The gateway supports multiple rate limiting algorithms:

1. **Token Bucket**: Handles burst traffic efficiently
2. **Fixed Window**: Simple counter-based limiting
3. **Sliding Window Log**: Precise time-based limiting
4. **Sliding Window Counter**: Memory-efficient sliding window
5. **Distributed (Redis)**: Horizontal scalability

## Monitoring

### Metrics
- Request/response latency histograms
- Error rates per service
- Throughput monitoring
- Rate limiting statistics
- Circuit breaker status
- Event processing metrics

### Tracing
- Jaeger UI available; tracing emission can be enabled with OpenTelemetry + Jaeger exporter

## Development

### Project Structure
```
api-gateway/
├── cmd/                    # Application entry points
├── internal/              # Private application code
├── pkg/                   # Public libraries
├── configs/               # Configuration files
├── scripts/               # Deployment scripts
└── k8s/                   # Kubernetes manifests
```

### Running Tests
```bash
go test ./...
go test -race ./...
go test -cover ./...
```

### Building
```bash
go build -o bin/gateway cmd/gateway/main.go
go build -o bin/config-server cmd/config-server/main.go
```

## Troubleshooting
- **Port already allocated**: Change host ports in `docker-compose.yml` (e.g., gateway `8081`, Grafana `3001`, Prom `9091`, Jaeger `16687`) or stop conflicting processes.
- **No upstream latency in Grafana**: Ensure you hit proxied routes like `/user_service`, not only `/health`.
- **Prometheus shows no targets**: Compose installs Prometheus with `gateway:9090` scrape; ensure the gateway is up and the network is healthy (`docker-compose ps`).
- **Redis/Postgres connection**: The gateway will log warnings if Redis is unavailable; it falls back appropriately where possible.
- **Event processing issues**: Check Kafka/RabbitMQ connectivity and topic configuration.
- **High memory usage**: Adjust `GOGC` environment variable and resource limits.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License - see LICENSE file for details.
