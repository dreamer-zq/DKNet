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
DOCKER_COMPOSE_FILE="$TESTS_DIR/docker/docker-compose.yaml"

# Default password for testing
DEFAULT_PASSWORD="TestPassword123!"

# JWT Configuration (matching Docker test setup)
JWT_SECRET="dknet-test-jwt-secret-key-2024"
JWT_ISSUER="dknet-test"

# Global JWT token variable
JWT_TOKEN=""

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

# Function to set encryption password
set_password() {
    local password="$1"
    if [ -z "$password" ]; then
        print_error "Password cannot be empty"
        exit 1
    fi
    
    export TSS_ENCRYPTION_PASSWORD="$password"
    print_success "Password set in environment variable TSS_ENCRYPTION_PASSWORD"
    print_warning "This password will be used for the current session only."
    print_warning "To persist across sessions, add 'export TSS_ENCRYPTION_PASSWORD=\"$password\"' to your shell profile."
}

# Function to get current password
get_current_password() {
    if [ -n "$TSS_ENCRYPTION_PASSWORD" ]; then
        echo "$TSS_ENCRYPTION_PASSWORD"
    else
        echo "$DEFAULT_PASSWORD"
    fi
}

# Function to start the test environment
start_env() {
    print_status "Starting DKNet TSS test environment..."
    
    # Set default password if not already set
    if [ -z "$TSS_ENCRYPTION_PASSWORD" ]; then
        export TSS_ENCRYPTION_PASSWORD="$DEFAULT_PASSWORD"
        print_warning "Using default password: $DEFAULT_PASSWORD"
        print_warning "To use a custom password, run: $0 set-password 'YourPassword'"
    else
        print_status "Using custom password from environment variable"
    fi
    
    cd "$PROJECT_ROOT"
    
    # Build and start services
    print_status "Building and starting services..."
    docker-compose -f "$DOCKER_COMPOSE_FILE" up -d --build
    
    print_status "Waiting for services to be ready..."
    sleep 15
    
    # Check service health
    print_status "Checking service health..."
    
    # Check validation service (no auth required)
    if curl -s http://localhost:8888/health >/dev/null 2>&1; then
        print_success "Validation service is healthy"
    else
        print_warning "Validation service may not be ready yet"
    fi
    
    # Generate JWT token for TSS node health checks
    if ! generate_jwt_token; then
        print_warning "Could not generate JWT token, TSS nodes may require authentication"
        print_warning "Attempting health checks without authentication..."
        
        # Check TSS nodes without auth (may fail if auth is enabled)
        for i in {1..3}; do
            port=$((8080 + i))
            if curl -s http://localhost:$port/health >/dev/null 2>&1; then
                print_success "TSS Node $i is healthy"
            else
                print_warning "TSS Node $i may not be ready yet or requires authentication"
            fi
        done
    else
        # Check TSS nodes with authentication
        for i in {1..3}; do
            port=$((8080 + i))
            if api_call "GET" "http://localhost:$port/health" "" "Checking TSS Node $i health" >/dev/null 2>&1; then
                print_success "TSS Node $i is healthy"
            else
                print_warning "TSS Node $i may not be ready yet"
            fi
        done
    fi
    
    print_success "Test environment started successfully!"
    print_status "Services available at:"
    echo "  - Validation Service: http://localhost:8888"
    echo "  - TSS Node 1: http://localhost:8081"
    echo "  - TSS Node 2: http://localhost:8082"
    echo "  - TSS Node 3: http://localhost:8083"
    echo ""
    print_status "Encryption password: $(get_current_password)"
    print_warning "All nodes use the same encryption password for testing purposes."
    echo ""
    print_status "JWT Authentication is enabled for all TSS nodes"
    print_status "JWT Secret: $JWT_SECRET"
    print_status "JWT Issuer: $JWT_ISSUER"
    print_warning "Use '$0 generate-token' to get a JWT token for API testing"
    print_warning "Use '$0 test-auth' to verify authentication is working"
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

# Function to run TSS tests
test_tss() {
    print_status "Running TSS functionality tests..."
    
    # Peer IDs from cluster-info.txt (Using new peer IDs generated by init-cluster)
    local NODE1_PEER_ID="12D3KooWGZCnvk6cX2UUhc1SHhkGvdfJNZicx4uXEb3niyHHN7ch"
    local NODE2_PEER_ID="12D3KooWEMke2yrVjg4nadKBBCZrWeZtxD4KucM4QzgH24JMo6JU"
    local NODE3_PEER_ID="12D3KooWT3TACsUvszChWcQwT7YpPa1udfwpb5k5qQ8zrBw4VqZ7"
    
    # Generate JWT token for authenticated API calls
    if ! generate_jwt_token; then
        print_error "Cannot run TSS tests without JWT token"
        exit 1
    fi
    
    # Check if environment is running with authentication
    print_status "Checking TSS nodes health with authentication..."
    local all_healthy=true
    for i in {1..3}; do
        port=$((8080 + i))
        if ! api_call "GET" "http://localhost:$port/health" "" "Checking TSS Node $i" >/dev/null 2>&1; then
            print_error "TSS Node $i is not healthy"
            all_healthy=false
        fi
    done
    
    if [ "$all_healthy" = false ]; then
        print_error "Test environment is not fully healthy. Please start it first with: $0 start"
        exit 1
    fi
    
    print_status "Testing keygen operation with authentication..."
    local keygen_data='{
        "threshold": 2,
        "parties": 3,
        "participants": ["'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"]
    }'
    
    # Capture the keygen response
    local keygen_response
    if keygen_response=$(api_call_with_response "POST" "http://localhost:8081/api/v1/keygen" "$keygen_data" "Starting keygen operation"); then
        print_success "Keygen operation initiated successfully"
        
        # Extract operation ID from response
        local operation_id
        operation_id=$(echo "$keygen_response" | grep -o '"operation_id":"[^"]*"' | cut -d'"' -f4 2>/dev/null || echo "")
        
        if [ -n "$operation_id" ]; then
            print_status "Testing operation status retrieval..."
            if api_call "GET" "http://localhost:8081/api/v1/operations/$operation_id" "" "Getting operation status"; then
                print_success "Operation status retrieved successfully"
            else
                print_warning "Failed to retrieve operation status"
            fi
        else
            print_warning "Could not extract operation ID from keygen response"
        fi
    else
        print_warning "Keygen test may have failed"
    fi
    
    print_success "TSS tests completed"
}

