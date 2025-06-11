#!/bin/bash

# DKNet Testing Script

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print colored output
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

# Test configuration
NODES=("localhost:8081" "localhost:8082" "localhost:8083")
THRESHOLD=2
PARTIES=3

# Wait for service to be ready
wait_for_service() {
    local url=$1
    local max_attempts=30
    local attempt=1
    
    print_status "Waiting for service at $url to be ready..."
    
    while [ $attempt -le $max_attempts ]; do
        if curl -s -f "$url/health" > /dev/null 2>&1; then
            print_success "Service at $url is ready!"
            return 0
        fi
        
        echo -n "."
        sleep 2
        ((attempt++))
    done
    
    print_error "Service at $url failed to become ready after $max_attempts attempts"
    return 1
}

# Test API endpoint
test_api_endpoint() {
    local node=$1
    local endpoint=$2
    local expected_status=${3:-200}
    
    print_status "Testing $endpoint on $node..."
    
    response=$(curl -s -w "%{http_code}" -o /tmp/response "$node$endpoint" || echo "000")
    
    if [[ "$response" == "$expected_status" ]]; then
        print_success "✓ $endpoint on $node returned $response"
        return 0
    else
        print_error "✗ $endpoint on $node returned $response, expected $expected_status"
        return 1
    fi
}

# Test TSS operation
test_tss_operation() {
    local operation=$1
    local node=${NODES[0]}
    
    print_status "Testing TSS $operation operation on $node..."
    
    case $operation in
        keygen)
            payload="{\"threshold\": $THRESHOLD, \"parties\": $PARTIES}"
            ;;
        sign)
            # First, we need to have a key generated
            payload='{"message": "test message for signing", "key_id": "test-key"}'
            ;;
        reshare)
            payload="{\"threshold\": $THRESHOLD, \"parties\": $PARTIES, \"key_id\": \"test-key\"}"
            ;;
        *)
            print_error "Unknown TSS operation: $operation"
            return 1
            ;;
    esac
    
    response=$(curl -s -w "%{http_code}" \
        -H "Content-Type: application/json" \
        -d "$payload" \
        -o /tmp/tss_response \
        "$node/api/tss/$operation" || echo "000")
    
    if [[ "$response" == "200" ]] || [[ "$response" == "202" ]]; then
        print_success "✓ TSS $operation operation initiated successfully"
        cat /tmp/tss_response | head -c 200
        echo ""
        return 0
    else
        print_error "✗ TSS $operation operation failed with status $response"
        cat /tmp/tss_response
        echo ""
        return 1
    fi
}

# Main test function
run_tests() {
    print_status "Starting DKNet integration tests..."
    
    # Wait for all nodes to be ready
    for node in "${NODES[@]}"; do
        if ! wait_for_service "http://$node"; then
            print_error "Failed to connect to node $node"
            exit 1
        fi
    done
    
    print_success "All nodes are ready!"
    
    # Test basic API endpoints
    local failed_tests=0
    
    for node in "${NODES[@]}"; do
        print_status "Testing node: $node"
        
        # Test health endpoint
        test_api_endpoint "http://$node" "/health" 200 || ((failed_tests++))
        
        # Test status endpoint
        test_api_endpoint "http://$node" "/api/status" 200 || ((failed_tests++))
        
        # Test peers endpoint
        test_api_endpoint "http://$node" "/api/peers" 200 || ((failed_tests++))
        
        echo ""
    done
    
    # Test TSS operations (commented out for now as they require proper setup)
    print_warning "TSS operation tests are currently disabled"
    print_warning "To enable them, ensure all nodes are properly configured with matching party IDs"
    
    # Uncomment these when ready for full TSS testing:
    # test_tss_operation "keygen" || ((failed_tests++))
    # sleep 5
    # test_tss_operation "sign" || ((failed_tests++))
    
    # Print test results
    echo ""
    if [ $failed_tests -eq 0 ]; then
        print_success "All tests passed! ✓"
        exit 0
    else
        print_error "$failed_tests test(s) failed! ✗"
        exit 1
    fi
}

# Parse command line arguments
case "${1:-run}" in
    run)
        run_tests
        ;;
    keygen)
        test_tss_operation "keygen"
        ;;
    sign)
        test_tss_operation "sign"
        ;;
    reshare)
        test_tss_operation "reshare"
        ;;
    *)
        echo "Usage: $0 [run|keygen|sign|reshare]"
        echo "  run     - Run all basic tests (default)"
        echo "  keygen  - Test key generation"
        echo "  sign    - Test signing"
        echo "  reshare - Test key resharing"
        exit 1
        ;;
esac 