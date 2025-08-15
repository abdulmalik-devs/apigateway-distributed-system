# Production Integration Guide for 100k+ Users

This guide provides step-by-step instructions for integrating the API Gateway with your real-world application and distributed event processing system.

## 🎯 Overview

This API Gateway is designed to handle enterprise-scale traffic with comprehensive event processing capabilities. It integrates with Kafka/RabbitMQ for distributed event streaming, enabling real-time analytics, audit logging, and system-wide event correlation.

## 📋 Prerequisites

### Infrastructure Requirements
- **Kubernetes Cluster**: v1.24+ with at least 8 CPU cores and 32GB RAM
- **Storage**: 500GB+ for databases, logs, and event storage
- **Network**: High-bandwidth, low-latency network for inter-service communication
- **Load Balancer**: Cloud load balancer or ingress controller (NGINX, Traefik)

### Software Requirements
- **kubectl**: v1.24+
- **helm**: v3.8+
- **docker**: v20.10+
- **git**: Latest version

### Security Requirements
- **SSL/TLS Certificates**: Valid certificates for your domain
- **Secrets Management**: Kubernetes secrets or external secret manager
- **Network Policies**: Proper network segmentation
- **RBAC**: Role-based access control configured

## 🚀 Phase 1: Infrastructure Setup

### 1.1 Cluster Preparation

```bash
# Verify cluster resources
kubectl get nodes -o wide
kubectl top nodes

# Ensure you have sufficient resources
# Minimum: 8 CPU cores, 32GB RAM across all nodes
```

### 1.2 Storage Setup

```bash
# Create storage classes for different workloads
kubectl apply -f k8s/storage-classes.yaml

# Verify storage classes
kubectl get storageclass
```

### 1.3 Network Policies

```bash
# Apply network policies for security
kubectl apply -f k8s/network-policies.yaml
```

## 🎯 Phase 2: Event Processing Infrastructure

### 2.1 Kafka Deployment

```bash
# Deploy Kafka cluster
kubectl apply -f k8s/kafka.yaml

# Verify deployment
kubectl get pods -n kafka
kubectl get svc -n kafka

# Create topics for your application
kubectl exec -it kafka-0 -n kafka -- kafka-topics.sh \
  --create --topic your-app-events \
  --bootstrap-server localhost:9092 \
  --partitions 6 --replication-factor 3

kubectl exec -it kafka-0 -n kafka -- kafka-topics.sh \
  --create --topic your-app-audit \
  --bootstrap-server localhost:9092 \
  --partitions 3 --replication-factor 3
```

### 2.2 Event Processing Configuration

Update `configs/production-config.yaml` with your event processing settings:

```yaml
event_processing:
  enabled: true
  provider: "kafka"  # or "rabbitmq"
  kafka:
    brokers:
      - "kafka-0.kafka-headless.kafka.svc.cluster.local:9092"
      - "kafka-1.kafka-headless.kafka.svc.cluster.local:9092"
      - "kafka-2.kafka-headless.kafka.svc.cluster.local:9092"
    topics:
      api_events: "api-gateway-events"
      user_events: "your-app-user-events"
      audit_logs: "your-app-audit"
      metrics: "your-app-metrics"
    consumer_group: "api-gateway-consumer"
    producer_config:
      acks: "all"
      compression: "snappy"
      batch_size: 16384
      linger_ms: 5
```

## 🔧 Phase 3: Application Integration

### 3.1 Connect Your Microservices

#### Step 1: Update Service Discovery

```bash
# Create service configurations for your microservices
kubectl patch configmap gateway-config -n api-gateway --patch-file - <<EOF
data:
  config.yaml: |
    routing:
      services:
        your_user_service:
          urls:
            - "http://your-user-service.your-namespace.svc.cluster.local:8080"
          load_balancer: "round_robin"
          timeout: "30s"
          retries: 3
          circuit_breaker:
            enabled: true
            failure_threshold: 5
            recovery_timeout: "30s"
            half_open_requests: 3
        
        your_order_service:
          urls:
            - "http://your-order-service.your-namespace.svc.cluster.local:8080"
          load_balancer: "least_connections"
          timeout: "45s"
          retries: 2
          circuit_breaker:
            enabled: true
            failure_threshold: 3
            recovery_timeout: "60s"
            half_open_requests: 5
EOF
```

#### Step 2: Configure Authentication

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
        expiration_time: "1h"
        refresh_time: "24h"
