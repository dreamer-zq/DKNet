# DKNet API Authentication

DKNet使用JWT Token进行API鉴权，保护所有API端点。

## JWT 鉴权

使用JSON Web Token进行鉴权，支持用户身份和角色管理。

**配置示例：**

```yaml
security:
  auth:
    enabled: true
    jwt_secret: "your-jwt-secret-key"
    jwt_issuer: "dknet-service"  # 可选
```

**JWT Token格式：**

```json
{
  "sub": "user123",
  "iss": "dknet-service",
  "exp": 1640995200,
  "roles": ["admin", "operator"]
}
```

**使用方式：**

HTTP请求：

```bash
curl -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
     http://localhost:8080/api/v1/keygen
```

gRPC请求：

```bash
grpcurl -H "authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." \
        localhost:8081 tss.v1.TSSService/StartKeygen
```

## 禁用鉴权

开发环境可以禁用鉴权：

```yaml
security:
  auth:
    enabled: false
```

## 角色和权限

### 获取认证上下文

在处理器中获取认证信息：

```go
func (s *Server) someHandler(c *gin.Context) {
    authCtx, ok := GetAuthContext(c.Request.Context())
    if !ok {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
        return
    }
    
    userID := authCtx.UserID
    roles := authCtx.Roles
    
    // 使用认证信息...
}
```

### 角色中间件

可以为特定端点要求特定角色：

```go
// HTTP
router.POST("/admin/keygen", RequireRole("admin"), s.keygenHandler)
```

## 错误处理

### HTTP错误响应

```json
{
  "error": "Authentication failed",
  "code": "UNAUTHORIZED"
}
```

### gRPC错误响应

```text
Code: UNAUTHENTICATED
Message: Authentication failed: invalid JWT token
```

## 安全建议

1. **使用HTTPS/TLS**: 在生产环境中始终启用TLS
2. **强密钥**: 使用足够长度的随机密钥
3. **密钥轮换**: 定期更换JWT密钥
4. **最小权限**: 只授予必要的权限
5. **监控**: 记录认证失败和可疑活动
6. **环境变量**: 将敏感配置存储在环境变量中

## 配置示例

完整的配置文件示例：

```yaml
# config.yaml
http:
  host: "0.0.0.0"
  port: 8080

grpc:
  host: "0.0.0.0"
  port: 8081

security:
  tls_enabled: true
  cert_file: "/etc/ssl/certs/server.crt"
  key_file: "/etc/ssl/private/server.key"
  auth:
    enabled: true
    jwt_secret: "${JWT_SECRET}"
    jwt_issuer: "dknet-service"
```
