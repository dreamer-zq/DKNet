#!/bin/bash

# DKNet TSS Signing Test Script
# This script performs positive tests for signing operations with authentication and validation

set -e

# Source common functions
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/test-common.sh"

# Function to test signing operation
test_signing() {
    local key_id="$1"
    local participants_str="$2"
    local message="$3"
    local test_name="$4"
    
    print_status "Testing signing operation: $test_name"
    
    local message_base64=$(echo -n "$message" | base64)
    
    local signing_data=$(cat <<EOF
{
    "message": "$message_base64",
    "key_id": "$key_id",
    "participants": [$participants_str]
}
EOF
)
    
    print_result "Signing Request Data:"
    echo "$signing_data" | jq .
    print_result "Original Message: $message"
    
    # Determine which node to send the request to based on participants
    # Extract the first participant ID and map it to the corresponding node port
    local first_participant=$(echo "$participants_str" | sed 's/^"//; s/".*$//')
    local target_port=8081  # Default to node 1
    
    # Map participant ID to node port
    if [ "$first_participant" = "$NODE1_PEER_ID" ]; then
        target_port=8081
    elif [ "$first_participant" = "$NODE2_PEER_ID" ]; then
        target_port=8082
    elif [ "$first_participant" = "$NODE3_PEER_ID" ]; then
        target_port=8083
    fi
    
    local response=$(api_call "POST" "http://localhost:$target_port/api/v1/sign" "$signing_data")
    local operation_id=$(echo "$response" | jq -r '.operation_id' 2>/dev/null)
    
    if [ "$operation_id" = "null" ] || [ -z "$operation_id" ]; then
        print_error "Failed to start signing operation"
        echo "Response: $response"
        return 1
    fi
    
    print_result "Signing Operation ID: $operation_id"
    
    # Wait for completion and get final result from the same node
    local final_response
    if final_response=$(wait_for_operation_on_port "$operation_id" "$target_port"); then
        local signature=$(echo "$final_response" | jq -r '.Result.SigningResult.signature' 2>/dev/null)
        print_result "Generated Signature: $signature"
        print_result "Operation ID: $operation_id"
        print_success "✓ Signing test '$test_name' passed"
        echo "$signature"  # Return signature for use by caller
        return 0
    else
        print_error "✗ Signing test '$test_name' failed"
        print_result "Failed Operation ID: $operation_id"
        return 1
    fi
}

# Function to load key IDs from keygen tests
load_key_ids() {
    if [ ! -f "/tmp/dknet-key-1.txt" ]; then
        print_error "Key IDs not found. Please run keygen tests first:"
        print_status "  ./tests/scripts/test-keygen.sh start"
        exit 1
    fi
    
    # Extract only the key ID (last line) from the files
    KEY_ID_1=$(tail -1 /tmp/dknet-key-1.txt 2>/dev/null | tr -d '\n\r' || echo "")
    KEY_ID_2=$(tail -1 /tmp/dknet-key-2.txt 2>/dev/null | tr -d '\n\r' || echo "")
    KEY_ID_3=$(tail -1 /tmp/dknet-key-3.txt 2>/dev/null | tr -d '\n\r' || echo "")
    KEY_ID_RESHARED_1=$(tail -1 /tmp/dknet-key-reshared-1.txt 2>/dev/null | tr -d '\n\r' || echo "")
    KEY_ID_RESHARED_2=$(tail -1 /tmp/dknet-key-reshared-2.txt 2>/dev/null | tr -d '\n\r' || echo "")
    
    print_result "Loaded Key IDs:"
    print_result "  - Key 1 (2-of-3): $KEY_ID_1"
    print_result "  - Key 2 (3-of-3): $KEY_ID_2"
    print_result "  - Key 3 (2-of-2): $KEY_ID_3"
    print_result "  - Reshared Key 1 (3-of-3): $KEY_ID_RESHARED_1"
    print_result "  - Reshared Key 2 (2-of-3): $KEY_ID_RESHARED_2"
}

