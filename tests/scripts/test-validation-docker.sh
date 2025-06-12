#!/bin/bash

# Test script for DKNet Validation Service in Docker environment
set -e

echo "=== DKNet Validation Service Docker Test ==="
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

# Step 1: Start services
print_status "INFO" "Starting Docker services..."
docker-compose up -d

# Step 2: Wait for services to be ready
echo
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

# Step 3: Test validation service directly
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

# Step 4: Test TSS integration
echo
print_status "INFO" "Testing TSS integration with validation service..."

# First, generate a key
print_status "INFO" "Generating TSS key..."
keygen_response=$(curl -s -X POST http://localhost:8081/api/v1/keygen \
    -H "Content-Type: application/json" \
    -d '{
        "threshold": 1,
        "parties": 3,
        "participants": ["node1", "node2", "node3"]
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
max_wait=60
wait_time=0
while [ $wait_time -lt $max_wait ]; do
    status_response=$(curl -s "http://localhost:8081/api/v1/operations/$operation_id")
    status=$(echo "$status_response" | jq -r '.status')
    
    if [ "$status" = "completed" ]; then
        key_id=$(echo "$status_response" | jq -r '.result.key_id')
        print_status "SUCCESS" "Keygen completed. Key ID: $key_id"
        break
    elif [ "$status" = "failed" ]; then
        print_status "ERROR" "Keygen failed"
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

# Test valid signing request
hello_world_hex=$(echo -n "Hello World" | xxd -p | tr -d '\n')
signing_response=$(curl -s -X POST http://localhost:8081/api/v1/sign \
    -H "Content-Type: application/json" \
    -d '{
        "message": "'$hello_world_hex'",
        "key_id": "'$key_id'",
        "participants": ["node1", "node2"]
    }')

signing_operation_id=$(echo "$signing_response" | jq -r '.operation_id')
if [ "$signing_operation_id" = "null" ] || [ -z "$signing_operation_id" ]; then
    print_status "ERROR" "Failed to start signing operation"
    echo "Response: $signing_response"
else
    print_status "SUCCESS" "Valid signing request accepted: $signing_operation_id"
fi

# Test invalid signing request (with forbidden word)
malicious_hex=$(echo -n "malicious content" | xxd -p | tr -d '\n')
invalid_signing_response=$(curl -s -X POST http://localhost:8081/api/v1/sign \
    -H "Content-Type: application/json" \
    -d '{
        "message": "'$malicious_hex'",
        "key_id": "'$key_id'",
        "participants": ["node1", "node2"]
    }')

# This should fail due to validation
if echo "$invalid_signing_response" | grep -q "validation"; then
    print_status "SUCCESS" "Invalid signing request correctly rejected by validation service"
else
    print_status "ERROR" "Invalid signing request was not rejected by validation service"
    echo "Response: $invalid_signing_response"
fi

# Step 5: Check logs
echo
print_status "INFO" "Checking validation service logs..."
docker-compose logs validation-service | tail -10

echo
print_status "SUCCESS" "All tests completed!"
print_status "INFO" "To stop services: docker-compose down"
print_status "INFO" "To view logs: docker-compose logs [service-name]" 