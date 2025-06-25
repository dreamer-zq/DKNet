#!/bin/bash

# Test script for 2-to-3 resharing operation
# This script tests the complete flow: keygen (2-of-2) -> reshare (3-of-3)

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}ℹ $1${NC}"
}

# JWT token for authentication (generated with generate-jwt.go)
JWT_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpYXQiOjE3NTA4Mzc2MDAsImlzcyI6ImRrbmV0LXRlc3QiLCJyb2xlcyI6WyJhZG1pbiIsIm9wZXJhdG9yIl0sInN1YiI6InRlc3QtdXNlciJ9.TW57-Ufg9RYUONiKqCUchBRxOl1oiJlB0YLrzLpyVxg"

# Base URLs
NODE1_URL="http://localhost:8081"
NODE2_URL="http://localhost:8082"
NODE3_URL="http://localhost:8083"

# Actual node IDs from config files
NODE1_ID="12D3KooWGZCnvk6cX2UUhc1SHhkGvdfJNZicx4uXEb3niyHHN7ch"
NODE2_ID="12D3KooWEMke2yrVjg4nadKBBCZrWeZtxD4KucM4QzgH24JMo6JU"
NODE3_ID="12D3KooWT3TACsUvszChWcQwT7YpPa1udfwpb5k5qQ8zrBw4VqZ7"

# Function to wait for operation completion
wait_for_operation_completion() {
    local node_url=$1
    local operation_id=$2
    local operation_type=$3
    local max_wait=90
    local wait_time=0
    
    print_info "Waiting for $operation_type operation $operation_id to complete..."
    
    while [ $wait_time -lt $max_wait ]; do
        local status_response
        status_response=$(curl -s -H "Authorization: Bearer $JWT_TOKEN" "$node_url/api/v1/operations/$operation_id" || echo "")
        
        if [ -z "$status_response" ]; then
            print_info "Waiting for operation to be created... ($wait_time/$max_wait seconds)"
            sleep 2
            wait_time=$((wait_time + 2))
            continue
        fi
        
        local status
        status=$(echo "$status_response" | jq -r '.status // empty')
        
        if [ "$status" = "3" ]; then
            if [ "$operation_type" = "keygen" ]; then
                keygen_key_id=$(echo "$status_response" | jq -r '.Result.KeygenResult.key_id // empty')
                print_success "$operation_type completed with key ID: $keygen_key_id"
                echo "$keygen_key_id"
                return 0
            elif [ "$operation_type" = "reshare" ]; then
                new_key_id=$(echo "$status_response" | jq -r '.Result.KeygenResult.key_id // empty')
                print_success "$operation_type completed with new key ID: $new_key_id"
                echo "$new_key_id"
                return 0
            fi
        elif [ "$status" = "4" ]; then
            print_error "$operation_type failed"
            echo "Response: $status_response"
            return 1
        fi
        
        print_info "$operation_type status: $status (waiting... $wait_time/$max_wait seconds)"
        sleep 2
        wait_time=$((wait_time + 2))
    done
    
    print_error "$operation_type operation timed out after $max_wait seconds"
    return 1
}

# Function to run keygen operation
run_keygen() {
    print_info "Starting 2-of-2 keygen operation..."
    
    # Start keygen on node1
    local keygen_response
    keygen_response=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $JWT_TOKEN" \
        -d "{
            \"threshold\": 1,
            \"participants\": [\"$NODE1_ID\", \"$NODE2_ID\"]
        }" \
        "$NODE1_URL/api/v1/keygen")
    
    local operation_id
    operation_id=$(echo "$keygen_response" | jq -r '.operation_id // empty')
    
    if [ -z "$operation_id" ]; then
        print_error "Failed to start keygen operation"
        echo "Response: $keygen_response"
        return 1
    fi
    
    print_info "Keygen operation started with ID: $operation_id"
    
    # Wait for completion and return key ID
    wait_for_operation_completion "$NODE1_URL" "$operation_id" "keygen"
}

# Function to run resharing operation
run_resharing() {
    local key_id=$1
    print_info "Starting 2-to-3 resharing operation for key: $key_id"
    
    # Start resharing on node1 (old participant)
    local reshare_response
    reshare_response=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $JWT_TOKEN" \
        -d "{
            \"key_id\": \"$key_id\",
            \"new_threshold\": 1,
            \"new_participants\": [\"$NODE1_ID\", \"$NODE2_ID\", \"$NODE3_ID\"]
        }" \
        "$NODE1_URL/api/v1/reshare")
    
    local operation_id
    operation_id=$(echo "$reshare_response" | jq -r '.operation_id // empty')
    
    if [ -z "$operation_id" ]; then
        print_error "Failed to start resharing operation"
        echo "Response: $reshare_response"
        return 1
    fi
    
    print_info "Resharing operation started with ID: $operation_id"
    
    # Wait for completion and return new key ID
    wait_for_operation_completion "$NODE1_URL" "$operation_id" "reshare"
}

# Main test function
main() {
    local mode=${1:-"test"}
    
    if [ "$mode" = "test" ]; then
        print_info "Running complete 2-to-3 resharing test..."
        
        # Step 1: Run keygen
        local key_id
        key_id=$(run_keygen)
        if [ $? -ne 0 ]; then
            print_error "Keygen failed"
            exit 1
        fi
        
        print_success "Keygen completed successfully with key ID: $key_id"
        
        # Wait a bit before resharing
        sleep 2
        
        # Step 2: Run resharing
        local new_key_id
        new_key_id=$(run_resharing "$key_id")
        if [ $? -ne 0 ]; then
            print_error "Resharing failed"
            exit 1
        fi
        
        print_success "Resharing completed successfully with new key ID: $new_key_id"
        print_success "Test completed successfully!"
        
    elif [ "$mode" = "keygen" ]; then
        run_keygen
    elif [ "$mode" = "reshare" ]; then
        if [ -z "$2" ]; then
            print_error "Key ID required for reshare mode"
            echo "Usage: $0 reshare <key_id>"
            exit 1
        fi
        run_resharing "$2"
    else
        echo "Usage: $0 [test|keygen|reshare <key_id>]"
        echo "  test     - Run complete keygen + resharing test"
        echo "  keygen   - Run only keygen operation"
        echo "  reshare  - Run only resharing operation (requires key_id)"
        exit 1
    fi
}

# Run main function with all arguments
main "$@" 