# Function to show environment status
show_status() {
    print_status "Checking DKNet TSS test environment status..."
    
    cd "$PROJECT_ROOT"
    docker-compose -f "$DOCKER_COMPOSE_FILE" ps
    
    echo ""
    print_status "Service health checks:"
    
    # Check validation service (no auth required)
    if curl -s http://localhost:8888/health >/dev/null 2>&1; then
        print_success "✓ Validation service (http://localhost:8888)"
    else
        print_error "✗ Validation service (http://localhost:8888)"
    fi
    
    # Generate JWT token for TSS node checks
    if generate_jwt_token >/dev/null 2>&1; then
        # Check TSS nodes with authentication
        for i in {1..3}; do
            port=$((8080 + i))
            if api_call "GET" "http://localhost:$port/health" "" "" >/dev/null 2>&1; then
                print_success "✓ TSS Node $i (http://localhost:$port) [Authenticated]"
            else
                print_error "✗ TSS Node $i (http://localhost:$port) [Auth Failed]"
            fi
        done
    else
        print_warning "Could not generate JWT token for TSS node health checks"
        # Fallback to unauthenticated checks
        for i in {1..3}; do
            port=$((8080 + i))
            if curl -s http://localhost:$port/health >/dev/null 2>&1; then
                print_success "✓ TSS Node $i (http://localhost:$port) [No Auth]"
            else
                print_error "✗ TSS Node $i (http://localhost:$port) [May require auth]"
            fi
        done
    fi
    
    echo ""
    print_status "Current encryption password: $(get_current_password)"
    if [ -z "$TSS_ENCRYPTION_PASSWORD" ]; then
        print_warning "Using default password. Set custom password with: $0 set-password 'YourPassword'"
    fi
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
    echo "  start              Start the test environment"
    echo "  stop               Stop the test environment"
    echo "  test               Run validation tests"
    echo "  test-tss           Run TSS functionality tests"
    echo "  test-auth          Test API authentication"
    echo "  generate-token     Generate JWT token for manual testing"
    echo "  status             Show environment status"
    echo "  logs               Show logs for all services"
    echo "  logs <service>     Show logs for specific service"
    echo "  cleanup            Stop environment and cleanup resources"
    echo "  set-password <pwd> Set encryption password for current session"
    echo "  help               Show this help message"
    echo ""
    echo "Environment Variables:"
    echo "  TSS_ENCRYPTION_PASSWORD  Set encryption password (recommended for production)"
    echo ""
    echo "Examples:"
    echo "  $0 start                           # Start with default password"
    echo "  $0 set-password 'MySecurePass123!' # Set custom password"
    echo "  $0 start                           # Start with custom password"
    echo "  $0 test-auth                       # Test JWT authentication"
    echo "  $0 test-tss                        # Run TSS functionality tests"
    echo "  $0 generate-token                  # Generate JWT token for manual use"
    echo "  $0 logs tss-node1                  # Show node1 logs"
    echo "  $0 cleanup                         # Cleanup everything"
    echo ""
    echo "Security Notes:"
    echo "  - Default password is 'TestPassword123!' for testing only"
    echo "  - Use TSS_ENCRYPTION_PASSWORD environment variable for custom passwords"
    echo "  - All nodes use the same password for testing purposes"
    echo "  - Never use default password in production environments"
    echo ""
    echo "JWT Authentication:"
    echo "  - JWT Secret: $JWT_SECRET"
    echo "  - JWT Issuer: $JWT_ISSUER"
    echo "  - All API endpoints require JWT authentication"
    echo "  - Use 'generate-token' command to get JWT token for manual testing"
}