# Function to run all signing tests
run_signing_tests() {
    print_status "Starting DKNet TSS Signing Tests"
    echo "=================================="
    
    # Display test environment info
    display_test_env_info
    echo ""
    
    # Check environment
    check_test_env
    
    # Load key IDs from keygen tests
    load_key_ids
    echo ""
    
    # Test validation service first
    if test_validation_service "Hello World from Signing Test"; then
        print_success "✓ Validation service test passed"
    else
        print_error "✗ Validation service test failed"
        return 1
    fi
    
    echo ""
    
    # Test 1: Sign with 2-of-3 key using 2 participants
    local signature_1
    if signature_1=$(test_signing "$KEY_ID_1" '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'"' "Hello World Test 1" "2-of-3 key with 2 participants"); then
        print_success "✓ Test 1 passed"
    else
        print_error "✗ Test 1 failed"
        return 1
    fi
    
    echo ""
    
    # Test 2: Sign with 2-of-3 key using different 2 participants
    local signature_2
    if signature_2=$(test_signing "$KEY_ID_1" '"'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"' "Hello World Test 2" "2-of-3 key with different 2 participants"); then
        print_success "✓ Test 2 passed"
    else
        print_error "✗ Test 2 failed"
        return 1
    fi
    
    echo ""
    
    # Test 3: Sign with 3-of-3 key using all 3 participants
    local signature_3
    if signature_3=$(test_signing "$KEY_ID_2" '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"' "Hello World Test 3" "3-of-3 key with all participants"); then
        print_success "✓ Test 3 passed"
    else
        print_error "✗ Test 3 failed"
        return 1
    fi
    
    echo ""
    
    # Test 4: Sign with 2-of-2 key using both participants
    local signature_4
    if signature_4=$(test_signing "$KEY_ID_3" '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'"' "Hello World Test 4" "2-of-2 key with both participants"); then
        print_success "✓ Test 4 passed"
    else
        print_error "✗ Test 4 failed"
        return 1
    fi
    
    echo ""
    
    # Test 5: Sign with reshared key (3-of-3)
    local signature_5
    if signature_5=$(test_signing "$KEY_ID_RESHARED_1" '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"' "Hello World Test 5" "Reshared 3-of-3 key"); then
        print_success "✓ Test 5 passed"
    else
        print_error "✗ Test 5 failed"
        return 1
    fi
    
    echo ""
    
    # Test 6: Sign with reshared key (2-of-3)
    local signature_6
    if signature_6=$(test_signing "$KEY_ID_RESHARED_2" '"'$NODE1_PEER_ID'", "'$NODE3_PEER_ID'"' "Hello World Test 6" "Reshared 2-of-3 key"); then
        print_success "✓ Test 6 passed"
    else
        print_error "✗ Test 6 failed"
        return 1
    fi
    
    echo ""
    
    # Test 7: Sign different message types
    local signature_7
    if signature_7=$(test_signing "$KEY_ID_1" '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'"' "Transaction: Send 100 ETH to 0x123..." "Transaction signing test"); then
        print_success "✓ Test 7 passed"
    else
        print_error "✗ Test 7 failed"
        return 1
    fi
    
    echo ""
    
    # Test 8: Sign JSON message
    local json_message='{"type":"transfer","amount":"100","to":"0x123","nonce":42}'
    local signature_8
    if signature_8=$(test_signing "$KEY_ID_1" '"'$NODE2_PEER_ID'", "'$NODE3_PEER_ID'"' "$json_message" "JSON message signing test"); then
        print_success "✓ Test 8 passed"
    else
        print_error "✗ Test 8 failed"
        return 1
    fi
    
    echo ""
    print_success "All signing tests passed!"
    
    # Summary
    print_result "Test Summary:"
    print_result "  - Test 1 Signature: $signature_1"
    print_result "  - Test 2 Signature: $signature_2"
    print_result "  - Test 3 Signature: $signature_3"
    print_result "  - Test 4 Signature: $signature_4"
    print_result "  - Test 5 Signature: $signature_5"
    print_result "  - Test 6 Signature: $signature_6"
    print_result "  - Test 7 Signature: $signature_7"
    print_result "  - Test 8 Signature: $signature_8"
    print_result "  - JWT Token: $JWT_TOKEN"
    print_result "  - All operations completed successfully"
}

# Function to run quick signing test with provided key
quick_signing_test() {
    local key_id="$1"
    local message="${2:-Hello World Quick Test}"
    
    if [ -z "$key_id" ]; then
        print_error "Key ID is required for quick test"
        exit 1
    fi
    
    print_status "Running quick signing test with key: $key_id"
    
    # Check environment
    check_test_env
    
    # Test signing
    local signature
    if signature=$(test_signing "$key_id" '"'$NODE1_PEER_ID'", "'$NODE2_PEER_ID'"' "$message" "Quick signing test"); then
        print_success "✓ Quick signing test passed"
        print_result "Signature: $signature"
        print_result "JWT Token: $JWT_TOKEN"
    else
        print_error "✗ Quick signing test failed"
        return 1
    fi
}

# Function to show help
show_help() {
    echo "DKNet TSS Signing Test Script"
    echo ""
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  test               Run all signing tests (requires keygen tests first)"
    echo "  quick <key_id>     Run quick signing test with provided key ID"
    echo "  quick <key_id> <message>  Run quick signing test with custom message"
    echo "  status             Show environment status"
    echo "  logs               Show logs for all services"
    echo "  logs <service>     Show logs for specific service"
    echo "  help               Show this help message"
    echo ""
    echo "Prerequisites:"
    echo "  - Test environment must be running"
    echo "  - For 'test' command: keygen tests must be run first"
    echo "  - Go must be installed for JWT token generation"
    echo "  - curl and jq must be available for API testing"
    echo ""
    echo "Examples:"
    echo "  $0 test                                    # Run all signing tests"
    echo "  $0 quick 0x1234567890abcdef               # Quick test with key ID"
    echo "  $0 quick 0x1234567890abcdef \"My Message\" # Quick test with custom message"
    echo "  $0 status                                  # Check environment status"
    echo "  $0 logs tss-node1                         # Show node1 logs"
    echo ""
    echo "Note: To run full tests, first run keygen tests:"
    echo "  ./tests/scripts/test-keygen.sh start"
    echo "  ./tests/scripts/test-signing.sh test"
}

# Main script logic
main() {
    case "${1:-test}" in
        test)
            run_signing_tests
            ;;
        quick)
            quick_signing_test "$2" "$3"
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