#!/bin/bash

# DKNet TSS Validation Service Test Script (Docker Environment)
# This script tests the validation service integration with TSS operations

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

# JWT Configuration (matching Docker test setup)
JWT_SECRET="dknet-test-jwt-secret-key-2024"
JWT_ISSUER="dknet-test"

# Global JWT token variable
JWT_TOKEN=""

# Peer IDs from cluster-info.txt (Phase 2: Using peer IDs directly as node IDs)
NODE1_PEER_ID="12D3KooWGZCnvk6cX2UUhc1SHhkGvdfJNZicx4uXEb3niyHHN7ch"
NODE2_PEER_ID="12D3KooWEMke2yrVjg4nadKBBCZrWeZtxD4KucM4QzgH24JMo6JU"
NODE3_PEER_ID="12D3KooWT3TACsUvszChWcQwT7YpPa1udfwpb5k5qQ8zrBw4VqZ7"

# Function to print colored output
print_status() {
    local level=$1
    local message=$2
    case $level in
        "INFO")
            echo -e "${BLUE}[INFO]${NC} $message"
            ;;
        "SUCCESS")
            echo -e "${GREEN}[SUCCESS]${NC} $message"
            ;;
        "WARNING")
            echo -e "${YELLOW}[WARNING]${NC} $message"
            ;;
        "ERROR")
            echo -e "${RED}[ERROR]${NC} $message"
            ;;
    esac
}

# Function to wait for service to be ready
wait_for_service() {
    local url=$1
    local service_name=$2
    local max_attempts=30
    local attempt=1
    local use_auth=$3  # Optional parameter for authentication
    
    print_status "INFO" "Waiting for $service_name to be ready..."
    
    while [ $attempt -le $max_attempts ]; do
        local success=false
        
        if [ "$use_auth" = "true" ]; then
            # Use authenticated request for TSS nodes
            if authenticated_curl "GET" "$url" "" > /dev/null 2>&1; then
                success=true
            fi
        else
            # Use regular curl for validation service
            if curl -s "$url" > /dev/null 2>&1; then
                success=true
            fi
        fi
        
        if [ "$success" = "true" ]; then
            print_status "SUCCESS" "$service_name is ready"
            return 0
        fi
        
        echo "Attempt $attempt/$max_attempts: $service_name not ready yet..."
        sleep 2
        attempt=$((attempt + 1))
    done
    
    print_status "ERROR" "$service_name failed to start within timeout"
    return 1
}

# Function to test API endpoint
test_api() {
    local url=$1
    local data=$2
    local expected_approved=$3
    local test_name=$4
    
    print_status "INFO" "Testing: $test_name"
    
    response=$(curl -s -X POST "$url" \
        -H "Content-Type: application/json" \
        -d "$data")
    
    if [ $? -ne 0 ]; then
        print_status "ERROR" "Failed to call API for $test_name"
        return 1
    fi
    
    approved=$(echo "$response" | jq -r '.approved' 2>/dev/null)
    reason=$(echo "$response" | jq -r '.reason' 2>/dev/null)
    
    if [ "$approved" = "$expected_approved" ]; then
        print_status "SUCCESS" "$test_name: $reason"
        return 0
    else
        print_status "ERROR" "$test_name: Expected approved=$expected_approved, got approved=$approved"
        echo "Response: $response"
        return 1
    fi
}

# Function to generate JWT token
generate_jwt_token() {
    print_status "INFO" "Generating JWT token for API authentication..."
    
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
        print_status "ERROR" "Failed to get JWT dependency. Please ensure Go is installed and has internet access."
        rm -rf "$temp_dir"
        return 1
    }
    
    JWT_TOKEN=$(go run generate_jwt.go "$JWT_SECRET" "$JWT_ISSUER" 2>/dev/null)
    local exit_code=$?
    
    # Cleanup
    rm -rf "$temp_dir"
    
    if [ $exit_code -eq 0 ] && [ -n "$JWT_TOKEN" ]; then
        print_status "SUCCESS" "JWT token generated successfully"
        return 0
    else
        print_status "ERROR" "Failed to generate JWT token"
        return 1
    fi
}

# Function to make authenticated API call
authenticated_curl() {
    local method="$1"
    local url="$2"
    local data="$3"
    
    if [ -z "$JWT_TOKEN" ]; then
        if ! generate_jwt_token; then
            print_status "ERROR" "Cannot make authenticated API call without JWT token"
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

# Step 1: Start services
print_status "INFO" "Starting Docker services..."
cd "$PROJECT_ROOT"
docker-compose -f "$DOCKER_COMPOSE_FILE" up -d

# Step 2: Wait for services to be ready
echo
print_status "INFO" "Waiting for services to start..."
sleep 10

# Check validation service (no auth required)
wait_for_service "http://localhost:8888/health" "Validation Service"

# Generate JWT token for TSS node health checks
if ! generate_jwt_token; then
    print_status "ERROR" "Failed to generate JWT token for TSS node health checks"
    exit 1
fi

# Check TSS nodes (with authentication)
wait_for_service "http://localhost:8081/health" "TSS Node 1" "true"
wait_for_service "http://localhost:8082/health" "TSS Node 2" "true"
wait_for_service "http://localhost:8083/health" "TSS Node 3" "true"

echo
print_status "SUCCESS" "All services are running"

# Step 3: Test validation service directly
echo
print_status "INFO" "Testing validation service directly..."

# Test 1: Valid request
hello_world_base64=$(echo -n "Hello World" | base64)
test_api "http://localhost:8888/validate" '{
    "message": "'$hello_world_base64'",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'"],
    "node_id": "'$NODE1_PEER_ID'",
    "timestamp": '$(date +%s)'
}' "true" "Valid request"