EOF
```

#### Step 3: Set Up Rate Limiting

```bash
# Configure rate limits for your user tiers
kubectl patch configmap gateway-config -n api-gateway --patch-file - <<EOF
data:
  config.yaml: |
    rate_limit:
      per_user:
        free:
          requests: 100
          window: "1m"
          burst: 20
        premium:
          requests: 1000
          window: "1m"
          burst: 100
        enterprise:
          requests: 5000
          window: "1m"
          burst: 500
EOF
```

### 3.2 Event Integration

#### Step 1: Implement Event Handlers

Create event handlers in your application to consume events from the gateway:

```go
// Example: Event consumer in your application
package main

import (
    "context"
    "encoding/json"
    "log"
    
    "github.com/Shopify/sarama"
)

type APIEvent struct {
    Timestamp   string            `json:"timestamp"`
    EventType   string            `json:"event_type"`
    UserID      string            `json:"user_id"`
    Service     string            `json:"service"`
    Path        string            `json:"path"`
    Method      string            `json:"method"`
    StatusCode  int               `json:"status_code"`
    Latency     int64             `json:"latency"`
    IPAddress   string            `json:"ip_address"`
    UserAgent   string            `json:"user_agent"`
    Metadata    map[string]string `json:"metadata"`
}

func handleAPIEvent(event *APIEvent) error {
    switch event.EventType {
    case "user_login":
        return handleUserLogin(event)
    case "user_logout":
        return handleUserLogout(event)
    case "api_request":
        return handleAPIRequest(event)
    case "error":
        return handleError(event)
    default:
        log.Printf("Unknown event type: %s", event.EventType)
        return nil
    }
}

func handleUserLogin(event *APIEvent) error {
    // Implement user login analytics
    log.Printf("User %s logged in from %s", event.UserID, event.IPAddress)
    return nil
}

func handleAPIRequest(event *APIEvent) error {
    // Implement API request analytics
    log.Printf("API request: %s %s by user %s", event.Method, event.Path, event.UserID)
    return nil
}
```

#### Step 2: Set Up Event Consumers

Deploy event consumers for your application:

```yaml
# k8s/event-consumers.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: event-consumer
  namespace: your-namespace
spec:
  replicas: 3
  selector:
    matchLabels:
      app: event-consumer
  template:
    metadata:
      labels:
        app: event-consumer
    spec:
      containers:
      - name: event-consumer
        image: your-registry/event-consumer:latest
        env:
        - name: KAFKA_BROKERS
          value: "kafka-0.kafka-headless.kafka.svc.cluster.local:9092,kafka-1.kafka-headless.kafka.svc.cluster.local:9092,kafka-2.kafka-headless.kafka.svc.cluster.local:9092"
        - name: KAFKA_TOPIC
          value: "api-gateway-events"
        - name: CONSUMER_GROUP
          value: "your-app-consumer"
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
```

## 📊 Phase 4: Monitoring & Observability

### 4.1 Deploy Monitoring Stack

```bash
# Deploy Prometheus and Grafana
helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  --set prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.resources.requests.storage=100Gi \
  --set grafana.adminPassword=your-secure-password

# Deploy Jaeger for tracing
helm install jaeger jaegertracing/jaeger \
  --namespace monitoring \
  --set storage.type=elasticsearch
```

### 4.2 Configure Dashboards

Import custom dashboards for your application:

```bash
# Apply Grafana dashboards
kubectl apply -f k8s/grafana-dashboards.yaml -n monitoring
```

### 4.3 Set Up Alerting

```yaml
# configs/alert_rules.yml
groups:
  - name: api-gateway
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.05
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "High error rate detected"
          description: "Error rate is {{ $value }} errors per second"
          
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
          
      - alert: KafkaLag
        expr: kafka_consumer_lag > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High Kafka consumer lag"
```

## 🔐 Phase 5: Security Configuration

### 5.1 TLS/SSL Setup

```bash
# Create TLS secret
kubectl create secret tls api-gateway-tls \
  --cert=path/to/your/cert.pem \
  --key=path/to/your/key.pem \
  -n api-gateway

# Apply ingress with TLS
kubectl apply -f k8s/ingress.yaml
```

### 5.2 Network Policies

```yaml
# k8s/network-policies.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: api-gateway-network-policy
  namespace: api-gateway
spec:
  podSelector:
    matchLabels:
      app: api-gateway
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: ingress-nginx
    ports:
    - protocol: TCP
      port: 8080
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: kafka
    ports:
    - protocol: TCP
      port: 9092
  - to:
    - namespaceSelector:
        matchLabels:
          name: your-namespace
    ports:
    - protocol: TCP
      port: 8080
