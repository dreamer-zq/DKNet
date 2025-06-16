# DKNet 测试脚本使用指南

## 概述

本目录包含了DKNet TSS系统的测试脚本，支持Docker环境下的完整测试流程，包括JWT鉴权功能。

## 脚本列表

### 1. `start-test-env.sh` - 主要测试环境管理脚本

这是主要的测试环境管理脚本，支持启动、停止、测试Docker环境中的DKNet TSS集群。

#### 新增功能（JWT鉴权支持）

- **JWT Token生成** - 自动生成用于API鉴权的JWT token
- **鉴权测试** - 验证API鉴权功能是否正常工作
- **认证API调用** - 所有TSS API调用都使用JWT鉴权

#### 主要命令

```bash
# 启动测试环境
./start-test-env.sh start

# 生成JWT token用于手动测试
./start-test-env.sh generate-token

# 测试JWT鉴权功能
./start-test-env.sh test-auth

# 运行TSS功能测试（包含鉴权）
./start-test-env.sh test-tss

# 查看环境状态
./start-test-env.sh status

# 停止环境
./start-test-env.sh stop

# 清理环境
./start-test-env.sh cleanup
```

#### JWT配置

- **JWT Secret**: `dknet-test-jwt-secret-key-2024`
- **JWT Issuer**: `dknet-test`
- **Token有效期**: 24小时
- **支持角色**: admin, operator

### 2. `test-auth-integration.sh` - JWT鉴权集成测试

专门用于测试JWT鉴权功能的集成测试脚本。

#### 测试内容

1. **环境检查** - 验证测试环境是否正在运行
2. **未认证请求测试** - 验证未认证请求被正确拒绝（HTTP 401）
3. **JWT流程测试** - 完整的JWT生成和认证流程
4. **手动JWT测试** - 测试手动提取和使用JWT token

#### 使用方法

```bash
# 运行所有集成测试
./test-auth-integration.sh

# 运行特定测试
./test-auth-integration.sh test-jwt
./test-auth-integration.sh test-unauth
./test-auth-integration.sh test-manual

# 检查环境状态
./test-auth-integration.sh check-env
```

## 使用流程

### 1. 启动测试环境

```bash
# 启动完整的测试环境
./start-test-env.sh start
```

输出示例：

```text
[INFO] Starting DKNet TSS test environment...
[INFO] Building and starting services...
[SUCCESS] Test environment started successfully!
[INFO] Services available at:
  - Validation Service: http://localhost:8888
  - TSS Node 1: http://localhost:8081
  - TSS Node 2: http://localhost:8082
  - TSS Node 3: http://localhost:8083

[INFO] JWT Authentication is enabled for all TSS nodes
[INFO] JWT Secret: dknet-test-jwt-secret-key-2024
[INFO] JWT Issuer: dknet-test
[WARNING] Use './start-test-env.sh generate-token' to get a JWT token for API testing
```

### 2. 生成JWT Token

```bash
# 生成JWT token
./start-test-env.sh generate-token
```

输出示例：

```text
[SUCCESS] JWT Token generated:
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3NTAxNDQxODgsImlhdCI6MTc1MDA1Nzc4OCwiaXNzIjoiZGtuZXQtdGVzdCIsInJvbGVzIjpbImFkbWluIiwib3BlcmF0b3IiXSwic3ViIjoidGVzdC11c2VyIn0.wrl0I29Q8R5zFm8TYUyVyGeyCKDfhCFEQO8k7l2wjU8

[INFO] Usage examples:
HTTP: curl -H "Authorization: Bearer <token>" http://localhost:8081/health
gRPC: grpcurl -H "authorization: Bearer <token>" localhost:9095 tss.v1.TSSService/GetOperation
```

### 3. 测试JWT鉴权

```bash
# 测试鉴权功能
./start-test-env.sh test-auth
```

### 4. 运行TSS功能测试

```bash
# 运行完整的TSS测试（包含鉴权）
./start-test-env.sh test-tss
```

### 5. 运行集成测试

```bash
# 运行完整的鉴权集成测试
./test-auth-integration.sh
```

## 手动API测试

### 使用curl进行HTTP API测试

```bash
# 1. 生成JWT token
TOKEN=$(./start-test-env.sh generate-token 2>/dev/null | grep -A1 "JWT Token generated:" | tail -1)

# 2. 测试健康检查
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/health

# 3. 启动密钥生成
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "threshold": 2,
    "parties": 3,
    "participants": ["QmVesSFq5FdNmoLyoe994jJdYLhqZqTyZajopMaxyBqbTF", "QmQjz2j7wFScU4Rj1cP3iwisbGwdhkNXmfmUYUHmvtEXY3", "QmPFTCTMKBtUg5fzeexHALdPniw98RV3W54Vg2Bphuc5qi"]
  }' \
  http://localhost:8081/api/v1/keygen

# 4. 查看操作列表
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/v1/operations
```

### 使用grpcurl进行gRPC API测试

```bash
# 1. 生成JWT token
TOKEN=$(./start-test-env.sh generate-token 2>/dev/null | grep -A1 "JWT Token generated:" | tail -1)

# 2. 测试gRPC调用
grpcurl -H "authorization: Bearer $TOKEN" \
  -d '{"operation_id": "test-id"}' \
  localhost:9095 tss.v1.TSSService/GetOperation
```

## 配置说明

### JWT配置

测试环境使用固定的JWT配置以确保一致性：

```yaml
security:
  auth:
    enabled: true
    jwt_secret: "dknet-test-jwt-secret-key-2024"
    jwt_issuer: "dknet-test"
```

### 端口映射

- **Validation Service**: 8888
- **TSS Node 1**: HTTP 8081, gRPC 9095
- **TSS Node 2**: HTTP 8082, gRPC 9096
- **TSS Node 3**: HTTP 8083, gRPC 9097

### 环境变量

- `TSS_ENCRYPTION_PASSWORD`: TSS加密密码（默认：TestPassword123!）

## 安全注意事项

1. **测试环境专用**: 这些配置和密钥仅用于测试环境
2. **JWT密钥安全**: 生产环境必须使用强随机密钥
3. **Token管理**: JWT token有24小时有效期，过期后需重新生成
4. **网络安全**: 测试环境在本地运行，生产环境需要适当的网络安全配置

## 故障排除

### 查看日志

```bash
# 查看所有服务日志
./start-test-env.sh logs

# 查看特定服务日志
./start-test-env.sh logs tss-node1
./start-test-env.sh logs validation-service
```

### 重置环境

```bash
# 完全清理并重启环境
./start-test-env.sh cleanup
./start-test-env.sh start
```

### 调试JWT问题

```bash
# 测试JWT生成
./start-test-env.sh generate-token

# 测试鉴权流程
./start-test-env.sh test-auth

# 运行集成测试
./test-auth-integration.sh
```

这些脚本提供了完整的JWT鉴权测试环境，确保DKNet TSS系统的安全性和功能性。