# Function to generate JWT token
generate_jwt_token() {
    print_status "Generating JWT token for API authentication..."
    
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
        print_success "JWT token generated successfully"
        print_status "Token expires in 24 hours"
        return 0
    else
        print_error "Failed to generate JWT token"
        return 1
    fi
}

# Function to make authenticated API call
api_call() {
    local method="$1"
    local url="$2"
    local data="$3"
    local description="$4"
    
    if [ -z "$JWT_TOKEN" ]; then
        print_warning "No JWT token available, attempting to generate..."
        if ! generate_jwt_token; then
            print_error "Cannot make authenticated API call without JWT token"
            return 1
        fi
    fi
    
    print_status "${description:-Making API call to $url}"
    
    local curl_args=(-s -w "%{http_code}")
    curl_args+=(-H "Authorization: Bearer $JWT_TOKEN")
    curl_args+=(-H "Content-Type: application/json")
    
    if [ "$method" = "POST" ] && [ -n "$data" ]; then
        curl_args+=(-X POST -d "$data")
    elif [ "$method" = "GET" ]; then
        curl_args+=(-X GET)
    fi
    
    curl_args+=("$url")
    
    local response
    response=$(curl "${curl_args[@]}" 2>/dev/null)
    local http_code="${response: -3}"
    local body="${response%???}"
    
    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        print_success "API call successful (HTTP $http_code)"
        if [ -n "$body" ] && [ "$body" != "null" ]; then
            echo "Response: $body"
        fi
        return 0
    else
        print_error "API call failed (HTTP $http_code)"
        if [ -n "$body" ]; then
            echo "Error response: $body"
        fi
        return 1
    fi
}

# Function to make authenticated API call and return response body
api_call_with_response() {
    local method="$1"
    local url="$2"
    local data="$3"
    local description="$4"
    
    if [ -z "$JWT_TOKEN" ]; then
        print_warning "No JWT token available, attempting to generate..."
        if ! generate_jwt_token; then
            print_error "Cannot make authenticated API call without JWT token"
            return 1
        fi
    fi
    
    print_status "${description:-Making API call to $url}"
    
    local curl_args=(-s -w "%{http_code}")
    curl_args+=(-H "Authorization: Bearer $JWT_TOKEN")
    curl_args+=(-H "Content-Type: application/json")
    
    if [ "$method" = "POST" ] && [ -n "$data" ]; then
        curl_args+=(-X POST -d "$data")
    elif [ "$method" = "GET" ]; then
        curl_args+=(-X GET)
    fi
    
    curl_args+=("$url")
    
    local response
    response=$(curl "${curl_args[@]}" 2>/dev/null)
    local http_code="${response: -3}"
    local body="${response%???}"
    
    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        print_success "API call successful (HTTP $http_code)"
        echo "$body"  # Return the response body
        return 0
    else
        print_error "API call failed (HTTP $http_code)"
        if [ -n "$body" ]; then
            echo "Error response: $body" >&2
        fi
        return 1
    fi
}

# Function to check API authentication
check_auth() {
    print_status "Testing API authentication..."
    
    # Test without authentication (should fail)
    print_status "Testing unauthenticated request (should fail)..."
    local response
    response=$(curl -s -w "%{http_code}" -o /dev/null http://localhost:8081/health 2>/dev/null || echo "000")
    
    if [ "$response" = "401" ]; then
        print_success "✓ Unauthenticated requests correctly rejected (HTTP 401)"
    else
        print_warning "⚠ Expected HTTP 401 for unauthenticated request, got HTTP $response"
    fi
    
    # Generate JWT token if not available
    if [ -z "$JWT_TOKEN" ]; then
        if ! generate_jwt_token; then
            print_error "Cannot test authenticated requests without JWT token"
            return 1
        fi
    fi
    
    # Test with authentication (should succeed)
    print_status "Testing authenticated request..."
    if api_call "GET" "http://localhost:8081/health" "" "Testing authenticated health check"; then
        print_success "✓ JWT authentication working correctly"
        return 0
    else
        print_error "✗ JWT authentication failed"
        return 1
    fi
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
        test-tss)
            test_tss
            ;;
        test-auth)
            check_auth
            ;;
        generate-token)
            if generate_jwt_token; then
                echo ""
                print_success "JWT Token generated:"
                echo "$JWT_TOKEN"
                echo ""
                print_status "Usage examples:"
                echo "HTTP: curl -H \"Authorization: Bearer $JWT_TOKEN\" http://localhost:8081/health"
                echo "gRPC: grpcurl -H \"authorization: Bearer $JWT_TOKEN\" localhost:9095 tss.v1.TSSService/GetOperation"
            fi
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
        set-password)
            set_password "$2"
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