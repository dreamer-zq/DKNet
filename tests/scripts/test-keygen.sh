#!/bin/bash

# DKNet TSS Keygen Test Script
# This script performs positive tests for keygen operations with authentication and validation

set -e

# Source common functions
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/test-common.sh"

# Function to test keygen operation
test_keygen() {
    local threshold="$1"
    local participants_str="$2"
    local test_name="$3"
    
    print_status "Testing keygen operation: $test_name"
    
    local keygen_data=$(cat <<EOF
{
    "threshold": $threshold,
    "participants": [$participants_str]
}
EOF
)
    
    print_result "Keygen Request Data:"
    echo "$keygen_data" | jq .
    
    local response=$(api_call "POST" "http://localhost:8081/api/v1/keygen" "$keygen_data")
    local operation_id=$(echo "$response" | jq -r '.operation_id' 2>/dev/null)
    
    if [ "$operation_id" = "null" ] || [ -z "$operation_id" ]; then
        print_error "Failed to start keygen operation"
        echo "Response: $response"
        return 1
    fi
    
    print_result "Keygen Operation ID: $operation_id"
    
    # Wait for completion and get final result
    local final_response
    if final_response=$(wait_for_operation "$operation_id"); then
        local key_id=$(echo "$final_response" | jq -r '.Result.KeygenResult.key_id' 2>/dev/null)
        print_result "Generated Key ID: $key_id"
        print_result "Operation ID: $operation_id"
        print_success "✓ Keygen test '$test_name' passed"
        echo "$key_id"  # Return key_id for use by caller
        return 0
    else
        print_error "✗ Keygen test '$test_name' failed"
        print_result "Failed Operation ID: $operation_id"
        return 1
    fi
}



# Function to run all keygen tests
run_keygen_tests() {
    print_status "Starting DKNet TSS Keygen Tests"
    echo "================================="
    
    # Display test environment info
    display_test_env_info
    echo ""
    
    # Check environment
    check_test_env
    
    # Test validation service first
    if test_validation_service "Hello World from Keygen Test"; then
        print_success "✓ Validation service test passed"
    else
        print_error "✗ Validation service test failed"
        return 1
    fi
    
    echo ""
    
    # Test 1: Simple 2-of-3 keygen
    local key_id_1
    if key_id_1=$(test_keygen 1 '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"' "2-of-3 threshold keygen"); then
        print_success "✓ Test 1 passed"
    else
        print_error "✗ Test 1 failed"
        return 1
    fi
    
    echo ""
    
    # Test 2: 3-of-3 keygen
    local key_id_2
    if key_id_2=$(test_keygen 2 '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"' "3-of-3 threshold keygen"); then
        print_success "✓ Test 2 passed"
    else
        print_error "✗ Test 2 failed"
        return 1
    fi
    
    echo ""
    
    # Test 3: 2-node keygen
    local key_id_3
    if key_id_3=$(test_keygen 1 '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'"' "2-of-2 threshold keygen"); then
        print_success "✓ Test 3 passed"
    else
        print_error "✗ Test 3 failed"
        return 1
    fi
    
    echo ""
    print_success "All keygen tests passed!"
    
    # Summary
    print_result "Test Summary:"
    print_result "  - Original Key 1 (2-of-3): $key_id_1"
    print_result "  - Original Key 2 (3-of-3): $key_id_2"
    print_result "  - Original Key 3 (2-of-2): $key_id_3"
    print_result "  - JWT Token: $JWT_TOKEN"
    print_result "  - All operations completed successfully"
    
    # Save key IDs for use by resharing and signing tests
    echo "$key_id_1" > /tmp/dknet-key-1.txt
    echo "$key_id_2" > /tmp/dknet-key-2.txt
    echo "$key_id_3" > /tmp/dknet-key-3.txt
    print_result "Key IDs saved to /tmp/dknet-key-*.txt for resharing and signing tests"
}

# Function to show help
show_help() {
    echo "DKNet TSS Keygen Test Script"
    echo ""
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  start              Start test environment and run keygen tests"
    echo "  test               Run keygen tests (requires running environment)"
    echo "  stop               Stop test environment"
    echo "  status             Show environment status"
    echo "  logs               Show logs for all services"
    echo "  logs <service>     Show logs for specific service"
    echo "  cleanup            Stop environment and cleanup resources"
    echo "  help               Show this help message"
    echo ""
    echo "Prerequisites:"
    echo "  - Docker and docker-compose must be installed"
    echo "  - Go must be installed for JWT token generation"
    echo "  - curl and jq must be available for API testing"
    echo ""
    echo "Examples:"
    echo "  $0 start           # Start environment and run tests"
    echo "  $0 test            # Run tests only"
    echo "  $0 status          # Check environment status"
    echo "  $0 logs tss-node1  # Show node1 logs"
    echo "  $0 cleanup         # Cleanup everything"
}

# Main script logic
main() {
    case "${1:-start}" in
        start)
            start_test_env
            echo ""
            run_keygen_tests
            ;;
        test)
            run_keygen_tests
            ;;
        stop)
            stop_test_env
            ;;
        status)
            show_test_env_status
            echo ""
            if [ -n "$JWT_TOKEN" ]; then
                print_result "Current JWT Token: $JWT_TOKEN"
            fi
            ;;
        logs)
            show_test_env_logs "$2"
            ;;
        cleanup)
            cleanup_test_env
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

# Run main function with all arguments only if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi 