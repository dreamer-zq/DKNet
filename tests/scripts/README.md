# DKNet TSS Test Scripts

本目录包含DKNet TSS系统的测试脚本，专注于正向测试，用于快速验证系统功能的正确性。

## 文件结构

- `test-all.sh` - **统一测试入口**，一次性运行keygen、resharing和signing测试（推荐使用）
- `test-common.sh` - 公共函数库，包含所有测试脚本需要的共用函数
- `test-keygen.sh` - Keygen操作测试脚本（仅密钥生成）
- `test-resharing.sh` - Resharing操作测试脚本（密钥重分享）
- `test-signing.sh` - Signing操作测试脚本

## 快速开始

### 推荐方式：使用统一测试脚本

```bash
# 运行完整测试套件（keygen + resharing + signing，推荐）
./tests/scripts/test-all.sh

# 或者明确指定
./tests/scripts/test-all.sh all

# 仅运行keygen测试
./tests/scripts/test-all.sh keygen

# 仅运行resharing测试（需要先运行keygen）
./tests/scripts/test-all.sh resharing

# 仅运行signing测试（需要先运行keygen和resharing）
./tests/scripts/test-all.sh signing
```

### 分别运行测试

#### 1. 运行Keygen测试

```bash
# 启动测试环境并运行所有keygen测试
./tests/scripts/test-keygen.sh start

# 或者只运行测试（需要环境已启动）
./tests/scripts/test-keygen.sh test
```

#### 2. 运行Resharing测试

```bash
# 运行所有resharing测试（需要先运行keygen测试）
./tests/scripts/test-resharing.sh test

# 快速resharing测试（使用指定的key_id和阈值）
./tests/scripts/test-resharing.sh quick 0x1234567890abcdef 1 2
```

#### 3. 运行Signing测试

```bash
# 运行所有signing测试（需要先运行keygen测试）
./tests/scripts/test-signing.sh test

# 快速签名测试（使用指定的key_id）
./tests/scripts/test-signing.sh quick 0x1234567890abcdef

# 快速签名测试（使用自定义消息）
./tests/scripts/test-signing.sh quick 0x1234567890abcdef "My custom message"
```

## 统一测试脚本特点

`test-all.sh` 提供了最便利的测试体验：

### 功能特点

- **一键运行**：自动启动环境，按顺序执行所有测试
- **完整覆盖**：包含3个keygen测试 + 6个signing测试 + 2个验证测试，共13个测试（resharing测试暂时跳过）
- **智能管理**：自动传递密钥ID，无需手动干预
- **详细输出**：显示所有Operation ID、JWT Token、Key ID和Signature
- **结果汇总**：提供完整的测试结果摘要

### 测试流程

1. 启动Docker测试环境（3个TSS节点 + 验证服务）
2. 生成JWT认证令牌
3. 执行Keygen测试（生成并保存密钥ID）
4. 跳过Resharing测试（由于TSS库v2.0.2的已知问题）
5. 执行Signing测试（使用生成的密钥）
6. 显示完整的测试结果摘要

## 测试覆盖范围

### Keygen测试包括

1. 2-of-3阈值密钥生成
2. 3-of-3阈值密钥生成  
3. 2-of-2阈值密钥生成

### Resharing测试（当前跳过）

**注意**: Resharing测试当前被跳过，因为TSS库v2.0.2存在已知的resharing操作问题。
要启用resharing测试，请在`test-all.sh`中设置`ENABLE_RESHARING_TESTS="true"`。

计划包含的测试：

1. 从2-of-3重分享到3-of-3
2. 从3-of-3重分享到2-of-3
3. 从2-of-3重分享到2-of-2（参与者变更）
4. 从2-of-2重分享到2-of-3（参与者扩展）

### Signing测试包括

1. 使用2-of-3密钥进行签名（不同参与者组合）
2. 使用3-of-3密钥进行签名
3. 使用2-of-2密钥进行签名
4. 不同消息类型的签名测试（文本、交易、JSON等）
5. 重分享密钥签名测试（当前跳过）

## 输出信息

每个测试都会输出详细信息，包括：

- **Operation ID** - 每个操作的唯一标识符
- **JWT Token** - 用于API认证的令牌
- **Key ID** - 生成的密钥标识符
- **Signature** - 生成的签名结果
- **请求数据** - 完整的API请求数据（JSON格式）

## 环境管理

### 使用统一测试脚本管理环境（推荐）

```bash
# 检查环境状态
./tests/scripts/test-all.sh status

# 查看所有服务日志
./tests/scripts/test-all.sh logs

# 查看特定服务日志
./tests/scripts/test-all.sh logs tss-node1
./tests/scripts/test-all.sh logs validation-service

# 停止环境
./tests/scripts/test-all.sh stop

# 清理环境
./tests/scripts/test-all.sh cleanup
```

### 使用单独脚本管理环境

```bash
# 启动测试环境
./tests/scripts/test-keygen.sh start

# 检查环境状态
./tests/scripts/test-keygen.sh status
# 或
./tests/scripts/test-signing.sh status

# 查看日志
./tests/scripts/test-keygen.sh logs
./tests/scripts/test-keygen.sh logs tss-node1

# 停止环境
./tests/scripts/test-keygen.sh stop

# 清理环境
./tests/scripts/test-keygen.sh cleanup
```

## 测试环境信息

- **验证服务**: <http://localhost:8888>
- **TSS节点1**: <http://localhost:8081>  
- **TSS节点2**: <http://localhost:8082>
- **TSS节点3**: <http://localhost:8083>
- **加密密码**: TestPassword123! (默认)
- **JWT密钥**: dknet-test-jwt-secret-key-2024
- **JWT签发者**: dknet-test

## 节点ID

- **节点1**: 12D3KooWGZCnvk6cX2UUhc1SHhkGvdfJNZicx4uXEb3niyHHN7ch
- **节点2**: 12D3KooWEMke2yrVjg4nadKBBCZrWeZtxD4KucM4QzgH24JMo6JU  
- **节点3**: 12D3KooWT3TACsUvszChWcQwT7YpPa1udfwpb5k5qQ8zrBw4VqZ7

## 依赖要求

- Docker和docker-compose
- Go（用于JWT令牌生成）
- curl和jq（用于API测试）

## 故障排除

1. **环境启动失败**: 检查Docker是否运行，端口是否被占用
2. **JWT令牌生成失败**: 确保Go已安装且可以访问网络
3. **Key ID未找到**: 确保先运行keygen测试再运行signing测试
4. **操作超时**: 检查节点健康状态和网络连接

## 手动验证

所有测试输出的Operation ID和JWT Token都可以用于手动验证：

```bash
# 使用JWT Token查询操作状态
curl -H "Authorization: Bearer <JWT_TOKEN>" \
     http://localhost:8081/api/v1/operations/<OPERATION_ID>

# 直接调用验证服务
curl -X POST http://localhost:8888/validate \
     -H "Content-Type: application/json" \
     -d '{"message":"SGVsbG8gV29ybGQ=","key_id":"0x123","participants":["node1"],"node_id":"node1","timestamp":1234567890}'
```
