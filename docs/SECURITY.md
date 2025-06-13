# DKNet TSS 安全部署指南

本文档提供了 DKNet TSS (Threshold Signature Scheme) 系统的全面安全部署指南，包括加密实现、密码管理、系统安全配置和最佳实践。

## 目录

- [加密实现](#加密实现)
- [密码管理](#密码管理)
- [系统安全](#系统安全)
- [容器安全](#容器安全)
- [网络安全](#网络安全)
- [监控和审计](#监控和审计)
- [备份和恢复](#备份和恢复)
- [合规性](#合规性)
- [安全检查清单](#安全检查清单)

## 加密实现

### 技术规格

DKNet TSS 使用业界标准的加密技术保护 TSS 私钥：

- **对称加密**: AES-256-GCM
- **密钥派生**: PBKDF2-SHA256 (100,000 轮迭代)
- **消息认证**: GCM 内置认证标签
- **随机性**: 每次加密使用新的随机 nonce
- **盐值**: 固定应用盐确保密钥派生一致性

### 加密流程

1. **密钥派生**: 用户密码 + 应用盐 → PBKDF2 → 256位加密密钥
2. **数据加密**: TSS私钥数据 + 随机nonce → AES-256-GCM → 加密数据
3. **存储格式**: nonce + 加密数据 + 认证标签
4. **解密验证**: 自动验证数据完整性和真实性

### 安全特性

- **前向安全**: 随机 nonce 确保相同明文产生不同密文
- **防篡改**: GCM 认证标签检测任何数据修改
- **密钥拉伸**: PBKDF2 增加暴力破解成本
- **选择性加密**: 仅加密 TSS 私钥，其他数据保持明文以优化性能

## 密码管理

### 支持的密码输入方式

DKNet TSS 支持两种安全的密码输入方式：

#### 1. 环境变量（推荐用于生产）

```bash
export TSS_ENCRYPTION_PASSWORD="YourVerySecurePassword123!"
./bin/dknet start --config config.yaml
```

**优势**:

- 适合自动化部署
- 不在命令行历史中留下痕迹
- 容器化环境友好
- 支持密钥管理系统集成

#### 2. 交互式输入（推荐用于开发）

```bash
./bin/dknet start --config config.yaml
# 系统会提示输入密码，输入时不会显示在屏幕上
```

**优势**:

- 密码不存储在任何地方
- 适合手动操作
- 支持密码确认
- 最高安全级别

### 密码策略

#### 强制要求

- **最少长度**: 8个字符
- **字符类型**: 必须包含大写字母、小写字母、数字和特殊字符
- **复杂性**: 避免常见密码模式

#### 推荐实践

```bash
# 好的密码示例
MyCompany@TSS2024!
Secure#DKNet$Key789
Enterprise!Crypto@2024

# 避免的密码
password123
12345678
company2024
```

#### 密码轮换

- **频率**: 建议每90天更换一次
- **流程**:
  1. 生成新密码
  2. 停止所有节点
  3. 使用旧密码解密数据
  4. 使用新密码重新加密
  5. 重启节点

### 密码存储安全

#### 禁止的做法

❌ **绝对不要**:

- 在配置文件中存储明文密码
- 在代码中硬编码密码
- 在日志中记录密码
- 通过不安全的渠道传输密码
- 在文件系统中存储明文密码

#### 推荐的做法

✅ **应该**:

- 使用环境变量传递密码
- 使用专业的密钥管理系统
- 实施密码轮换策略
- 限制密码访问权限
- 定期审计密码使用

## 系统安全

### 用户和权限

#### 专用用户账户

```bash
# 创建专用用户
sudo useradd -r -s /bin/false -d /opt/dknet dknet-user

# 设置目录权限
sudo mkdir -p /opt/dknet/{bin,config,data,logs}
sudo chown -R dknet-user:dknet-user /opt/dknet
sudo chmod 750 /opt/dknet
sudo chmod 700 /opt/dknet/data
```

#### 文件权限

```bash
# 二进制文件
sudo chmod 755 /opt/dknet/bin/dknet

# 配置文件
sudo chmod 640 /opt/dknet/config/*.yaml

# 数据目录
sudo chmod 700 /opt/dknet/data

# 日志目录
sudo chmod 750 /opt/dknet/logs
```

### 系统加固

#### 防火墙配置

```bash
# 仅允许必要端口
sudo ufw allow 8080/tcp  # HTTP API
sudo ufw allow 9090/tcp  # gRPC API
sudo ufw allow 4001/tcp  # P2P 通信

# 限制源IP（根据实际需求调整）
sudo ufw allow from 10.0.0.0/8 to any port 8080
sudo ufw allow from 10.0.0.0/8 to any port 9090
```

#### 系统监控

```bash
# 安装监控工具
sudo apt install auditd fail2ban

# 配置审计规则
echo "-w /opt/dknet/data -p rwxa -k tss-data-access" >> /etc/audit/rules.d/tss.rules
sudo systemctl restart auditd
```

### 进程安全

#### Systemd 服务配置

```ini
[Unit]
Description=DKNet TSS Server
After=network.target

[Service]
Type=simple
User=dknet-user
Group=dknet-user
WorkingDirectory=/opt/dknet
ExecStart=/opt/dknet/bin/dknet start --config /opt/dknet/config/config.yaml
Restart=always
RestartSec=10

# 安全限制
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/dknet/data /opt/dknet/logs
PrivateTmp=true
ProtectKernelTunables=true
ProtectControlGroups=true

# 环境变量
Environment=TSS_ENCRYPTION_PASSWORD=YourSecurePassword

[Install]
WantedBy=multi-user.target
```

## 容器安全

### Docker 安全配置

#### 安全的 Dockerfile

```dockerfile
FROM alpine:3.18

# 创建非root用户
RUN addgroup -g 1001 tss && \
    adduser -D -u 1001 -G tss tss

# 安装必要的包
RUN apk add --no-cache ca-certificates

# 复制二进制文件
COPY --chown=dknet:dknet bin/dknet /usr/local/bin/

# 设置工作目录
WORKDIR /app
RUN chown tss:tss /app

# 切换到非root用户
USER tss

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

EXPOSE 8080 9090 4001

CMD ["dknet", "start", "--config", "/app/config.yaml"]
```

#### Docker Compose 安全配置

```yaml
version: '3.8'

services:
  tss-node:
    build: .
    user: "1001:1001"
    read_only: true
    tmpfs:
      - /tmp:noexec,nosuid,size=100m
    volumes:
      - ./config:/app/config:ro
      - tss-data:/app/data
    environment:
      - TSS_ENCRYPTION_PASSWORD=${TSS_ENCRYPTION_PASSWORD}
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
    cap_add:
      - NET_BIND_SERVICE
    networks:
      - tss-network

volumes:
  tss-data:
    driver: local

networks:
  tss-network:
    driver: bridge
    internal: false
```

### Kubernetes 安全配置

#### SecurityContext

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dknet-server
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1001
        runAsGroup: 1001
        fsGroup: 1001
      containers:
      - name: dknet-server
        image: dknet/dknet:latest
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
        env:
        - name: TSS_ENCRYPTION_PASSWORD
          valueFrom:
            secretKeyRef:
              name: tss-secrets
              key: encryption-password
        volumeMounts:
        - name: config
          mountPath: /app/config
          readOnly: true
        - name: data
          mountPath: /app/data
        - name: tmp
          mountPath: /tmp
      volumes:
      - name: config
        configMap:
          name: tss-config
      - name: data
        persistentVolumeClaim:
          claimName: tss-data
      - name: tmp
        emptyDir: {}
```

#### Secret 管理

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: tss-secrets
type: Opaque
data:
  encryption-password: <base64-encoded-password>
```

## 网络安全

### TLS 配置

#### 证书生成

```bash
# 生成CA证书
openssl genrsa -out ca-key.pem 4096
openssl req -new -x509 -days 365 -key ca-key.pem -sha256 -out ca.pem

# 生成服务器证书
openssl genrsa -out server-key.pem 4096
openssl req -subj "/CN=dknet-server" -sha256 -new -key server-key.pem -out server.csr
openssl x509 -req -days 365 -sha256 -in server.csr -CA ca.pem -CAkey ca-key.pem -out server-cert.pem
```

#### 配置文件

```yaml
# config.yaml
server:
  tls:
    enabled: true
    cert_file: "/app/certs/server-cert.pem"
    key_file: "/app/certs/server-key.pem"
    ca_file: "/app/certs/ca.pem"
    min_version: "1.3"
```

### 网络隔离

#### VPC 配置（AWS 示例）

```bash
# 创建专用VPC
aws ec2 create-vpc --cidr-block 10.0.0.0/16

# 创建私有子网
aws ec2 create-subnet --vpc-id vpc-xxx --cidr-block 10.0.1.0/24

# 配置安全组
aws ec2 create-security-group --group-name tss-sg --description "TSS Security Group"
aws ec2 authorize-security-group-ingress --group-id sg-xxx --protocol tcp --port 8080 --source-group sg-xxx
```

## 监控和审计

### 日志配置

#### 结构化日志

```yaml
# config.yaml
logging:
  level: "info"
  format: "json"
  output: "/app/logs/dknet.log"
  max_size: 100  # MB
  max_backups: 10
  max_age: 30    # days
  audit:
    enabled: true
    file: "/app/logs/audit.log"
```

#### 关键事件监控

监控以下安全相关事件：

- 密钥生成和使用
- 认证失败
- 配置更改
- 异常网络连接
- 系统错误和异常

### 指标收集

#### Prometheus 配置

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'dknet-server'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
    scrape_interval: 30s
```

#### 关键指标

- `tss_keygen_total`: 密钥生成次数
- `tss_sign_total`: 签名操作次数
- `tss_errors_total`: 错误计数
- `tss_active_connections`: 活跃连接数

### 告警规则

```yaml
# alerts.yml
groups:
- name: tss-security
  rules:
  - alert: TSS_HighErrorRate
    expr: rate(tss_errors_total[5m]) > 0.1
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "High error rate in TSS operations"

  - alert: TSS_UnauthorizedAccess
    expr: rate(tss_auth_failures_total[5m]) > 0.05
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Potential unauthorized access attempt"
```

## 备份和恢复

### 数据备份策略

#### 加密数据备份

```bash
#!/bin/bash
# backup-tss-data.sh

BACKUP_DIR="/secure/backups/tss"
DATA_DIR="/opt/dknet/data"
DATE=$(date +%Y%m%d_%H%M%S)

# 创建备份目录
mkdir -p "$BACKUP_DIR"

# 备份加密数据
tar -czf "$BACKUP_DIR/tss-data-$DATE.tar.gz" -C "$DATA_DIR" .

# 验证备份
if [ $? -eq 0 ]; then
    echo "Backup completed: $BACKUP_DIR/tss-data-$DATE.tar.gz"
    
    # 清理旧备份（保留30天）
    find "$BACKUP_DIR" -name "tss-data-*.tar.gz" -mtime +30 -delete
else
    echo "Backup failed!"
    exit 1
fi
```

#### 自动化备份

```bash
# 添加到 crontab
0 2 * * * /opt/dknet/scripts/backup-tss-data.sh
```

### 灾难恢复

#### 恢复流程

1. **准备新环境**

   ```bash
   # 安装 DKNet 服务器
   # 配置系统用户和权限
   # 设置网络和防火墙
   ```

2. **恢复数据**

   ```bash
   # 停止服务
   sudo systemctl stop dknet
   
   # 恢复备份数据
   cd /opt/dknet/data
   sudo tar -xzf /secure/backups/tss/tss-data-20240101_020000.tar.gz
   
   # 设置权限
   sudo chown -R dknet-user:dknet-user /opt/dknet/data
   ```

3. **验证恢复**

   ```bash
   # 启动服务（需要原始密码）
   sudo systemctl start dknet
   
   # 验证密钥可用性
   curl -X GET http://localhost:8080/api/v1/keys
   ```

### 密钥恢复注意事项

⚠️ **重要提醒**:

- **密码丢失 = 数据永久丢失**: 如果忘记加密密码，无法恢复 TSS 私钥
- **备份密码**: 在安全的地方备份密码（如密钥管理系统）
- **测试恢复**: 定期测试备份恢复流程
- **多重备份**: 考虑在不同地理位置存储备份

## 合规性

### 数据保护法规

#### GDPR 合规

- **数据最小化**: 仅收集必要的数据
- **加密存储**: 所有敏感数据加密存储
- **访问控制**: 实施严格的访问控制
- **数据删除**: 提供数据删除机制

#### SOX 合规

- **审计日志**: 完整的操作审计日志
- **职责分离**: 不同角色的权限分离
- **变更控制**: 配置变更的审批流程
- **定期审查**: 定期安全审查和评估

### 行业标准

#### ISO 27001

- **信息安全管理**: 建立ISMS体系
- **风险评估**: 定期进行风险评估
- **安全培训**: 员工安全意识培训
- **事件响应**: 建立安全事件响应流程

#### NIST 框架

- **识别**: 资产和风险识别
- **保护**: 实施保护措施
- **检测**: 安全事件检测
- **响应**: 事件响应计划
- **恢复**: 业务连续性计划

## 安全检查清单

### 部署前检查

#### 系统配置

- [ ] 创建专用用户账户
- [ ] 设置正确的文件权限
- [ ] 配置防火墙规则
- [ ] 启用系统审计
- [ ] 安装安全更新

#### 应用配置

- [ ] 设置强密码策略
- [ ] 配置 TLS 加密
- [ ] 启用访问日志
- [ ] 配置监控告警
- [ ] 测试备份恢复

#### 网络安全

- [ ] 网络隔离配置
- [ ] VPN 或专线连接
- [ ] DDoS 防护
- [ ] 入侵检测系统
- [ ] 网络监控

### 运行时检查

#### 日常监控

- [ ] 检查系统日志
- [ ] 监控性能指标
- [ ] 验证备份完整性
- [ ] 检查安全告警
- [ ] 审查访问日志

#### 定期审查

- [ ] 密码轮换（90天）
- [ ] 证书更新（年度）
- [ ] 安全评估（季度）
- [ ] 权限审查（月度）
- [ ] 漏洞扫描（周度）

### 事件响应

#### 安全事件

- [ ] 事件检测和分类
- [ ] 影响评估
- [ ] 遏制措施
- [ ] 根因分析
- [ ] 恢复计划

#### 业务连续性

- [ ] 灾难恢复计划
- [ ] 备用站点准备
- [ ] 数据恢复测试
- [ ] 通信计划
- [ ] 业务影响分析

## 联系和支持

### 安全问题报告

如果发现安全漏洞，请通过以下方式报告：

- **邮箱**: `dreamer.x.cn>@gmail.com>`
- **加密**: 使用 PGP 公钥加密敏感信息
- **响应时间**: 24小时内确认，72小时内初步响应

### 技术支持

- **文档**: 查看最新的安全文档
- **社区**: 参与安全讨论
- **培训**: 参加安全培训课程
- **咨询**: 获取专业安全咨询

---

**免责声明**: 本文档提供的安全建议基于当前最佳实践，但安全是一个持续的过程。请根据您的具体环境和需求调整安全措施，并定期更新安全配置以应对新的威胁。
