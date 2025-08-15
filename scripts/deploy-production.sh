#!/bin/bash

# Production Deployment Script for API Gateway with 100k+ Users
# This script sets up the complete infrastructure including event processing

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="api-gateway"
KAFKA_NAMESPACE="kafka"
MONITORING_NAMESPACE="monitoring"
REGISTRY="your-registry"
IMAGE_TAG="latest"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check prerequisites
check_prerequisites() {
    print_status "Checking prerequisites..."
    
    # Check if kubectl is installed
    if ! command -v kubectl &> /dev/null; then
        print_error "kubectl is not installed"
        exit 1
    fi
    
    # Check if helm is installed
    if ! command -v helm &> /dev/null; then
        print_error "helm is not installed"
        exit 1
    fi
    
    # Check if docker is installed
    if ! command -v docker &> /dev/null; then
        print_error "docker is not installed"
        exit 1
    fi
    
    # Check cluster connectivity
    if ! kubectl cluster-info &> /dev/null; then
        print_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    
    print_success "Prerequisites check passed"
}

# Function to create namespaces
create_namespaces() {
    print_status "Creating namespaces..."
    
    kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    kubectl create namespace $KAFKA_NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    kubectl create namespace $MONITORING_NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    print_success "Namespaces created"
}

# Function to deploy monitoring stack
deploy_monitoring() {
    print_status "Deploying monitoring stack..."
    
    # Add Prometheus Helm repository
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
    helm repo add grafana https://grafana.github.io/helm-charts
    helm repo update
    
    # Deploy Prometheus stack
    helm install prometheus prometheus-community/kube-prometheus-stack \
        --namespace $MONITORING_NAMESPACE \
        --create-namespace \
        --set prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.resources.requests.storage=50Gi \
        --set prometheus.prometheusSpec.retention=30d \
        --set grafana.adminPassword=admin \
        --set grafana.persistence.enabled=true \
        --set grafana.persistence.size=10Gi
    
    # Deploy Jaeger
    helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
    helm install jaeger jaegertracing/jaeger \
        --namespace $MONITORING_NAMESPACE \
        --set storage.type=elasticsearch \
        --set storage.options.es.server-urls=http://elasticsearch-master:9200 \
        --set storage.options.es.num-shards=1 \
        --set storage.options.es.num-replicas=0
    
    print_success "Monitoring stack deployed"
}

# Function to deploy Kafka
deploy_kafka() {
    print_status "Deploying Kafka cluster..."
    
    # Deploy Zookeeper first
    kubectl apply -f k8s/kafka.yaml -n $KAFKA_NAMESPACE
    
    # Wait for Zookeeper to be ready
    print_status "Waiting for Zookeeper to be ready..."
    kubectl wait --for=condition=ready pod -l app=zookeeper -n $KAFKA_NAMESPACE --timeout=300s
    
    # Wait for Kafka to be ready
    print_status "Waiting for Kafka to be ready..."
    kubectl wait --for=condition=ready pod -l app=kafka -n $KAFKA_NAMESPACE --timeout=600s
    
    # Create Kafka topics
    create_kafka_topics
    
    print_success "Kafka cluster deployed"
}

# Function to create Kafka topics
create_kafka_topics() {
    print_status "Creating Kafka topics..."
    
    # Wait for Kafka to be fully ready
    sleep 30
    
    # Create topics using kubectl exec
    kubectl exec -n $KAFKA_NAMESPACE kafka-0 -- kafka-topics.sh \
        --create --topic api-gateway-events \
        --bootstrap-server localhost:9092 \
        --partitions 6 --replication-factor 3
    
    kubectl exec -n $KAFKA_NAMESPACE kafka-0 -- kafka-topics.sh \
        --create --topic user-events \
        --bootstrap-server localhost:9092 \
        --partitions 6 --replication-factor 3
    
    kubectl exec -n $KAFKA_NAMESPACE kafka-0 -- kafka-topics.sh \
        --create --topic audit-logs \
        --bootstrap-server localhost:9092 \
        --partitions 3 --replication-factor 3
    
    kubectl exec -n $KAFKA_NAMESPACE kafka-0 -- kafka-topics.sh \
        --create --topic metrics-stream \
        --bootstrap-server localhost:9092 \
        --partitions 3 --replication-factor 3
    
    print_success "Kafka topics created"
}

# Function to deploy Redis cluster
deploy_redis() {
    print_status "Deploying Redis cluster..."
    
    # Add Bitnami Helm repository
    helm repo add bitnami https://charts.bitnami.com/bitnami
    helm repo update
    
    # Deploy Redis cluster
    helm install redis-cluster bitnami/redis-cluster \
        --namespace $NAMESPACE \
        --set cluster.nodes=6 \
        --set cluster.replicas=1 \
        --set persistence.size=50Gi \
        --set password=your-redis-password \
        --set cluster.update.addNodes=true \
        --set cluster.update.deleteNodes=true
    
    print_success "Redis cluster deployed"
}

# Function to deploy PostgreSQL
deploy_postgresql() {
    print_status "Deploying PostgreSQL..."
    
    # Deploy PostgreSQL with read replicas
    helm install postgresql bitnami/postgresql \
        --namespace $NAMESPACE \
        --set global.postgresql.auth.postgresPassword=your-production-password \
        --set global.postgresql.auth.database=gateway_db \
        --set readReplicas.persistence.size=100Gi \
        --set readReplicas.replicaCount=2 \
        --set primary.persistence.size=100Gi \
        --set architecture=replication
    
    print_success "PostgreSQL deployed"
}

