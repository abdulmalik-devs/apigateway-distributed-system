# Makefile for API Gateway

# Variables
APP_NAME := api-gateway
VERSION := 1.0.0
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_COMMIT)

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod

# Docker parameters
DOCKER_REGISTRY ?= 
IMAGE_NAME := $(APP_NAME)
IMAGE_TAG ?= latest

# Kubernetes parameters
NAMESPACE := api-gateway

.PHONY: help build clean test deps docker-build docker-push deploy k8s-deploy local-run

help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

deps: ## Download dependencies
	$(GOMOD) download
	$(GOMOD) verify

build: ## Build the application
	@echo "Building $(APP_NAME)..."
	@mkdir -p bin
	CGO_ENABLED=0 $(GOBUILD) -ldflags "$(LDFLAGS)" -o bin/gateway cmd/gateway/main.go
	@echo "Build completed: bin/gateway"

build-all: ## Build for all platforms
	@echo "Building for all platforms..."
	@./scripts/build.sh

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -rf bin/

test: ## Run tests
	$(GOTEST) -v ./...

test-race: ## Run tests with race detection
	$(GOTEST) -race -v ./...

test-coverage: ## Run tests with coverage
	$(GOTEST) -cover -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

lint: ## Run linter
	golangci-lint run

format: ## Format code
	gofmt -s -w .
	goimports -w .

docker-build: ## Build Docker image
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

docker-push: docker-build ## Build and push Docker image
ifdef DOCKER_REGISTRY
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(DOCKER_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	docker push $(DOCKER_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
else
	@echo "DOCKER_REGISTRY not set, skipping push"
endif

docker-run: docker-build ## Run Docker container locally
	docker run -p 8080:8080 -p 9090:9090 --name $(APP_NAME) $(IMAGE_NAME):$(IMAGE_TAG)

docker-compose-up: ## Start with docker-compose
	docker-compose up -d

docker-compose-down: ## Stop docker-compose
	docker-compose down

docker-compose-logs: ## Show docker-compose logs
	docker-compose logs -f gateway

local-run: build ## Run locally
	./bin/gateway

dev: ## Run in development mode
	$(GOCMD) run cmd/gateway/main.go

k8s-deploy: ## Deploy to Kubernetes
	@./scripts/deploy.sh

k8s-undeploy: ## Remove from Kubernetes
	kubectl delete -f k8s/ --ignore-not-found=true

k8s-status: ## Check Kubernetes deployment status
	kubectl get all -n $(NAMESPACE)

k8s-logs: ## Show Kubernetes logs
	kubectl logs -n $(NAMESPACE) -l app.kubernetes.io/name=api-gateway -f

k8s-port-forward: ## Port forward to local machine
	kubectl port-forward -n $(NAMESPACE) service/api-gateway-service 8080:80

prometheus-config: ## Generate Prometheus configuration
	@echo "global:" > configs/prometheus.yml
	@echo "  scrape_interval: 15s" >> configs/prometheus.yml
	@echo "scrape_configs:" >> configs/prometheus.yml
	@echo "  - job_name: 'api-gateway'" >> configs/prometheus.yml
	@echo "    static_configs:" >> configs/prometheus.yml
	@echo "      - targets: ['gateway:9090']" >> configs/prometheus.yml

init-examples: ## Initialize example services
	@mkdir -p examples/user-service examples/order-service examples/payment-service
	@echo '{"service": "user-service", "status": "healthy", "version": "1.0.0"}' > examples/user-service/health.json
	@echo '{"service": "order-service", "status": "healthy", "version": "1.0.0"}' > examples/order-service/health.json
	@echo '{"service": "payment-service", "status": "healthy", "version": "1.0.0"}' > examples/payment-service/health.json

benchmark: ## Run benchmarks
	$(GOTEST) -bench=. -benchmem ./...

security-scan: ## Run security scan
	gosec ./...

mod-update: ## Update Go modules
	$(GOMOD) tidy
	$(GOMOD) verify

install-tools: ## Install development tools
	$(GOGET) github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOGET) golang.org/x/tools/cmd/goimports@latest
	$(GOGET) github.com/securecodewarrior/gosec/v2/cmd/gosec@latest

all: clean deps build test ## Clean, get dependencies, build and test

.DEFAULT_GOAL := help
