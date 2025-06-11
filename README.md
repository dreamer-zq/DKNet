# DKNet

阈值签名方案（Threshold Signature Scheme）服务器，提供分布式密钥生成、签名和密钥管理功能。

## 项目概述

DKNet 是一个高性能的阈值签名服务，支持：

- 🔑 **分布式密钥生成**：安全的多方密钥生成协议
- ✍️ **阈值签名**：需要最少数量参与方的安全签名
- 🔄 **密钥重新分享**：动态调整阈值和参与方
- 🌐 **双协议支持**：HTTP RESTful API 和 gRPC
- 📊 **操作管理**：完整的操作状态跟踪和管理
- 🏥 **健康监控**：实时健康状态检查

## 快速开始

### 构建项目

```bash
# 克隆仓库
git clone <repository-url>
cd tss-server

# 安装依赖
go mod tidy

# 生成 protobuf 代码
make proto-gen

# 构建所有组件
make build
```

### 启动服务器

```bash
# 启动 TSS 服务器
./bin/tss-server

# 服务器将在以下端口启动：
# HTTP API: http://localhost:8080
# gRPC API: localhost:9001
```

### 使用客户端工具

```bash
# 启动密钥生成
./bin/tss-client keygen \
  --threshold 2 \
  --parties 3 \
  --participants peer1,peer2,peer3

# 查询操作状态
./bin/tss-client operation <operation-id>
```

## 项目结构

```text
tss-server/
├── cmd/
│   ├── tss-server/          # 服务器主程序
│   └── tss-client/          # 客户端命令行工具
├── internal/
│   ├── api/                 # HTTP/gRPC API 服务器
│   ├── tss/                 # TSS 核心逻辑
│   └── config/              # 配置管理
├── proto/
│   ├── tss/v1/              # TSS 服务 protobuf 定义
│   └── health/v1/           # 健康检查服务定义
├── docs/                    # 项目文档
├── Makefile                 # 构建和开发命令
└── README.md               # 项目说明
```

## 文档

### 使用指南

- **[服务器使用指南](docs/server-usage.md)** - DKNet 的完整配置、部署和管理说明
- **[客户端使用指南](docs/client-usage.md)** - TSS Client 命令行工具的详细使用教程

### API 文档

- **[HTTP API 文档](docs/api.md)** - RESTful API 接口说明
- **[gRPC API 文档](docs/grpc-api.md)** - gRPC 服务接口文档

## 核心功能

### 支持的操作

- **密钥生成 (Keygen)**: 分布式生成阈值密钥
- **数字签名 (Signing)**: 使用阈值密钥进行安全签名
- **密钥重新分享 (Resharing)**: 更改密钥阈值或参与方

## 开发和构建

### 环境要求

- Go 1.21+
- Protocol Buffers 编译器 (`protoc`)
- gRPC Go 插件

### 主要构建命令

```bash
# 构建服务器和客户端
make build

# 生成 protobuf 代码
make proto-gen

# 运行测试
make test

# 清理构建产物
make clean
```

## 部署

### Docker 部署

```bash
# 构建 Docker 镜像
make docker-build

# 启动开发集群
make docker-dev

# 启动生产集群
make docker-prod
```

### 本地部署

```bash
# 生成本地集群配置
make init-local-cluster

# 启动服务器
./bin/tss-server --config config.yaml
```

## 安全注意事项

⚠️ **重要安全提醒**：

1. **网络安全**：生产环境必须使用 TLS 加密
2. **密钥管理**：妥善保护生成的密钥材料
3. **访问控制**：限制 API 访问权限和网络访问
4. **审计日志**：启用完整的操作审计记录
5. **参与方验证**：验证所有参与方的身份和权限

## 技术架构

DKNet 采用模块化设计：

- **API Layer**: HTTP 和 gRPC 双协议支持
- **Business Logic**: TSS 核心算法实现
- **Storage Layer**: 操作状态和结果持久化
- **Network Layer**: 安全的参与方通信

详细的架构说明请参见：[架构设计文档](docs/architecture.md)

## 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

## 贡献指南

欢迎贡献代码！请遵循以下步骤：

1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

### 代码规范

- 遵循 Go 官方代码风格
- 添加适当的注释和文档
- 编写单元测试
- 确保 proto 文件符合 buf lint 规范

---

**阈值签名方案 (TSS)** 是一种先进的加密技术，允许一组参与者共同生成数字签名，而无需暴露完整的私钥。这种技术在区块链、加密货币钱包、多重签名系统和企业安全应用中有着广泛的应用前景。
