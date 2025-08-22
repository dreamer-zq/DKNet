# DKNet CLI 使用指南

DKNet CLI 是 DKNet TSS 系统的命令行客户端工具，提供了与 TSS 服务器交互的完整功能。

## 安装

```bash
# 构建客户端
make build-client

# 或者从发布版本下载
wget https://github.com/dreamer-zq/DKNet/releases/latest/download/dknet-cli-linux-amd64
chmod +x dknet-cli-linux-amd64
mv dknet-cli-linux-amd64 /usr/local/bin/dknet-cli
```

## 全局配置

### 连接配置

```bash
# 使用 HTTP 连接（默认）
./bin/dknet-cli --server localhost:8080 <command>

# 使用 gRPC 连接
./bin/dknet-cli --grpc --server localhost:9001 <command>
```

### 全局参数

| 参数        | 简写 | 默认值           | 说明                         |
| ----------- | ---- | ---------------- | ---------------------------- |
| `--server`  | `-s` | `localhost:8080` | Server address (host:port)   |
| `--grpc`    | `-g` | `false`          | Use gRPC instead of HTTP     |
| `--timeout` | `-t` | `30s`            | Request timeout              |
| `--output`  | `-o` | `text`           | Output format (text\|json)   |
| `--token`   |      |                  | JWT token for authentication |

### 认证配置

JWT 认证是可选的。当服务器启用认证时，可以通过以下方式提供 token：

```bash
# 1. 命令行参数
./bin/dknet-cli --token="your-jwt-token" <command>

# 2. 环境变量
export DKNET_JWT_TOKEN="your-jwt-token"
./bin/dknet-cli <command>

# 3. 生成 token（在服务器节点上执行）
dknet generate-token --config=/path/to/config.yaml
```

## 子命令

### keygen - 密钥生成

启动新的阈值密钥生成操作。

#### 命令说明

密钥生成操作会在指定的参与方之间创建一个共享密钥，使用 (t+1)-of-n 阈值方案，其中 t 是故障容忍阈值，n 是参与方总数。

#### 使用示例

```bash
# 基本密钥生成
./bin/dknet-cli keygen \
  --threshold 2 \
  --participants nodeID1,nodeID2,nodeID3

# 使用 gRPC 连接
./bin/dknet-cli --grpc --server localhost:9001 keygen \
  --threshold 2 \
  --participants nodeID1,nodeID2,nodeID3

# 使用 JSON 输出格式
./bin/dknet-cli --output json keygen \
  --threshold 2 \
  --participants nodeID1,nodeID2,nodeID3

# 带认证的密钥生成
./bin/dknet-cli --token="your-jwt-token" keygen \
  --threshold 2 \
  --participants nodeID1,nodeID2,nodeID3
```

#### 参数说明

| 参数             | 简写 | 类型     | 必需 | 说明                                                                                                                    |
| ---------------- | ---- | -------- | ---- | ----------------------------------------------------------------------------------------------------------------------- |
| `--threshold`    | `-r` | int      | ✓    | Fault tolerance threshold (t in (t+1)-of-n scheme). Max number of parties that can fail. Minimum signers required = t+1 |
| `--participants` | `-P` | []string | ✓    | List of participant IDs                                                                                                 |

#### 返回结果

- **Operation ID**: 用于后续查询操作状态
- **Status**: 操作状态（PENDING/IN_PROGRESS/COMPLETED/FAILED）
- **Created At**: 操作创建时间

### sign - 数字签名

使用指定密钥对消息进行数字签名。

#### 命令说明

签名操作需要至少 (t+1) 个参与方协作完成，其中 t 是密钥的阈值。可以对文本消息或十六进制编码的消息进行签名。

#### 使用示例

```bash
# 对文本消息进行签名
./bin/dknet-cli sign \
  --key-id <key-id> \
  --message "Hello, World!" \
  --participants nodeID1,nodeID2

# 对十六进制编码的消息进行签名
./bin/dknet-cli sign \
  --key-id <key-id> \
  --message "48656c6c6f2c20576f726c6421" \
  --hex \
  --participants nodeID1,nodeID2,nodeID3

# 使用 JSON 输出格式
./bin/dknet-cli --output json sign \
  --key-id <key-id> \
  --message "Important transaction data" \
  --participants nodeID1,nodeID2
```

#### 参数说明

| 参数             | 简写 | 类型     | 必需 | 说明                        |
| ---------------- | ---- | -------- | ---- | --------------------------- |
| `--message`      | `-m` | string   | ✓    | Message to sign             |
| `--key-id`       | `-k` | string   | ✓    | Key ID to use for signing   |
| `--hex`          |      | bool     |      | Treat message as hex string |
| `--participants` | `-P` | []string | ✓    | List of participant IDs     |

#### 返回结果

- **Operation ID**: 用于后续查询操作状态
- **Status**: 操作状态
- **Created At**: 操作创建时间

### reshare - 密钥重新分享

重新分享现有密钥以更改阈值或参与方。

#### 命令说明

密钥重新分享操作允许修改现有密钥的阈值设置或参与方列表，生成新的密钥份额但保持相同的公钥。

#### 使用示例

```bash
# 更改阈值和参与方
./bin/dknet-cli reshare \
  --key-id <key-id> \
  --new-threshold 3 \
  --new-participants nodeID1,nodeID2,nodeID3,node4,node5

# 仅更改参与方（保持阈值不变）
./bin/dknet-cli reshare \
  --key-id <key-id> \
  --new-threshold 2 \
  --new-participants nodeID1,nodeID3,node4

# 使用 JSON 输出格式
./bin/dknet-cli --output json reshare \
  --key-id <key-id> \
  --new-threshold 2 \
  --new-participants nodeID1,nodeID2,nodeID3
```

