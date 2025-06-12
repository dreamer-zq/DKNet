# DKNet Makefile

.PHONY: help build test clean proto-gen proto-clean docker-build

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

# Docker configuration
DOCKER_IMAGE_NAME ?= dknet/tss-server
DOCKER_TAG ?= latest
DOCKER_REGISTRY ?= 
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build commands
build: build-server build-client

build-server:
	@echo "Building DKNet..."
	@mkdir -p bin
	go build -o bin/tss-server ./cmd/tss-server

build-client:
	@echo "Building TSS client..."
	@mkdir -p bin
	go build -o bin/tss-client ./cmd/tss-client

run: build-server
	@echo "Starting DKNet..."
	./bin/tss-server

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