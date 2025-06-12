# DKNet TSS CI/CD 流水线

本文档描述了 DKNet TSS 项目的持续集成和持续部署 (CI/CD) 流水线。

## 概述

我们的 CI/CD 流水线使用 GitHub Actions 实现，包含以下主要组件：

- **持续集成 (CI)**: 代码质量检查、测试、构建
- **持续部署 (CD)**: 自动化部署到不同环境
- **发布管理**: 自动化版本发布和资产构建

## 工作流程

### 1. 主 CI/CD 流水线 (`.github/workflows/ci.yml`)

**触发条件:**
- 推送到 `main` 或 `develop` 分支
- 针对 `main` 分支的 Pull Request

**流水线阶段:**

#### 阶段 1: 代码质量和安全检查
- **Lint 检查**: 使用 `golangci-lint` 进行代码风格检查
- **安全扫描**: 使用 `gosec` 进行安全漏洞扫描
- **SARIF 上传**: 将安全扫描结果上传到 GitHub Security 标签

#### 阶段 2: 测试套件
- **单元测试**: 运行所有 Go 单元测试
- **竞态条件检测**: 使用 `-race` 标志检测竞态条件
- **覆盖率报告**: 生成代码覆盖率报告并上传到 Codecov
- **测试报告**: 生成 JUnit 格式的测试报告

#### 阶段 3: Docker 构建
- **多平台构建**: 支持 `linux/amd64` 和 `linux/arm64`
- **镜像推送**: 推送到 GitHub Container Registry (ghcr.io)
- **缓存优化**: 使用 GitHub Actions 缓存加速构建

#### 阶段 4: 验证服务集成测试
- **环境启动**: 启动完整的 Docker Compose 测试环境
- **功能测试**: 运行验证服务功能测试
- **集成测试**: 运行 TSS 与验证服务的集成测试
- **环境清理**: 自动清理测试环境

#### 阶段 5: 安全漏洞扫描
- **容器扫描**: 使用 Trivy 扫描 Docker 镜像漏洞
- **结果上传**: 将扫描结果上传到 GitHub Security 标签

#### 阶段 6: 部署 (仅 main 分支)
- **分阶段部署**: 先部署到 staging，再部署到 production
- **冒烟测试**: 部署后运行基本功能测试
- **通知**: 通过 Slack 通知部署状态

#### 阶段 7: 性能基准测试 (仅 main 分支)
- **基准测试**: 运行 Go 基准测试
- **性能监控**: 跟踪性能变化趋势
- **性能告警**: 性能下降超过 200% 时触发告警

### 2. 发布流水线 (`.github/workflows/release.yml`)

**触发条件:**
- 推送版本标签 (格式: `v*`)

**流水线阶段:**

#### 阶段 1: 创建发布
- **版本提取**: 从 Git 标签提取版本号
- **变更日志**: 自动生成变更日志
- **GitHub Release**: 创建 GitHub 发布页面

#### 阶段 2: 构建二进制文件
- **多平台构建**: 支持 Linux、macOS、Windows
- **多架构支持**: 支持 amd64 和 arm64 架构
- **版本注入**: 将版本信息编译到二进制文件中
- **资产上传**: 上传二进制文件到 GitHub Release

#### 阶段 3: 构建和推送 Docker 镜像
- **语义化版本**: 支持 `latest`、`v1.2.3`、`v1.2`、`v1` 标签
- **多平台镜像**: 构建 amd64 和 arm64 镜像
- **验证服务镜像**: 同时构建验证服务镜像

#### 阶段 4: 发布测试
- **镜像测试**: 测试发布的 Docker 镜像
- **集成测试**: 使用发布镜像运行完整集成测试

#### 阶段 5: 文档更新
- **版本更新**: 自动更新 README 中的版本信息
- **文档提交**: 自动提交文档更改

#### 阶段 6: 通知
- **成功通知**: 发布成功时通知相关人员
- **失败告警**: 发布失败时发送告警

## 环境配置

### 必需的 Secrets

