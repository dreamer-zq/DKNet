# TSS Client 使用指南

TSS Client 是与 DKNet 交互的命令行工具，支持 HTTP 和 gRPC 两种协议。

## 安装和构建

```bash
# 构建客户端
make build-client

# 或者构建所有组件
make build
```

## 基本用法

### 全局选项

- `--server, -s`: 服务器地址 (默认: `localhost:8080`)
- `--grpc, -g`: 使用 gRPC 而不是 HTTP
- `--timeout, -t`: 请求超时时间 (默认: `30s`)

### 协议选择

**HTTP 调用 (默认)**:

```bash
./bin/tss-client --server localhost:8080 <command>
```

**gRPC 调用**:

```bash
./bin/tss-client --grpc --server localhost:9001 <command>
```

## 命令详解

### 1. 密钥生成 (Keygen)

启动新的阈值密钥生成操作：

```bash
./bin/tss-client keygen \
  --threshold 2 \
  --parties 3 \
  --participants peer1,peer2,peer3
```

**参数说明**:

- `--threshold, -r`: 签名所需的最小参与方数量
- `--parties, -p`: 总参与方数量
- `--participants, -P`: 参与方 ID 列表

**示例输出**:

```text
✅ Keygen operation started successfully
Operation ID: keygen-abc123
Status: OPERATION_STATUS_PENDING
Created At: 2024-06-11T13:45:30Z
```

### 2. 签名 (Sign)

使用指定密钥对消息进行签名：

```bash
# 签名文本消息
./bin/tss-client sign \
  --message "Hello, World!" \
  --key-id "key-abc123" \
  --participants peer1,peer2

# 签名十六进制消息
./bin/tss-client sign \
  --message "48656c6c6f2c20576f726c6421" \
  --hex \
  --key-id "key-abc123" \
  --participants peer1,peer2
```

**参数说明**:

- `--message, -m`: 要签名的消息
- `--key-id, -k`: 用于签名的密钥 ID
- `--participants, -P`: 参与签名的参与方 ID 列表
- `--hex`: 将消息视为十六进制字符串

**示例输出**:

```text
✅ Signing operation started successfully
Operation ID: sign-def456
Status: OPERATION_STATUS_PENDING
Created At: 2024-06-11T13:45:35Z
```

### 3. 密钥重新分享 (Reshare)

更改密钥的阈值或参与方：

```bash
./bin/tss-client reshare \
  --key-id "key-abc123" \
  --new-threshold 3 \
  --new-parties 5 \
  --old-participants peer1,peer2,peer3 \
  --new-participants peer1,peer2,peer3,peer4,peer5
```

**参数说明**:

- `--key-id, -k`: 要重新分享的密钥 ID
- `--new-threshold`: 新的阈值
- `--new-parties`: 新的总参与方数量
- `--old-participants`: 原有参与方 ID 列表
- `--new-participants`: 新的参与方 ID 列表

### 4. 查询操作状态

获取操作的详细状态和结果：

```bash
./bin/tss-client operation <operation-id>
```

**示例**:

```bash
./bin/tss-client operation keygen-abc123
```

**示例输出**:

```text
📋 Operation Details
Operation ID: keygen-abc123
Type: OPERATION_TYPE_KEYGEN
Session ID: session-xyz789
Status: OPERATION_STATUS_COMPLETED
Participants: peer1, peer2, peer3
Created At: 2024-06-11T13:45:30Z
Completed At: 2024-06-11T13:46:15Z
🎯 Result:
  Public Key: 04a1b2c3d4e5f6...
  Key ID: key-generated-123
```

## 完整使用示例

### 场景：完整的密钥生成和签名流程

```bash

# 1. 启动密钥生成
./bin/tss-client keygen \
  --threshold 2 \
  --parties 3 \
  --participants alice,bob,charlie

# 输出：Operation ID: keygen-abc123

# 2. 查询密钥生成状态
./bin/tss-client operation keygen-abc123

# 3. 使用生成的密钥进行签名
./bin/tss-client sign \
  --message "Important transaction data" \
  --key-id "key-generated-123" \
  --participants alice,bob

# 输出：Operation ID: sign-def456

# 4. 查询签名结果
./bin/tss-client operation sign-def456
```

### 场景：使用 gRPC 协议

```bash
# 使用 gRPC 进行所有操作（假设服务器在 9001 端口）
export TSS_SERVER="localhost:9001"

# 密钥生成
./bin/tss-client --grpc --server $TSS_SERVER keygen \
  --threshold 2 --parties 3 --participants alice,bob,charlie

```
