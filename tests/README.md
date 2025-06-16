# DKNet TSS 测试套件

这个目录包含了 DKNet TSS (Threshold Signature Scheme) 项目的完整测试环境和相关工具。

## 目录结构

```text
tests/
├── README.md                    # 本文档
├── validation-service/          # 验证服务实现
│   ├── main.go                 # 验证服务源码
│   └── Dockerfile              # 验证服务容器配置
├── scripts/                    # 测试和管理脚本
│   ├── start-test-env.sh       # 统一环境管理脚本
│   ├── test-validation-docker.sh  # Docker 环境验证测试
│   └── test-validation-simple.sh  # 简单验证服务测试
├── docker/                     # Docker 配置和节点配置
│   ├── docker-compose.yml      # 测试环境编排配置
│   ├── cluster-info.txt        # 集群配置信息
│   ├── node1/                  # TSS 节点 1 配置
│   ├── node2/                  # TSS 节点 2 配置
│   └── node3/                  # TSS 节点 3 配置
└── docs/                       # 测试相关文档
    └── docker-deployment.md    # Docker 部署指南
```

## 安全特性

### Session Encryption (会话加密)

DKNet TSS 支持 **Session Encryption** 功能，为 TSS 消息提供额外的应用层加密保护：

- **加密算法**: AES-256-GCM（对称加密）
- **密钥派生**: SHA256(seed_key + session_id)
- **动态密钥**: 每个会话使用不同的派生密钥
- **透明集成**: 自动加密/解密所有 TSS 相关消息

#### 配置说明

Session Encryption 通过配置文件启用：

```yaml
security:
  session_encryption:
    enabled: true
    seed_key: "a9abe2b4fa5490ff80d59e01731fc2ce8d90429b2b824e0712264985f421210b"
```

**重要**:

- 所有参与方必须配置相同的 `seed_key`
- `seed_key` 应通过安全的带外通道分发
- 可使用 `./bin/dknet keyseed` 命令生成新的种子密钥

### 加密存储

DKNet TSS 使用 **AES-256-GCM** 对称加密来保护存储的 TSS 私钥：

- **加密算法**: AES-256-GCM（业界标准）
- **密钥派生**: PBKDF2（100,000轮迭代）
- **消息认证**: 内置防篡改保护
- **随机nonce**: 每次加密结果都不同

### 密码管理

服务器支持两种安全的密码输入方式：

1. **环境变量**: `TSS_ENCRYPTION_PASSWORD`（推荐用于生产和自动化）
2. **交互式输入**（推荐用于开发和手动操作）

**安全考虑**: 不支持密码文件以避免密码泄露风险。

## 快速开始

### 1. 启动测试环境（默认密码）

```bash
# 进入项目根目录
cd /path/to/DKNet

# 启动完整的测试环境（使用默认测试密码）
./tests/scripts/start-test-env.sh start
```

### 2. 使用自定义密码

```bash
# 方式1: 设置环境变量（推荐）
export TSS_ENCRYPTION_PASSWORD="MySecurePassword123!"
./tests/scripts/start-test-env.sh start

# 方式2: 使用脚本设置（当前会话有效）
./tests/scripts/start-test-env.sh set-password "MySecurePassword123!"
./tests/scripts/start-test-env.sh start
```

### 3. 运行测试

```bash
# 运行验证服务测试
./tests/scripts/start-test-env.sh test

# 运行 TSS 功能测试
./tests/scripts/start-test-env.sh test-tss
```

### 4. 查看服务状态

```bash
# 查看所有服务状态和密码信息
./tests/scripts/start-test-env.sh status
```

### 5. 停止测试环境

```bash
# 停止测试环境
./tests/scripts/start-test-env.sh stop

# 或者完全清理环境
./tests/scripts/start-test-env.sh cleanup
```

## 环境管理脚本

`tests/scripts/start-test-env.sh` 是统一的环境管理脚本，支持以下命令：

- `start` - 启动测试环境
- `stop` - 停止测试环境
- `test` - 运行验证测试
- `test-tss` - 运行 TSS 功能测试
- `status` - 查看环境状态
- `logs [service]` - 查看日志
- `set-password <password>` - 设置加密密码（当前会话）
- `cleanup` - 清理环境
- `help` - 显示帮助信息

### 密码管理命令

```bash
# 设置自定义密码（当前会话有效）
./tests/scripts/start-test-env.sh set-password "YourSecurePassword123!"

# 查看当前使用的密码
./tests/scripts/start-test-env.sh status
```

## 生产环境部署

### 密码配置

生产环境中，**强烈建议**使用以下方式配置密码：

#### 方式1: 环境变量（推荐）

```bash
export TSS_ENCRYPTION_PASSWORD="MySecurePassword123!"
./bin/dknet start --config config.yaml
```

