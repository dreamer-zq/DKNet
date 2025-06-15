# 访问控制使用指南

## 概述

DKNet的访问控制功能提供基于PeerID的白名单机制，确保只有授权的节点能够建立P2P连接和参与TSS操作。

## 配置

### 启用访问控制

在配置文件中添加以下配置：

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

## 获取节点的PeerID

每个节点在启动时会显示其PeerID，例如：

```
2024-01-01T12:00:00.000Z INFO p2p Host ID: 12D3KooWBhZKGWdvx7YdCUFqAE6CbU6GGLw3n7p9R1v8qN2XY3Zh
```

你也可以通过调用API获取：

```bash
curl http://localhost:8080/api/v1/network/info
```

## 配置集群

### 步骤1：生成集群配置

使用dknet工具生成集群配置：

```bash
./dknet init-cluster --nodes 3 --output ./cluster
```

### 步骤2：收集PeerID

从每个节点的`node-info.txt`文件中获取PeerID，或者启动节点查看日志。

### 步骤3：更新配置文件

在每个节点的`config.yaml`中添加其他节点的PeerID：

**node1/config.yaml:**
```yaml
security:
  access_control:
    enabled: true
    allowed_peers:
      - "12D3KooWNode2PeerID..."  # node2的PeerID
      - "12D3KooWNode3PeerID..."  # node3的PeerID
```

**node2/config.yaml:**
```yaml
security:
  access_control:
    enabled: true
    allowed_peers:
      - "12D3KooWNode1PeerID..."  # node1的PeerID
      - "12D3KooWNode3PeerID..."  # node3的PeerID
```

**node3/config.yaml:**
```yaml
security:
  access_control:
    enabled: true
    allowed_peers:
      - "12D3KooWNode1PeerID..."  # node1的PeerID
      - "12D3KooWNode2PeerID..."  # node2的PeerID
```

### 步骤4：启动节点

启动所有节点：

```bash
# 启动node1
./dknet start --config ./cluster/node1/config.yaml

# 启动node2
./dknet start --config ./cluster/node2/config.yaml

# 启动node3
./dknet start --config ./cluster/node3/config.yaml
```

## 验证访问控制

### 正常连接

当授权节点连接时，你会看到类似的日志：

```
2024-01-01T12:00:00.000Z INFO p2p Accepted connection from authorized peer peer_id=12D3KooW...
```

### 拒绝连接

当未授权节点尝试连接时，你会看到：

```
2024-01-01T12:00:00.000Z WARN p2p Rejected stream from unauthorized peer peer_id=12D3KooW... protocol=/tss/keygen/1.0.0
```

## 故障排除

### 问题1：节点无法连接

**症状：** 节点启动正常，但无法与其他节点建立连接

**解决方案：**
1. 检查`allowed_peers`配置是否包含目标节点的PeerID
2. 确认PeerID格式正确（以`12D3KooW`开头）
3. 检查网络连接和防火墙设置

### 问题2：PeerID格式错误

**症状：** 配置文件中的PeerID格式不正确

**解决方案：**
1. PeerID必须是有效的libp2p peer ID格式
2. 通常以`12D3KooW`开头，长度为52个字符
3. 从节点启动日志或API获取正确的PeerID

### 问题3：访问控制意外禁用

**症状：** 虽然配置了`enabled: true`，但所有节点都能连接

**解决方案：**
1. 检查配置文件格式是否正确（YAML缩进）
2. 确认配置文件路径正确
3. 重启节点以应用新配置

## 最佳实践

### 1. 安全配置

- **最小权限原则：** 只添加必要的节点到白名单
- **定期审核：** 定期检查和更新允许的节点列表
- **配置保护：** 确保配置文件有适当的文件系统权限

### 2. 运维建议

- **监控日志：** 定期检查访问拒绝日志，发现潜在的安全问题
- **备份配置：** 备份包含访问控制配置的文件
- **测试验证：** 在生产环境应用前，在测试环境验证配置

### 3. 集群管理

- **统一配置：** 确保集群中所有节点的白名单配置一致
- **新节点加入：** 添加新节点时，需要更新所有现有节点的配置
- **节点移除：** 移除节点时，及时从其他节点的白名单中删除

## 示例配置

完整的示例配置文件请参考：`examples/config-with-access-control.yaml` 