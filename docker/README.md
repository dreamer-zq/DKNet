# DKNet Docker Deployment

This directory contains Docker configuration files for deploying the DKNet in a containerized environment.

## Quick Start

### 1. Development Environment (3-node cluster)

```bash
# Start the cluster
./docker/scripts/deploy.sh --env dev --action up

# Check status
./docker/scripts/deploy.sh --env dev --action status

# View logs
./docker/scripts/deploy.sh --env dev --action logs

# Stop the cluster
./docker/scripts/deploy.sh --env dev --action down
```

### 2. Production Environment (with Load Balancer)

```bash
# Start the cluster with Nginx load balancer
./docker/scripts/deploy.sh --env prod --action up

# Check status
./docker/scripts/deploy.sh --env prod --action status
```

### 3. Run Tests

```bash
# Run basic connectivity tests
./docker/scripts/test.sh

# Test specific TSS operations
./docker/scripts/test.sh keygen
./docker/scripts/test.sh sign
```

## Services and Ports

### Development Environment

| Service | HTTP API | gRPC API | P2P Port |
|---------|----------|----------|----------|
| Node 1  | 8081     | 9091     | 4001     |
| Node 2  | 8082     | 9092     | 4002     |
| Node 3  | 8083     | 9093     | 4003     |

### Production Environment

| Service        | HTTP API | gRPC API | P2P Port |
|----------------|----------|----------|----------|
| Node 1         | 8081     | 9091     | 4001     |
| Node 2         | 8082     | 9092     | 4002     |
| Node 3         | 8083     | 9093     | 4003     |
| Load Balancer  | 80       | -        | -        |

## API Endpoints

### Health Check

```bash
curl http://localhost:8081/health
```

### Node Status

```bash
curl http://localhost:8081/api/status
```

### List Peers

```bash
curl http://localhost:8081/api/peers
```

### TSS Operations

#### Key Generation

```bash
curl -X POST http://localhost:8081/api/tss/keygen \
  -H "Content-Type: application/json" \
  -d '{"threshold": 2, "parties": 3}'
```

#### Message Signing

```bash
curl -X POST http://localhost:8081/api/tss/sign \
  -H "Content-Type: application/json" \
  -d '{"message": "hello world", "key_id": "your-key-id"}'
```

## Configuration

### Node Configuration Files

- `docker/configs/node1.yaml` - Node 1 configuration
- `docker/configs/node2.yaml` - Node 2 configuration  
- `docker/configs/node3.yaml` - Node 3 configuration

### Network Configuration

The Docker network uses subnet `172.20.0.0/16` with the following setup:

- Node discovery through Docker hostname resolution
- P2P communication between nodes via internal Docker network
- External API access through mapped ports

### Data Persistence

Each node stores data in Docker volumes:

- `tss-node1-data` (or `tss-node1-dev-data` for dev)
- `tss-node2-data` (or `tss-node2-dev-data` for dev)
- `tss-node3-data` (or `tss-node3-dev-data` for dev)

## Customization

### Modify Node Configuration

Edit the configuration files in `docker/configs/` and restart:

```bash
./docker/scripts/deploy.sh --action restart
```

### Scale Deployment

To add more nodes:

1. Create additional config files (`node4.yaml`, etc.)
2. Add services to `docker-compose.yml`
3. Update the scripts accordingly

### Cross-Network Deployment

For deployment across different networks, modify the `bootstrap_peers` in config files to use external IP addresses:

```yaml
p2p:
  bootstrap_peers:
    - "/ip4/203.123.45.67/tcp/4001"  # External IP
    - "/ip4/198.51.100.123/tcp/4001"
```

## Troubleshooting

### Check Container Logs

```bash
docker-compose -f docker-compose.dev.yml logs tss-node1
```

### Enter Container for Debugging

```bash
docker exec -it tss-node1-dev /bin/sh
```

### Check Network Connectivity

```bash
# Test P2P connectivity between nodes
docker exec tss-node1-dev wget -O- http://tss-node2:8080/health
```

### Reset All Data

```bash
# Stop cluster and remove volumes
./docker/scripts/deploy.sh --action down
docker volume prune -f
```

## Security Considerations

- The current setup uses HTTP (not HTTPS) for simplicity
- Private keys are stored in Docker volumes
- For production, consider:
  - Enabling TLS/SSL
  - Using external key management systems
  - Implementing proper access controls
  - Network segmentation
