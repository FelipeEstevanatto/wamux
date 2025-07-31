# Evolution Go API Makefile

# Variables
APP_NAME = evolution-go
BINARY_NAME = server
CMD_PATH = ./cmd/evolution-go
MAIN_FILE = $(CMD_PATH)/main.go
BUILD_DIR = ./build
DOCKER_IMAGE = evolution-go
DOCKER_TAG = latest

# Go parameters
GOCMD = go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get
GOMOD = $(GOCMD) mod
GOFMT = $(GOCMD) fmt
GOVET = $(GOCMD) vet

# Default target
.PHONY: help
help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: dev
dev: ## Run the application in development mode
	@echo "Starting development server..."
	$(GOCMD) run $(MAIN_FILE) -dev

.PHONY: build
build: ## Build the application
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: build-local
build-local: ## Build the application for local OS
	@echo "Building $(APP_NAME) for local platform..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)
	@echo "Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: run
run: build-local ## Build and run the application locally
	@echo "Running $(APP_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

.PHONY: test
test: ## Run all tests
	@echo "Running tests..."
	$(GOTEST) -v ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

.PHONY: deps
deps: ## Download and install dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

.PHONY: deps-update
deps-update: ## Update dependencies
	@echo "Updating dependencies..."
	$(GOMOD) tidy
	$(GOGET) -u ./...

.PHONY: fmt
fmt: ## Format Go code
	@echo "Formatting code..."
	$(GOFMT) ./...

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	$(GOVET) ./...

.PHONY: lint
lint: fmt vet ## Run linting tools
	@echo "Linting completed"

.PHONY: swagger
swagger: ## Generate Swagger documentation
	@echo "Generating Swagger documentation..."
	swag init -g $(MAIN_FILE) -o ./docs

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

.PHONY: docker-run
docker-run: ## Run Docker container
	@echo "Running Docker container..."
	docker run --rm -p 8081:8081 $(DOCKER_IMAGE):$(DOCKER_TAG)

.PHONY: docker-run-dev
docker-run-dev: ## Run Docker container in development mode
	@echo "Running Docker container in development mode..."
	docker run --rm -p 8081:8081 -v $(PWD):/app $(DOCKER_IMAGE):$(DOCKER_TAG) -dev

.PHONY: docker-clean
docker-clean: ## Remove Docker images
	@echo "Cleaning Docker images..."
	docker rmi $(DOCKER_IMAGE):$(DOCKER_TAG) || true
	docker system prune -f

.PHONY: logs
logs: ## Show application logs
	@echo "Showing logs..."
	tail -f logs/*.log

.PHONY: install-tools
install-tools: ## Install development tools
	@echo "Installing development tools..."
	$(GOGET) github.com/swaggo/swag/cmd/swag@latest
	$(GOGET) golang.org/x/tools/cmd/goimports@latest

.PHONY: check
check: deps lint test ## Run all checks (dependencies, linting, tests)
	@echo "All checks completed successfully!"

.PHONY: all
all: clean deps build ## Clean, install deps, and build

# Development workflow
.PHONY: setup
setup: deps install-tools ## Setup development environment
	@echo "Development environment setup completed!"

.PHONY: quick-start
quick-start: deps dev ## Quick start for development

# Production workflow
.PHONY: release
release: check build docker-build ## Prepare release (run checks, build, docker build)
	@echo "Release preparation completed!"

# File watchers (requires 'entr' tool: brew install entr)
.PHONY: watch
watch: ## Watch for changes and restart dev server (requires entr)
	@echo "Watching for changes... (Press Ctrl+C to stop)"
	find . -name "*.go" | entr -r make dev

.PHONY: watch-test
watch-test: ## Watch for changes and run tests (requires entr)
	@echo "Watching for test changes... (Press Ctrl+C to stop)"
	find . -name "*.go" | entr -c make test