在 GitHub 仓库设置中配置以下 secrets：

```bash
# Slack 通知 (可选)
SLACK_WEBHOOK=https://hooks.slack.com/services/...

# Codecov 令牌 (可选，用于私有仓库)
CODECOV_TOKEN=your-codecov-token
```

### 环境变量

```yaml
GO_VERSION: '1.23'                    # Go 版本
DOCKER_REGISTRY: ghcr.io             # Docker 镜像仓库
IMAGE_NAME: ${{ github.repository }}  # 镜像名称
```

## 分支策略

### 主分支 (`main`)
- 生产就绪代码
- 触发完整 CI/CD 流水线
- 自动部署到生产环境
- 运行性能基准测试

### 开发分支 (`develop`)
- 开发中的功能
- 触发 CI 流水线（不包括部署）
- 用于集成测试

### 功能分支 (`feature/*`)
- 通过 Pull Request 合并到 `develop`
- 触发 CI 流水线进行验证

## 部署环境

### Staging 环境
- **目的**: 预生产测试
- **触发**: 推送到 `main` 分支
- **测试**: 冒烟测试
- **访问**: 内部团队

### Production 环境
- **目的**: 生产服务
- **触发**: Staging 测试通过后自动部署
- **监控**: 全面监控和告警
- **访问**: 公开访问

## 质量门禁

代码必须通过以下检查才能合并：

1. **代码风格**: golangci-lint 检查通过
2. **安全扫描**: gosec 安全扫描通过
3. **单元测试**: 所有单元测试通过
4. **集成测试**: 验证服务集成测试通过
5. **代码覆盖率**: 覆盖率不低于现有水平
6. **构建成功**: Docker 镜像构建成功

## 监控和告警

### 构建监控
- **构建状态**: GitHub Actions 状态徽章
- **测试结果**: 测试报告和覆盖率趋势
- **性能监控**: 基准测试结果跟踪

### 告警机制
- **构建失败**: 立即通知维护者
- **安全漏洞**: 高危漏洞立即告警
- **性能下降**: 性能下降超过阈值时告警
- **部署状态**: 部署成功/失败通知

## 最佳实践

### 提交规范
使用 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

```
feat: add new validation rule
fix: resolve memory leak in TSS service
docs: update API documentation
test: add integration tests for validation service
ci: update GitHub Actions workflow
```

### Pull Request 流程
1. 创建功能分支
2. 实现功能并添加测试
3. 确保所有 CI 检查通过
4. 请求代码审查
5. 合并到目标分支

### 发布流程
1. 更新版本号和变更日志
2. 创建版本标签: `git tag v1.2.3`
3. 推送标签: `git push origin v1.2.3`
4. 自动触发发布流水线
5. 验证发布资产和镜像

## 故障排除

### 常见问题

#### CI 构建失败
1. 检查 Go 版本兼容性
2. 验证依赖项是否可用
3. 检查测试环境配置

#### Docker 构建失败
1. 检查 Dockerfile 语法
2. 验证基础镜像可用性
3. 检查构建上下文大小

#### 集成测试失败
1. 检查 Docker Compose 配置
2. 验证网络连接
3. 检查服务启动顺序

#### 部署失败
1. 检查环境配置
2. 验证访问权限
3. 检查资源可用性

### 调试技巧

#### 本地运行 CI 测试
```bash
# 运行 linter
golangci-lint run

# 运行测试
go test -v -race -coverprofile=coverage.out ./...

# 构建 Docker 镜像
docker build -t dknet-test .

# 运行集成测试
cd tests/scripts
./start-test-env.sh test
```

#### 查看详细日志
- GitHub Actions 工作流日志
- Docker 容器日志
- 应用程序日志

## 持续改进

### 定期审查
- 每月审查 CI/CD 性能
- 分析构建时间趋势
- 优化缓存策略

### 工具更新
- 定期更新 GitHub Actions
- 升级 Go 版本和依赖
- 更新安全扫描工具

### 流程优化
- 收集团队反馈
- 简化复杂流程
- 自动化手动步骤 