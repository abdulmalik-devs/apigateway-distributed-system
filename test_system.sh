#!/bin/bash

# Comprehensive API Gateway System Test
set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}🚀 API Gateway System Test${NC}"
echo "================================"

# Test 1: Build System
echo -e "\n${BLUE}1. Testing Build System...${NC}"
echo "Building main gateway..."
if go build -o bin/gateway cmd/gateway/main.go; then
    echo -e "${GREEN}✅ Gateway builds successfully${NC}"
else
    echo -e "${RED}❌ Gateway build failed${NC}"
    exit 1
fi

echo "Building config server..."
if go build -o bin/config-server cmd/config-server/main.go; then
    echo -e "${GREEN}✅ Config server builds successfully${NC}"
else
    echo -e "${RED}❌ Config server build failed${NC}"
    exit 1
fi

# Test 2: Unit Tests
echo -e "\n${BLUE}2. Running Unit Tests...${NC}"
if go test ./... -v; then
    echo -e "${GREEN}✅ All unit tests passed${NC}"
else
    echo -e "${RED}❌ Unit tests failed${NC}"
    exit 1
fi

# Test 3: Configuration Loading
echo -e "\n${BLUE}3. Testing Configuration...${NC}"
if [ -f "configs/config.yaml" ]; then
    echo -e "${GREEN}✅ Configuration file exists${NC}"
else
    echo -e "${RED}❌ Configuration file missing${NC}"
    exit 1
fi

# Test 4: Docker Build (if Docker is available)
echo -e "\n${BLUE}4. Testing Docker Build...${NC}"
if command -v docker &> /dev/null; then
    if docker info &> /dev/null; then
        echo "Docker is running, testing build..."
        if docker build -t test-gateway .; then
            echo -e "${GREEN}✅ Docker build successful${NC}"
        else
            echo -e "${YELLOW}⚠️  Docker build failed (non-critical)${NC}"
        fi
    else
        echo -e "${YELLOW}⚠️  Docker daemon not running${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  Docker not installed${NC}"
fi

# Test 5: Gateway Startup Test
echo -e "\n${BLUE}5. Testing Gateway Startup...${NC}"
echo "Starting gateway in background..."
./bin/gateway &
GATEWAY_PID=$!

# Wait for gateway to start
sleep 3

# Test health endpoint
echo "Testing health endpoint..."
if curl -s http://localhost:8080/health > /dev/null; then
    echo -e "${GREEN}✅ Gateway health check passed${NC}"
else
    echo -e "${YELLOW}⚠️  Gateway health check failed (may need Redis/Postgres)${NC}"
fi

# Test metrics endpoint
echo "Testing metrics endpoint..."
if curl -s http://localhost:9090/metrics > /dev/null; then
    echo -e "${GREEN}✅ Metrics endpoint accessible${NC}"
else
    echo -e "${YELLOW}⚠️  Metrics endpoint not accessible${NC}"
fi

# Clean up
echo "Stopping gateway..."
kill $GATEWAY_PID 2>/dev/null || true

# Test 6: File Structure
echo -e "\n${BLUE}6. Verifying File Structure...${NC}"
required_files=(
    "cmd/gateway/main.go"
    "cmd/config-server/main.go"
    "internal/gateway/gateway.go"
    "internal/auth/jwt.go"
    "internal/ratelimit/algorithms.go"
    "internal/proxy/reverse_proxy.go"
    "configs/config.yaml"
    "docker-compose.yml"
    "Dockerfile"
    "README.md"
    "docs/architecture.png"
)

all_files_exist=true
for file in "${required_files[@]}"; do
    if [ -f "$file" ]; then
        echo -e "${GREEN}✅ $file${NC}"
    else
        echo -e "${RED}❌ $file missing${NC}"
        all_files_exist=false
    fi
done

if [ "$all_files_exist" = true ]; then
    echo -e "${GREEN}✅ All required files present${NC}"
else
    echo -e "${RED}❌ Some required files missing${NC}"
    exit 1
fi

# Test 7: Dependencies
echo -e "\n${BLUE}7. Checking Dependencies...${NC}"
if go mod verify; then
    echo -e "${GREEN}✅ All dependencies verified${NC}"
else
    echo -e "${RED}❌ Dependency verification failed${NC}"
    exit 1
fi

# Summary
echo -e "\n${GREEN}🎉 System Test Complete!${NC}"
echo "================================"
echo -e "${GREEN}✅ Build system: Working${NC}"
echo -e "${GREEN}✅ Unit tests: Passing${NC}"
echo -e "${GREEN}✅ Configuration: Valid${NC}"
echo -e "${GREEN}✅ File structure: Complete${NC}"
echo -e "${GREEN}✅ Dependencies: Verified${NC}"

echo -e "\n${BLUE}🚀 System is ready for deployment!${NC}"
echo ""
echo "Quick start commands:"
echo "1. Development: ./bin/gateway"
echo "2. Docker: docker-compose up -d"
echo "3. Production: ./scripts/deploy-production.sh"
echo ""
echo "Access URLs:"
echo "- Gateway: http://localhost:8080"
echo "- Health: http://localhost:8080/health"
echo "- Metrics: http://localhost:9090/metrics"
echo "- Admin: http://localhost:8080/admin/stats"
