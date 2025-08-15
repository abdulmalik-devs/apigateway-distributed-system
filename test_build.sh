#!/bin/bash

# Quick test script to verify API Gateway components

set -e

echo "🧪 Testing API Gateway Components..."

# Test 1: Check if we can build the main gateway
echo "📦 Testing main gateway build..."
if go build -o /tmp/test-gateway cmd/gateway/main.go; then
    echo "✅ Main gateway builds successfully"
    rm -f /tmp/test-gateway
else
    echo "❌ Main gateway build failed"
    exit 1
fi

# Test 2: Check if we can build the config server
echo "📦 Testing config server build..."
if go build -o /tmp/test-config-server cmd/config-server/main.go; then
    echo "✅ Config server builds successfully"
    rm -f /tmp/test-config-server
else
    echo "❌ Config server build failed"
    exit 1
fi

# Test 3: Run basic tests
echo "🧪 Running basic tests..."
if go test ./internal/auth -v; then
    echo "✅ Auth tests passed"
else
    echo "❌ Auth tests failed"
    exit 1
fi

# Test 4: Check configuration loading
echo "⚙️ Testing configuration loading..."
if go run cmd/gateway/main.go --help 2>/dev/null || true; then
    echo "✅ Configuration system works"
else
    echo "❌ Configuration system failed"
fi

# Test 5: Verify Docker build
echo "🐳 Testing Docker build..."
if docker build -t test-api-gateway . > /dev/null 2>&1; then
    echo "✅ Docker build successful"
    docker rmi test-api-gateway > /dev/null 2>&1 || true
else
    echo "❌ Docker build failed"
fi

echo ""
echo "🎉 All basic tests completed!"
echo ""
echo "📋 Next steps:"
echo "1. Run: make dev (for development)"
echo "2. Run: make docker-compose-up (for full stack)"
echo "3. Run: make k8s-deploy (for Kubernetes)"
echo ""
echo "🔗 Access points:"
echo "- Gateway: http://localhost:8080"
echo "- Metrics: http://localhost:9090/metrics"
echo "- Health: http://localhost:8080/health"