# Test 2: Request with forbidden word
malicious_base64=$(echo -n "malicious attack" | base64)
test_api "http://localhost:8888/validate" '{
    "message": "'$malicious_base64'",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'"],
    "node_id": "'$NODE1_PEER_ID'",
    "timestamp": '$(date +%s)'
}' "false" "Request with forbidden word"

# Test 3: Request with insufficient participants
test_api "http://localhost:8888/validate" '{
    "message": "'$hello_world_base64'",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["'$NODE1_PEER_ID'"],
    "node_id": "'$NODE1_PEER_ID'",
    "timestamp": '$(date +%s)'
}' "false" "Request with insufficient participants"

# Step 4: Test TSS integration
echo
print_status "INFO" "Testing TSS integration with validation service..."

# First, generate a key (with authentication)
print_status "INFO" "Generating TSS key..."
keygen_response=$(authenticated_curl "POST" "http://localhost:8081/api/v1/keygen" '{
    "threshold": 1,
    "participants": ["'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"]
}')

operation_id=$(echo "$keygen_response" | jq -r '.operation_id')
if [ "$operation_id" = "null" ] || [ -z "$operation_id" ]; then
    print_status "ERROR" "Failed to start keygen operation"
    echo "Response: $keygen_response"
    exit 1
fi

print_status "SUCCESS" "Keygen operation started: $operation_id"

# Wait for keygen to complete
print_status "INFO" "Waiting for keygen to complete..."
# Increased timeout for CI environments where keygen may take longer
max_wait=180
wait_time=0
while [ $wait_time -lt $max_wait ]; do
    status_response=$(authenticated_curl "GET" "http://localhost:8081/api/v1/operations/$operation_id" "")
    status=$(echo "$status_response" | jq -r '.status')
    
    # Status codes: 1=PENDING, 2=IN_PROGRESS, 3=COMPLETED, 4=FAILED, 5=CANCELLED
    if [ "$status" = "3" ]; then
        key_id=$(echo "$status_response" | jq -r '.Result.KeygenResult.key_id')
        print_status "SUCCESS" "Keygen completed. Key ID: $key_id"
        break
    elif [ "$status" = "4" ]; then
        print_status "ERROR" "Keygen failed"
        echo "Response: $status_response"
        exit 1
    elif [ "$status" = "5" ]; then
        print_status "ERROR" "Keygen cancelled"
        echo "Response: $status_response"
        exit 1
    fi
    
    sleep 2
    wait_time=$((wait_time + 2))
done

if [ $wait_time -ge $max_wait ]; then
    print_status "ERROR" "Keygen timed out"
    exit 1
fi

# Test signing with validation
print_status "INFO" "Testing signing with validation service..."

# Test valid signing request (TSS API expects base64 encoding)
hello_world_base64=$(echo -n "Hello World" | base64)
signing_response=$(authenticated_curl "POST" "http://localhost:8081/api/v1/sign" '{
    "message": "'$hello_world_base64'",
    "key_id": "'$key_id'",
    "participants": ["'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'"]
}')

signing_operation_id=$(echo "$signing_response" | jq -r '.operation_id')
if [ "$signing_operation_id" = "null" ] || [ -z "$signing_operation_id" ]; then
    print_status "ERROR" "Failed to start signing operation"
    echo "Response: $signing_response"
else
    print_status "SUCCESS" "Valid signing request accepted: $signing_operation_id"
fi

# Test invalid signing request (with forbidden word)
malicious_base64=$(echo -n "malicious content" | base64)
invalid_signing_response=$(authenticated_curl "POST" "http://localhost:8081/api/v1/sign" '{
    "message": "'$malicious_base64'",
    "key_id": "'$key_id'",
    "participants": ["'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'"]
}')

# This should fail due to validation
if echo "$invalid_signing_response" | grep -q "validation"; then
    print_status "SUCCESS" "Invalid signing request correctly rejected by validation service"
elif echo "$invalid_signing_response" | grep -q "forbidden"; then
    print_status "SUCCESS" "Invalid signing request correctly rejected by validation service"
else
    print_status "ERROR" "Invalid signing request was not rejected by validation service"
    echo "Response: $invalid_signing_response"
fi

# Step 5: Check logs
echo
print_status "INFO" "Checking validation service logs..."
docker-compose -f "$DOCKER_COMPOSE_FILE" logs validation-service | tail -10

echo
print_status "SUCCESS" "All tests completed!"
print_status "INFO" "To stop services: docker-compose -f $DOCKER_COMPOSE_FILE down"
print_status "INFO" "To view logs: docker-compose -f $DOCKER_COMPOSE_FILE logs [service-name]" 