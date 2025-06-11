#!/bin/bash

# DKNet Docker Deployment Script

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

# Default values
ENVIRONMENT="dev"
ACTION="up"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -e|--env)
            ENVIRONMENT="$2"
            shift 2
            ;;
        -a|--action)
            ACTION="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo "Options:"
            echo "  -e, --env ENV        Environment (dev|prod) [default: dev]"
            echo "  -a, --action ACTION  Action (up|down|restart|logs|status) [default: up]"
            echo "  -h, --help           Show this help message"
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate environment
if [[ "$ENVIRONMENT" != "dev" && "$ENVIRONMENT" != "prod" ]]; then
    print_error "Invalid environment: $ENVIRONMENT. Use 'dev' or 'prod'"
    exit 1
fi

# Set compose file based on environment
if [[ "$ENVIRONMENT" == "dev" ]]; then
    COMPOSE_FILE="docker-compose.dev.yml"
else
    COMPOSE_FILE="docker-compose.yml"
fi

# Change to project root directory
cd "$(dirname "$0")/../.."

print_status "Using environment: $ENVIRONMENT"
print_status "Using compose file: $COMPOSE_FILE"

# Execute action
case $ACTION in
    up)
        print_status "Starting TSS cluster..."
        docker-compose -f $COMPOSE_FILE up -d
        print_success "TSS cluster started successfully!"
        
        print_status "Waiting for services to be ready..."
        sleep 10
        
        print_status "Checking service status..."
        docker-compose -f $COMPOSE_FILE ps
        
        echo ""
        print_success "TSS cluster is running!"
        echo "API endpoints:"
        echo "  Node 1: http://localhost:8081"
        echo "  Node 2: http://localhost:8082"
        echo "  Node 3: http://localhost:8083"
        if [[ "$ENVIRONMENT" == "prod" ]]; then
            echo "  Load Balancer: http://localhost"
        fi
        ;;
    down)
        print_status "Stopping TSS cluster..."
        docker-compose -f $COMPOSE_FILE down
        print_success "TSS cluster stopped successfully!"
        ;;
    restart)
        print_status "Restarting TSS cluster..."
        docker-compose -f $COMPOSE_FILE restart
        print_success "TSS cluster restarted successfully!"
        ;;
    logs)
        print_status "Showing TSS cluster logs..."
        docker-compose -f $COMPOSE_FILE logs -f
        ;;
    status)
        print_status "TSS cluster status:"
        docker-compose -f $COMPOSE_FILE ps
        
        echo ""
        print_status "Service health checks:"
        for port in 8081 8082 8083; do
            if curl -s -f "http://localhost:$port/health" > /dev/null; then
                print_success "Node on port $port: healthy"
            else
                print_error "Node on port $port: unhealthy"
            fi
        done
        ;;
    *)
        print_error "Invalid action: $ACTION. Use 'up', 'down', 'restart', 'logs', or 'status'"
        exit 1
        ;;
esac 