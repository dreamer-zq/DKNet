# DKNet Docker 部署指南

本文档介绍如何使用 Docker Compose 部署 DKNet TSS 集群和验证服务。

## 架构概述

部署包含以下服务：

- **验证服务** (`validation-service`): HTTP API 验证服务，用于在签名前验证请求
- **TSS 节点 1** (`tss-node1`): TSS 集群的第一个节点
- **TSS 节点 2** (`tss-node2`): TSS 集群的第二个节点  
- **TSS 节点 3** (`tss-node3`): TSS 集群的第三个节点

## 网络配置

所有服务运行在自定义 Docker 网络 `tss-network` (172.20.0.0/16) 中：

- `validation-service`: 172.20.0.2:8888
- `tss-node1`: 172.20.0.3:8080 (HTTP), 172.20.0.3:9090 (gRPC), 172.20.0.3:4001 (P2P)
- `tss-node2`: 172.20.0.4:8080 (HTTP), 172.20.0.4:9090 (gRPC), 172.20.0.4:4001 (P2P)
- `tss-node3`: 172.20.0.5:8080 (HTTP), 172.20.0.5:9090 (gRPC), 172.20.0.5:4001 (P2P)

## 端口映射

| 服务 | 内部端口 | 外部端口 | 协议 | 用途 |
|------|----------|----------|------|------|
| validation-service | 8888 | 8888 | HTTP | 验证 API |
| tss-node1 | 8080 | 8081 | HTTP | TSS HTTP API |
| tss-node1 | 9090 | 9095 | gRPC | TSS gRPC API |
| tss-node1 | 4001 | 4001 | TCP | P2P 通信 |
| tss-node2 | 8080 | 8082 | HTTP | TSS HTTP API |
| tss-node2 | 9090 | 9096 | gRPC | TSS gRPC API |
| tss-node2 | 4001 | 4002 | TCP | P2P 通信 |
| tss-node3 | 8080 | 8083 | HTTP | TSS HTTP API |
| tss-node3 | 9090 | 9098 | gRPC | TSS gRPC API |
| tss-node3 | 4001 | 4003 | TCP | P2P 通信 |

## 部署步骤

### 1. 启动服务

```bash
# 启动所有服务
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看服务日志
docker-compose logs -f
```

### 2. 验证服务状态

```bash
# 检查验证服务
curl http://localhost:8888/health

# 检查 TSS 节点
curl http://localhost:8081/health
curl http://localhost:8082/health  
curl http://localhost:8083/health
```

### 3. 运行测试

```bash
# 运行验证服务测试
./test-validation-simple.sh
```

## 验证服务配置

每个 TSS 节点都配置了验证服务：

```yaml
tss:
  node_id: node1
  moniker: TSS Node 1
  validation_service:
    enabled: true
    url: "http://validation-service:8888/validate"
    timeout_seconds: 30
    headers:
      X-Node-ID: "node1"
    insecure_skip_verify: false
```

## 验证规则

当前验证服务实现了以下规则：

1. **消息长度检查**: 拒绝空消息或超过 1KB 的消息
2. **内容过滤**: 拒绝包含禁用词的消息 (`malicious`, `hack`, `exploit`)
3. **密钥白名单**: 只允许特定的密钥 ID（测试时会放宽限制）
4. **时间戳验证**: 拒绝超过 5 分钟的旧请求
5. **参与者数量**: 要求至少 2 个参与者

## API 使用示例

### 验证服务 API

```bash
# 测试验证请求
curl -X POST http://localhost:8888/validate \
  -H "Content-Type: application/json" \
  -d '{
    "message": "48656c6c6f20576f726c64",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["node1", "node2"],
    "node_id": "node1",
    "timestamp": '$(date +%s)'
  }'
```

### TSS 签名 API

```bash
# 发送签名请求（消息需要 base64 编码）
MESSAGE_BASE64=$(echo -n "Hello World" | base64)
curl -X POST http://localhost:8081/api/v1/sign \
  -H "Content-Type: application/json" \
  -d '{
    "message": "'$MESSAGE_BASE64'",
    "key_id": "your-key-id",
    "participants": ["node1", "node2"]
  }'
```

## 日志查看

```bash
# 查看特定服务日志
docker-compose logs validation-service
docker-compose logs tss-node1
docker-compose logs tss-node2
docker-compose logs tss-node3

# 实时跟踪日志
docker-compose logs -f validation-service

# 查看最近的日志
docker-compose logs --tail=50 validation-service
```

## 故障排除

### 1. 服务启动失败

```bash
# 检查服务状态
docker-compose ps

# 查看错误日志
docker-compose logs [service-name]

# 重新构建镜像
docker-compose build --no-cache
```

### 2. P2P 连接问题

```bash
# 检查网络配置
docker network inspect dknet_tss-network

# 查看 P2P 连接日志
docker-compose logs tss-node1 | grep -i p2p
```

### 3. 验证服务连接问题

```bash
# 测试验证服务连通性
docker exec tss-node1 wget -qO- http://validation-service:8888/health

# 检查验证服务日志
docker-compose logs validation-service
```

## 停止和清理

```bash
# 停止所有服务
docker-compose down

# 停止并删除数据卷
docker-compose down -v

# 清理未使用的镜像
docker system prune -f
```

## 生产环境注意事项

1. **安全配置**:
   - 启用 TLS 加密
   - 配置防火墙规则
   - 使用强密码和证书

2. **监控和日志**:
   - 配置日志轮转
   - 设置监控告警
   - 定期备份数据

3. **高可用性**:
   - 使用多个验证服务实例
   - 配置负载均衡
   - 实施故障转移机制

4. **性能优化**:
   - 调整资源限制
   - 优化网络配置
   - 监控性能指标
