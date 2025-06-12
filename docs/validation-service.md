# TSS 验证服务 (Validation Service)

## 概述

TSS验证服务是一个可选的安全功能，允许在执行签名操作之前对签名请求进行外部验证。这可以防止恶意或未授权的签名请求被处理。

## 功能特性

- **可选配置**: 验证服务是完全可选的，如果未配置则跳过验证
- **HTTP API**: 通过HTTP POST请求与外部验证服务通信
- **灵活验证**: 支持自定义验证逻辑和规则
- **安全性**: 支持HTTPS、自定义头部、超时控制等安全特性
- **同步验证**: 支持主动签名和同步签名操作的验证

## 配置

在TSS节点配置文件中添加验证服务配置：

```yaml
tss:
  node_id: "node1"
  moniker: "DKNet Node 1"
  validation_service:
    enabled: true                              # 启用验证服务
    url: "http://localhost:8888/validate"      # 验证服务URL
    timeout_seconds: 30                        # 请求超时时间（秒）
    headers:                                   # 自定义HTTP头部
      Authorization: "Bearer your-api-token"
      X-API-Version: "v1"
    insecure_skip_verify: false               # 是否跳过TLS验证（仅开发环境）
```

### 配置参数说明

- `enabled`: 是否启用验证服务（默认: false）
- `url`: 验证服务的HTTP端点URL（必需，当enabled=true时）
- `timeout_seconds`: HTTP请求超时时间，单位秒（默认: 30）
- `headers`: 发送给验证服务的自定义HTTP头部（可选）
- `insecure_skip_verify`: 是否跳过TLS证书验证，仅用于开发环境（默认: false）

## 验证流程

1. **签名请求接收**: TSS节点接收到签名请求
2. **验证服务调用**: 如果配置了验证服务，TSS节点向验证服务发送验证请求
3. **验证决策**: 验证服务根据自定义规则返回批准或拒绝决策
4. **签名执行**: 只有验证通过的请求才会继续执行TSS签名流程

## API 接口

### 验证请求 (Validation Request)

TSS节点向验证服务发送的请求格式：

```json
{
  "message": "48656c6c6f20576f726c64",           // 待签名消息（十六进制编码）
  "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",  // 使用的密钥ID
  "participants": ["node1", "node2", "node3"],   // 参与节点列表
  "node_id": "node1",                            // 发起请求的节点ID
  "timestamp": 1703123456,                       // 请求时间戳
  "metadata": {                                  // 附加元数据
    "message_length": 11
  }
}
```

### 验证响应 (Validation Response)

验证服务返回的响应格式：

```json
{
  "approved": true,                              // 是否批准签名
  "reason": "All validation checks passed",     // 批准/拒绝原因
  "metadata": {                                  // 附加响应元数据
    "validated_at": 1703123456,
    "message_length": 11
  }
}
```

## 示例验证服务

项目提供了一个示例验证服务实现，位于 `examples/validation-service/main.go`。

### 运行示例验证服务

```bash
# 编译并运行示例验证服务
cd examples/validation-service
go run main.go
```

示例服务将在 `http://localhost:8888` 启动，提供以下端点：

- `POST /validate` - 验证签名请求
- `GET /health` - 健康检查

### 示例验证规则

示例验证服务包含以下验证规则：

1. **消息长度检查**: 拒绝空消息或超过1KB的消息
2. **内容过滤**: 拒绝包含禁用词汇的消息
3. **密钥白名单**: 只允许特定密钥ID的签名请求
4. **时间戳验证**: 拒绝超过5分钟的旧请求
5. **参与者数量**: 要求至少2个参与者

## 测试验证服务

### 1. 启动验证服务

```bash
cd examples/validation-service
go run main.go
```

### 2. 配置TSS节点

在现有配置文件中添加验证服务配置：

```bash
# 编辑配置文件，添加validation_service配置
vim config.yaml
```

在 `tss` 部分添加验证服务配置：

```yaml
tss:
  node_id: "node1"
  moniker: "DKNet Node 1"
  validation_service:
    enabled: true
    url: "http://localhost:8888/validate"
    timeout_seconds: 30
```

### 3. 测试签名请求

发送签名请求，观察验证服务的日志输出：

```bash
# 发送合法的签名请求
curl -X POST http://localhost:8080/api/v1/sign \
  -H "Content-Type: application/json" \
  -d '{
    "message": "SGVsbG8gV29ybGQ=",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["node1", "node2"]
  }'

# 发送包含禁用词汇的请求（应被拒绝）
curl -X POST http://localhost:8080/api/v1/sign \
  -H "Content-Type: application/json" \
  -d '{
    "message": "bWFsaWNpb3VzIGF0dGFjaw==",
    "key_id": "0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4",
    "participants": ["node1", "node2"]
  }'
```

## 自定义验证服务

您可以实现自己的验证服务，只需要：

1. **实现HTTP API**: 提供POST端点接收验证请求
2. **处理请求格式**: 解析TSS节点发送的JSON请求
3. **实现验证逻辑**: 根据业务需求实现自定义验证规则
4. **返回标准响应**: 返回包含`approved`字段的JSON响应

### 验证服务最佳实践

1. **性能优化**: 确保验证逻辑高效，避免阻塞TSS签名流程
2. **错误处理**: 妥善处理网络错误、超时等异常情况
3. **日志记录**: 记录所有验证决策以便审计
4. **安全性**: 使用HTTPS、API认证等安全措施
5. **高可用**: 考虑验证服务的高可用性和故障恢复

## 故障处理

- **验证服务不可用**: TSS节点将记录错误并拒绝签名请求
- **网络超时**: 根据配置的超时时间拒绝请求
- **验证服务错误**: 任何HTTP错误状态码都将导致签名请求被拒绝

## 安全考虑

1. **网络安全**: 在生产环境中使用HTTPS保护验证请求
2. **认证授权**: 实现适当的API认证机制
3. **输入验证**: 验证服务应验证所有输入参数
4. **审计日志**: 记录所有验证决策以便安全审计
5. **故障安全**: 验证服务故障时应拒绝签名请求（fail-safe）
