#!/bin/bash

# DKNet TSS Test Environment Management Script
# This script manages the Docker-based test environment for DKNet TSS

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TESTS_DIR="$PROJECT_ROOT/tests"
DOCKER_COMPOSE_FILE="$TESTS_DIR/docker/docker-compose.yml"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if Docker is running
check_docker() {
    if ! docker info >/dev/null 2>&1; then
        print_error "Docker is not running. Please start Docker first."
        exit 1
    fi
}

# Function to check if docker-compose is available
check_docker_compose() {
    if ! command -v docker-compose >/dev/null 2>&1; then
        print_error "docker-compose is not installed. Please install docker-compose first."
        exit 1
    fi
}

# Function to start the test environment
start_env() {
    print_status "Starting DKNet TSS test environment..."
    
    cd "$PROJECT_ROOT"
    
    # Build and start services
    docker-compose -f "$DOCKER_COMPOSE_FILE" up -d --build
    
    print_status "Waiting for services to be ready..."
    sleep 10
    
    # Check service health
    print_status "Checking service health..."
    
    # Check validation service
    if curl -s http://localhost:8888/health >/dev/null 2>&1; then
        print_success "Validation service is healthy"
    else
        print_warning "Validation service may not be ready yet"
    fi
    
    # Check TSS nodes
    for i in {1..3}; do
        port=$((8080 + i))
        if curl -s http://localhost:$port/health >/dev/null 2>&1; then
            print_success "TSS Node $i is healthy"
        else
            print_warning "TSS Node $i may not be ready yet"
        fi
    done
    
    print_success "Test environment started successfully!"
    print_status "Services available at:"
    echo "  - Validation Service: http://localhost:8888"
    echo "  - TSS Node 1: http://localhost:8081"
    echo "  - TSS Node 2: http://localhost:8082"
    echo "  - TSS Node 3: http://localhost:8083"
}

# Function to stop the test environment
stop_env() {
    print_status "Stopping DKNet TSS test environment..."
    
    cd "$PROJECT_ROOT"
    docker-compose -f "$DOCKER_COMPOSE_FILE" down
    
    print_success "Test environment stopped successfully!"
}

# Function to run validation tests
test_validation() {
    print_status "Running validation service tests..."
    
    # Check if environment is running
    if ! curl -s http://localhost:8888/health >/dev/null 2>&1; then
        print_error "Test environment is not running. Please start it first with: $0 start"
        exit 1
    fi
    
    cd "$TESTS_DIR"
    
    # Run validation tests
    if [ -f "scripts/test-validation-docker.sh" ]; then
        bash scripts/test-validation-docker.sh
    else
        print_error "Test script not found: scripts/test-validation-docker.sh"
        exit 1
    fi
}

# Function to show environment status
show_status() {
    print_status "Checking DKNet TSS test environment status..."
    
    cd "$PROJECT_ROOT"
    docker-compose -f "$DOCKER_COMPOSE_FILE" ps
    
    echo ""
    print_status "Service health checks:"
    
    # Check validation service
    if curl -s http://localhost:8888/health >/dev/null 2>&1; then
        print_success "✓ Validation service (http://localhost:8888)"
    else
        print_error "✗ Validation service (http://localhost:8888)"
    fi
    
    # Check TSS nodes
    for i in {1..3}; do
        port=$((8080 + i))
        if curl -s http://localhost:$port/health >/dev/null 2>&1; then
            print_success "✓ TSS Node $i (http://localhost:$port)"
        else
            print_error "✗ TSS Node $i (http://localhost:$port)"
        fi
    done
}

# Function to show logs
show_logs() {
    local service="$1"
    
    cd "$PROJECT_ROOT"
    
    if [ -z "$service" ]; then
        print_status "Showing logs for all services..."
        docker-compose -f "$DOCKER_COMPOSE_FILE" logs -f
    else
        print_status "Showing logs for service: $service"
        docker-compose -f "$DOCKER_COMPOSE_FILE" logs -f "$service"
    fi
}

# Function to cleanup environment
cleanup_env() {
    print_status "Cleaning up DKNet TSS test environment..."
    
    cd "$PROJECT_ROOT"
    
    # Stop and remove containers, networks, and volumes
    docker-compose -f "$DOCKER_COMPOSE_FILE" down -v --remove-orphans
    
    # Remove unused images
    print_status "Removing unused Docker images..."
    docker image prune -f
    
    print_success "Environment cleaned up successfully!"
}

# Function to show help
show_help() {
    echo "DKNet TSS Test Environment Management Script"
    echo ""
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  start     Start the test environment"
    echo "  stop      Stop the test environment"
    echo "  test      Run validation tests"
    echo "  status    Show environment status"
    echo "  logs      Show logs for all services"
    echo "  logs <service>  Show logs for specific service"
    echo "  cleanup   Stop environment and cleanup resources"
    echo "  help      Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 start                    # Start the test environment"
    echo "  $0 test                     # Run validation tests"
    echo "  $0 logs validation-service  # Show validation service logs"
    echo "  $0 cleanup                  # Cleanup everything"
}

# Main script logic
main() {
    # Check prerequisites
    check_docker
    check_docker_compose
    
    case "$1" in
        start)
            start_env
            ;;
        stop)
            stop_env
            ;;
        test)
            test_validation
            ;;
        status)
            show_status
            ;;
        logs)
            show_logs "$2"
            ;;
        cleanup)
            cleanup_env
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            print_error "Unknown command: $1"
            echo ""
            show_help
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@" 