.PHONY: all build run test lint clean docker-build docker-run docker-stop help

# Variables
BINARY_NAME=asf-stac-proxy
DOCKER_IMAGE=asf-stac-proxy
DOCKER_TAG=latest
GO=go
PORT=8080

# Default target
all: test build

# Build the binary
build:
	$(GO) build -o $(BINARY_NAME) ./cmd/server

# Build with optimizations (smaller binary)
build-release:
	CGO_ENABLED=0 $(GO) build -ldflags="-w -s" -o $(BINARY_NAME) ./cmd/server

# Run the server
run: build
	STAC_BASE_URL=http://localhost:$(PORT) ./$(BINARY_NAME)

# Run with CMR backend
run-cmr: build
	STAC_BASE_URL=http://localhost:$(PORT) BACKEND_TYPE=cmr ./$(BINARY_NAME)

# Run without building (for development)
dev:
	STAC_BASE_URL=http://localhost:$(PORT) $(GO) run ./cmd/server

# Run with CMR backend (development)
dev-cmr:
	STAC_BASE_URL=http://localhost:$(PORT) BACKEND_TYPE=cmr $(GO) run ./cmd/server

# Run tests
test:
	$(GO) test ./...

# Run tests with verbose output
test-v:
	$(GO) test -v ./...

# Run tests with coverage
test-cover:
	$(GO) test -cover ./...

# Run tests with coverage report
test-cover-html:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run linter
lint:
	golangci-lint run

# Format code
fmt:
	$(GO) fmt ./...

# Tidy dependencies
tidy:
	$(GO) mod tidy

# Build Docker image (current platform)
docker-build:
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

# Build multi-arch Docker image (amd64 + arm64)
docker-build-multiarch:
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) .

# Build and push multi-arch Docker image
docker-build-push:
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) --push .

# Run Docker container
docker-run:
	docker run -d --name $(BINARY_NAME) \
		-p $(PORT):8080 \
		-e STAC_BASE_URL=http://localhost:$(PORT) \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# Run Docker container with CMR backend
docker-run-cmr:
	docker run -d --name $(BINARY_NAME) \
		-p $(PORT):8080 \
		-e STAC_BASE_URL=http://localhost:$(PORT) \
		-e BACKEND_TYPE=cmr \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# Stop Docker container
docker-stop:
	docker stop $(BINARY_NAME) || true
	docker rm $(BINARY_NAME) || true

# Restart Docker container
docker-restart: docker-stop docker-run

# View Docker logs
docker-logs:
	docker logs -f $(BINARY_NAME)

# Run with docker-compose
compose-up:
	docker-compose up -d

# Stop docker-compose
compose-down:
	docker-compose down

# View docker-compose logs
compose-logs:
	docker-compose logs -f

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	docker rmi $(DOCKER_IMAGE):$(DOCKER_TAG) 2>/dev/null || true

# Compare backends (requires network access)
compare:
	$(GO) run ./scripts/compare_backends.go

# Help
help:
	@echo "ASF-STAC Proxy Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build targets:"
	@echo "  build          Build the binary"
	@echo "  build-release  Build optimized binary (smaller size)"
	@echo "  clean          Remove build artifacts"
	@echo ""
	@echo "Run targets:"
	@echo "  run            Build and run with ASF backend"
	@echo "  run-cmr        Build and run with CMR backend"
	@echo "  dev            Run without building (go run)"
	@echo "  dev-cmr        Run with CMR backend (go run)"
	@echo ""
	@echo "Test targets:"
	@echo "  test           Run tests"
	@echo "  test-v         Run tests (verbose)"
	@echo "  test-cover     Run tests with coverage"
	@echo "  test-cover-html Generate HTML coverage report"
	@echo "  compare        Compare ASF vs CMR backend results"
	@echo ""
	@echo "Code quality:"
	@echo "  lint           Run golangci-lint"
	@echo "  fmt            Format code"
	@echo "  tidy           Tidy go.mod"
	@echo ""
	@echo "Docker targets:"
	@echo "  docker-build           Build Docker image (current platform)"
	@echo "  docker-build-multiarch Build multi-arch image (amd64 + arm64)"
	@echo "  docker-build-push      Build and push multi-arch image"
	@echo "  docker-run             Run container (ASF backend)"
	@echo "  docker-run-cmr         Run container (CMR backend)"
	@echo "  docker-stop            Stop and remove container"
	@echo "  docker-restart         Restart container"
	@echo "  docker-logs            Follow container logs"
	@echo ""
	@echo "Docker Compose:"
	@echo "  compose-up     Start with docker-compose"
	@echo "  compose-down   Stop docker-compose"
	@echo "  compose-logs   Follow compose logs"
