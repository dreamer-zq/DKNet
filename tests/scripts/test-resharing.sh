#!/bin/bash

# DKNet TSS Resharing Test Script
# This script performs positive tests for resharing operations with authentication and validation

set -e

# Source common functions
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/test-common.sh"

# Function to test resharing operation
test_resharing() {
    local key_id="$1"
    local new_threshold="$2"
    local new_participants_str="$3"
    local test_name="$4"
    
    print_status "Testing resharing operation: $test_name"
    
    local resharing_data=$(cat <<EOF
{
    "key_id": "$key_id",
    "new_threshold": $new_threshold,
    "new_participants": [$new_participants_str]
}
EOF
)
    
    print_result "Resharing Request Data:"
    echo "$resharing_data" | jq .
    
    local response=$(api_call "POST" "http://localhost:8081/api/v1/reshare" "$resharing_data")
    print_result "API Response:"
    echo "$response" | jq . 2>/dev/null || echo "Raw response: $response"
    
    local operation_id=$(echo "$response" | jq -r '.operation_id' 2>/dev/null)
    
    if [ "$operation_id" = "null" ] || [ -z "$operation_id" ]; then
        print_error "Failed to start resharing operation"
        echo "Response: $response"
        return 1
    fi
    
    print_result "Resharing Operation ID: $operation_id"
    
    # Wait for completion and get final result
    local final_response
    if final_response=$(wait_for_operation "$operation_id"); then
        local new_key_id=$(echo "$final_response" | jq -r '.Result.ResharingResult.new_key_id' 2>/dev/null)
        print_result "New Key ID after Resharing: $new_key_id"
        print_result "Operation ID: $operation_id"
        print_success "✓ Resharing test '$test_name' passed"
        echo "$new_key_id"  # Return new key_id for use by caller
        return 0
    else
        print_error "✗ Resharing test '$test_name' failed"
        print_result "Failed Operation ID: $operation_id"
        return 1
    fi
}

# Function to load key IDs from keygen tests
load_key_ids_for_resharing() {
    if [ ! -f "/tmp/dknet-key-1.txt" ] || [ ! -f "/tmp/dknet-key-2.txt" ]; then
        print_error "Key IDs not found. Please run keygen tests first:"
        print_status "  ./tests/scripts/test-keygen.sh start"
        print_status "  or"
        print_status "  ./tests/scripts/test-all.sh keygen"
        exit 1
    fi
    
    # Extract only the key ID (last line) from the files
    KEY_ID_1=$(tail -1 /tmp/dknet-key-1.txt 2>/dev/null | tr -d '\n\r' || echo "")
    KEY_ID_2=$(tail -1 /tmp/dknet-key-2.txt 2>/dev/null | tr -d '\n\r' || echo "")
    
    if [ -z "$KEY_ID_1" ] || [ -z "$KEY_ID_2" ]; then
        print_error "Invalid key IDs found. Please regenerate keys with keygen tests."
        exit 1
    fi
    
    print_result "Loaded Key IDs for Resharing:"
    print_result "  - Key 1 (2-of-3): $KEY_ID_1"
    print_result "  - Key 2 (3-of-3): $KEY_ID_2"
}

