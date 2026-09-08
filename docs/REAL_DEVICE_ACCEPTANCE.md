# My API 实机与副本验收清单

本清单用于完成移动端、Full/Lite/Desktop、安装/升级/切换、S5-P 与恢复的最后验收。当前 Legacy LAN/macOS/Windows 条目保留为历史可执行合同；服务器 Lite、个人电脑 Lite、public 向导、统一更新器与 S5-P 实机验收仍待实现后执行。S5-P 纯内核和 Release Manifest 结构 selector 的合成测试不是实机或安装验收。它只描述验证步骤，**不会自动重启生产、配置防火墙/路由器或发布制品**。

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
5. 在「渠道」页面从额度变化面板继续向下滚动，确认渠道卡片、筛选和分页不会被面板或固定视口裁剪。
6. 记录设备型号、浏览器版本、视口宽度、截图和结果；截图不得包含密钥、Cookie 或完整敏感响应。

如果概览中没有“账户额度变化”卡片，先确认当前账号拥有管理员 `channel.read` 权限；没有该权限时组件会安全隐藏且不会请求上游额度。拥有权限但显示空状态时，先在「渠道」中对目标渠道执行一次余额/Account Info 查询，或在系统设置启用 `CHANNEL_QUOTA_SYNC_ENABLED`，等待下一轮采样后再刷新。普通用户不会通过此面板看到上游账户额度。

验收时不要把 `Accounts tracked` 当作真实订阅账户数；多 Key 渠道在当前版本不会拆分各 Key 额度，面板应明确显示渠道序列和数据质量状态。

代码层面的替代证据包括：

```bash
npm run --silent brand:check
npm run --silent website:check
# 在带 web 依赖的环境运行相关 Vitest
bun run test -- src/features/usage-logs/components/__tests__/usage-logs-table.test.tsx
```

这些命令不能替代真实画面检查，只能证明合同和状态分支存在。

### 无界面运行时认证探针

在无法立即使用手机时，可以先用仓库自带的只读探针复核部署的认证链路。凭据只从
进程环境读取，输出只包含 HTTP 状态和固定错误分类，不会打印或保存密码、访问令牌、
Cookie 或响应正文：

```bash
MYAPI_PROBE_URL=http://127.0.0.1:3000 \
MYAPI_PROBE_USERNAME='<管理员用户名>' \
MYAPI_PROBE_PASSWORD='<管理员密码>' \
node tools/runtime/auth-probe.mjs
```

探针检查 `/api/status`、登录、`/api/user/self`、额度变化和管理员日志接口。它只能
证明服务端认证/权限链路可用，不能代替手机浏览器的视觉、触控、滚动和日志正文验收。

## 3. Legacy LAN 与当前 Desktop 验收

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

这些检查不证明服务器 Lite、public 可达、形态切换、完整 Desktop 更新或 Linux Desktop 支持；这些目标必须在下列新矩阵中验收。

## 4. Full/Lite/Desktop、访问模式与 S5-P 联合验收（实现后执行）

对每个受支持的功能版、安装形态、OS/arch 和数据引擎记录 Release Manifest、制品 hash/digest、安装记录、数据目录、当前/目标版本与 operator。至少覆盖：

1. 同一入口只展示当前 OS/CPU 支持的 Full 服务器、Lite 服务器、个人电脑 Lite、Desktop；不支持项给出准确原因。
2. 新装、旧 Legacy LAN 升级与 Full↔Lite 切换均保留账号、Key、渠道、权限、账务、日志、S5-P 记录和 Codex 应用备份；切换不自动改变 local/LAN/public。
3. 初始个人电脑 Lite/Desktop 仅本机；LAN 需显式确认；public 向导分别验证监听、权限、防火墙、NAT/CGNAT、IPv4/IPv6、DNS/HTTPS/反代或用户选择隧道，并用外部网络证明可达。仅本机健康成功不得标记 public 已验证。
4. Desktop 分别验证窗口关闭、退出应用、停止后台服务、开机启动、休眠/断网/重启后的提示和恢复；更新后恢复原有运行状态或清晰报告未恢复原因。
5. 手动检查、下载、安装与自动更新设置分别验证；自动更新默认关闭、维护窗口生效、并发点击/崩溃/重启不会双重迁移。程序回退与数据库恢复分别演练。
6. S5-P 默认关闭零采集/零模型调用；授权、用户隔离、撤权、二层脱敏、样本删除、时间/次数双触发去重、预算/未知费用、并发编辑、Codex 文件冲突/权限/链接攻击/回滚均用合成数据和隔离目标验证。真实日志、付费模型、宿主文件写入另获授权。

Linux、macOS、Windows 只能对实际有制品、CI 和真实设备记录的平台标为支持。未签名/未公证安装包只能标测试制品，不能标 trusted release。

## 5. 升级与数据库恢复副本

1. 从与当前版本一致的脱敏副本开始，保存 SQLite 文件或经批准的 PostgreSQL 逻辑备份校验和。
2. 先执行 `myapi upgrade --dry-run --json`，确认版本、镜像发行版、URL、端口和资源限制。
3. 执行升级，验证健康检查、登录、API 请求、日志查询、额度趋势和 Key 访问方案。
4. 停止副本，从备份恢复数据库，再验证用户、Key、渠道、日志索引和额度快照。
5. 记录升级耗时、恢复耗时、镜像 digest、迁移结果和未覆盖项；失败时保留旧镜像和旧数据目录。

`--dry-run` 只验证升级输入和写入边界，不会访问 GHCR、调用 Docker 或恢复数据库。数据库恢复必须在副本上按组织批准的流程完成。

## 6. 交付证据模板

```text
commit/tag:
release manifest/version:
edition/install shape/access mode:
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
