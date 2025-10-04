# DataBridge Makefile
# Comprehensive build, test, and development workflow automation

.PHONY: help build clean test test-unit test-integration test-coverage lint fmt vet deps deps-update
.PHONY: run run-dev run-debug dev dev-fresh docker docker-build docker-run docker-clean
.PHONY: frontend frontend-dev frontend-build frontend-test frontend-lint
.PHONY: docs docs-serve api-docs coverage-html benchmark release
.PHONY: setup dev-setup pre-commit install-tools check-tools security-scan
.DEFAULT_GOAL := help

# Project Configuration
PROJECT_NAME := databridge
MAIN_PATH := ./cmd/databridge
BINARY_NAME := databridge
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build Configuration
BUILD_DIR := ./build
DIST_DIR := ./dist
COVERAGE_DIR := ./coverage
DOCS_DIR := ./docs
FRONTEND_DIR := ./frontend

# Go Configuration
GO_VERSION := 1.21
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
CGO_ENABLED ?= 0

# Build Flags
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_COMMIT) -s -w"
BUILD_FLAGS := $(LDFLAGS) -trimpath

# Test Configuration
TEST_TIMEOUT := 30m
TEST_COVERAGE_THRESHOLD := 100
INTEGRATION_TEST_TIMEOUT := 60m

# Docker Configuration
DOCKER_IMAGE := databridge
DOCKER_TAG := $(VERSION)
DOCKER_REGISTRY ?= localhost:5000

