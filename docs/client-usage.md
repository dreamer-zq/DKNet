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

## 基本用法

### 连接配置

```bash
# 使用 HTTP 连接（默认）
./bin/dknet-cli --server localhost:8080 <command>

# 使用 gRPC 连接
./bin/dknet-cli --grpc --server localhost:9001 <command>
```

### 密钥生成

```bash
# 基本密钥生成
./bin/dknet-cli keygen \
  --threshold 2 \
  --parties 3 \
  --participants node1,node2,node3

# 带超时的密钥生成
./bin/dknet-cli keygen \
  --threshold 2 \
  --parties 3 \
  --participants node1,node2,node3 \
  --timeout 60s
```

### 数字签名

```bash
# 对消息进行签名
./bin/dknet-cli sign \
  --key-id <key-id> \
  --message "Hello, World!" \
  --participants node1,node2

./bin/dknet-cli sign \
  --key-id <key-id> \
  --message-file ./message.txt \
  --participants node1,node2,node3
```

### 密钥重新分享

```bash
# 重新分享密钥（更改阈值或参与方）
./bin/dknet-cli reshare \
  --key-id <key-id> \
  --new-threshold 3 \
  --new-parties 5 \
  --old-participants node1,node2,node3 \
  --new-participants node1,node2,node3,node4,node5
```

### 操作管理

```bash
# 查询操作状态
./bin/dknet-cli operation <operation-id>

# 示例
./bin/dknet-cli operation keygen-abc123
```

## 完整示例

### 端到端工作流

```bash
# 1. 生成密钥
./bin/dknet-cli keygen \
  --threshold 2 \
  --parties 3 \
  --participants node1,node2,node3

# 2. 等待密钥生成完成，获取 key-id
./bin/dknet-cli operation keygen-abc123

# 3. 使用生成的密钥进行签名
./bin/dknet-cli sign \
  --key-id <generated-key-id> \
  --message "Important transaction data" \
  --participants node1,node2

# 4. 查看签名操作状态
./bin/dknet-cli operation sign-def456
```

### 使用环境变量

```bash
# 设置默认服务器
export TSS_SERVER=localhost:8080

# 使用 gRPC
export TSS_USE_GRPC=true

# 执行命令
./bin/dknet-cli --grpc --server $TSS_SERVER keygen \
  --threshold 2 \
  --parties 3 \
  --participants node1,node2,node3
```
