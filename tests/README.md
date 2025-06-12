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

## 快速开始

### 1. 启动测试环境

```bash
# 进入项目根目录
cd /path/to/DKNet

# 启动完整的测试环境
./tests/scripts/start-test-env.sh start
```

### 2. 运行验证测试

```bash
# 运行验证服务测试
./tests/scripts/start-test-env.sh test
```

### 3. 查看服务状态

```bash
# 查看所有服务状态
./tests/scripts/start-test-env.sh status
```

### 4. 停止测试环境

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
- `status` - 查看环境状态
- `logs [service]` - 查看日志
- `cleanup` - 清理环境
- `help` - 显示帮助信息

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

### 验证服务配置

验证服务的配置通过环境变量或配置文件进行设置，包括：

- 验证规则开关
- 白名单配置
- 日志级别等

## 故障排除

### 常见问题

1. **服务启动失败**

   ```bash
   # 查看服务日志
   ./tests/scripts/start-test-env.sh logs
   
   # 查看特定服务日志
   ./tests/scripts/start-test-env.sh logs validation-service
   ```

2. **端口冲突**
   - 确保端口 8081-8083, 8888, 9095-9097 没有被其他服务占用

3. **Docker 相关问题**

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

## 注意事项

- 这是测试环境，不适用于生产环境
- 验证服务的密钥和配置仅用于测试
- 定期清理 Docker 资源避免占用过多磁盘空间
- 测试完成后建议停止环境释放系统资源