#### 参数说明

| 参数                 | 简写 | 类型     | 必需 | 说明                                                                                                                        |
| -------------------- | ---- | -------- | ---- | --------------------------------------------------------------------------------------------------------------------------- |
| `--key-id`           | `-k` | string   | ✓    | Key ID to reshare                                                                                                           |
| `--new-threshold`    |      | int      | ✓    | New fault tolerance threshold (t in (t+1)-of-n scheme). Max number of parties that can fail. Minimum signers required = t+1 |
| `--new-participants` |      | []string | ✓    | List of new participant IDs                                                                                                 |

#### 返回结果

- **Operation ID**: 用于后续查询操作状态
- **Status**: 操作状态
- **Created At**: 操作创建时间

### operation - 查询操作状态

查询指定操作的状态和结果。

#### 命令说明

通过操作 ID 查询 keygen、sign 或 reshare 操作的详细状态信息，包括操作结果、错误信息和原始请求参数。

#### 使用示例

```bash
# 查询操作状态
./bin/dknet-cli operation keygen-abc123

# 使用 JSON 输出格式
./bin/dknet-cli --output json operation sign-def456

# 查询重新分享操作
./bin/dknet-cli operation reshare-ghi789
```

#### 参数说明

| 参数             | 类型   | 必需 | 说明                  |
| ---------------- | ------ | ---- | --------------------- |
| `<operation-id>` | string | ✓    | Operation ID to query |

#### 返回结果

**基本信息**:

- **Operation ID**: 操作唯一标识符
- **Type**: 操作类型（KEYGEN/SIGNING/RESHARING）
- **Session ID**: 会话标识符
- **Status**: 操作状态（PENDING/IN_PROGRESS/COMPLETED/FAILED/CANCELED）
- **Participants**: 参与方列表
- **Created At**: 创建时间
- **Completed At**: 完成时间（如果已完成）

**结果信息**（操作完成时）:

对于 **keygen** 操作：

- **Public Key**: 生成的公钥
- **Key ID**: 密钥唯一标识符

对于 **sign** 操作：

- **Signature**: 完整的签名
- **R**: 签名的 R 分量
- **S**: 签名的 S 分量

对于 **reshare** 操作：

- **New Public Key**: 新的公钥（应该与原公钥相同）
- **New Key ID**: 新的密钥标识符

**原始请求信息**:

- 显示创建操作时使用的原始参数

### key-metadata - 获取密钥元数据

获取指定密钥的元数据信息。

#### 命令说明

查询密钥的基本元数据，包括助记符、阈值设置和参与方列表。

#### 使用示例

```bash
# 获取密钥元数据
./bin/dknet-cli key-metadata <key-id>

# 使用 JSON 输出格式
./bin/dknet-cli --output json key-metadata <key-id>

# 使用 gRPC 连接
./bin/dknet-cli --grpc key-metadata <key-id>
```

#### 参数说明

| 参数       | 类型   | 必需 | 说明            |
| ---------- | ------ | ---- | --------------- |
| `<key-id>` | string | ✓    | Key ID to query |

#### 返回结果

- **Moniker**: 密钥助记符/别名
- **Threshold**: 密钥的阈值设置
- **Participants**: 参与方列表

### version - 版本信息

显示 DKNet CLI 的版本信息。

#### 命令说明

显示当前 DKNet CLI 二进制文件的版本信息，包括版本号、构建时间等详细信息。

#### 使用示例

```bash
# 显示版本信息
./bin/dknet-cli version
```

#### 参数说明

此命令无需额外参数。

#### 返回结果

- **Version**: 版本号
- **Build Info**: 构建信息（如果可用）

## 完整示例

### 端到端工作流

```bash
# 1. 生成密钥
./bin/dknet-cli keygen \
  --threshold 2 \
  --participants nodeID1,nodeID2,nodeID3

# 输出示例:
# ✅ Operation started successfully
# Operation ID: keygen-abc123
# Status: PENDING
# Created At: 2024-01-01T12:00:00Z

# 2. 等待密钥生成完成，获取 key-id
./bin/dknet-cli operation keygen-abc123

# 输出示例:
# 📋 Operation Details
# Operation ID: keygen-abc123
# Type: KEYGEN
# Status: COMPLETED
# 🎯 Result:
#   Public Key: 04a1b2c3d4e5f6...
#   Key ID: key-xyz789

# 3. 使用生成的密钥进行签名
./bin/dknet-cli sign \
  --key-id key-xyz789 \
  --message "Important transaction data" \
  --participants nodeID1,nodeID2

# 4. 查看签名操作状态
./bin/dknet-cli operation sign-def456

# 5. 获取密钥元数据
./bin/dknet-cli key-metadata key-xyz789
```

### 使用环境变量

```bash
# 设置环境变量
export DKNET_JWT_TOKEN="your-jwt-token"

# 使用不同的输出格式
./bin/dknet-cli --output json keygen \
  --threshold 2 \
  --participants nodeID1,nodeID2,nodeID3

# 使用 gRPC 连接
./bin/dknet-cli --grpc --server localhost:9001 sign \
  --key-id key-xyz789 \
  --message "Hello, World!" \
  --participants nodeID1,nodeID2
```

### 错误处理

```bash
# 当操作失败时，查看详细错误信息
./bin/dknet-cli operation failed-operation-id

# 输出示例:
# 📋 Operation Details
# Operation ID: failed-operation-id
# Status: FAILED
# ❌ Error: insufficient participants for threshold requirement
```
