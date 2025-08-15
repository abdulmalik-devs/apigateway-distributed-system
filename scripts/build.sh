#!/bin/bash

# Build script for API Gateway

set -e

echo "Building API Gateway..."

# Build info
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
VERSION=${VERSION:-"1.0.0"}

# Build flags
LDFLAGS="-X main.version=${VERSION} -X main.buildDate=${BUILD_DATE} -X main.gitCommit=${GIT_COMMIT}"

# Clean previous builds
echo "Cleaning previous builds..."
rm -rf bin/

# Create bin directory
mkdir -p bin/

# Build for different platforms
echo "Building for Linux (amd64)..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
    -ldflags "${LDFLAGS}" \
    -o bin/gateway-linux-amd64 \
    cmd/gateway/main.go

echo "Building for macOS (amd64)..."
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build \
    -ldflags "${LDFLAGS}" \
    -o bin/gateway-darwin-amd64 \
    cmd/gateway/main.go

echo "Building for macOS (arm64)..."
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build \
    -ldflags "${LDFLAGS}" \
    -o bin/gateway-darwin-arm64 \
    cmd/gateway/main.go

echo "Building for Windows (amd64)..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build \
    -ldflags "${LDFLAGS}" \
    -o bin/gateway-windows-amd64.exe \
    cmd/gateway/main.go

# Build config server
echo "Building config server..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
    -ldflags "${LDFLAGS}" \
    -o bin/config-server-linux-amd64 \
    cmd/config-server/main.go

echo "Build completed successfully!"
echo "Binaries available in bin/ directory:"
ls -la bin/