# Color output
RED := \033[31m
GREEN := \033[32m
YELLOW := \033[33m
BLUE := \033[34m
MAGENTA := \033[35m
CYAN := \033[36m
WHITE := \033[37m
RESET := \033[0m

##@ Help

help: ## Display this help message
	@echo "$(BLUE)DataBridge Development Commands$(RESET)"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make $(CYAN)<target>$(RESET)\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  $(CYAN)%-20s$(RESET) %s\n", $$1, $$2 } /^##@/ { printf "\n$(BLUE)%s$(RESET)\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

setup: deps install-tools dev-setup ## Complete development environment setup
	@echo "$(GREEN)✓ Development environment setup complete$(RESET)"

dev-setup: ## Set up development environment
	@echo "$(YELLOW)Setting up development environment...$(RESET)"
	@mkdir -p $(BUILD_DIR) $(DIST_DIR) $(COVERAGE_DIR)
	@cp -n .env.example .env 2>/dev/null || true
	@echo "$(GREEN)✓ Development environment ready$(RESET)"

install-tools: ## Install development tools
	@echo "$(YELLOW)Installing development tools...$(RESET)"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/swaggo/swag/cmd/swag@latest
	@go install go.uber.org/mock/mockgen@latest
	@go install github.com/axw/gocov/gocov@latest
	@go install github.com/AlekSi/gocov-xml@latest
	@echo "$(GREEN)✓ Development tools installed$(RESET)"

check-tools: ## Verify development tools are installed
	@echo "$(YELLOW)Checking development tools...$(RESET)"
	@command -v golangci-lint >/dev/null 2>&1 || { echo "$(RED)✗ golangci-lint not installed$(RESET)"; exit 1; }
	@command -v gosec >/dev/null 2>&1 || { echo "$(RED)✗ gosec not installed$(RESET)"; exit 1; }
	@command -v goimports >/dev/null 2>&1 || { echo "$(RED)✗ goimports not installed$(RESET)"; exit 1; }
	@command -v swag >/dev/null 2>&1 || { echo "$(RED)✗ swag not installed$(RESET)"; exit 1; }
	@echo "$(GREEN)✓ All development tools are installed$(RESET)"

pre-commit: fmt lint vet test-unit ## Run pre-commit checks (format, lint, vet, test)
	@echo "$(GREEN)✓ Pre-commit checks passed$(RESET)"

##@ Dependencies

deps: ## Install project dependencies
	@echo "$(YELLOW)Installing dependencies...$(RESET)"
	@go mod download
	@go mod verify
	@echo "$(GREEN)✓ Dependencies installed$(RESET)"

deps-update: ## Update dependencies
	@echo "$(YELLOW)Updating dependencies...$(RESET)"
	@go get -u ./...
	@go mod tidy
	@echo "$(GREEN)✓ Dependencies updated$(RESET)"

deps-clean: ## Clean dependency cache
	@echo "$(YELLOW)Cleaning dependency cache...$(RESET)"
	@go clean -modcache
	@echo "$(GREEN)✓ Dependency cache cleaned$(RESET)"

##@ Build

build: clean ## Build the application
	@echo "$(YELLOW)Building $(PROJECT_NAME) $(VERSION)...$(RESET)"
	@CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(GREEN)✓ Build complete: $(BUILD_DIR)/$(BINARY_NAME)$(RESET)"

build-all: ## Build for all platforms
	@echo "$(YELLOW)Building for all platforms...$(RESET)"
	@mkdir -p $(DIST_DIR)
	@for os in linux darwin windows; do \
		for arch in amd64 arm64; do \
			if [ "$$os" = "windows" ]; then ext=".exe"; else ext=""; fi; \
			echo "Building $$os/$$arch..."; \
			CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
				go build $(BUILD_FLAGS) \
				-o $(DIST_DIR)/$(BINARY_NAME)-$$os-$$arch$$ext $(MAIN_PATH); \
		done; \
	done
	@echo "$(GREEN)✓ Multi-platform build complete$(RESET)"

clean: ## Clean build artifacts
	@echo "$(YELLOW)Cleaning build artifacts...$(RESET)"
	@rm -rf $(BUILD_DIR)/* $(DIST_DIR)/*
	@go clean -cache
	@echo "$(GREEN)✓ Clean complete$(RESET)"

##@ Code Quality

fmt: ## Format code
	@echo "$(YELLOW)Formatting code...$(RESET)"
	@gofmt -s -w .
	@goimports -w .
	@echo "$(GREEN)✓ Code formatted$(RESET)"

lint: ## Run linters
	@echo "$(YELLOW)Running linters...$(RESET)"
	@golangci-lint run --timeout=5m ./...
	@echo "$(GREEN)✓ Linting complete$(RESET)"

vet: ## Run go vet
	@echo "$(YELLOW)Running go vet...$(RESET)"
	@go vet ./...
	@echo "$(GREEN)✓ Vet complete$(RESET)"

security-scan: ## Run security scan
	@echo "$(YELLOW)Running security scan...$(RESET)"
	@gosec -fmt json -out $(BUILD_DIR)/security-report.json -stdout ./... || true
	@echo "$(GREEN)✓ Security scan complete$(RESET)"

##@ Testing

test: test-unit test-integration ## Run all tests
	@echo "$(GREEN)✓ All tests completed$(RESET)"

test-unit: ## Run unit tests
	@echo "$(YELLOW)Running unit tests...$(RESET)"
	@mkdir -p $(COVERAGE_DIR)
	@go test -v -race -timeout=$(TEST_TIMEOUT) \
		-coverprofile=$(COVERAGE_DIR)/unit.out \
		-covermode=atomic \
		./...
	@echo "$(GREEN)✓ Unit tests completed$(RESET)"

test-integration: build ## Run integration tests
	@echo "$(YELLOW)Running integration tests...$(RESET)"
	@mkdir -p $(COVERAGE_DIR)
	@go test -v -race -timeout=$(INTEGRATION_TEST_TIMEOUT) \
		-tags=integration \
		-coverprofile=$(COVERAGE_DIR)/integration.out \
		-covermode=atomic \
		./test/integration/...
	@echo "$(GREEN)✓ Integration tests completed$(RESET)"

test-coverage: test-unit ## Generate and display test coverage
	@echo "$(YELLOW)Generating coverage report...$(RESET)"
	@go tool cover -func=$(COVERAGE_DIR)/unit.out
	@go tool cover -html=$(COVERAGE_DIR)/unit.out -o $(COVERAGE_DIR)/coverage.html
	@echo "$(GREEN)✓ Coverage report generated: $(COVERAGE_DIR)/coverage.html$(RESET)"

coverage-html: test-coverage ## Open coverage report in browser
	@echo "$(YELLOW)Opening coverage report in browser...$(RESET)"
	@open $(COVERAGE_DIR)/coverage.html 2>/dev/null || \
		xdg-open $(COVERAGE_DIR)/coverage.html 2>/dev/null || \
		echo "Please open $(COVERAGE_DIR)/coverage.html manually"

benchmark: ## Run benchmarks
	@echo "$(YELLOW)Running benchmarks...$(RESET)"
	@mkdir -p $(BUILD_DIR)
	@go test -bench=. -benchmem -cpuprofile=$(BUILD_DIR)/cpu.prof \
		-memprofile=$(BUILD_DIR)/mem.prof ./...
	@echo "$(GREEN)✓ Benchmarks completed$(RESET)"

##@ Running

run: build ## Build and run the application
	@echo "$(YELLOW)Starting DataBridge...$(RESET)"
	@$(BUILD_DIR)/$(BINARY_NAME)

run-dev: ## Run in development mode with INFO logging
	@echo "$(YELLOW)Starting DataBridge in development mode...$(RESET)"
	@go run $(MAIN_PATH) --log-level info --data-dir ./data-dev

run-debug: ## Run with DEBUG logging (verbose)
	@echo "$(YELLOW)Starting DataBridge in debug mode (very verbose)...$(RESET)"
	@go run $(MAIN_PATH) --log-level debug --data-dir ./data-debug

kill: ## Stop all DataBridge processes (backend and frontend)
	@./kill-all.sh

kill-all: kill ## Alias for kill

dev: ## Start both backend and frontend in development mode (installs deps first)
	@echo "$(YELLOW)Starting DataBridge full stack in development mode...$(RESET)"
	@echo "$(CYAN)Checking frontend dependencies...$(RESET)"
	@cd $(FRONTEND_DIR) && [ -d "node_modules" ] || npm install
	@echo "$(CYAN)Backend will run on http://localhost:8080 (log level: INFO)$(RESET)"
	@echo "$(CYAN)Frontend will run on http://localhost:3000$(RESET)"
	@echo "$(CYAN)Tip: Use 'make dev-debug' for verbose DEBUG logs$(RESET)"
	@echo ""
	@trap 'kill 0' EXIT; \
		(go run $(MAIN_PATH) --log-level info --data-dir ./data-dev & \
		 cd $(FRONTEND_DIR) && npm run dev) || true

dev-debug: ## Start both backend (DEBUG logs) and frontend
	@echo "$(YELLOW)Starting DataBridge full stack in DEBUG mode (verbose)...$(RESET)"
	@echo "$(CYAN)Checking frontend dependencies...$(RESET)"
	@cd $(FRONTEND_DIR) && [ -d "node_modules" ] || npm install
	@echo "$(CYAN)Backend will run on http://localhost:8080 (log level: DEBUG)$(RESET)"
	@echo "$(CYAN)Frontend will run on http://localhost:3000$(RESET)"
	@echo "$(RED)Warning: DEBUG mode produces very verbose logs$(RESET)"
	@echo ""
	@trap 'kill 0' EXIT; \
		(go run $(MAIN_PATH) --log-level debug --data-dir ./data-dev & \
		 cd $(FRONTEND_DIR) && npm run dev) || true

dev-fresh: deps frontend-setup ## Fresh start: install all deps and start dev servers
	@echo "$(YELLOW)Starting fresh development environment...$(RESET)"
	@echo "$(CYAN)Clearing Next.js cache...$(RESET)"
	@cd $(FRONTEND_DIR) && rm -rf .next
	@$(MAKE) dev

##@ Docker

docker: docker-build ## Build and run Docker container

docker-build: ## Build Docker image
	@echo "$(YELLOW)Building Docker image...$(RESET)"
	@docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest
	@echo "$(GREEN)✓ Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)$(RESET)"

docker-run: ## Run Docker container
	@echo "$(YELLOW)Running Docker container...$(RESET)"
	@docker run --rm -it -p 8080:8080 $(DOCKER_IMAGE):$(DOCKER_TAG)

docker-push: ## Push Docker image to registry
	@echo "$(YELLOW)Pushing Docker image to $(DOCKER_REGISTRY)...$(RESET)"
	@docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	@docker push $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo "$(GREEN)✓ Docker image pushed$(RESET)"

docker-clean: ## Clean Docker images
	@echo "$(YELLOW)Cleaning Docker images...$(RESET)"
	@docker rmi $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest 2>/dev/null || true
	@echo "$(GREEN)✓ Docker images cleaned$(RESET)"

##@ Frontend

frontend-setup: ## Set up frontend development environment
	@echo "$(YELLOW)Setting up frontend environment...$(RESET)"
	@cd $(FRONTEND_DIR) && npm install
	@echo "$(GREEN)✓ Frontend environment ready$(RESET)"

frontend-dev: ## Start frontend development server
	@echo "$(YELLOW)Starting frontend development server...$(RESET)"
	@cd $(FRONTEND_DIR) && npm run dev

frontend-build: ## Build frontend for production
	@echo "$(YELLOW)Building frontend for production...$(RESET)"
	@cd $(FRONTEND_DIR) && npm run build
	@echo "$(GREEN)✓ Frontend build complete$(RESET)"

frontend-test: ## Run frontend tests
	@echo "$(YELLOW)Running frontend tests...$(RESET)"
	@cd $(FRONTEND_DIR) && npm run test
	@echo "$(GREEN)✓ Frontend tests completed$(RESET)"

frontend-lint: ## Lint frontend code
	@echo "$(YELLOW)Linting frontend code...$(RESET)"
	@cd $(FRONTEND_DIR) && npm run lint
	@echo "$(GREEN)✓ Frontend linting complete$(RESET)"

##@ Documentation

docs: ## Generate documentation
	@echo "$(YELLOW)Generating documentation...$(RESET)"
	@mkdir -p $(DOCS_DIR)
	@go doc -all ./... > $(DOCS_DIR)/api.txt
	@echo "$(GREEN)✓ Documentation generated$(RESET)"

api-docs: ## Generate API documentation with Swagger
	@echo "$(YELLOW)Generating API documentation...$(RESET)"
	@swag init -g $(MAIN_PATH)/main.go -o $(DOCS_DIR)/swagger
	@echo "$(GREEN)✓ API documentation generated$(RESET)"

docs-serve: docs ## Serve documentation locally
	@echo "$(YELLOW)Serving documentation at http://localhost:6060...$(RESET)"
	@godoc -http=:6060

##@ Release

release: clean test lint build-all ## Create a release build
	@echo "$(YELLOW)Creating release $(VERSION)...$(RESET)"
	@mkdir -p $(DIST_DIR)
	@for file in $(DIST_DIR)/$(BINARY_NAME)-*; do \
		if [ -f "$$file" ]; then \
			echo "Creating archive for $$file..."; \
			base=$$(basename "$$file"); \
			if [[ "$$base" == *windows* ]]; then \
				zip -j "$$file.zip" "$$file" README.md LICENSE CHANGELOG.md; \
			else \
				tar -czf "$$file.tar.gz" -C $(DIST_DIR) "$$(basename "$$file")" \
					-C .. README.md LICENSE CHANGELOG.md; \
			fi; \
		fi; \
	done
	@echo "$(GREEN)✓ Release $(VERSION) created in $(DIST_DIR)/$(RESET)"

##@ Utilities

version: ## Show version information
	@echo "$(BLUE)DataBridge Version Information$(RESET)"
	@echo "Version:    $(VERSION)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Go Version: $(shell go version)"
	@echo "Platform:   $(GOOS)/$(GOARCH)"

info: ## Show project information
	@echo "$(BLUE)DataBridge Project Information$(RESET)"
	@echo "Project:    $(PROJECT_NAME)"
	@echo "Main Path:  $(MAIN_PATH)"
	@echo "Binary:     $(BINARY_NAME)"
	@echo "Build Dir:  $(BUILD_DIR)"
	@echo "Dist Dir:   $(DIST_DIR)"
	@echo "Coverage:   $(COVERAGE_DIR)"

env: ## Show environment variables
	@echo "$(BLUE)Environment Variables$(RESET)"
	@echo "GOOS:         $(GOOS)"
	@echo "GOARCH:       $(GOARCH)"
	@echo "CGO_ENABLED:  $(CGO_ENABLED)"
	@echo "GO_VERSION:   $(GO_VERSION)"
	@env | grep -E '^(DATABRIDGE|DB)_' || echo "No DataBridge environment variables set"

logs: ## Show application logs (if running)
	@echo "$(YELLOW)Showing recent application logs...$(RESET)"
	@tail -f ./data/logs/*.log 2>/dev/null || echo "No log files found in ./data/logs/"

##@ Maintenance

update: deps-update ## Update all dependencies and tools
	@echo "$(YELLOW)Updating development tools...$(RESET)"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@echo "$(GREEN)✓ All updates complete$(RESET)"

reset: clean ## Reset development environment
	@echo "$(YELLOW)Resetting development environment...$(RESET)"
	@rm -rf ./data ./data-dev ./data-debug
	@rm -f .env
	@echo "$(GREEN)✓ Development environment reset$(RESET)"

health: ## Check system health
	@echo "$(YELLOW)Checking system health...$(RESET)"
	@go version
	@docker --version 2>/dev/null || echo "Docker not available"
	@node --version 2>/dev/null || echo "Node.js not available"
	@npm --version 2>/dev/null || echo "npm not available"
	@echo "$(GREEN)✓ System health check complete$(RESET)"

# Advanced targets for CI/CD integration
ci-test: deps lint vet security-scan test-coverage ## Complete CI test suite
	@echo "$(GREEN)✓ CI test suite completed$(RESET)"

ci-build: clean build build-all ## CI build process
	@echo "$(GREEN)✓ CI build completed$(RESET)"

# Database maintenance
db-reset: ## Reset local databases
	@echo "$(YELLOW)Resetting local databases...$(RESET)"
	@rm -rf ./data/flowfiles ./data-dev/flowfiles ./data-debug/flowfiles
	@echo "$(GREEN)✓ Databases reset$(RESET)"

db-backup: ## Backup local databases
	@echo "$(YELLOW)Backing up databases...$(RESET)"
	@mkdir -p ./backups/$(shell date +%Y%m%d_%H%M%S)
	@cp -r ./data ./backups/$(shell date +%Y%m%d_%H%M%S)/ 2>/dev/null || echo "No data directory found"
	@echo "$(GREEN)✓ Database backup complete$(RESET)"