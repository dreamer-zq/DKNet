#!/bin/bash

# DKNet JWT Authentication Integration Test
# This script tests the complete JWT authentication flow

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
START_ENV_SCRIPT="$SCRIPT_DIR/start-test-env.sh"

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

# Function to check if environment is running
check_environment() {
    print_status "Checking if test environment is running..."
    
    # Check validation service
    if ! curl -s http://localhost:8888/health >/dev/null 2>&1; then
        print_error "Test environment is not running"
        print_status "Please start the environment first:"
        print_status "  $START_ENV_SCRIPT start"
        exit 1
    fi
    
    print_success "Test environment is running"
}

# Function to test JWT authentication flow
test_jwt_flow() {
    print_status "Testing complete JWT authentication flow..."
    
    # Test 1: Generate JWT token
    print_status "Step 1: Generating JWT token..."
    if ! $START_ENV_SCRIPT generate-token >/dev/null 2>&1; then
        print_error "Failed to generate JWT token"
        return 1
    fi
    print_success "JWT token generated successfully"
    
    # Test 2: Test authentication
    print_status "Step 2: Testing API authentication..."
    if ! $START_ENV_SCRIPT test-auth; then
        print_error "JWT authentication test failed"
        return 1
    fi
    print_success "JWT authentication test passed"
    
    # Test 3: Test TSS operations with authentication
    print_status "Step 3: Testing TSS operations with authentication..."
    if ! $START_ENV_SCRIPT test-tss; then
        print_error "TSS operations test failed"
        return 1
    fi
    print_success "TSS operations test passed"
    
    return 0
}

# Function to test manual JWT usage
test_manual_jwt() {
    print_status "Testing manual JWT token usage..."
    
    # Generate token and extract it
    local token_output
    token_output=$($START_ENV_SCRIPT generate-token 2>/dev/null)
    local jwt_token
    jwt_token=$(echo "$token_output" | grep -A1 "JWT Token generated:" | tail -1)
    
    if [ -z "$jwt_token" ]; then
        print_error "Could not extract JWT token"
        return 1
    fi
    
    print_status "Testing manual API call with JWT token..."
    
    # Test authenticated request
    local response
    response=$(curl -s -w "%{http_code}" \
        -H "Authorization: Bearer $jwt_token" \
        -H "Content-Type: application/json" \
        http://localhost:8081/health 2>/dev/null)
    
    local http_code="${response: -3}"
    local body="${response%???}"
    
    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        print_success "Manual JWT authentication successful (HTTP $http_code)"
        return 0
    else
        print_error "Manual JWT authentication failed (HTTP $http_code)"
        if [ -n "$body" ]; then
            echo "Response: $body"
        fi
        return 1
    fi
}

# Function to test unauthenticated requests
test_unauthenticated() {
    print_status "Testing unauthenticated requests (should fail)..."
    
    local response
    response=$(curl -s -w "%{http_code}" -o /dev/null http://localhost:8081/health 2>/dev/null || echo "000")
    
    if [ "$response" = "401" ]; then
        print_success "Unauthenticated requests correctly rejected (HTTP 401)"
        return 0
    else
        print_warning "Expected HTTP 401 for unauthenticated request, got HTTP $response"
        return 1
    fi
}

# Function to run all tests
run_all_tests() {
    print_status "Starting DKNet JWT Authentication Integration Tests"
    echo "=================================================="
    
    local tests_passed=0
    local tests_failed=0
    
    # Test 1: Check environment
    if check_environment; then
        ((tests_passed++))
    else
        ((tests_failed++))
        return 1
    fi
    
    # Test 2: Test unauthenticated requests
    if test_unauthenticated; then
        ((tests_passed++))
    else
        ((tests_failed++))
    fi
    
    # Test 3: Test JWT flow
    if test_jwt_flow; then
        ((tests_passed++))
    else
        ((tests_failed++))
    fi
    
    # Test 4: Test manual JWT usage
    if test_manual_jwt; then
        ((tests_passed++))
    else
        ((tests_failed++))
    fi
    
    # Summary
    echo ""
    print_status "Test Summary"
    print_status "============"
    print_success "Tests passed: $tests_passed"
    if [ $tests_failed -gt 0 ]; then
        print_error "Tests failed: $tests_failed"
        return 1
    else
        print_success "All tests passed!"
        return 0
    fi
}

# Function to show help
show_help() {
    echo "DKNet JWT Authentication Integration Test"
    echo ""
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  run-all            Run all integration tests (default)"
    echo "  check-env          Check if test environment is running"
    echo "  test-unauth        Test unauthenticated requests"
    echo "  test-jwt           Test JWT authentication flow"
    echo "  test-manual        Test manual JWT token usage"
    echo "  help               Show this help message"
    echo ""
    echo "Prerequisites:"
    echo "  - Test environment must be running (use start-test-env.sh start)"
    echo "  - Go must be installed for JWT token generation"
    echo "  - curl must be available for API testing"
    echo ""
    echo "Examples:"
    echo "  $0                 # Run all tests"
    echo "  $0 test-jwt        # Test JWT authentication only"
    echo "  $0 check-env       # Check environment status"
}

# Main script logic
main() {
    case "${1:-run-all}" in
        run-all)
            run_all_tests
            ;;
        check-env)
            check_environment
            ;;
        test-unauth)
            test_unauthenticated
            ;;
        test-jwt)
            test_jwt_flow
            ;;
        test-manual)
            test_manual_jwt
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