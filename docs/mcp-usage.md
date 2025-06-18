# DKNet MCP 服务器使用指南

DKNet MCP 服务器是一个桥接工具，允许 LLM 应用程序通过自然语言与现有的 DKNet 集群进行交互。

## 概述

DKNet MCP 服务器基于 [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) 构建，作为轻量级客户端连接到现有的 DKNet 网络，而不是作为网络的参与节点。这样的设计使得 LLM 可以通过自然语言调用 TSS 操作，如密钥生成和签名。

## 功能特性

- **密钥生成 (keygen)**: 生成分布式门限签名密钥
- **消息签名 (sign)**: 使用生成的密钥对消息进行签名
- **自然语言交互**: 通过 LLM 使用自然语言描述操作
- **连接现有网络**: 连接到已运行的 DKNet 集群

## 系统要求

- Go 1.21 或更高版本
- 运行中的 DKNet 集群
- 支持 MCP 的 LLM 客户端（如 Claude Desktop）

## 安装和构建

1. 构建 MCP 服务器：

```bash
make build-mcp
# 或者
go build -o bin/dknet-mcp ./cmd/dknet-mcp
```

## 使用方法

### 基本语法

```bash
./bin/dknet-mcp --node-addr <NODE_ADDRESS> --node-id <NODE_ID> [--jwt-token <TOKEN>]
```

### 参数说明

- `--node-addr`: DKNet 节点的 gRPC 地址（默认: `localhost:9095`）
- `--node-id`: 节点 ID，用于 X-Node-ID 头部（必需）
- `--jwt-token`: JWT 认证令牌（可选，如果集群启用了认证）

### 示例

连接到本地 DKNet 节点：

```bash
./bin/dknet-mcp --node-addr localhost:9095 --node-id 12D3KooWGZCnvk6cX2UUhc1SHhkGvdfJNZicx4uXEb3niyHHN7ch
```

连接到远程节点并使用 JWT 认证：

```bash
./bin/dknet-mcp \
  --node-addr 192.168.1.100:9095 \
  --node-id 12D3KooWGZCnvk6cX2UUhc1SHhkGvdfJNZicx4uXEb3niyHHN7ch \
  --jwt-token "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

## MCP 工具

DKNet MCP 服务器提供以下工具：

### 1. tss_keygen - 密钥生成

生成新的分布式门限签名密钥。

**参数:**
- `threshold` (number): 最小签名方数量 (t in t-of-n)
- `parties` (number): 总参与方数量 (n in t-of-n)
- `participants` (string): 参与密钥生成的节点 ID 列表（逗号分隔）
- `operation_id` (string, 可选): 操作 ID，用于幂等性

**示例自然语言指令:**
- "请生成一个 2-of-3 的门限签名密钥，参与节点为 node1, node2, node3"
- "创建一个新的 TSS 密钥，需要 3 个节点中的任意 2 个来签名"

### 2. tss_sign - 消息签名

使用现有的门限签名密钥对消息进行签名。

**参数:**
- `message` (string): 要签名的消息（纯文本或十六进制）
- `key_id` (string): 密钥 ID（来自密钥生成操作）
- `participants` (string): 参与签名的节点 ID 列表（逗号分隔）
- `operation_id` (string, 可选): 操作 ID
- `message_format` (string, 可选): 消息格式，'text' 或 'hex'（默认: text）

**示例自然语言指令:**
- "使用密钥 key-12345 签名消息 'Hello World'，参与节点为 node1, node2"
- "对十六进制消息进行签名，使用之前生成的密钥"

## 与 Claude Desktop 集成

### 配置文件

创建或编辑 Claude Desktop 配置文件：

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "dknet-tss": {
      "command": "/path/to/your/dknet-mcp",
      "args": [
        "--node-addr", "localhost:9095",
        "--node-id", "12D3KooWGZCnvk6cX2UUhc1SHhkGvdfJNZicx4uXEb3niyHHN7ch"
      ],
      "env": {}
    }
  }
}
```

### 使用 JWT 认证

如果您的 DKNet 集群启用了 JWT 认证：

```json
{
  "mcpServers": {
    "dknet-tss": {
      "command": "/path/to/your/dknet-mcp",
      "args": [
        "--node-addr", "localhost:9095",
        "--node-id", "12D3KooWGZCnvk6cX2UUhc1SHhkGvdfJNZicx4uXEb3niyHHN7ch",
        "--jwt-token", "your-jwt-token-here"
      ],
      "env": {}
    }
  }
}
```

## 使用示例

配置完成后，重启 Claude Desktop，您就可以使用自然语言与 DKNet 集群交互：

### 密钥生成示例

**用户**: "我需要创建一个新的 TSS 密钥，使用 2-of-3 门限方案。参与的节点是：12D3KooWGZCnvk6cX2UUhc1SHhkGvdfJNZicx4uXEb3niyHHN7ch, 12D3KooWEMke2yrVjg4nadKBBCZrWeZtxD4KucM4QzgH24JMo6JU, 12D3KooWT3TACsUvszChWcQwT7YpPa1udfwpb5k5qQ8zrBw4VqZ7"

**Claude 回应**: Claude 会调用 `tss_keygen` 工具并返回生成的密钥信息。

### 消息签名示例

**用户**: "请使用密钥 abc123 对消息 'Transaction: Alice sends 10 BTC to Bob' 进行签名，使用节点 node1 和 node2"

**Claude 回应**: Claude 会调用 `tss_sign` 工具并返回签名结果。

## 故障排除

### 连接问题

1. **无法连接到 DKNet 节点**
   - 检查节点地址和端口是否正确
   - 确认 DKNet 节点正在运行
   - 检查防火墙设置

2. **认证失败**
   - 验证 JWT 令牌是否正确且未过期
   - 确认节点 ID 是否匹配

3. **操作超时**
   - TSS 操作可能需要较长时间，特别是密钥生成
   - 检查网络延迟和所有参与节点的状态

### 日志调试

MCP 服务器会输出详细的日志信息，包括：
- 连接状态
- 操作进度
- 错误信息

## 安全注意事项

1. **私钥安全**: MCP 服务器本身不存储私钥，所有密钥材料都分布存储在 DKNet 节点中
2. **网络安全**: 确保 DKNet 节点间的通信是加密的
3. **访问控制**: 使用 JWT 令牌控制对 TSS 操作的访问
4. **操作审计**: 所有操作都有详细的日志记录

## API 参考

有关完整的 API 参考，请参阅：
- [DKNet API 文档](./server-usage.md)
- [TSS 服务文档](./validation-service.md)

## 常见问题

**Q: MCP 服务器是否需要成为 DKNet 网络的一部分？**
A: 不需要。MCP 服务器作为客户端连接到现有的 DKNet 集群，不参与共识或 P2P 通信。

**Q: 可以同时运行多个 MCP 服务器实例吗？**
A: 可以。每个 MCP 服务器实例可以连接到不同的 DKNet 节点或集群。

**Q: 支持哪些 LLM 客户端？**
A: 任何支持 MCP 协议的客户端都可以使用，包括 Claude Desktop、自定义 MCP 客户端等。

## 更多资源

- [Model Context Protocol 官方文档](https://modelcontextprotocol.io/)
- [DKNet 项目文档](../README.md)
- [TSS 算法说明](./SECURITY.md) 