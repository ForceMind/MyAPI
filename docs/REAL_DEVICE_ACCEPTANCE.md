# MyAPI 实机与副本验收清单

本清单用于完成移动端、macOS/Windows LAN Lite 和升级恢复的最后验收。它只描述验证步骤，**不会自动重启生产、配置防火墙或发布制品**。

## 1. 验收前安全条件

- 先确认目标是测试实例或脱敏副本；不得把生产 `data`、`logs`、`.env` 或数据库文件复制到公开位置。
- 记录当前镜像 digest、compose 文件路径和数据备份校验和；不要在命令输出中打印会话密钥、API Key、Cookie 或 OAuth JSON。
- 确认 CPU、内存和磁盘余量；本地构建/测试使用不超过 2 个 CPU 和 2 GB 内存，避免与正在运行的服务争抢资源。
- 任何重建、重启、局域网开放或防火墙变更，都要在执行前得到部署方明确授权。

## 2. 手机浏览器验收

在包含最新 MyAPI 镜像的测试实例上，用真实登录账号执行：

1. 打开「使用日志 → Common Logs」，确认筛选栏下方出现日志卡片，而不是只有筛选器。
2. 打开一条日志的详情，验证请求正文、响应正文、响应分片和脱敏结果可见；普通用户只能看到自己的日志。
3. 断开网络或使用受控 5xx 测试端点，确认页面显示“加载日志失败”和“重试”，而不是“暂无日志”。
4. 打开「概览」和「渠道 → 账户额度变化」，验证长渠道名、额度值和折线图在窄屏不横向溢出。
5. 记录设备型号、浏览器版本、视口宽度、截图和结果；截图不得包含密钥、Cookie 或完整敏感响应。

如果概览中没有“账户额度变化”卡片，先确认当前账号拥有管理员 `channel.read` 权限；没有该权限时组件会安全隐藏且不会请求上游额度。拥有权限但显示空状态时，先在「渠道」中对目标渠道执行一次余额/Account Info 查询，或在系统设置启用 `CHANNEL_QUOTA_SYNC_ENABLED`，等待下一轮采样后再刷新。普通用户不会通过此面板看到上游账户额度。

代码层面的替代证据包括：

```bash
npm run --silent brand:check
npm run --silent website:check
# 在带 web 依赖的环境运行相关 Vitest
bun run test -- src/features/usage-logs/components/__tests__/usage-logs-table.test.tsx
```

这些命令不能替代真实画面检查，只能证明合同和状态分支存在。

## 3. macOS/Windows LAN Lite 验收

对每个平台分别记录版本、安装包 SHA256 和系统版本：

1. 默认启动，确认只监听回环地址；同一设备可打开状态页并发送测试请求。
2. 使用明确的 `--allow-lan` 和私网地址启动，确认状态页显示实际端点；从另一台局域网设备用独立 MyAPI API Key 发起请求。
3. 不带 `--allow-lan` 使用私网地址，确认初始化失败且不创建项目目录。
4. 验证单实例、托盘状态、停止/启动和确认后的端口切换。
5. 在测试副本中模拟健康检查失败，确认旧镜像和环境文件保留，可回滚。

自动合同检查：

```bash
npm run lan:check -- --skip-docker
npm run desktop:check
npm run upgrade:check -- --json
```

## 4. 升级与数据库恢复副本

1. 从与当前版本一致的脱敏副本开始，保存 SQLite 文件或经批准的 PostgreSQL 逻辑备份校验和。
2. 先执行 `myapi upgrade --dry-run --json`，确认版本、镜像发行版、URL、端口和资源限制。
3. 执行升级，验证健康检查、登录、API 请求、日志查询、额度趋势和 Key 访问方案。
4. 停止副本，从备份恢复数据库，再验证用户、Key、渠道、日志索引和额度快照。
5. 记录升级耗时、恢复耗时、镜像 digest、迁移结果和未覆盖项；失败时保留旧镜像和旧数据目录。

`--dry-run` 只验证升级输入和写入边界，不会访问 GHCR、调用 Docker 或恢复数据库。数据库恢复必须在副本上按组织批准的流程完成。

## 5. 交付证据模板

```text
commit/tag:
image digest:
platform/device:
browser/OS:
viewport:
test instance (not production):
checks run:
screenshots/artifacts:
result:
rollback verified:
operator/approval:
```

完成本清单的实机部分后，才可以把主计划中对应的“真实设备/副本演练”从待验证改为已完成。无设备或无脱敏副本时，必须保留待验证状态。
