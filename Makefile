# DKNet Makefile

.PHONY: build test clean proto-gen proto-clean docker-build lint security-scan docker-start docker-stop

# Docker configuration
DOCKER_IMAGE_NAME ?= dknet/dknet
DOCKER_TAG ?= latest
DOCKER_REGISTRY ?= 
VERSION ?= $(shell git describe --tags --always --dirty)
GIT_COMMIT ?= $(shell git rev-parse HEAD)

# Go build flags
LDFLAGS = -X github.com/dreamer-zq/DKNet/version.Version=$(VERSION) \
          -X github.com/dreamer-zq/DKNet/version.GitCommit=$(GIT_COMMIT)

# Build commands
build: build-server build-client

build-all: build-server build-client build-mcp

build-server:
	@echo "Building DKNet..."
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/dknet ./cmd/dknet

build-client:
	@echo "Building TSS client..."
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/dknet-cli ./cmd/dknet-cli

build-mcp:
	@echo "Building DKNet MCP Server..."
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/dknet-mcp ./cmd/dknet-mcp

install:
	@echo "Installing DKNet..."
	go install -ldflags "$(LDFLAGS)" ./cmd/dknet
	go install -ldflags "$(LDFLAGS)" ./cmd/dknet-cli
	go install -ldflags "$(LDFLAGS)" ./cmd/dknet-mcp

run: build-server
	@echo "Starting DKNet..."
	./bin/dknet

test:
	@echo "Running tests..."
	go test ./...

test-e2e: docker-start
	@echo "Running e2e tests..."
	@go test -v ./tests/e2e/... ; \
	EXIT_CODE=$$? ; \
	echo "Stopping e2e environment after tests..." ; \
	$(MAKE) docker-stop ; \
	exit $$EXIT_CODE

clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	go clean

# Protobuf generation
proto-gen:
	@echo "Generating protobuf code..."
	@mkdir -p proto/tss/v1 proto/health/v1
	protoc --go_out=. --go_opt=module=github.com/dreamer-zq/DKNet \
		--go-grpc_out=. --go-grpc_opt=module=github.com/dreamer-zq/DKNet \
		proto/tss/v1/tss.proto proto/health/v1/health.proto

proto-clean:
	@echo "Cleaning generated protobuf code..."
	rm -f proto/tss/v1/*.pb.go
	rm -f proto/health/v1/*.pb.go

docker-build:
	@echo "Building development Docker image: $(DOCKER_IMAGE_NAME):latest"
	docker build -t $(DOCKER_IMAGE_NAME):latest .
	@echo "Development Docker image built successfully: $(DOCKER_IMAGE_NAME):latest"

docker-start:
	@echo "Starting e2e test environment and waiting for it to be healthy..."
	docker compose -f tests/docker/docker-compose.yaml up -d --wait --build
	@echo "E2E test environment is up and healthy."

docker-stop:
	@echo "Stopping e2e test environment..."
	docker compose -f tests/docker/docker-compose.yaml down
	@echo "E2E test environment stopped successfully"

lint:
	@echo "Running code linting..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found. Install it with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		go vet ./...; \
		go fmt ./...; \
	fi

security-scan:
	@echo "Running security scanning..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not found. Install it with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"; \
	fi