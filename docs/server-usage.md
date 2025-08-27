# DKNet 服务器命令行使用指南

本文档详细说明了 DKNet TSS 服务器的命令行工具使用方法。

## 安装

```bash
# 构建服务器
make build

# 构建后的二进制文件位于
./bin/dknet
```

## 全局配置

### 全局参数

| 参数        | 简写 | 默认值 | 说明         |
| ----------- | ---- | ------ | ------------ |
| `--help`    | `-h` |        | 显示命令帮助 |
| `--version` |      |        | 显示版本信息 |

## 子命令

### start - 启动服务器

启动 DKNet TSS 服务器。

#### 命令说明

启动 DKNet TSS 服务器，提供 HTTP 和 gRPC API 服务。服务器需要指定节点目录，该目录包含配置文件、节点密钥和数据存储。

#### 使用示例

```bash
# 启动服务器
./bin/dknet start --node-dir ./node1

# 使用不同的节点目录
./bin/dknet start --node-dir /opt/dknet/config

# 查看帮助
./bin/dknet start --help
```

#### 参数说明

| 参数         | 简写 | 类型   | 必需 | 说明                                           |
| ------------ | ---- | ------ | ---- | ---------------------------------------------- |
| `--node-dir` |      | string | ✓    | 节点目录，包含 config.yaml, node_key, and data |

#### 环境变量

| 环境变量                  | 说明                        | 默认值 |
| ------------------------- | --------------------------- | ------ |
| `TSS_ENCRYPTION_PASSWORD` | TSS 私钥加密密码 (required) |        |

#### 返回结果

服务器启动后会显示：

- 服务器启动状态
- HTTP API 地址（默认：<http://localhost:8080>）
- gRPC API 地址（默认：<localhost:9090>）
- 日志输出

### init-cluster - 初始化集群

为 TSS 集群生成配置文件和 P2P 密钥。

#### 命令说明

生成多节点 TSS 集群的完整配置，包括每个节点的 P2P 私钥、配置文件和引导节点配置。主要用于测试环境，生产环境中每个组织应独立生成密钥。

#### 使用示例

```bash
# 初始化 3 节点集群
./bin/dknet init-cluster --nodes 3 --output ./tss-cluster

# 生成 Docker 集群配置
./bin/dknet init-cluster --nodes 5 --docker --output ./docker-cluster

# 使用默认设置（3 节点）
./bin/dknet init-cluster
```

#### 参数说明

| 参数       | 简写 | 类型   | 必需 | 默认值        | 说明             |
| ---------- | ---- | ------ | ---- | ------------- | ---------------- |
| `--nodes`  | `-n` | int    |      | `3`           | 集群节点数       |
| `--output` | `-o` | string |      | `tss-cluster` | 输出目录         |
| `--docker` | `-d` | bool   |      | `false`       | 生成 Docker 配置 |

#### 返回结果

生成的文件结构：

```text
tss-cluster/
├── node1/
│   ├── config.yaml
│   ├── node_key
│   └── node-info.txt
├── node2/
│   ├── config.yaml
│   ├── node_key
│   └── node-info.txt
├── node3/
│   ├── config.yaml
│   ├── node_key
│   └── node-info.txt
└── docker-compose.yaml  # 仅在 --docker 模式下生成
```

### init-node - 初始化单个节点

初始化单个节点的配置和密钥文件。

#### 命令说明

为单个节点生成配置文件和 P2P 密钥。适用于独立部署或手动配置集群的场景。

#### 使用示例

```bash
# 基本节点初始化
./bin/dknet init-node --moniker "my-node" --output-dir ./my-node

# 指定网络参数
./bin/dknet init-node \
  --moniker "node1" \
  --output-dir ./node1 \
  --http-port 8080 \
  --grpc-port 9090 \
  --p2p-port 4001 \
  --listen-addr 0.0.0.0

# 配置引导节点
./bin/dknet init-node \
  --moniker "node2" \
  --output-dir ./node2 \
  --bootstrap-peers "/ip4/192.168.1.100/tcp/4001/p2p/12D3KooW..."
```

#### 参数说明

| 参数                | 简写 | 类型     | 必需 | 默认值    | 说明                    |
| ------------------- | ---- | -------- | ---- | --------- | ----------------------- |
| `--moniker`         |      | string   |      | `node1`   | 人类可读的节点名称      |
| `--output-dir`      | `-o` | string   |      | `./`      | 输出目录 for node files |
| `--bootstrap-peers` | `-b` | []string |      | `[]`      | 引导节点地址            |
| `--http-port`       |      | int      |      | `8080`    | HTTP API 端口           |
| `--grpc-port`       |      | int      |      | `9090`    | gRPC API 端口           |
| `--p2p-port`        |      | int      |      | `4001`    | P2P 监听端口            |
| `--listen-addr`     |      | string   |      | `0.0.0.0` | 监听地址                |

#### 返回结果

生成的文件：

