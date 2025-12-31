.PHONY: all build build-hub build-agent test lint fmt clean dev help

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

# Go variables
GOCMD := go
GOTEST := $(GOCMD) test
GOBUILD := $(GOCMD) build
GOVET := $(GOCMD) vet
GOFMT := gofmt

# Output directories
BIN_DIR := bin

all: build ## Build all binaries

build: build-hub build-agent ## Build hub and agent

build-hub: ## Build the hub server
	@echo "Building hub..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/hub ./cmd/hub

build-agent: ## Build the agent
	@echo "Building agent..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/agent ./cmd/agent

test: ## Run tests
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-short: ## Run short tests
	$(GOTEST) -v -short ./...

lint: ## Run linters
	@echo "Running go vet..."
	$(GOVET) ./...
	@echo "Checking formatting..."
	@test -z "$$($(GOFMT) -l .)" || (echo "Files need formatting:" && $(GOFMT) -l . && exit 1)

fmt: ## Format code
	$(GOFMT) -w .

tidy: ## Tidy dependencies
	$(GOCMD) mod tidy

clean: ## Clean build artifacts
	rm -rf $(BIN_DIR)
	rm -f coverage.out

# Development
dev: ## Start development server
	@echo "Starting hub in development mode..."
	$(GOCMD) run ./cmd/hub serve

dev-agent: ## Start agent in development mode
	@echo "Starting agent in development mode..."
	$(GOCMD) run ./cmd/agent run

# Web frontend
web-install: ## Install web dependencies
	cd web && npm install

web-dev: ## Start web development server
	cd web && npm run dev

web-build: ## Build web frontend
	cd web && npm run build

web-lint: ## Lint web frontend
	cd web && npm run lint

# Docker
docker-build: ## Build Docker images
	docker build -t sentinel-hub:$(VERSION) -f Dockerfile.hub .
	docker build -t sentinel-agent:$(VERSION) -f Dockerfile.agent .

docker-push: ## Push Docker images
	docker push sentinel-hub:$(VERSION)
	docker push sentinel-agent:$(VERSION)

# Database
migrate-up: ## Run database migrations
	@echo "Running migrations..."
	# TODO: Add migration command

migrate-down: ## Rollback database migrations
	@echo "Rolling back migrations..."
	# TODO: Add migration rollback command

migrate-create: ## Create a new migration (usage: make migrate-create NAME=migration_name)
	@echo "Creating migration: $(NAME)"
	# TODO: Add migration creation command

# Help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