# Function to build and push Docker image
build_and_push_image() {
    print_status "Building and pushing Docker image..."
    
    # Build the image
    docker build -t $REGISTRY/api-gateway:$IMAGE_TAG .
    
    # Push the image
    docker push $REGISTRY/api-gateway:$IMAGE_TAG
    
    print_success "Docker image built and pushed"
}

# Function to deploy API Gateway
deploy_api_gateway() {
    print_status "Deploying API Gateway..."
    
    # Apply Kubernetes manifests
    kubectl apply -f k8s/namespace.yaml
    kubectl apply -f k8s/configmap.yaml
    kubectl apply -f k8s/deployment.yaml
    kubectl apply -f k8s/service.yaml
    kubectl apply -f k8s/redis.yaml
    
    # Update deployment with new image
    kubectl set image deployment/api-gateway api-gateway=$REGISTRY/api-gateway:$IMAGE_TAG -n $NAMESPACE
    
    # Scale for high availability
    kubectl scale deployment api-gateway --replicas=3 -n $NAMESPACE
    
    # Wait for deployment to be ready
    kubectl rollout status deployment/api-gateway -n $NAMESPACE --timeout=300s
    
    print_success "API Gateway deployed"
}

# Function to deploy ingress
deploy_ingress() {
    print_status "Deploying ingress..."
    
    # Create TLS secret (you need to provide your own certificates)
    kubectl create secret tls api-gateway-tls \
        --cert=path/to/your/cert.pem \
        --key=path/to/your/key.pem \
        -n $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    # Apply ingress
    kubectl apply -f k8s/ingress.yaml
    
    print_success "Ingress deployed"
}

# Function to configure monitoring
configure_monitoring() {
    print_status "Configuring monitoring..."
    
    # Apply Prometheus configuration
    kubectl apply -f k8s/monitoring.yaml -n $MONITORING_NAMESPACE
    
    # Apply alerting rules
    kubectl apply -f configs/alert_rules.yml -n $MONITORING_NAMESPACE
    
    # Import Grafana dashboards
    kubectl apply -f k8s/grafana-dashboards.yaml -n $MONITORING_NAMESPACE
    
    print_success "Monitoring configured"
}

# Function to run load tests
run_load_tests() {
    print_status "Running load tests..."
    
    # Deploy k6 for load testing
    helm install k6 grafana/k6 -n $MONITORING_NAMESPACE
    
    # Create load test job
    kubectl apply -f k8s/load-test.yaml -n $NAMESPACE
    
    print_success "Load tests initiated"
}

# Function to display deployment information
display_info() {
    print_success "Deployment completed successfully!"
    echo
    echo "=== Deployment Information ==="
    echo "API Gateway: https://api.yourdomain.com"
    echo "Grafana: http://localhost:3001 (admin/admin)"
    echo "Prometheus: http://localhost:9091"
    echo "Jaeger: http://localhost:16687"
    echo "Kafka UI: http://localhost:8080 (if deployed)"
    echo
    echo "=== Useful Commands ==="
    echo "Check gateway status: kubectl get pods -n $NAMESPACE"
    echo "View gateway logs: kubectl logs -f deployment/api-gateway -n $NAMESPACE"
    echo "Check Kafka topics: kubectl exec -it kafka-0 -n $KAFKA_NAMESPACE -- kafka-topics.sh --list --bootstrap-server localhost:9092"
    echo "Scale gateway: kubectl scale deployment api-gateway --replicas=5 -n $NAMESPACE"
    echo
    echo "=== Next Steps ==="
    echo "1. Update DNS to point api.yourdomain.com to your ingress IP"
    echo "2. Configure your authentication provider"
    echo "3. Set up alerting in Grafana"
    echo "4. Configure backup strategies for databases"
    echo "5. Set up CI/CD pipeline for automated deployments"
}

# Main deployment function
main() {
    print_status "Starting production deployment for 100k+ users..."
    
    check_prerequisites
    create_namespaces
    deploy_monitoring
    deploy_kafka
    deploy_redis
    deploy_postgresql
    build_and_push_image
    deploy_api_gateway
    deploy_ingress
    configure_monitoring
    run_load_tests
    display_info
}

# Handle script arguments
case "${1:-}" in
    --help|-h)
        echo "Usage: $0 [OPTIONS]"
        echo "Options:"
        echo "  --help, -h     Show this help message"
        echo "  --skip-build   Skip Docker build and push"
        echo "  --skip-tests   Skip load testing"
        echo "  --monitoring-only   Deploy only monitoring stack"
        echo "  --kafka-only   Deploy only Kafka"
        exit 0
        ;;
    --skip-build)
        SKIP_BUILD=true
        shift
        ;;
    --skip-tests)
        SKIP_TESTS=true
        shift
        ;;
    --monitoring-only)
        check_prerequisites
        create_namespaces
        deploy_monitoring
        exit 0
        ;;
    --kafka-only)
        check_prerequisites
        create_namespaces
        deploy_kafka
        exit 0
        ;;
esac

# Run main deployment
main "$@"
