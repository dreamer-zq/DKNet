# DKNet Makefile

.PHONY: help build test clean proto-gen proto-clean docker-build ci-test lint security-scan

# Default target
help:
	@echo "DKNet Build Commands:"
	@echo "  build            - Build server and client binaries"
	@echo "  build-server     - Build DKNet binary"
	@echo "  build-client     - Build TSS client binary"
	@echo "  test             - Run tests"
	@echo "  clean            - Clean build artifacts"
	@echo ""
	@echo "Protocol Buffers:"
	@echo "  proto-gen        - Generate Go code from protobuf definitions"
	@echo "  proto-clean      - Clean generated protobuf code"
	@echo ""
	@echo "Docker Commands:"
	@echo "  docker-build     - Build development Docker image with latest tag"
	@echo ""
	@echo "CI/CD Commands:"
	@echo "  ci-test          - Run CI test suite locally"
	@echo "  lint             - Run code linting"
	@echo "  security-scan    - Run security scanning"

# Docker configuration
DOCKER_IMAGE_NAME ?= dknet/dknet
DOCKER_TAG ?= latest
DOCKER_REGISTRY ?= 
VERSION ?= $(shell git describe --tags --always --dirty)
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT ?= $(shell git rev-parse HEAD)

# Go build flags
LDFLAGS = -X main.version=$(VERSION) \
          -X main.buildTime=$(BUILD_TIME) \
          -X main.gitCommit=$(GIT_COMMIT)

# Build commands
build: build-server build-client

build-server:
	@echo "Building DKNet..."
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/dknet ./cmd/dknet

build-client:
	@echo "Building TSS client..."
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o bin/dknet-cli ./cmd/dknet-cli

run: build-server
	@echo "Starting DKNet..."
	./bin/dknet

test:
	@echo "Running tests..."
	go test ./...

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

# CI/CD commands
ci-test: lint test
	@echo "Running validation service tests..."
	@cd tests/scripts && ./start-test-env.sh start
	@cd tests/scripts && ./test-validation-simple.sh
	@cd tests/scripts && ./start-test-env.sh stop
	@echo "All CI tests passed!"

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