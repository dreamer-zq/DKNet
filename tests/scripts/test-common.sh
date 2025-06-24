#!/bin/bash

# DKNet TSS Test Common Functions
# This file contains common functions used by all test scripts

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
DOCKER_COMPOSE_FILE="$TESTS_DIR/docker/docker-compose.yaml"

# Default password for testing
DEFAULT_PASSWORD="TestPassword123!"

# JWT Configuration
JWT_SECRET="dknet-test-jwt-secret-key-2024"
JWT_ISSUER="dknet-test"
JWT_TOKEN=""

# Peer IDs from cluster-info.txt
NODE1_PEER_ID="12D3KooWGZCnvk6cX2UUhc1SHhkGvdfJNZicx4uXEb3niyHHN7ch"
NODE2_PEER_ID="12D3KooWEMke2yrVjg4nadKBBCZrWeZtxD4KucM4QzgH24JMo6JU"
NODE3_PEER_ID="12D3KooWT3TACsUvszChWcQwT7YpPa1udfwpb5k5qQ8zrBw4VqZ7"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1" >&2
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" >&2
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" >&2
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

print_result() {
    echo -e "${GREEN}[RESULT]${NC} $1" >&2
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

# Function to get current password
get_current_password() {
    if [ -n "$TSS_ENCRYPTION_PASSWORD" ]; then
        echo "$TSS_ENCRYPTION_PASSWORD"
    else
        echo "$DEFAULT_PASSWORD"
    fi
}

# Function to generate JWT token
generate_jwt_token() {
    # Create temporary Go file for JWT generation
    local temp_dir="/tmp/dknet-jwt-$$"
    mkdir -p "$temp_dir"
    
    cat > "$temp_dir/generate_jwt.go" << 'EOF'
package main

import (
	"fmt"
	"os"
	"time"
	"github.com/golang-jwt/jwt/v5"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <secret> <issuer>\n", os.Args[0])
		os.Exit(1)
	}
	
	secret := os.Args[1]
	issuer := os.Args[2]

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "test-user",
		"iss":   issuer,
		"exp":   time.Now().Add(24 * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"roles": []string{"admin", "operator"},
	})

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating token: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(tokenString)
}
EOF

    # Initialize go module and generate token
    cd "$temp_dir"
    go mod init jwt-gen >/dev/null 2>&1 || true
    go get github.com/golang-jwt/jwt/v5 >/dev/null 2>&1 || {
        print_error "Failed to get JWT dependency. Please ensure Go is installed and has internet access."
        rm -rf "$temp_dir"
        return 1
    }
    
    JWT_TOKEN=$(go run generate_jwt.go "$JWT_SECRET" "$JWT_ISSUER" 2>/dev/null)
    local exit_code=$?
    
    # Cleanup
    rm -rf "$temp_dir"
    
    if [ $exit_code -eq 0 ] && [ -n "$JWT_TOKEN" ]; then
        return 0
    else
        return 1
    fi
}

# Function to make authenticated API call
api_call() {
    local method="$1"
    local url="$2"
    local data="$3"
    
    if [ -z "$JWT_TOKEN" ]; then
        if ! generate_jwt_token; then
            print_error "Cannot make authenticated API call without JWT token"
            return 1
        fi
    fi
    
    local curl_args=(-s)
    curl_args+=(-H "Authorization: Bearer $JWT_TOKEN")
    curl_args+=(-H "Content-Type: application/json")
    
    if [ "$method" = "POST" ] && [ -n "$data" ]; then
        curl_args+=(-X POST -d "$data")
    elif [ "$method" = "GET" ]; then
        curl_args+=(-X GET)
    fi
    
    curl_args+=("$url")
    
    curl "${curl_args[@]}"
}

# Function to wait for operation completion on a specific port
wait_for_operation_on_port() {
    local operation_id="$1"
    local port="$2"
    local max_wait=180
    local wait_time=0
    
    print_status "Waiting for operation $operation_id to complete..."
    
    while [ $wait_time -lt $max_wait ]; do
        local response=$(api_call "GET" "http://localhost:$port/api/v1/operations/$operation_id" "")
        local status=$(echo "$response" | jq -r '.status' 2>/dev/null)
        
        # Status codes: 1=PENDING, 2=IN_PROGRESS, 3=COMPLETED, 4=FAILED, 5=CANCELLED
        case "$status" in
            "3")
                print_success "Operation $operation_id completed successfully"
                echo "$response"
                return 0
                ;;
            "4")
                print_error "Operation $operation_id failed"
                echo "$response"
                return 1
                ;;
            "5")
                print_error "Operation $operation_id was cancelled"
                echo "$response"
                return 1
                ;;
            "1"|"2")
                # Still in progress
                ;;
            *)
                print_warning "Unknown status: $status"
                ;;
        esac
        
        sleep 2
        wait_time=$((wait_time + 2))
    done
    
    print_error "Operation $operation_id timed out"
    return 1
}

# Function to wait for operation completion (legacy - uses Node 1)
wait_for_operation() {
    local operation_id="$1"
    wait_for_operation_on_port "$operation_id" "8081"
}