#### 方式2: 交互式输入

```bash
# 直接运行，系统会提示输入密码
./bin/dknet start --config config.yaml
```

### 密码要求

为了确保安全性，密码应满足以下要求：

- **最少8个字符**
- **包含大写字母**
- **包含小写字母**
- **包含数字**
- **包含特殊字符** (!@#$%^&*等)

## 服务说明

### 验证服务 (Validation Service)

- **端口**: 8888
- **功能**: 为 TSS 签名请求提供外部验证
- **健康检查**: `http://localhost:8888/health`
- **验证端点**: `http://localhost:8888/validate`

### TSS 节点

- **节点 1**: <http://localhost:8081> (gRPC: 9095)
- **节点 2**: <http://localhost:8082> (gRPC: 9096)
- **节点 3**: <http://localhost:8083> (gRPC: 9097)

每个节点都配置了验证服务集成，在签名前会调用验证服务进行请求验证。

**重要**: 所有节点的 TSS 私钥都使用相同的密码进行加密存储。

## 验证规则

当前验证服务实现了以下验证规则：

1. **消息长度检查**: 消息不能为空且不能超过 1000 字符
2. **内容过滤**: 拒绝包含恶意关键词的消息 ("malicious", "hack", "exploit")
3. **密钥白名单**: 只允许特定的公钥进行签名
4. **时间戳验证**: 检查请求时间戳的有效性
5. **参与者数量**: 确保有足够的参与者进行签名

## 测试脚本

### test-validation-docker.sh

完整的 Docker 环境测试，包括：

- 密钥生成测试
- 有效签名请求测试
- 无效签名请求测试（验证拒绝）

### test-validation-simple.sh

简单的验证服务功能测试，直接测试验证 API 端点。

## 配置说明

### Docker 配置

- `docker/docker-compose.yml`: 测试环境的 Docker Compose 配置
- `docker/node*/config.yaml`: 各个 TSS 节点的配置文件

### 环境变量

测试环境支持以下环境变量：

- `TSS_ENCRYPTION_PASSWORD`: 设置加密密码
- `TSS_CONFIG_FILE`: TSS 节点配置文件路径

### 验证服务配置

验证服务的配置通过环境变量或配置文件进行设置，包括：

- 验证规则开关
- 白名单配置
- 日志级别等

## 故障排除

### 常见问题

1. **密码相关错误**

   ```bash
   # 检查当前密码设置
   ./tests/scripts/start-test-env.sh status
   
   # 重新设置密码
   ./tests/scripts/start-test-env.sh set-password "NewPassword123!"
   ```

2. **服务启动失败**

   ```bash
   # 查看服务日志
   ./tests/scripts/start-test-env.sh logs
   
   # 查看特定服务日志
   ./tests/scripts/start-test-env.sh logs tss-node1
   ```

3. **密钥解密失败**
   - 确保所有节点使用相同的密码
   - 检查环境变量设置
   - 查看节点日志确认错误详情

4. **端口冲突**
   - 确保端口 8081-8083, 8888, 9095-9097 没有被其他服务占用

5. **Docker 相关问题**

   ```bash
   # 清理环境重新开始
   ./tests/scripts/start-test-env.sh cleanup
   ./tests/scripts/start-test-env.sh start
   ```

### 调试模式

可以通过修改 Docker Compose 配置启用调试模式：

```yaml
environment:
  - LOG_LEVEL=debug
  - TSS_DEBUG=true
```

## 开发指南

### 添加新的验证规则

1. 修改 `validation-service/main.go` 中的验证逻辑
2. 重新构建验证服务镜像
3. 更新测试脚本验证新规则

### 添加新的测试用例

1. 在 `scripts/` 目录下创建新的测试脚本
2. 更新 `start-test-env.sh` 脚本添加新的测试命令
3. 更新文档说明新的测试功能

## 安全注意事项

### 测试环境

- 默认密码为 `TestPassword123!` - **仅用于测试**
- 所有节点共享相同密码以简化测试
- 密码通过环境变量传递，避免文件泄露

### 生产环境

- **必须使用强密码**并定期更换
- **不要**将密码存储在代码仓库中
- 使用环境变量或安全的密钥管理系统
- 定期备份加密的密钥数据
- 监控密钥访问日志

### 密钥恢复

- 如果忘记密码，**无法恢复**已加密的 TSS 私钥
- 建议在安全的地方备份密码
- 考虑使用密钥托管或多重备份策略

## 注意事项

- 这是测试环境，不适用于生产环境
- 验证服务的密钥和配置仅用于测试
- 定期清理 Docker 资源避免占用过多磁盘空间
- 测试完成后建议停止环境释放系统资源
- **生产环境中必须使用安全的密码管理策略**
- **永远不要在文件中存储明文密码**
