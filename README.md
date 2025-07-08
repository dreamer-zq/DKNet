# DKNet

[![CI/CD](https://github.com/dreamer-zq/DKNet/actions/workflows/ci.yml/badge.svg)](https://github.com/dreamer-zq/DKNet/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.23+-blue.svg)](https://golang.org/dl/)
[![Docker](https://img.shields.io/badge/docker-supported-blue.svg)](https://hub.docker.com/r/dreamer-zq/dknet)

## 特性

- **阈值签名方案 (TSS)**: 支持 t-of-n 阈值签名
- **分布式架构**: 多节点协作，无单点故障
- **安全存储**: AES-256-GCM 加密存储私钥
- **多协议支持**: HTTP REST API 和 gRPC
- **P2P 通信**: 基于 libp2p 的节点间端到端加密通信
- **验证服务**: 可配置的签名请求验证
- **容器化部署**: 支持 Docker 和 Kubernetes
- **MCP 集成**: 支持 Model Context Protocol，允许 LLM 通过自然语言调用 TSS 功能

## 快速开始

### 构建

```bash
make build
```

### 运行

```bash
# 启动服务器
./bin/dknet start --config config.yaml

# 使用客户端进行密钥生成
./bin/dknet-cli keygen \
  --threshold 2 \
  --parties 3 \
  --participants node1,node2,node3

# 查看操作状态
./bin/dknet-cli operation <operation-id>
```

## 项目结构

```text
DKNet/
├── cmd/
│   ├── dknet/              # 服务器主程序
│   ├── dknet-cli/          # 客户端命令行工具
│   └── dknet-mcp/          # MCP 服务器（LLM 集成）
├── internal/               # 内部包
│   ├── api/               # API 层 (HTTP/gRPC)
│   ├── app/               # 应用程序层
│   ├── config/            # 配置管理
│   ├── common/            # 工具函数
│   ├── crypto/            # 加密相关
│   ├── p2p/               # P2P 网络
│   ├── storage/           # 存储层
│   ├── tss/               # TSS 核心逻辑
│   └── plugin/            # 插件
├── proto/                 # Protocol Buffers 定义
├── tests/                 # 测试和验证
├── docs/                  # 文档
└── examples/              # 示例配置
```

## 安全特性

### 加密存储

- **算法**: AES-256-GCM 对称加密
- **密钥派生**: PBKDF2-SHA256 (100,000 轮迭代)
- **认证**: 内置消息认证防篡改
- **随机性**: 每次加密使用随机 nonce

### 密码管理

支持两种安全的密码输入方式：

1. **环境变量** (推荐用于生产)
2. **交互式输入** (推荐用于开发)

## MCP 集成（LLM 支持）

DKNet 支持 Model Context Protocol (MCP)，允许 LLM 应用通过自然语言与 TSS 集群交互。

### 构建 MCP 服务器

```bash
make build-mcp
```

### 与 Claude Desktop 集成

1. 配置 Claude Desktop：

```json
{
  "mcpServers": {
    "dknet-tss": {
      "command": "/path/to/DKNet/bin/dknet-mcp",
      "args": [
        "--node-addr", "localhost:9095",
        "--node-id", "12D3KooWGZCnvk6cX2UUhc1SHhkGvdfJNZicx4uXEb3niyHHN7ch"
      ]
    }
  }
}
```

2. 重启 Claude Desktop

3. 使用自然语言进行 TSS 操作：
   - "请生成一个 2-of-3 的门限签名密钥"
   - "使用密钥 xyz 对消息 'Hello World' 进行签名"

详细的 MCP 使用指南请参考 [MCP 文档](docs/mcp-usage.md)。

## 部署

### Docker

```bash
# 使用 Docker Compose 启动测试环境
make docker-start
```

### 生产环境

```bash
# 设置加密密码
export TSS_ENCRYPTION_PASSWORD="YourSecurePassword123!"

# 启动服务
./bin/dknet start --config config.yaml
```

详细的部署指南请参考 [安全部署文档](docs/SECURITY.md)。

## API 文档

### HTTP API

- `GET /health` - 健康检查
- `POST /api/v1/keygen` - 密钥生成
- `POST /api/v1/sign` - 签名操作
- `POST /api/v1/reshare` - 密钥重分享(**暂不可用**)
- `GET /api/v1/operations/{id}` - 查询操作状态

### gRPC API

详细的 API 文档请参考 [API 文档](docs/api.md)。

## 测试

```bash
# 运行单元测试
make test

# 运行端到端测试
make test-e2e
```

## 贡献

欢迎贡献代码！请先阅读 [贡献指南](CONTRIBUTING.md)。

## 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件。