- `config/config.yaml` - 节点配置文件
- `config/node_key` - P2P 私钥文件
- `config/node-info.txt` - 节点信息文件，包含节点 ID、Peer ID、Multiaddr、网络配置和引导节点。

### show-node - 显示节点信息

显示节点信息，包括 Multiaddr 和网络配置。

#### 命令说明

从生成的配置文件中读取并显示节点信息，包括节点 ID、Peer ID、Multiaddr、网络配置和引导节点。用于部署和与其他节点共享节点信息。

#### 使用示例

```bash
# 显示节点信息
./bin/dknet show-node --node-dir ./node1

# JSON 格式输出
./bin/dknet show-node --node-dir ./node1 --json

# 仅显示 multiaddr
./bin/dknet show-node --node-dir ./node1 --multiaddr-only
```

#### 参数说明

| 参数               | 简写 | 类型   | 必需 | 说明                                   |
| ------------------ | ---- | ------ | ---- | -------------------------------------- |
| `--node-dir`       |      | string | ✓    | 节点目录，包含 config.yaml 和 node_key |
| `--json`           | `-j` | bool   |      | 以 JSON 格式输出                       |
| `--multiaddr-only` | `-m` | bool   |      | 仅输出 Multiaddr                       |

#### 返回结果

**文本格式输出**：

```text
TSS Node Information
====================

Node Details:
  Peer ID:    12D3KooWABC123...
  Moniker:    my-node
  Multiaddr:  /ip4/192.168.1.100/tcp/4001/p2p/12D3KooWABC123...

Network Configuration:
  Listen Address: 0.0.0.0:4001
  HTTP Port:      8080
  gRPC Port:      9090

Bootstrap Peers:
  1. /ip4/192.168.1.101/tcp/4001/p2p/12D3KooWDEF456...
```

**JSON 格式输出**：

```json
{
  "peer_id": "12D3KooWABC123...",
  "moniker": "my-node",
  "multiaddr": "/ip4/192.168.1.100/tcp/4001/p2p/12D3KooWABC123...",
  "listen_addr": "0.0.0.0",
  "port": 4001,
  "bootstrap_peers": [
    "/ip4/192.168.1.101/tcp/4001/p2p/12D3KooWDEF456..."
  ],
  "http_port": 8080,
  "grpc_port": 9090
}
```

### generate-token - 生成认证令牌

生成用于 API 认证的 JWT 令牌。

#### 命令说明

使用服务器的 JWT 配置生成认证令牌，客户端可以使用该令牌进行 API 认证。需要服务器配置中启用 JWT 认证。

#### 使用示例

```bash
# 生成默认令牌
./bin/dknet generate-token --node-dir ./node1

# 指定用户和角色
./bin/dknet generate-token \
  --node-dir ./node1 \
  --user "admin" \
  --roles "admin,operator"

# 设置过期时间
./bin/dknet generate-token \
  --node-dir ./node1 \
  --expires 48

# 生成永久令牌
./bin/dknet generate-token \
  --node-dir ./node1 \
  --expires 0

# JSON 格式输出
./bin/dknet generate-token \
  --node-dir ./node1 \
  --output json
```

#### 参数说明

| 参数         | 简写 | 类型     | 必需 | 默认值             | 说明                                           |
| ------------ | ---- | -------- | ---- | ------------------ | ---------------------------------------------- |
| `--node-dir` |      | string   | ✓    |                    | 节点目录，包含 config.yaml, node_key, and data |
| `--output`   | `-o` | string   |      | `text`             | 输出格式 (text\|json)                          |
| `--user`     | `-u` | string   |      | `admin-user`       | 用户 ID 用于令牌                               |
| `--roles`    | `-r` | []string |      | `[admin,operator]` | 角色用于令牌                                   |
| `--expires`  | `-e` | int      |      | `24`               | 令牌过期时间 (使用 0 表示不设置过期)           |

#### 返回结果

**文本格式输出**：

```text
🔑 JWT Token Generated Successfully
Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
User ID: admin-user
Issuer: dknet-test
Roles: [admin operator]
Expires: 2024-01-02T12:00:00Z (24h)

Usage with dknet-cli:
  dknet-cli --token="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." <command>

Usage with curl:
  curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." <api-endpoint>
```

**JSON 格式输出**：

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user_id": "admin-user",
  "issuer": "dknet-test",
  "roles": ["admin", "operator"],
  "expires_in": "24h",
  "expires_at": "2024-01-02T12:00:00Z"
}
```

### version - 版本信息

显示 DKNet 服务器的版本信息。

#### 命令说明

显示当前 DKNet 服务器二进制文件的版本信息，包括版本号、构建时间等详细信息。

#### 使用示例

```bash
# 显示版本信息
./bin/dknet version
```

#### 参数说明

此命令无需额外参数。

#### 返回结果

```text
DKNet TSS Server
Version: v1.0.0
Build Date: 2024-01-01T12:00:00Z
Git Commit: abc123def456
Go Version: go1.21.0
```
