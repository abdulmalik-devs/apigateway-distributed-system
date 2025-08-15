#!/bin/bash

# Deployment script for API Gateway

set -e

# Configuration
NAMESPACE="api-gateway"
IMAGE_NAME="api-gateway"
IMAGE_TAG=${IMAGE_TAG:-"latest"}

echo "Deploying API Gateway to Kubernetes..."

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo "kubectl is not installed or not in PATH"
    exit 1
fi

# Check if cluster is accessible
if ! kubectl cluster-info &> /dev/null; then
    echo "Cannot connect to Kubernetes cluster"
    exit 1
fi

# Create namespace if it doesn't exist
echo "Creating namespace ${NAMESPACE}..."
kubectl apply -f k8s/namespace.yaml

# Apply configuration
echo "Applying configuration..."
kubectl apply -f k8s/configmap.yaml

# Deploy Redis
echo "Deploying Redis..."
kubectl apply -f k8s/redis.yaml

# Wait for Redis to be ready
echo "Waiting for Redis to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment/redis -n ${NAMESPACE}

# Build and push Docker image
if [[ "${SKIP_BUILD}" != "true" ]]; then
    echo "Building Docker image..."
    docker build -t ${IMAGE_NAME}:${IMAGE_TAG} .
    
    # If using a registry, push the image
    if [[ -n "${DOCKER_REGISTRY}" ]]; then
        echo "Pushing image to registry..."
        docker tag ${IMAGE_NAME}:${IMAGE_TAG} ${DOCKER_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}
        docker push ${DOCKER_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}
        
        # Update deployment with registry image
        sed -i.bak "s|image: api-gateway:latest|image: ${DOCKER_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}|g" k8s/deployment.yaml
    fi
fi

# Deploy API Gateway
echo "Deploying API Gateway..."
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml

# Wait for deployment to be ready
echo "Waiting for API Gateway to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment/api-gateway -n ${NAMESPACE}

# Get service information
echo "Getting service information..."
kubectl get services -n ${NAMESPACE}

# Get pods status
echo "Getting pods status..."
kubectl get pods -n ${NAMESPACE}

# Show logs
echo "Recent logs:"
kubectl logs -n ${NAMESPACE} -l app.kubernetes.io/name=api-gateway --tail=20

echo "Deployment completed successfully!"
echo ""
echo "Access the API Gateway:"
echo "- Health check: http://<external-ip>/health"
echo "- Metrics: http://<external-ip>:9090/metrics"
echo "- API endpoints: http://<external-ip>/api/*"
echo ""
echo "Get external IP with:"
echo "kubectl get service api-gateway-service -n ${NAMESPACE}"

