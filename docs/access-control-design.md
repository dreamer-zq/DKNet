# 访问控制系统设计文档（简化版）

## 概述

为DKNet节点添加基于PeerID的白名单访问控制机制，确保只有经过授权的节点才能建立P2P连接。

## 需求分析

### 核心需求
1. **白名单机制**：只允许配置文件中指定的PeerID建立连接
2. **P2P层验证**：在P2P连接建立时进行权限验证
3. **简单配置**：通过配置文件静态配置授权节点列表

### 安全目标
- 防止未授权节点建立P2P连接
- 简单可靠的权限控制机制

## 架构设计

### 配置扩展

```yaml
security:
  tls_enabled: false
  cert_file: ""
  key_file: ""
  
  # 访问控制配置
  access_control:
    # 是否启用访问控制
    enabled: true
    
    # 允许连接的节点列表（白名单）
    allowed_peers:
      - "12D3KooWExample1PeerID..."
      - "12D3KooWExample2PeerID..."
      - "12D3KooWExample3PeerID..."
```

### 组件设计

#### 1. AccessControl模块 (`internal/security/access_control.go`)

```go
type AccessController interface {
    // 检查PeerID是否被授权
    IsAuthorized(peerID string) bool
    
    // 获取所有授权节点（用于调试）
    GetAuthorizedPeers() []string
}

type Config struct {
    Enabled      bool     `yaml:"enabled"`
    AllowedPeers []string `yaml:"allowed_peers"`
}

type Controller struct {
    config       *Config
    allowedPeers map[string]bool
    logger       *zap.Logger
}

func NewController(config *Config, logger *zap.Logger) *Controller {
    controller := &Controller{
        config:       config,
        allowedPeers: make(map[string]bool),
        logger:       logger,
    }
    
    // 构建快速查找的map
    for _, peerID := range config.AllowedPeers {
        controller.allowedPeers[peerID] = true
    }
    
    return controller
}

func (c *Controller) IsAuthorized(peerID string) bool {
    if !c.config.Enabled {
        return true // 未启用时允许所有连接
    }
    return c.allowedPeers[peerID]
}
```

#### 2. P2P网络层集成

在P2P网络层添加连接过滤器：

```go
// internal/p2p/network.go
type Network struct {
    // ... 现有字段
    accessController *security.AccessController
}

// 在连接建立时验证权限
func (n *Network) handleNewConnection(conn network.Conn) {
    peerID := conn.RemotePeer()
    
    // 权限验证
    if n.accessController != nil && !n.accessController.IsAuthorized(peerID.String()) {
        n.logger.Warn("Rejected connection from unauthorized peer",
            zap.String("peer_id", peerID.String()))
        conn.Close()
        return
    }
    
    n.logger.Info("Accepted connection from authorized peer",
        zap.String("peer_id", peerID.String()))
    
    // 处理授权的连接...
}
```

## 实现计划

### 阶段1：基础结构（1天）
- [ ] 扩展配置结构体，添加access_control配置
- [ ] 创建简化的AccessControl模块
- [ ] 添加基本的权限验证逻辑

### 阶段2：P2P层集成（1天）
- [ ] 在P2P网络层添加连接过滤
- [ ] 实现连接拒绝机制
- [ ] 添加相关日志

### 阶段3：测试和验证（1天）
- [ ] 编写单元测试
- [ ] 集成测试验证功能
- [ ] 更新用户文档

## 配置示例

### 启用访问控制
```yaml
security:
  access_control:
    enabled: true
    allowed_peers:
      - "12D3KooWBhZKGWdvx7YdCUFqAE6CbU6GGLw3n7p9R1v8qN2XY3Zh"
      - "12D3KooWCaE3LGUvJbxFweaQ7p8xR2nZ5Kv6Y9Mw3X4Bv7Nj2L8Q"
      - "12D3KooWDfH9LPQr3nK5z6Y8Mv7JtR4Xq9Bw2E6Fg3Lp5Nm1Zv7S"
```

### 禁用访问控制（默认）
```yaml
security:
  access_control:
    enabled: false
```

## 工作流程

1. **节点启动**：加载配置文件，初始化AccessController
2. **P2P连接建立**：检查对方PeerID是否在白名单中
3. **连接处理**：
   - 如果在白名单中：接受连接，正常处理
   - 如果不在白名单中：拒绝连接，记录日志

## 安全考虑

### 防护措施
1. **连接层验证**：在P2P连接建立时直接验证和拒绝
2. **配置保护**：确保配置文件有适当的文件系统权限
3. **日志记录**：记录所有连接拒绝事件

### 向后兼容性
- 默认情况下访问控制功能关闭（enabled: false）
- 配置文件向后兼容，旧配置文件仍然有效
- 不影响现有的API和功能 