# Function to start the test environment
start_test_env() {
    print_status "Starting DKNet TSS test environment..."
    
    # Check prerequisites
    check_docker
    check_docker_compose
    
    # Set default password if not already set
    if [ -z "$TSS_ENCRYPTION_PASSWORD" ]; then
        export TSS_ENCRYPTION_PASSWORD="$DEFAULT_PASSWORD"
    fi
    
    cd "$PROJECT_ROOT"
    
    # Build and start services
    print_status "Building and starting services..."
    docker-compose -f "$DOCKER_COMPOSE_FILE" up -d --build
    
    print_status "Waiting for services to be ready..."
    sleep 15
    
    # Generate JWT token
    if ! generate_jwt_token; then
        print_error "Failed to generate JWT token"
        exit 1
    fi
    
    # Check service health
    print_status "Checking service health..."
    
    # Check validation service
    if curl -s http://localhost:8888/health >/dev/null 2>&1; then
        print_success "✓ Validation service is healthy"
    else
        print_error "✗ Validation service is not healthy"
        exit 1
    fi
    
    # Check TSS nodes
    for i in {1..3}; do
        port=$((8080 + i))
        if api_call "GET" "http://localhost:$port/health" "" >/dev/null 2>&1; then
            print_success "✓ TSS Node $i is healthy"
        else
            print_error "✗ TSS Node $i is not healthy"
            exit 1
        fi
    done
    
    print_success "Test environment started successfully!"
}

# Function to check if environment is running (quiet version)
check_test_env_quiet() {
    # Check validation service
    if ! curl -s http://localhost:8888/health >/dev/null 2>&1; then
        return 1
    fi
    
    # Generate JWT token
    if ! generate_jwt_token >/dev/null 2>&1; then
        return 1
    fi
    
    # Check TSS nodes
    for i in {1..3}; do
        port=$((8080 + i))
        if ! api_call "GET" "http://localhost:$port/health" "" >/dev/null 2>&1; then
            return 1
        fi
    done
    
    return 0
}

# Function to check if environment is running
check_test_env() {
    print_status "Checking if test environment is running..."
    
    # Check validation service
    if ! curl -s http://localhost:8888/health >/dev/null 2>&1; then
        print_error "Test environment is not running"
        print_status "Please start the environment first:"
        print_status "  ./tests/scripts/test-all.sh start"
        exit 1
    fi
    
    # Generate JWT token
    if ! generate_jwt_token; then
        print_error "Failed to generate JWT token"
        exit 1
    fi
    
    # Check TSS nodes
    for i in {1..3}; do
        port=$((8080 + i))
        if ! api_call "GET" "http://localhost:$port/health" "" >/dev/null 2>&1; then
            print_error "TSS Node $i is not healthy"
            exit 1
        fi
    done
    
    print_success "Test environment is running and healthy"
}

# Function to stop the test environment
stop_test_env() {
    print_status "Stopping DKNet TSS test environment..."
    
    cd "$PROJECT_ROOT"
    docker-compose -f "$DOCKER_COMPOSE_FILE" down
    
    print_success "Test environment stopped successfully!"
}

# Function to show environment status
show_test_env_status() {
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
    
    # Generate JWT token if needed
    if [ -z "$JWT_TOKEN" ]; then
        generate_jwt_token >/dev/null 2>&1
    fi
    
    # Check TSS nodes
    for i in {1..3}; do
        port=$((8080 + i))
        if [ -n "$JWT_TOKEN" ] && api_call "GET" "http://localhost:$port/health" "" >/dev/null 2>&1; then
            print_success "✓ TSS Node $i (http://localhost:$port)"
        else
            print_error "✗ TSS Node $i (http://localhost:$port)"
        fi
    done
}

# Function to display test environment information
display_test_env_info() {
    print_result "Test Environment Information:"
    print_result "  - Validation Service: http://localhost:8888"
    print_result "  - TSS Node 1: http://localhost:8081"
    print_result "  - TSS Node 2: http://localhost:8082"
    print_result "  - TSS Node 3: http://localhost:8083"
    print_result "  - Encryption Password: $(get_current_password)"
    print_result "  - JWT Secret: $JWT_SECRET"
    print_result "  - JWT Issuer: $JWT_ISSUER"
    print_result "  - JWT Token: $JWT_TOKEN"
    echo ""
    print_result "Node IDs:"
    print_result "  - Node 1: $NODE1_PEER_ID"
    print_result "  - Node 2: $NODE2_PEER_ID"
    print_result "  - Node 3: $NODE3_PEER_ID"
}

# Function to test validation service directly
test_validation_service() {
    local message="$1"
    
    print_status "Testing validation service..."
    
    local message_base64=$(echo -n "$message" | base64)
    
    local validation_data=$(cat <<EOF
{
    "message": "$message_base64",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["$NODE1_PEER_ID", "$NODE2_PEER_ID"],
    "node_id": "$NODE1_PEER_ID",
    "timestamp": $(date +%s)
}
EOF
)
    
    print_result "Validation Request Data:"
    echo "$validation_data" | jq .
    print_result "Original Message: $message"
    
    local response=$(curl -s -X POST "http://localhost:8888/validate" \
        -H "Content-Type: application/json" \
        -d "$validation_data")
    
    local approved=$(echo "$response" | jq -r '.approved' 2>/dev/null)
    local reason=$(echo "$response" | jq -r '.reason' 2>/dev/null)
    
    if [ "$approved" = "true" ]; then
        print_success "✓ Validation service approved the request"
        print_result "Reason: $reason"
        return 0
    else
        print_error "✗ Validation service rejected the request"
        print_result "Reason: $reason"
        return 1
    fi
}

# Function to cleanup environment
cleanup_test_env() {
    print_status "Cleaning up DKNet TSS test environment..."
    
    cd "$PROJECT_ROOT"
    
    # Stop and remove containers, networks, and volumes
    docker-compose -f "$DOCKER_COMPOSE_FILE" down -v --remove-orphans
    
    # Remove unused images
    print_status "Removing unused Docker images..."
    docker image prune -f
    
    print_success "Environment cleaned up successfully!"
}

# Function to show logs
show_test_env_logs() {
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