# Function to run all resharing tests
run_resharing_tests() {
    print_status "Starting DKNet TSS Resharing Tests"
    echo "==================================="
    
    # Display test environment info
    display_test_env_info
    echo ""
    
    # Check environment
    check_test_env
    
    # Load key IDs from keygen tests
    load_key_ids_for_resharing
    echo ""
    
    # Test validation service first
    if test_validation_service "Hello World from Resharing Test"; then
        print_success "✓ Validation service test passed"
    else
        print_error "✗ Validation service test failed"
        return 1
    fi
    
    echo ""
    
    # Test 1: Resharing from 2-of-3 to 3-of-3
    local new_key_id_1
    if new_key_id_1=$(test_resharing "$KEY_ID_1" 2 '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"' "Resharing 2-of-3 to 3-of-3"); then
        print_success "✓ Test 1 passed"
    else
        print_error "✗ Test 1 failed"
        return 1
    fi
    
    echo ""
    
    # Test 2: Resharing from 3-of-3 to 2-of-3
    local new_key_id_2
    if new_key_id_2=$(test_resharing "$KEY_ID_2" 1 '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"' "Resharing 3-of-3 to 2-of-3"); then
        print_success "✓ Test 2 passed"
    else
        print_error "✗ Test 2 failed"
        return 1
    fi
    
    echo ""
    
    # Test 3: Resharing with participant change (2-of-3 to 2-of-2)
    local new_key_id_3
    if new_key_id_3=$(test_resharing "$new_key_id_2" 1 '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'"' "Resharing 2-of-3 to 2-of-2 with participant change"); then
        print_success "✓ Test 3 passed"
    else
        print_error "✗ Test 3 failed"
        return 1
    fi
    
    echo ""
    
    # Test 4: Resharing with participant expansion (2-of-2 back to 2-of-3)
    local new_key_id_4
    if new_key_id_4=$(test_resharing "$new_key_id_3" 1 '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"' "Resharing 2-of-2 to 2-of-3 with participant expansion"); then
        print_success "✓ Test 4 passed"
    else
        print_error "✗ Test 4 failed"
        return 1
    fi
    
    echo ""
    print_success "All resharing tests passed!"
    
    # Summary
    print_result "Test Summary:"
    print_result "  - Original Key 1 (2-of-3): $KEY_ID_1"
    print_result "  - Original Key 2 (3-of-3): $KEY_ID_2"
    print_result "  - Reshared Key 1 (3-of-3): $new_key_id_1"
    print_result "  - Reshared Key 2 (2-of-3): $new_key_id_2"
    print_result "  - Reshared Key 3 (2-of-2): $new_key_id_3"
    print_result "  - Reshared Key 4 (2-of-3): $new_key_id_4"
    print_result "  - JWT Token: $JWT_TOKEN"
    print_result "  - All operations completed successfully"
    
    # Save reshared key IDs for use by signing tests
    echo "$new_key_id_1" > /tmp/dknet-key-reshared-1.txt
    echo "$new_key_id_2" > /tmp/dknet-key-reshared-2.txt
    echo "$new_key_id_3" > /tmp/dknet-key-reshared-3.txt
    echo "$new_key_id_4" > /tmp/dknet-key-reshared-4.txt
    print_result "Reshared Key IDs saved to /tmp/dknet-key-reshared-*.txt for signing tests"
}

# Function to run quick resharing test with provided key
quick_resharing_test() {
    local key_id="$1"
    local new_threshold="$2"
    
    if [ -z "$key_id" ] || [ -z "$new_threshold" ]; then
        print_error "Usage: quick_resharing_test <key_id> <new_threshold>"
        exit 1
    fi
    
    print_status "Running quick resharing test with key: $key_id"
    
    # Check environment
    check_test_env
    
    # Test resharing
    local new_key_id
    if new_key_id=$(test_resharing "$key_id" "$new_threshold" '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"' "Quick resharing test"); then
        print_success "✓ Quick resharing test passed"
        print_result "New Key ID: $new_key_id"
        print_result "JWT Token: $JWT_TOKEN"
    else
        print_error "✗ Quick resharing test failed"
        return 1
    fi
}

# Function to show help
show_help() {
    echo "DKNet TSS Resharing Test Script"
    echo ""
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  test               Run all resharing tests (requires keygen tests first)"
    echo "  quick <key_id> <new_threshold>  Run quick resharing test"
    echo "  status             Show environment status"
    echo "  logs               Show logs for all services"
    echo "  logs <service>     Show logs for specific service"
    echo "  help               Show this help message"
    echo ""
    echo "Prerequisites:"
    echo "  - Test environment must be running"
    echo "  - Keygen tests must be run first to generate initial keys"
    echo "  - Go must be installed for JWT token generation"
    echo "  - curl and jq must be available for API testing"
    echo ""
    echo "Examples:"
    echo "  $0 test                                        # Run all resharing tests"
    echo "  $0 quick 0x1234567890abcdef 2                 # Quick test: reshare to 3-of-3"
    echo "  $0 status                                      # Check environment status"
    echo "  $0 logs tss-node1                             # Show node1 logs"
    echo ""
    echo "Typical workflow:"
    echo "  1. ./tests/scripts/test-all.sh start          # Start environment"
    echo "  2. ./tests/scripts/test-keygen.sh test         # Generate initial keys"
    echo "  3. ./tests/scripts/test-resharing.sh test      # Run resharing tests"
    echo "  4. ./tests/scripts/test-signing.sh test        # Test signing with reshared keys"
    echo ""
    echo "Note: Resharing tests require existing keys from keygen tests."
    echo "The script will automatically load key IDs from /tmp/dknet-key-*.txt files."
}

# Main script logic
main() {
    case "${1:-test}" in
        test)
            run_resharing_tests
            ;;
        quick)
            quick_resharing_test "$2" "$3"
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