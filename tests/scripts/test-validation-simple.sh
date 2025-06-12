#!/bin/bash

# Simple test script for DKNet Validation Service
set -e

echo "=== DKNet Validation Service Test ==="
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    if [ "$status" = "SUCCESS" ]; then
        echo -e "${GREEN}✅ $message${NC}"
    elif [ "$status" = "ERROR" ]; then
        echo -e "${RED}❌ $message${NC}"
    elif [ "$status" = "INFO" ]; then
        echo -e "${YELLOW}ℹ️  $message${NC}"
    fi
}

# Function to wait for service to be ready
wait_for_service() {
    local url=$1
    local service_name=$2
    local max_attempts=30
    local attempt=1
    
    print_status "INFO" "Waiting for $service_name to be ready..."
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s "$url" > /dev/null 2>&1; then
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

# Wait for services to be ready
print_status "INFO" "Waiting for services to start..."
sleep 10

# Check validation service
wait_for_service "http://localhost:8888/health" "Validation Service"

# Check TSS nodes
wait_for_service "http://localhost:8081/health" "TSS Node 1"
wait_for_service "http://localhost:8082/health" "TSS Node 2" 
wait_for_service "http://localhost:8083/health" "TSS Node 3"

echo
print_status "SUCCESS" "All services are running"

# Test validation service directly
echo
print_status "INFO" "Testing validation service directly..."

# Test 1: Valid request
test_api "http://localhost:8888/validate" '{
    "message": "48656c6c6f20576f726c64",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["node1", "node2"],
    "node_id": "node1",
    "timestamp": '$(date +%s)'
}' "true" "Valid request"

# Test 2: Request with forbidden word
malicious_hex=$(echo -n "malicious attack" | xxd -p | tr -d '\n')
test_api "http://localhost:8888/validate" '{
    "message": "'$malicious_hex'",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["node1", "node2"],
    "node_id": "node1",
    "timestamp": '$(date +%s)'
}' "false" "Request with forbidden word"

# Test 3: Request with insufficient participants
test_api "http://localhost:8888/validate" '{
    "message": "48656c6c6f20576f726c64",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["node1"],
    "node_id": "node1",
    "timestamp": '$(date +%s)'
}' "false" "Request with insufficient participants"

# Test 4: Test TSS signing integration
echo
print_status "INFO" "Testing TSS signing integration with validation service..."

# Test valid signing request (should be accepted by validation service)
# Convert message to base64 for TSS API
hello_world_base64=$(echo -n "Hello World" | base64)
signing_response=$(curl -s -X POST http://localhost:8081/api/v1/sign \
    -H "Content-Type: application/json" \
    -d '{
        "message": "'$hello_world_base64'",
        "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
        "participants": ["node1", "node2"]
    }')

# Check if the request was accepted (should get an operation_id or validation error)
if echo "$signing_response" | jq -e '.operation_id' > /dev/null 2>&1; then
    operation_id=$(echo "$signing_response" | jq -r '.operation_id')
    print_status "SUCCESS" "Valid signing request accepted by validation service: $operation_id"
elif echo "$signing_response" | grep -q "validation"; then
    print_status "INFO" "Signing request processed by validation service (may have failed for other reasons)"
    echo "Response: $signing_response"
else
    print_status "ERROR" "Unexpected response from signing request"
    echo "Response: $signing_response"
fi

# Test invalid signing request (should be rejected by validation service)
malicious_base64=$(echo -n "malicious content" | base64)
invalid_signing_response=$(curl -s -X POST http://localhost:8081/api/v1/sign \
    -H "Content-Type: application/json" \
    -d '{
        "message": "'$malicious_base64'",
        "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
        "participants": ["node1", "node2"]
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

# Check validation service logs
echo
print_status "INFO" "Checking validation service logs..."
docker-compose logs validation-service | tail -10

echo
print_status "SUCCESS" "Validation service tests completed!"
print_status "INFO" "To stop services: docker-compose down"
print_status "INFO" "To view logs: docker-compose logs [service-name]" 