# DKNet 服务器使用指南

本文档详细说明了 DKNet TSS 服务器的配置、部署和使用方法。

## 快速开始

### 构建和安装

```bash
# 克隆项目
git clone <repository-url>
cd dknet

# 构建服务器
make build

# 构建客户端工具
make build-client
```

### 基本使用

```bash
# 启动服务器（使用默认配置）
./bin/dknet

# 使用指定配置文件启动
./bin/dknet --node-dir ./node1

# 查看帮助信息
./bin/dknet --help
```

### 后台运行

```bash
# 后台运行服务器
nohup ./bin/dknet > dknet.log 2>&1 &

# 查看进程
ps aux | grep dknet

# 查看日志
tail -f dknet.log
```

## 服务器配置

### 默认配置

DKNet 使用默认配置启动，监听以下端口：

- **HTTP API**: `http://localhost:8080`
- **gRPC API**: `localhost:9001`

### 自定义配置

创建配置文件 `config.yaml`：

```yaml
# HTTP 服务配置
http:
  host: "0.0.0.0"
  port: 8080

# gRPC 服务配置
grpc:
  host: "0.0.0.0"
  port: 9001

# 安全配置
security:
  tls_enabled: false
  cert_file: ""
  key_file: ""

# TSS 配置
tss:
  # TSS 相关配置项
```

## 启动服务器

### 基本启动

```bash
# 使用默认配置启动
./bin/dknet

# 使用自定义配置文件
./bin/dknet --node-dir ./node1
```

### 开发模式

```bash
# 开发模式启动（包含详细日志）
make dev-server
```

### 后台服务

```bash
# 作为后台服务运行
nohup ./bin/dknet > dknet.log 2>&1 &

# 检查服务状态
ps aux | grep dknet

# 查看日志
tail -f dknet.log
```

## API 服务

### HTTP RESTful API

服务器启动后，HTTP API 在 `http://localhost:8080` 提供服务：

| 端点 | 方法 | 描述 |
|------|------|------|
| `/health` | GET | 健康检查 |
| `/api/v1/keygen` | POST | 启动密钥生成 |
| `/api/v1/sign` | POST | 启动签名操作 |
| `/api/v1/reshare` | POST | 启动密钥重新分享 |
| `/operations/:id` | GET | 获取操作状态 |
| `/operations/:id` | DELETE | 取消操作 |

### gRPC API

gRPC 服务在 `localhost:9001` 提供服务，包含以下服务：

- **TSSService**: 所有 TSS 操作
- **HealthService**: 健康检查和监控

## 集群部署

### Docker 容器部署

```bash
# 构建 Docker 镜像
make docker-build

# 启动开发集群（3节点）
make docker-dev

# 启动生产集群
make docker-prod

# 查看集群状态
make docker-status

# 查看集群日志
make docker-logs

# 停止集群
make docker-stop
```

### 本地集群部署

```bash
# 生成本地集群配置
make init-local-cluster

# 这将在 ./local-cluster 目录生成：
# - node1/ node2/ node3/ 各节点配置
# - 每个节点的私钥和配置文件
```

## 节点管理

### 初始化单个节点

```bash
# 生成节点配置
./bin/dknet init-node \
  --node-id node1 \
  --listen-addr /ip4/0.0.0.0/tcp/4001 \
  --api-addr localhost:8080 \
  --grpc-addr localhost:9090 \
  --output ./node1

# 快速示例
make init-node-example
```

### 查看节点信息

```bash
# 显示节点详细信息
make show-node NODE_DIR=./nodes/my-org

# JSON 格式输出
make show-node-json NODE_DIR=./nodes/my-org

# 仅显示多地址
make show-multiaddr NODE_DIR=./nodes/my-org
```

## 监控和健康检查

### 健康检查端点

```bash
# HTTP 健康检查
curl http://localhost:8080/health

# 响应示例：
{
  "status": "HEALTH_STATUS_SERVING",
  "timestamp": "2024-06-11T13:45:30Z",
  "details": "DKNet is healthy",
  "metadata": {
    "service": "dknet",
    "version": "1.0.0"
  }
}
```

### 服务监控

```bash
# 检查 HTTP 服务
curl -f http://localhost:8080/health || echo "HTTP service down"

# 检查 gRPC 服务（需要 grpcurl 工具）
grpcurl -plaintext localhost:9001 health.v1.HealthService/Check
```

## 操作管理

### 查询操作状态

```bash
# HTTP API
curl http://localhost:8080/operations/{operation-id}

# 使用客户端工具
./bin/dknet-cli operation {operation-id}
```

### 取消操作

```bash
# HTTP API
curl -X DELETE http://localhost:8080/operations/{operation-id}

# 使用客户端工具
./bin/dknet-cli cancel-operation {operation-id}
```

## 安全配置

### TLS 配置

```yaml
# config.yaml
security:
  tls_enabled: true
  cert_file: "/path/to/server.crt"
  key_file: "/path/to/server.key"
```

```bash
# 生成自签名证书（仅用于测试）
openssl req -new -newkey rsa:2048 -days 365 -nodes -x509 \
  -keyout server.key -out server.crt

# 启动 TLS 服务器
./bin/dknet --node-dir ./node1
```

### 访问控制

在生产环境中建议：

1. 使用防火墙限制端口访问
2. 配置反向代理（如 Nginx）
3. 实施 API 认证和授权
4. 启用访问日志记录

### 日志配置

```yaml
# config.yaml
logging:
  level: "info"  # debug, info, warn, error
  format: "json" # json, text
  output: "stdout" # stdout, stderr, 文件路径
```

## 性能调优

### 连接池配置

```yaml
# config.yaml
server:
  max_connections: 1000
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "60s"
```

## 生产部署建议

### 高可用配置

1. **负载均衡**: 使用 Nginx 或 HAProxy
2. **服务发现**: 集成 Consul 或 etcd
3. **监控告警**: 配置 Prometheus + Grafana
4. **日志收集**: 使用 ELK Stack 或 Loki

### 自动化部署

```bash
# 使用 systemd 管理服务
sudo cp dknet.service /etc/systemd/system/
sudo systemctl enable dknet
sudo systemctl start dknet
```

示例 systemd 服务文件：

```ini
[Unit]
Description=DKNet TSS Server
After=network.target

[Service]
Type=simple
User=dknet
Group=dknet
WorkingDirectory=/opt/dknet
ExecStart=/opt/dknet/bin/dknet --node-dir /opt/dknet/node1
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```