```

## 🚀 Phase 6: Deployment & Testing

### 6.1 Deploy API Gateway

```bash
# Run the production deployment script
./scripts/deploy-production.sh

# Or deploy components individually
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
```

### 6.2 Load Testing

```bash
# Deploy k6 for load testing
helm install k6 grafana/k6 -n monitoring

# Create load test
kubectl apply -f k8s/load-test.yaml
```

### 6.3 Performance Testing

```bash
# Run performance tests
kubectl exec -it k6-0 -n monitoring -- k6 run \
  --vus 100 \
  --duration 5m \
  /scripts/load-test.js
```

## 📈 Phase 7: Scaling & Optimization

### 7.1 Horizontal Scaling

```bash
# Scale API Gateway based on load
kubectl scale deployment api-gateway --replicas=5 -n api-gateway

# Scale event consumers
kubectl scale deployment event-consumer --replicas=10 -n your-namespace
```

### 7.2 Resource Optimization

```yaml
# Optimize resource limits
resources:
  requests:
    memory: "1Gi"
    cpu: "500m"
  limits:
    memory: "2Gi"
    cpu: "1000m"
```

### 7.3 Auto-scaling

```yaml
# k8s/hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: api-gateway-hpa
  namespace: api-gateway
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api-gateway
  minReplicas: 3
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

## 🔄 Phase 8: CI/CD Integration

### 8.1 GitHub Actions Workflow

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

## 📋 Integration Checklist

### Infrastructure
- [ ] Kubernetes cluster with sufficient resources
- [ ] Storage classes configured
- [ ] Network policies applied
- [ ] TLS certificates installed
- [ ] Load balancer configured

### Event Processing
- [ ] Kafka cluster deployed and configured
- [ ] Event topics created
- [ ] Event consumers implemented
- [ ] Event handlers integrated with your application

### Application Integration
- [ ] Service discovery configured
- [ ] Authentication provider integrated
- [ ] Rate limiting configured for your user tiers
- [ ] Circuit breakers configured for your services

### Monitoring
- [ ] Prometheus and Grafana deployed
- [ ] Custom dashboards imported
- [ ] Alerting rules configured
- [ ] Jaeger tracing enabled

### Security
- [ ] TLS/SSL certificates installed
- [ ] Network policies applied
- [ ] RBAC configured
- [ ] Secrets management implemented

### Deployment
- [ ] API Gateway deployed and tested
- [ ] Load testing completed
- [ ] Performance benchmarks established
- [ ] Auto-scaling configured

### CI/CD
- [ ] Automated testing pipeline
- [ ] Automated deployment pipeline
- [ ] Rollback procedures tested
- [ ] Monitoring and alerting for deployments

## 🆘 Troubleshooting

### Common Issues

#### 1. Event Processing Not Working
```bash
# Check Kafka connectivity
kubectl exec -it kafka-0 -n kafka -- kafka-topics.sh --list --bootstrap-server localhost:9092

# Check event processor logs
kubectl logs -f deployment/api-gateway -n api-gateway | grep -i event
```

#### 2. High Latency
```bash
# Check resource usage
kubectl top pods -n api-gateway

# Check circuit breaker status
kubectl exec -it deployment/api-gateway -n api-gateway -- curl localhost:8080/admin/circuit-breakers
```

#### 3. Authentication Issues
```bash
# Check JWT configuration
kubectl get configmap gateway-config -n api-gateway -o yaml

# Test authentication endpoint
curl -X POST https://api.yourdomain.com/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"test","password":"test"}'
```

#### 4. Rate Limiting Issues
```bash
# Check rate limit status
kubectl exec -it deployment/api-gateway -n api-gateway -- curl localhost:8080/admin/stats

# Check Redis connectivity
kubectl exec -it deployment/api-gateway -n api-gateway -- redis-cli -h redis ping
```

## 📞 Support

For additional support and questions:

1. **Documentation**: Check the main README.md for detailed documentation
2. **Issues**: Create an issue in the GitHub repository
3. **Discussions**: Use GitHub Discussions for community support
4. **Enterprise Support**: Contact for enterprise-grade support

## 🎯 Next Steps

After successful integration:

1. **Performance Tuning**: Optimize based on your specific load patterns
2. **Security Hardening**: Implement additional security measures
3. **Disaster Recovery**: Set up backup and recovery procedures
4. **Compliance**: Ensure compliance with industry standards
5. **Training**: Train your team on the new infrastructure

---

**Congratulations!** You now have a production-ready API Gateway with distributed event processing capable of handling 100k+ users. The system provides comprehensive monitoring, security, and scalability features to support your enterprise application.
