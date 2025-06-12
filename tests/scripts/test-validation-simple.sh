#!/bin/bash

# Simple validation service test script
set -e

echo "=== DKNet Validation Service Simple Test ==="
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

# Check if validation service is running
print_status "INFO" "Checking if validation service is running..."
if ! curl -s http://localhost:8888/health > /dev/null 2>&1; then
    print_status "ERROR" "Validation service is not running. Please start it first."
    print_status "INFO" "Run: ./tests/scripts/start-test-env.sh start"
    exit 1
fi

print_status "SUCCESS" "Validation service is running"

# Test validation service directly
echo
print_status "INFO" "Testing validation service API..."

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

# Test 4: Request with empty message
test_api "http://localhost:8888/validate" '{
    "message": "",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["node1", "node2"],
    "node_id": "node1",
    "timestamp": '$(date +%s)'
}' "false" "Request with empty message"

# Test 5: Request with very old timestamp
old_timestamp=$(($(date +%s) - 3600))  # 1 hour ago
test_api "http://localhost:8888/validate" '{
    "message": "48656c6c6f20576f726c64",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["node1", "node2"],
    "node_id": "node1",
    "timestamp": '$old_timestamp'
}' "false" "Request with old timestamp"

echo
print_status "SUCCESS" "All validation service tests completed!"
print_status "INFO" "Validation service is working correctly"

# Show some validation service logs
echo
print_status "INFO" "Recent validation service logs:"
docker logs validation-service --tail 10 2>/dev/null || echo "Could not fetch logs"

echo
print_status "INFO" "To stop services: ./tests/scripts/start-test-env.sh stop"
print_status "INFO" "To view logs: ./tests/scripts/start-test-env.sh logs validation-service" 